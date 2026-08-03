package gateway

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

// pushResync is the fallback interval for re-rendering a cluster's snapshot
// even without a change notification — a safety net against a missed/dropped
// Notify (e.g. a goroutine race at startup) rather than the primary trigger.
var pushResync = 60 * time.Second

// SetPushResyncForTest lowers the resync interval for tests that need to
// observe a resync tick without waiting a full minute. Returns a restore func.
func SetPushResyncForTest(d time.Duration) func() {
	orig := pushResync
	pushResync = d
	return func() { pushResync = orig }
}

// Repos bundles the GORM repositories the gateway server reads/writes at
// runtime, replacing the controller-runtime client this server used to hold.
// Exceptions is unused by the gateway's own render/push path today but lives
// here so console-api (internal/console) can share this same bundle type
// (task 9: console-api GORM handlers) instead of defining a second one.
type Repos struct {
	Clusters       *dbrepo.ClusterRepo
	Policies       *dbrepo.PolicyRepo
	Groups         *dbrepo.GroupRepo
	Distributions  *dbrepo.DistributionRepo
	Exceptions     *dbrepo.ExceptionRepo
	Admission      *dbrepo.AdmissionRepo
	Incidents      *dbrepo.IncidentRepo
	Platform       *dbrepo.PlatformObservabilityRepo
	InstallRecords *dbrepo.InstallRecordRepo
	UninstallTasks *dbrepo.UninstallTaskRepo
}

// Server implements the gatewaypb.Hub gRPC service.
type Server struct {
	gatewaypb.UnimplementedHubServer
	repos       Repos
	namespace   string
	hb          *HeartbeatTracker
	changes     *Notifier
	sessions    *sessionRegistry
	connected   *connectRegistry
	sharedToken string
	// cleaner, when set, receives a full-cleanup trigger on an
	// admission-event-settings NotifyChange. Left nil by every non-leader
	// path and by tests, both of which have no cleanup loop running.
	cleaner                     *AdmissionCleaner
	admissionObservationEnabled atomic.Bool
	platformObservationEnabled  atomic.Bool
	incidentCleaner             *IncidentCleaner
	gatewayIncidentTracker      *admissionobs.Tracker
}

func NewServer(repos Repos, namespace string, hb *HeartbeatTracker, changes *Notifier, sharedToken string) *Server {
	return &Server{
		repos:                  repos,
		namespace:              namespace,
		hb:                     hb,
		changes:                changes,
		sessions:               newSessionRegistry(),
		connected:              newConnectRegistry(),
		sharedToken:            sharedToken,
		gatewayIncidentTracker: admissionobs.NewTracker(admissionobs.ComponentGateway, os.Getenv("POD_NAME")),
	}
}

// SetAdmissionCleaner attaches the cleanup loop this server should trigger when
// the admission event configuration changes. Separate from NewServer because
// only the elected leader runs a cleaner, and the leader builds both in the
// same callback.
func (s *Server) SetAdmissionCleaner(cleaner *AdmissionCleaner) { s.cleaner = cleaner }

func (s *Server) SetAdmissionObservationEnabled(enabled bool) {
	s.admissionObservationEnabled.Store(enabled)
	if !enabled {
		s.gatewayIncidentTracker.Reset()
	}
}

func (s *Server) SetIncidentCleaner(cleaner *IncidentCleaner) {
	s.incidentCleaner = cleaner
}

// SetPlatformObservationEnabled gates component sampling. Rejecting samples at
// the gateway is the backstop for members that have not yet received the
// updated config; the members stop generating them on their next push.
func (s *Server) SetPlatformObservationEnabled(enabled bool) {
	s.platformObservationEnabled.Store(enabled)
}

func (s *Server) PlatformObservationEnabled() bool {
	return s.platformObservationEnabled.Load()
}

// ack carries the platform observation switch on every acknowledgement. Members
// refresh from it each heartbeat, which is the only path that also reaches a
// standby operator.
func (s *Server) ack() *gatewaypb.Ack {
	return &gatewaypb.Ack{PlatformObservationEnabled: s.platformObservationEnabled.Load()}
}

