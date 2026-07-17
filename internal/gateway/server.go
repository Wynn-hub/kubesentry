package gateway

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
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
type Repos struct {
	Clusters      *dbrepo.ClusterRepo
	Policies      *dbrepo.PolicyRepo
	Groups        *dbrepo.GroupRepo
	Distributions *dbrepo.DistributionRepo
}

// Server implements the gatewaypb.Hub gRPC service.
type Server struct {
	gatewaypb.UnimplementedHubServer
	repos     Repos
	namespace string
	hb        *HeartbeatTracker
	changes   *Notifier
}

func NewServer(repos Repos, namespace string, hb *HeartbeatTracker, changes *Notifier) *Server {
	return &Server{repos: repos, namespace: namespace, hb: hb, changes: changes}
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

	// 校验一次性注册 token：gateway 只存哈希。
	if subtle.ConstantTimeCompare([]byte(cl.RegTokenHash), []byte(HashToken(req.RegistrationToken))) != 1 {
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

// syncSession serializes stream.Send between the recv loop's Acks and the
// pusher goroutine, and remembers the last pushed snapshot for StatusReport
// correlation.
type syncSession struct {
	mu      sync.Mutex
	stream  gatewaypb.Hub_SyncServer
	lastRes RenderResult
	hasLast bool
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
	return Render(toV1alpha1Cluster(cl), toV1alpha1Dists(dists), toV1alpha1Groups(groups), toV1alpha1Policies(policies)), nil
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
	for {
		// Capture the wait channel before doing any work this iteration: a
		// Notify() firing anytime during render/send/markPushed below must
		// still be observed by this same channel in the select — Notifier's
		// close-and-replace broadcast only wakes waiters registered before
		// the Notify() call, so capturing after the work would lose it.
		waitCh := s.changes.Wait()
		if res, err := s.renderFor(ctx, cluster); err == nil {
			last, has := sess.last()
			if !has || last.Hash != res.Hash || forceSend {
				if err := sess.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_DesiredState{
					DesiredState: &gatewaypb.DesiredState{VersionHash: res.Hash, Objects: ToProto(res.Objects)},
				}}); err != nil {
					return // stream already broken; recv loop will wind down too
				}
				sess.setLast(res)
			}
			s.markPushed(ctx, cluster, res)
		}
		forceSend = false
		select {
		case <-ctx.Done():
			return
		case <-waitCh:
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
		return status.Error(codes.Unauthenticated, err.Error())
	}

	sess := &syncSession{stream: stream}
	pushCtx, cancelPush := context.WithCancel(stream.Context())
	defer cancelPush()
	go s.pushLoop(pushCtx, sess, cluster)

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch m := msg.Msg.(type) {
		case *gatewaypb.MemberMessage_Heartbeat:
			s.hb.MarkAlive(cluster, m.Heartbeat.OperatorVersion)
		case *gatewaypb.MemberMessage_StatusReport:
			rep := m.StatusReport
			s.hb.RecordStatus(cluster, rep.ReadyPolicies, rep.InvalidPolicies)
			if last, has := sess.last(); has {
				s.markReported(stream.Context(), cluster, last, rep)
			}
		}
		if err := sess.send(&gatewaypb.HubMessage{Msg: &gatewaypb.HubMessage_Ack{Ack: &gatewaypb.Ack{}}}); err != nil {
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