func (s *Server) Register(ctx context.Context, req *gatewaypb.RegisterRequest) (*gatewaypb.RegisterResponse, error) {
	if req.ClusterName == "" || req.RegistrationToken == "" || req.ClusterUid == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_name, registration_token and cluster_uid are required")
	}

	cl, err := s.repos.Clusters.GetByName(ctx, req.ClusterName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "cluster not registered on gateway")
		}
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	if cl.Phase == v1alpha1.ClusterPhaseUninstalling || cl.Phase == v1alpha1.ClusterPhaseUninstalled {
		return nil, status.Error(codes.FailedPrecondition, "cluster is being uninstalled or has been uninstalled")
	}

	// 校验一次性注册 token：gateway 只存哈希。
	if subtle.ConstantTimeCompare([]byte(cl.RegTokenHash), []byte(HashToken(req.RegistrationToken))) != 1 {
		s.recordGatewayIncident(admissionobs.ReasonAuthFailure, 1, admissionobs.ReasonRegisterAuthFailed, time.Now().UTC())
		return nil, status.Error(codes.PermissionDenied, "invalid registration token")
	}

	// clusterUID 规则：跨 Cluster 唯一；重新接入必须 UID 一致。
	all, err := s.repos.Clusters.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list clusters: %v", err)
	}
	for _, other := range all {
		if other.Name != req.ClusterName && other.ClusterUID == req.ClusterUid {
			return nil, status.Errorf(codes.AlreadyExists, "cluster UID already registered as %q", other.Name)
		}
	}
	if cl.ClusterUID != "" && cl.ClusterUID != req.ClusterUid {
		return nil, status.Error(codes.PermissionDenied, "cluster UID does not match previously registered identity")
	}

	// 签发长期 token：gateway 只存哈希。
	token, err := NewToken()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "issue token: %v", err)
	}
	cl.AuthTokenHash = HashToken(token)
	cl.ClusterUID = req.ClusterUid
	cl.OperatorVersion = req.OperatorVersion
	if cl.Phase == "" || cl.Phase == v1alpha1.ClusterPhaseRegistering {
		// 保持 Registering；Online 由首个心跳 flush 置位。
		cl.Phase = v1alpha1.ClusterPhaseRegistering
	}
	// 消费注册 token（一次性）：清空哈希而非删除单独的 Secret。
	cl.RegTokenHash = ""
	if err := s.repos.Clusters.Update(ctx, cl); err != nil {
		return nil, status.Errorf(codes.Internal, "update cluster: %v", err)
	}

	return &gatewaypb.RegisterResponse{ClusterToken: token}, nil
}

// validateSyncAuth is used by Sync: validates the long-lived credential by
// cluster name + token, returning an error if the cluster is unknown or the
// token doesn't match.
func (s *Server) validateSyncAuth(ctx context.Context, clusterName, token string) error {
	cl, err := s.repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("cluster not registered")
	}
	if subtle.ConstantTimeCompare([]byte(cl.AuthTokenHash), []byte(HashToken(token))) != 1 {
		return fmt.Errorf("invalid cluster token")
	}
	return nil
}

// dispatchTimeout bounds how long Dispatch waits for a correlated reply
// envelope before giving up. Fixed, not configurable — D11a scopes this
// task to laying down the plumbing, not tuning it.
const dispatchTimeout = 10 * time.Second

// syncSession serializes stream.Send between the recv loop's Acks and the
// pusher goroutine, and remembers the last pushed snapshot for StatusReport
// correlation. pending tracks in-flight Dispatch calls awaiting a reply
// Envelope with a matching correlation id.
type syncSession struct {
	mu      sync.Mutex
	stream  gatewaypb.Hub_SyncServer
	lastRes RenderResult
	hasLast bool
	pending map[string]chan *gatewaypb.Envelope
	pushMu  sync.Mutex
	cancel  context.CancelFunc
	pushSeq uint64
}

func (ss *syncSession) send(m *gatewaypb.HubMessage) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.stream.Send(m)
}

func (ss *syncSession) setLast(r RenderResult) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.lastRes, ss.hasLast = r, true
}

func (ss *syncSession) last() (RenderResult, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.lastRes, ss.hasLast
}

func (ss *syncSession) startPush(parent context.Context, run func(context.Context)) {
	ss.pushMu.Lock()
	defer ss.pushMu.Unlock()
	if ss.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	ss.pushSeq++
	sequence := ss.pushSeq
	ss.cancel = cancel
	go func() {
		run(ctx)
		ss.pushMu.Lock()
		if ss.pushSeq == sequence {
			ss.cancel = nil
		}
		ss.pushMu.Unlock()
	}()
}

func (ss *syncSession) stopPush() {
	ss.pushMu.Lock()
	defer ss.pushMu.Unlock()
	if ss.cancel != nil {
		ss.cancel()
		ss.cancel = nil
		ss.pushSeq++
	}
}

// deliverReply routes a reply Envelope received on the member->hub half of
// the Sync stream to the pending Dispatch call awaiting that correlation id,
// if any (a reply with no matching waiter, e.g. one arriving after
// dispatch's timeout already gave up, is silently dropped).
func (ss *syncSession) deliverReply(env *gatewaypb.Envelope) {
	ss.mu.Lock()
	ch, ok := ss.pending[env.CorrelationId]
	ss.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- env:
	default:
	}
}

// dispatch forwards a Dispatch payload to this cluster over the existing
// Sync stream and waits up to dispatchTimeout for a correlated reply
// Envelope. This is a skeleton: nothing on the member side sends a reply
// Envelope yet (that requires member/operator changes out of this task's
// scope), so in practice every dispatch to a real member currently ends in
// DeadlineExceeded — the wire path and correlation bookkeeping are laid
// down for a later task to complete.
func (ss *syncSession) dispatch(ctx context.Context, to string, payload []byte) (*gatewaypb.DispatchResponse, error) {
	corrID, err := NewToken()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate correlation id: %v", err)
	}
	replyCh := make(chan *gatewaypb.Envelope, 1)
	ss.mu.Lock()
	if ss.pending == nil {
		ss.pending = map[string]chan *gatewaypb.Envelope{}
	}
	ss.pending[corrID] = replyCh
	ss.mu.Unlock()
	defer func() {
		ss.mu.Lock()
		delete(ss.pending, corrID)
		ss.mu.Unlock()
	}()

	env := &gatewaypb.Envelope{To: to, CorrelationId: corrID, Payload: payload}
	if err := ss.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_Envelope{Envelope: env}}); err != nil {
		return nil, status.Errorf(codes.Unavailable, "send dispatch envelope: %v", err)
	}

	select {
	case reply := <-replyCh:
		return &gatewaypb.DispatchResponse{Payload: reply.Payload}, nil
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	case <-time.After(dispatchTimeout):
		return nil, status.Error(codes.DeadlineExceeded, "dispatch reply timed out")
	}
}

// renderFor lists rows from the repos and renders one cluster's snapshot.
// Cluster-not-found renders the empty snapshot: a decommissioned cluster's
// managed objects cascade away.
func (s *Server) renderFor(ctx context.Context, cluster string) (RenderResult, error) {
	cl, err := s.repos.Clusters.GetByName(ctx, cluster)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return RenderResult{}, err
		}
		cl = &dbrepo.Cluster{Name: cluster}
	}
	dists, err := s.repos.Distributions.List(ctx)
	if err != nil {
		return RenderResult{}, err
	}
	groups, err := s.repos.Groups.List(ctx)
	if err != nil {
		return RenderResult{}, err
	}
	policies, err := s.repos.Policies.List(ctx)
	if err != nil {
		return RenderResult{}, err
	}
	var exceptions []dbrepo.PolicyException
	if s.repos.Exceptions != nil {
		exceptions, err = s.repos.Exceptions.List(ctx)
		if err != nil {
			return RenderResult{}, err
		}
	}
	res := RenderWithExceptions(toV1alpha1Cluster(cl), toV1alpha1Dists(dists), toV1alpha1Groups(groups), toV1alpha1Policies(policies), toV1alpha1Exceptions(exceptions))
	if s.repos.UninstallTasks == nil || cl.ID == 0 {
		return s.withAdmissionEventConfig(ctx, res)
	}
	tasks, err := s.repos.UninstallTasks.ListActiveByClusterID(ctx, cl.ID)
	if err != nil {
		return RenderResult{}, err
	}
	res.UninstallTasks = ToProtoUninstallTasks(tasks)
	res.Hash = hashWithUninstallTasks(res.Hash, res.UninstallTasks)
	return s.withAdmissionEventConfig(ctx, res)
}

func (s *Server) withAdmissionEventConfig(ctx context.Context, res RenderResult) (RenderResult, error) {
	if s.repos.Admission == nil {
		return res, nil
	}
	settings, err := s.repos.Admission.GetSettings(ctx)
	if err != nil {
		return RenderResult{}, err
	}
	policies, err := s.repos.Admission.ListPolicies(ctx)
	if err != nil {
		return RenderResult{}, err
	}
	config := &gatewaypb.AdmissionEventConfig{
		Version:              fmt.Sprint(settings.StoragePolicyVersion),
		DefaultRetentionDays: int32(settings.DefaultRetentionDays),
		RetryEnabled:         settings.RetryEnabled,
		MaxRetryAttempts:     int32(settings.MaxRetryAttempts),
		QueueCapacity:        int32(settings.QueueCapacity),
		// Pipeline observability is configured alongside the admission event
		// settings it instruments, so it rides the same config the member
		// clusters already receive.
		ObservationEnabled: settings.ObservationEnabled,
	}
	s.SetAdmissionObservationEnabled(settings.ObservationEnabled)
	if s.repos.Platform != nil {
		platformSettings, err := s.repos.Platform.GetSettings(ctx)
		if err != nil {
			return RenderResult{}, err
		}
		config.PlatformObservationEnabled = platformSettings.Enabled
		s.SetPlatformObservationEnabled(platformSettings.Enabled)
	}
	for _, p := range policies {
		config.StoragePolicies = append(config.StoragePolicies, &gatewaypb.AdmissionStoragePolicy{Priority: int32(p.Priority), MatchMode: p.MatchMode, Verdicts: p.Verdicts, Clusters: p.Clusters, Gvks: p.GVKs, Namespaces: p.Namespaces})
	}
	res.AdmissionEventConfig = config
	res.Hash = hashWithAdmissionEventConfig(res.Hash, config)
	return res, nil
}

// pushLoop renders and pushes a DesiredState snapshot immediately, then again
// whenever s.changes fires (only if the content hash changed, to avoid noisy
// resends during informer event bursts) or whenever pushResync elapses (always
// resent regardless of hash, to correct member-side drift on managed objects
// that the gateway can't otherwise observe). Returns once the stream errors (the
// recv loop in Sync will observe the same broken stream and return too) or ctx
// is canceled (Sync's defer cancelPush on the way out).
func (s *Server) pushLoop(ctx context.Context, sess *syncSession, cluster string) {
	ticker := time.NewTicker(pushResync)
	defer ticker.Stop()
	forceSend := false
	// wokeByNotify records whether the *previous* iteration's select woke via
	// waitCh (a genuine Notify/NotifyWithTrace event) rather than ticker.C
	// (resync fallback) or the implicit first pass (initial connect send).
	// Only a waitCh-driven wakeup that also has a real content change earns
	// a trace_id — see the gating below.
	wokeByNotify := false
	for {
		// Capture the wait channel before doing any work this iteration: a
		// Notify() firing anytime during render/send/markPushed below must
		// still be observed by this same channel in the select — Notifier's
		// close-and-replace broadcast only wakes waiters registered before
		// the Notify() call, so capturing after the work would lose it.
		waitCh := s.changes.Wait()
		// Non-destructive peek: every connected cluster's pushLoop goroutine
		// wakes on the same Notify() and sees the same stashed value here.
		// Which ones actually attach it to their outgoing DesiredState is
		// decided per-goroutine below, based on whether THIS cluster has a
		// real content change to send this iteration.
		traceID := s.changes.lastTraceID()
		if res, err := s.renderFor(ctx, cluster); err == nil {
			last, has := sess.last()
			changed := !has || last.Hash != res.Hash
			if changed || forceSend {
				sendTraceID := ""
				// Only attach the trace_id when this wakeup was a genuine
				// Notify (not the ticker resync, not the initial connect
				// send) AND this cluster actually has a content change to
				// report — a ticker-forced resend of an unchanged hash, or
				// another cluster's unrelated hash change, must never carry
				// a trace_id that belongs to a different trigger.
				if wokeByNotify && changed {
					sendTraceID = traceID
				}
				if err := sess.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_DesiredState{
					DesiredState: &gatewaypb.DesiredState{VersionHash: res.Hash, Objects: ToProto(res.Objects), TraceId: sendTraceID, UninstallTasks: res.UninstallTasks, AdmissionEventConfig: res.AdmissionEventConfig},
				}}); err != nil {
					return // stream already broken; recv loop will wind down too
				}
				sess.setLast(res)
			}
			s.markPushed(ctx, cluster, res)
		}
		forceSend = false
		wokeByNotify = false
		select {
		case <-ctx.Done():
			return
		case <-waitCh:
			wokeByNotify = true
		case <-ticker.C:
			// pushResync fallback: force a resend even if the render hash is
			// unchanged, so member-side drift on managed objects (which never
			// changes the gateway-side render inputs) gets corrected periodically
			// instead of persisting until something on the gateway actually changes.
			forceSend = true
		}
	}
}

// Sync handles the long-lived member stream: auth from metadata once, then
// two concurrent halves share one stream — the recv loop below answers
// Heartbeat/StatusReport with an Ack, while a pushLoop goroutine renders and
// pushes DesiredState snapshots. Both funnel sends through sess.send's mutex
// since grpc.ServerStream.Send is not safe for concurrent use.
func (s *Server) Sync(stream gatewaypb.Hub_SyncServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	cluster := firstMD(md, MDClusterKey)
	token := strings.TrimPrefix(firstMD(md, MDAuthKey), "Bearer ")
	if cluster == "" || token == "" {
		return status.Error(codes.Unauthenticated, "missing cluster identity metadata")
	}
	if err := s.validateSyncAuth(stream.Context(), cluster, token); err != nil {
		s.recordGatewayIncident(admissionobs.ReasonAuthFailure, 1, admissionobs.ReasonSyncAuthFailed, time.Now().UTC())
		return status.Error(codes.Unauthenticated, err.Error())
	}

	first, err := stream.Recv()
	if err != nil {
		return err
	}
	instance, role := cluster, "leader" // legacy clients before MemberIdentity
	var pending *gatewaypb.MemberMessage
	if identity := first.GetIdentity(); identity != nil {
		instance = strings.TrimSpace(identity.Instance)
		role = strings.TrimSpace(identity.Role)
		if instance == "" || (role != "leader" && role != "standby") {
			return status.Error(codes.InvalidArgument, "first identity frame requires instance and role")
		}
		if err := (&syncSession{stream: stream}).send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_Ack{Ack: s.ack()}}); err != nil {
			return err
		}
	} else {
		pending = first
	}

	sess := &syncSession{stream: stream}
	updateRole := func(next string) {
		if next != "leader" && next != "standby" {
			return
		}
		role = next
		for _, demoted := range s.sessions.put(cluster, instance, role, sess) {
			demoted.stopPush()
			slog.Warn("multiple operator leaders reported; newest claim selected",
				"cluster", cluster, "instance", instance)
		}
		if role == "leader" {
			if leader, ok := s.sessions.leader(cluster); ok && leader == sess {
				sess.startPush(stream.Context(), func(ctx context.Context) {
					s.pushLoop(ctx, sess, cluster)
				})
			}
		} else {
			sess.stopPush()
		}
	}
	updateRole(role)
	defer func() {
		sess.stopPush()
		if s.sessions.remove(cluster, instance, sess) && s.repos.Platform != nil {
			if row, err := s.repos.Clusters.GetByName(context.Background(), cluster); err == nil {
				if err := s.repos.Platform.RecordSessionLost(
					context.Background(), platformobs.ComponentOperator,
					row.ID, row.Name, instance, time.Now()); err != nil {
					slog.Warn("record lost operator leader session",
						"cluster", cluster, "instance", instance, "error", err)
				}
			}
		}
	}()

	for {
		msg := pending
		pending = nil
		if msg == nil {
			var err error
			msg, err = stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}
		switch m := msg.Msg.(type) {
		case *gatewaypb.MemberMessage_Heartbeat:
			updateRole(m.Heartbeat.Role)
			s.hb.MarkAlive(cluster, m.Heartbeat.OperatorVersion)
		case *gatewaypb.MemberMessage_Identity:
			return status.Error(codes.InvalidArgument, "identity frame is only valid as the first message")
		case *gatewaypb.MemberMessage_StatusReport:
			rep := m.StatusReport
			s.hb.RecordStatus(cluster, rep.ReadyPolicies, rep.InvalidPolicies, inventoryOf(rep))
			if last, has := sess.last(); has {
				s.markReported(stream.Context(), cluster, last, rep)
			}
		case *gatewaypb.MemberMessage_Envelope:
			// Dispatch reply path: route to the pending correlation id, if any.
			sess.deliverReply(m.Envelope)
		case *gatewaypb.MemberMessage_UninstallTaskReport:
			s.handleUninstallTaskReport(stream.Context(), cluster, m.UninstallTaskReport)
		case *gatewaypb.MemberMessage_AdmissionEventBatch:
			ack, err := s.handleAdmissionEventBatch(stream.Context(), cluster, m.AdmissionEventBatch)
			if err != nil {
				slog.Error("store admission event batch", "cluster", cluster, "error", err)
				ack = admissionEventFailureAck(ack, m.AdmissionEventBatch, "gateway_storage_failed")
			}
			if err := sess.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_AdmissionEventBatchAck{AdmissionEventBatchAck: ack}}); err != nil {
				return err
			}
			continue
		case *gatewaypb.MemberMessage_LinkIncidentBatch:
			ack := s.handleLinkIncidentBatch(stream.Context(), cluster, m.LinkIncidentBatch)
			if err := sess.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_LinkIncidentBatchAck{LinkIncidentBatchAck: ack}}); err != nil {
				return err
			}
			continue
		case *gatewaypb.MemberMessage_ComponentSampleBatch:
			if err := s.handleMemberComponentSamples(stream.Context(), cluster, instance, role, m.ComponentSampleBatch); err != nil {
				slog.Warn("store member platform samples", "cluster", cluster, "instance", instance, "error", err)
			}
		}
		if err := sess.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_Ack{Ack: s.ack()}}); err != nil {
			return err
		}
	}
}

func firstMD(md metadata.MD, key string) string {
	if vs := md.Get(key); len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func (s *Server) handleUninstallTaskReport(ctx context.Context, clusterName string, report *gatewaypb.UninstallTaskReport) {
	if s.repos.UninstallTasks == nil || report == nil || report.TaskId == 0 {
		return
	}
	cluster, err := s.repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		slog.Warn("ignore uninstall task report for unknown cluster", "cluster", clusterName, "error", err)
		return
	}
	task, err := s.repos.UninstallTasks.GetByID(ctx, report.TaskId)
	if err != nil || task.ClusterID != cluster.ID {
		slog.Warn("ignore uninstall task report with invalid task", "cluster", clusterName, "task_id", report.TaskId, "error", err)
		return
	}
	now := time.Now().UTC()
	var expected, next string
	switch report.State {
	case gatewaypb.UninstallTaskReport_RECEIVED:
		expected, next = dbrepo.UninstallTaskPending, dbrepo.UninstallTaskReceived
	case gatewaypb.UninstallTaskReport_SUCCEEDED:
		expected, next = dbrepo.UninstallTaskReceived, dbrepo.UninstallTaskSucceeded
	case gatewaypb.UninstallTaskReport_FAILED:
		expected, next = dbrepo.UninstallTaskReceived, dbrepo.UninstallTaskFailed
	default:
		return
	}
	if err := s.repos.UninstallTasks.Transition(ctx, task.ID, expected, next, report.Error, now); err != nil {
		if !errors.Is(err, dbrepo.ErrConflict) {
			slog.Warn("record uninstall task report", "task_id", task.ID, "error", err)
		}
		return
	}
	switch next {
	case dbrepo.UninstallTaskSucceeded:
		err = s.repos.Clusters.UpdatePhase(ctx, cluster.ID, v1alpha1.ClusterPhaseUninstalled)
	case dbrepo.UninstallTaskFailed:
		// A failed self-uninstall leaves the member operator running and still
		// connected. Keep its operational phase truthful so the console keeps
		// offering an uninstall action; the durable task retains the failure
		// reason and retry history.
		err = s.repos.Clusters.UpdatePhase(ctx, cluster.ID, v1alpha1.ClusterPhaseOnline)
	}
	if err != nil {
		slog.Warn("update cluster phase after uninstall report", "cluster", clusterName, "task_id", task.ID, "error", err)
	}
	s.changes.Notify()
}

// NotifyChange fires the shared change notifier, waking every Sync
// session's pushLoop to re-render and (if the hash changed) push a fresh
// snapshot. No target/topic-based filtering — every session re-renders and
// only actually resends if its own content hash changed.
func (s *Server) NotifyChange(ctx context.Context, req *gatewaypb.NotifyChangeRequest) (*gatewaypb.NotifyChangeResponse, error) {
	// req.TraceId rides along to the next pushLoop-sent DesiredState (spec
	// D18) via Notifier.NotifyWithTrace/lastTraceID, and from there gets
	// written as the kubesentry.io/last-trace-id annotation on the member
	// CRs the operator applies (internal/operator/hublink/apply.go).
	slog.Info("notify change", "topic", req.Topic, "trace_id", req.TraceId)
	s.changes.NotifyWithTrace(req.TraceId)
	// A storage-rule change also applies retroactively to stored events, so
	// kick a full cleanup pass. Fire-and-forget: the console write already
	// committed, and the daily pass is the fallback if this is dropped.
	if req.Topic == AdmissionEventSettingsTopic {
		s.cleaner.TriggerFullCleanup()
		if s.incidentCleaner != nil {
			s.incidentCleaner.Trigger()
		}
		// The pipeline observation flag lives in the admission settings, so a
		// write there is what toggles it. Reload eagerly rather than waiting for
		// the next render, which is what gates the gateway's own instrumentation.
		if s.repos.Admission != nil {
			if settings, err := s.repos.Admission.GetSettings(ctx); err != nil {
				slog.Warn("reload admission observation setting", "error", err)
			} else {
				s.SetAdmissionObservationEnabled(settings.ObservationEnabled)
			}
		}
	}
	if req.Topic == PlatformObservabilitySettingsTopic {
		if s.repos.Platform != nil {
			if settings, err := s.repos.Platform.GetSettings(ctx); err != nil {
				slog.Warn("reload platform observability settings", "error", err)
			} else {
				s.SetPlatformObservationEnabled(settings.Enabled)
			}
		}
	}
	return &gatewaypb.NotifyChangeResponse{}, nil
}

// Dispatch is a dumb relay (spec D11a): the caller names the exact target
// cluster via `to`, gateway does zero destination resolution — just looks
// up the live Sync session and forwards. Unavailable if not connected, no
// offline queueing.
func (s *Server) Dispatch(ctx context.Context, req *gatewaypb.DispatchRequest) (*gatewaypb.DispatchResponse, error) {
	if req.To == "" {
		return nil, status.Error(codes.InvalidArgument, "to is required")
	}
	sess, ok := s.sessions.leader(req.To)
	if !ok {
		return nil, status.Errorf(codes.Unavailable, "cluster %q not connected", req.To)
	}
	return sess.dispatch(ctx, req.To, req.Payload)
}

// Connect is the registration stream for non-cluster components (console-api
// today). Auth is a shared static token (spec D11c) carried as a marshaled
// ConnectRequest in the first frame's payload — lighter than the cluster
// Register/rotate flow, which is not appropriate across this trust boundary.
func (s *Server) Connect(stream gatewaypb.Hub_ConnectServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	var req gatewaypb.ConnectRequest
	if err := proto.Unmarshal(first.Payload, &req); err != nil {
		return status.Error(codes.InvalidArgument, "first frame payload must be a marshaled ConnectRequest")
	}
	if req.ComponentId == "" {
		return status.Error(codes.InvalidArgument, "component_id is required")
	}
	// s.sharedToken == "" means GATEWAY_SHARED_TOKEN was never configured —
	// reject outright rather than letting an empty-token caller "match" it
	// (two empty byte slices compare equal under ConstantTimeCompare).
	if s.sharedToken == "" {
		return status.Error(codes.FailedPrecondition, "gateway shared token not configured")
	}
	if subtle.ConstantTimeCompare([]byte(req.SharedToken), []byte(s.sharedToken)) != 1 {
		s.recordGatewayIncident(admissionobs.ReasonAuthFailure, 1, "shared token authentication failed", time.Now().UTC())
		return status.Error(codes.Unauthenticated, "invalid shared token")
	}

	s.connected.register(req.ComponentId, stream)
	defer s.connected.unregister(req.ComponentId, stream)
	for {
		frame, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if frame.To == platformSampleEnvelopeTarget {
			var batch gatewaypb.ComponentSampleBatch
			if err := proto.Unmarshal(frame.Payload, &batch); err != nil {
				slog.Warn("decode console platform samples", "component", req.ComponentId, "error", err)
				continue
			}
			if err := s.handleGatewayComponentSamples(stream.Context(), &batch); err != nil {
				slog.Warn("store console platform samples", "component", req.ComponentId, "error", err)
			}
		}
	}
}
