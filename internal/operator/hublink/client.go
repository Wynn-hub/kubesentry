// Package hublink is the member-cluster side of the hub connection:
// one-time registration, then a heartbeat loop over the Sync stream with
// exponential-backoff reconnect. It runs as a manager Runnable inside the
// operator; when no hub address is configured the operator never starts it,
// which is the entire single-cluster compatibility story.
package hublink

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/admissionpolicy"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/operator/uninstall"
	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

const (
	registerSecretName = "kubesentry-hub-register"
	authSecretName     = "kubesentry-hub-auth"
	keyToken           = "token"
	keyClusterToken    = "clusterToken"

	maxBackoff                   = 60 * time.Second
	defaultEventMaxRetryAttempts = 3
	defaultEventQueueCapacity    = 1024
	eventAckTimeout              = 5 * time.Second
	eventSendTick                = 500 * time.Millisecond
)

type Config struct {
	HubAddress      string // host:port；空 = 不启用（调用方保证）
	TLSCAFile       string // hub CA 证书路径；为空或文件不存在时回退到 tlssetup.DefaultCertBundle
	TLSServerName   string // 证书 DNS 名覆盖（经 NodePort/LB 直连 IP 时必填）
	ClusterName     string
	Namespace       string // operator 自身 namespace（Secret 所在）
	TLSSecretName   string // webhook TLS Secret；卸载时需要删除
	OperatorVersion string
	Reader          client.Reader // 非缓存读（kube-system UID、Secret）
	Client          client.Client // 写成员侧 auth Secret
	HeartbeatEvery  time.Duration
	// APIServerAddress is the apiserver URL as seen from inside the member
	// (rest.Config.Host); reported to the hub as part of cluster inventory.
	APIServerAddress string
	// Discovery/NodeLister feed cluster inventory (Kubernetes version, node
	// counts) into StatusReport. Nil disables that part of the inventory, which
	// is how single-cluster and older builds behave.
	Discovery  DiscoveryInterface
	NodeLister NodeLister
	// DialOverride 仅测试注入 bufconn；nil 时走真实 TLS 拨号。
	DialOverride func(ctx context.Context) (*grpc.ClientConn, error)
}

// DiscoveryInterface is the discovery surface the inventory collector needs.
// client-go's discovery.DiscoveryInterface satisfies it.
type DiscoveryInterface interface {
	ServerVersion() (*version.Info, error)
}

// NodeLister abstracts listing nodes so tests can stub the node inventory
// without a fake controller-runtime client.
type NodeLister interface {
	ListNodes(ctx context.Context) ([]corev1.Node, error)
}

type Link struct {
	cfg         Config
	leader      atomic.Bool
	eventsMu    sync.Mutex
	events      map[string]*queuedAdmissionEvent
	eventsWake  chan struct{}
	prefix      string
	configMu    sync.RWMutex
	eventConfig *gatewaypb.AdmissionEventConfig

	observationsMu      sync.Mutex
	observations        map[string]*queuedObservationReport
	observationsWake    chan struct{}
	observationStats    *admissionobs.AtomicCounters
	eventQueueGauge     admissionobs.QueueGauge
	observationSeries   string
	observationSequence atomic.Uint64
}

type queuedAdmissionEvent struct {
	event       *gatewaypb.AdmissionEvent
	retryCount  int
	attempt     int32
	inFlight    bool
	ackDeadline time.Time
}

func New(cfg Config) *Link {
	if cfg.HeartbeatEvery == 0 {
		cfg.HeartbeatEvery = 15 * time.Second
	}
	seriesID, err := admissionobs.NewSeriesID()
	if err != nil {
		panic("generate operator observation series id: " + err.Error())
	}
	link := &Link{
		cfg:               cfg,
		events:            map[string]*queuedAdmissionEvent{},
		eventsWake:        make(chan struct{}, 1),
		observations:      map[string]*queuedObservationReport{},
		observationsWake:  make(chan struct{}, 1),
		observationStats:  admissionobs.NewAtomicCounters(),
		observationSeries: seriesID,
	}
	link.eventQueueGauge.Observe(0, defaultEventQueueCapacity)
	return link
}

// NeedLeaderElection: only the elected operator holds the hub link.
func (l *Link) NeedLeaderElection() bool { return true }

// IsLeader is the final correctness gate for the local event HTTP endpoint.
// Service endpoint labels only optimize routing and can be stale during a
// lease handover; a non-leader must never acknowledge a batch.
func (l *Link) IsLeader() bool { return l.leader.Load() }

func (l *Link) SubmitAdmissionEvents(batch *gatewaypb.AdmissionEventBatch) ([]*gatewaypb.AdmissionEventBatchAck_Result, bool) {
	if !l.IsLeader() || batch == nil || len(batch.Events) == 0 {
		return nil, false
	}
	results := make([]*gatewaypb.AdmissionEventBatchAck_Result, 0, len(batch.Events))
	l.eventsMu.Lock()
	defer l.eventsMu.Unlock()
	_, _, queueCapacity := l.admissionDeliveryConfig()
	l.observationStats.Add(admissionobs.Counters{Received: int64(len(batch.Events))})
	for _, event := range batch.Events {
		if event == nil || event.EventId == "" {
			continue
		}
		if l.shouldDiscardAdmissionEvent(event) {
			l.observationStats.Add(admissionobs.Counters{PolicyFiltered: 1})
			l.observationStats.AddReason(admissionobs.ReasonFilter, admissionobs.ReasonStoragePolicyNoRetain, 1)
			results = append(results, &gatewaypb.AdmissionEventBatchAck_Result{EventId: event.EventId, Status: gatewaypb.AdmissionEventBatchAck_Result_DISCARDED, Reason: "storage_policy_no_retain"})
			continue
		}
		stored := proto.Clone(event).(*gatewaypb.AdmissionEvent)
		l.configMu.RLock()
		if l.eventConfig != nil {
			stored.StoragePolicyVersion = l.eventConfig.Version
		}
		l.configMu.RUnlock()
		if l.prefix != "" {
			stored.EventId = l.prefix + "-" + event.EventId
		}
		if _, exists := l.events[stored.EventId]; exists {
			results = append(results, &gatewaypb.AdmissionEventBatchAck_Result{EventId: event.EventId, Status: gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED})
			continue
		}
		if len(l.events) >= queueCapacity {
			l.observationStats.Add(admissionobs.Counters{QueueRejected: 1})
			l.observationStats.AddReason(admissionobs.ReasonPressure, admissionobs.ReasonOperatorEventQueueFull, 1)
			results = append(results, &gatewaypb.AdmissionEventBatchAck_Result{EventId: event.EventId, Status: gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE, Reason: "operator_event_queue_full"})
			continue
		}
		l.events[stored.EventId] = &queuedAdmissionEvent{event: stored}
		l.eventQueueGauge.Observe(int64(len(l.events)), int64(queueCapacity))
		results = append(results, &gatewaypb.AdmissionEventBatchAck_Result{EventId: event.EventId, Status: gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED})
	}
	select {
	case l.eventsWake <- struct{}{}:
	default:
	}
	return results, true
}

func (l *Link) Start(ctx context.Context) error {
	l.leader.Store(true)
	go l.generateOperatorObservations(ctx)
	if uid, err := l.clusterUID(ctx); err == nil && len(uid) >= 8 {
		l.prefix = uid[:8]
	}
	_ = l.setEventLeaderLabel(ctx, true)
	defer func() { l.leader.Store(false); _ = l.setEventLeaderLabel(context.Background(), false) }()
	backoff := time.Second
	for {
		connected, err := l.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if connected {
			// A session that reached "connected" was a healthy connection,
			// not a persistent failure — start the next retry fast.
			backoff = time.Second
		}
		slog.Warn("hub link disconnected, will retry", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// shouldDiscardAdmissionEvent applies the synced storage rules locally so events
// the hub would not store never occupy queue capacity or bandwidth. Rules are an
// allow-list: matching any rule stores the event, matching none discards it, and
// an empty rule set (or config not yet received) stores everything.
func (l *Link) shouldDiscardAdmissionEvent(event *gatewaypb.AdmissionEvent) bool {
	l.configMu.RLock()
	config := l.eventConfig
	l.configMu.RUnlock()
	if config == nil {
		return false
	}
	rules := make([]admissionpolicy.Rule, 0, len(config.StoragePolicies))
	for _, policy := range config.StoragePolicies {
		if policy == nil {
			continue
		}
		rules = append(rules, admissionpolicy.Rule{
			MatchMode:  policy.MatchMode,
			Verdicts:   policy.Verdicts,
			Clusters:   policy.Clusters,
			GVKs:       policy.Gvks,
			Namespaces: policy.Namespaces,
		})
	}
	return !admissionpolicy.ShouldStore(rules, admissionpolicy.Event{
		Verdict:     event.Verdict,
		ClusterName: l.cfg.ClusterName,
		APIGroup:    event.ApiGroup,
		Resource:    event.Resource,
		Namespace:   event.Namespace,
	})
}

func (l *Link) setEventLeaderLabel(ctx context.Context, enabled bool) error {
	podName := os.Getenv("POD_NAME")
	if podName == "" || l.cfg.Reader == nil || l.cfg.Client == nil {
		return nil
	}
	var pod corev1.Pod
	// Use the API reader rather than the manager's cached client. A cached
	// read of Pod causes controller-runtime to create a cluster-wide Pod
	// informer, which would require unnecessary list/watch permissions.
	if err := l.cfg.Reader.Get(ctx, types.NamespacedName{Namespace: l.cfg.Namespace, Name: podName}, &pod); err != nil {
		return err
	}
	patch := client.MergeFrom(pod.DeepCopy())
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	if enabled {
		pod.Labels["kubesentry.io/event-leader"] = "true"
	} else {
		delete(pod.Labels, "kubesentry.io/event-leader")
	}
	return l.cfg.Client.Patch(ctx, &pod, patch)
}

// runOnce dials, registers, and runs the heartbeat loop until it fails.
// connected reports whether the Sync stream was ever successfully opened,
// regardless of what happened afterwards — Start uses it to decide whether
// to reset the reconnect backoff.
func (l *Link) runOnce(ctx context.Context) (connected bool, err error) {
	conn, err := l.dial(ctx)
	if err != nil {
		return false, fmt.Errorf("dial hub: %w", err)
	}
	defer conn.Close()
	hc := gatewaypb.NewHubClient(conn)

	token, err := l.ensureRegistered(ctx, hc)
	if err != nil {
		return false, err
	}

	streamCtx := metadata.AppendToOutgoingContext(ctx,
		gateway.MDClusterKey, l.cfg.ClusterName,
		gateway.MDAuthKey, "Bearer "+token)
	stream, err := hc.Sync(streamCtx)
	if err != nil {
		l.clearCredentialIfUnauthenticated(ctx, err)
		return false, fmt.Errorf("open sync stream: %w", err)
	}
	slog.Info("hub link established", "hub", l.cfg.HubAddress, "cluster", l.cfg.ClusterName)

	var sendMu sync.Mutex
	send := func(m *gatewaypb.MemberMessage) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(m)
	}
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer func() {
		cancelSession()
		l.failInFlightAdmissionEvents("sync_stream_closed")
		l.failInFlightObservationReports(admissionobs.ReasonSyncStreamClosed)
	}()
	go l.sendAdmissionEvents(sessionCtx, send)

	var appliedMu sync.Mutex
	lastApplied := ""
	getApplied := func() string { appliedMu.Lock(); defer appliedMu.Unlock(); return lastApplied }
	setApplied := func(h string) { appliedMu.Lock(); defer appliedMu.Unlock(); lastApplied = h }

	statusReport := func(appliedVersion, applyErr string) *gatewaypb.MemberMessage {
		ready, invalid, err := policyCounts(ctx, l.cfg.Client)
		if err != nil {
			slog.Warn("count policies for status report", "error", err)
		}
		k8sVersion, nodeCount, readyNodeCount := l.clusterInventory(ctx)
		return &gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_StatusReport{StatusReport: &gatewaypb.StatusReport{
			ReadyPolicies:     ready,
			InvalidPolicies:   invalid,
			AppliedVersion:    appliedVersion,
			ApplyError:        applyErr,
			KubernetesVersion: k8sVersion,
			ApiServerAddress:  l.cfg.APIServerAddress,
			NodeCount:         nodeCount,
			ReadyNodeCount:    readyNodeCount,
		}}}
	}

	// Heartbeats on their own goroutine; every 4th tick doubles as a
	// periodic StatusReport so policy counts stay fresh after async
	// compilation settles. Send errors surface on the main Recv below.
	hbCtx, cancelHB := context.WithCancel(ctx)
	defer cancelHB()
	go func() {
		ticker := time.NewTicker(l.cfg.HeartbeatEvery)
		defer ticker.Stop()
		const statusEvery = 4
		for tick := 0; ; tick++ {
			if err := send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_Heartbeat{
				Heartbeat: &gatewaypb.Heartbeat{OperatorVersion: l.cfg.OperatorVersion},
			}}); err != nil {
				return
			}
			if tick%statusEvery == statusEvery-1 {
				if err := send(statusReport(getApplied(), "")); err != nil {
					return
				}
			}
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	// Server-side Sync validates auth before entering its recv loop and
	// returns codes.Unauthenticated immediately on a stale/rotated token —
	// that rejection surfaces here, on the first Recv, not from Sync()
	// itself.
	for {
		msg, err := stream.Recv()
		if err != nil {
			l.clearCredentialIfUnauthenticated(ctx, err)
			if ctx.Err() != nil {
				return true, nil
			}
			return true, fmt.Errorf("recv: %w", err)
		}
		switch m := msg.Msg.(type) {
		case *gatewaypb.HubMessage_Ack:
			// heartbeat/status-report acknowledged; nothing to do
		case *gatewaypb.HubMessage_AdmissionEventBatchAck:
			l.ackAdmissionEvents(m.AdmissionEventBatchAck)
		case *gatewaypb.HubMessage_ObservationReportBatchAck:
			l.ackObservationReports(m.ObservationReportBatchAck)
		case *gatewaypb.HubMessage_DesiredState:
			ds := m.DesiredState
			l.configMu.Lock()
			l.eventConfig = ds.AdmissionEventConfig
			l.configMu.Unlock()
			if len(ds.UninstallTasks) > 0 {
				for _, task := range ds.UninstallTasks {
					if err := sendUninstallReport(send, task.TaskId, gatewaypb.UninstallTaskReport_RECEIVED, ""); err != nil {
						return true, fmt.Errorf("report uninstall task received: %w", err)
					}
					runner := uninstall.Runner{Client: l.cfg.Client, Namespace: l.cfg.Namespace, TLSSecretName: l.cfg.TLSSecretName}
					err := runner.Run(ctx)
					state, detail := gatewaypb.UninstallTaskReport_SUCCEEDED, ""
					if err != nil {
						state, detail = gatewaypb.UninstallTaskReport_FAILED, err.Error()
					}
					if err := sendUninstallReport(send, task.TaskId, state, detail); err != nil {
						return true, fmt.Errorf("report uninstall task completion: %w", err)
					}
					if state == gatewaypb.UninstallTaskReport_SUCCEEDED {
						if err := runner.Finalize(ctx); err != nil {
							slog.Warn("finalize member uninstall after hub acknowledgement", "error", err)
						}
						return true, nil
					}
				}
			}
			// No same-hash skip here: applyDesiredState's own upsertObject
			// already compares each object's spec+labels and skips writes
			// that are already correct, so resending the same hash is cheap
			// when there's no drift — and it's exactly how member-side edits
			// to managed objects get corrected back (see pushResync in
			// internal/hub/server.go).
			applyErr := applyDesiredState(ctx, l.cfg.Client, ds)
			applyMsg := ""
			if applyErr != nil {
				applyMsg = applyErr.Error()
				slog.Warn("apply desired state", "hash", ds.VersionHash, "error", applyErr)
			} else {
				setApplied(ds.VersionHash)
				slog.Info("applied desired state", "hash", ds.VersionHash, "objects", len(ds.Objects))
			}
			if err := send(statusReport(ds.VersionHash, applyMsg)); err != nil {
				return true, fmt.Errorf("send status report: %w", err)
			}
		}
	}
}

func sendUninstallReport(send func(*gatewaypb.MemberMessage) error, taskID int64, state gatewaypb.UninstallTaskReport_State, detail string) error {
	return send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_UninstallTaskReport{UninstallTaskReport: &gatewaypb.UninstallTaskReport{TaskId: taskID, State: state, Error: detail}}})
}

func (l *Link) sendAdmissionEvents(ctx context.Context, send func(*gatewaypb.MemberMessage) error) {
	ticker := time.NewTicker(eventSendTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.eventsWake:
		case <-l.observationsWake:
		case <-ticker.C:
		}

		now := time.Now()
		if !l.sendObservationReports(now, send) {
			return
		}
		l.eventsMu.Lock()
		for id, queued := range l.events {
			if queued.inFlight && !queued.ackDeadline.After(now) {
				l.failAdmissionEventLocked(id, "gateway_ack_timeout")
			}
		}
		events := make([]*gatewaypb.AdmissionEvent, 0, len(l.events))
		eventIDs := make([]string, 0, len(l.events))
		for id, queued := range l.events {
			if queued.inFlight {
				continue
			}
			queued.inFlight = true
			queued.ackDeadline = now.Add(eventAckTimeout)
			queued.attempt++
			outgoing := proto.Clone(queued.event).(*gatewaypb.AdmissionEvent)
			outgoing.DeliveryAttempt = queued.attempt
			events = append(events, outgoing)
			eventIDs = append(eventIDs, id)
		}
		l.eventsMu.Unlock()
		if len(events) == 0 {
			continue
		}
		if err := send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_AdmissionEventBatch{AdmissionEventBatch: &gatewaypb.AdmissionEventBatch{Events: events}}}); err != nil {
			l.eventsMu.Lock()
			for _, id := range eventIDs {
				if queued := l.events[id]; queued != nil && queued.inFlight {
					l.failAdmissionEventLocked(id, "send_failed")
				}
			}
			l.eventsMu.Unlock()
			return
		}
		l.observationStats.Add(admissionobs.Counters{Forwarded: int64(len(events))})
	}
}

func (l *Link) ackAdmissionEvents(ack *gatewaypb.AdmissionEventBatchAck) {
	if ack == nil {
		return
	}
	l.eventsMu.Lock()
	defer l.eventsMu.Unlock()
	for _, result := range ack.Results {
		if result == nil {
			continue
		}
		queued := l.events[result.EventId]
		if queued == nil {
			continue
		}
		if result.DeliveryAttempt != 0 && result.DeliveryAttempt != queued.attempt {
			continue
		}
		switch result.Status {
		case gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED, gatewaypb.AdmissionEventBatchAck_Result_DISCARDED:
			delete(l.events, result.EventId)
			_, _, capacity := l.admissionDeliveryConfig()
			l.eventQueueGauge.Observe(int64(len(l.events)), int64(capacity))
		case gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE:
			l.failAdmissionEventLocked(result.EventId, result.Reason)
		}
	}
}

func (l *Link) admissionDeliveryConfig() (retryEnabled bool, maxRetryAttempts, queueCapacity int) {
	maxRetryAttempts = defaultEventMaxRetryAttempts
	queueCapacity = defaultEventQueueCapacity
	l.configMu.RLock()
	defer l.configMu.RUnlock()
	if l.eventConfig == nil {
		return false, maxRetryAttempts, queueCapacity
	}
	retryEnabled = l.eventConfig.RetryEnabled
	if l.eventConfig.MaxRetryAttempts > 0 {
		maxRetryAttempts = int(l.eventConfig.MaxRetryAttempts)
	}
	if l.eventConfig.QueueCapacity > 0 {
		queueCapacity = int(l.eventConfig.QueueCapacity)
	}
	return retryEnabled, maxRetryAttempts, queueCapacity
}

func (l *Link) failAdmissionEventLocked(eventID, reason string) {
	queued := l.events[eventID]
	if queued == nil {
		return
	}
	retryEnabled, maxRetryAttempts, _ := l.admissionDeliveryConfig()
	attempts := queued.retryCount + 1
	l.observationStats.AddReason(admissionobs.ReasonFault, reason, 1)
	if reason == admissionobs.ReasonGatewayACKTimeout {
		l.observationStats.Add(admissionobs.Counters{ACKTimeout: 1})
	}
	if !retryEnabled || queued.retryCount >= maxRetryAttempts {
		delete(l.events, eventID)
		l.observationStats.Add(admissionobs.Counters{Dropped: 1, RetryExhausted: 1})
		l.observationStats.AddReason(admissionobs.ReasonFault, admissionobs.ReasonRetryExhausted, 1)
		_, _, capacity := l.admissionDeliveryConfig()
		l.eventQueueGauge.Observe(int64(len(l.events)), int64(capacity))
		slog.Warn("admission event delivery dropped", "event", eventID, "attempts", attempts, "reason", reason, "retry_enabled", retryEnabled)
		return
	}
	queued.retryCount++
	queued.inFlight = false
	queued.ackDeadline = time.Time{}
	l.observationStats.Add(admissionobs.Counters{Retry: 1})
	slog.Warn("admission event delivery will retry", "event", eventID, "retry", queued.retryCount, "max_retries", maxRetryAttempts, "reason", reason)
	select {
	case l.eventsWake <- struct{}{}:
	default:
	}
}

func (l *Link) failInFlightAdmissionEvents(reason string) {
	l.eventsMu.Lock()
	defer l.eventsMu.Unlock()
	for id, queued := range l.events {
		if queued.inFlight {
			l.failAdmissionEventLocked(id, reason)
		}
	}
}

func (l *Link) dial(ctx context.Context) (*grpc.ClientConn, error) {
	if l.cfg.DialOverride != nil {
		return l.cfg.DialOverride(ctx)
	}
	creds, err := l.transportCredentials()
	if err != nil {
		return nil, err
	}
	return grpc.NewClient(l.cfg.HubAddress, grpc.WithTransportCredentials(creds))
}

// transportCredentials trusts a custom CA file when one is mounted
// (gateway.caBundle was set), otherwise falls back in code to
// tlssetup.DefaultCertBundle's baked-in CA — the same default the gateway
// itself uses when its own tls.ca/cert/key chart values are unset. This
// keeps the common case (no custom cert on either side) working with zero
// chart-level cert plumbing.
func (l *Link) transportCredentials() (credentials.TransportCredentials, error) {
	if l.cfg.TLSCAFile != "" {
		if _, err := os.Stat(l.cfg.TLSCAFile); err == nil {
			creds, err := credentials.NewClientTLSFromFile(l.cfg.TLSCAFile, l.cfg.TLSServerName)
			if err != nil {
				return nil, fmt.Errorf("load hub ca: %w", err)
			}
			return creds, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat hub ca file: %w", err)
		}
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(tlssetup.DefaultCertBundle().CACert) {
		return nil, fmt.Errorf("parse default hub ca")
	}
	serverName := l.cfg.TLSServerName
	if serverName == "" {
		serverName = tlssetup.DefaultServerName
	}
	return credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: serverName}), nil
}

// ensureRegistered returns the long-lived cluster token, registering with
// the one-time token first if no stored credential exists yet.
func (l *Link) ensureRegistered(ctx context.Context, hc gatewaypb.HubClient) (string, error) {
	var auth corev1.Secret
	err := l.cfg.Reader.Get(ctx,
		types.NamespacedName{Namespace: l.cfg.Namespace, Name: authSecretName}, &auth)
	if err == nil && len(auth.Data[keyClusterToken]) > 0 {
		return string(auth.Data[keyClusterToken]), nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("read auth secret: %w", err)
	}

	var reg corev1.Secret
	if err := l.cfg.Reader.Get(ctx,
		types.NamespacedName{Namespace: l.cfg.Namespace, Name: registerSecretName}, &reg); err != nil {
		return "", fmt.Errorf("no stored credential and no registration token secret: %w", err)
	}

	uid, err := l.clusterUID(ctx)
	if err != nil {
		return "", err
	}
	resp, err := hc.Register(ctx, &gatewaypb.RegisterRequest{
		ClusterName:       l.cfg.ClusterName,
		RegistrationToken: string(reg.Data[keyToken]),
		ClusterUid:        uid,
		OperatorVersion:   l.cfg.OperatorVersion,
	})
	if err != nil {
		return "", fmt.Errorf("register: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: l.cfg.Namespace, Name: authSecretName},
		Data:       map[string][]byte{keyClusterToken: []byte(resp.ClusterToken)},
	}
	if err := l.cfg.Client.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			var existing corev1.Secret
			if err := l.cfg.Reader.Get(ctx, types.NamespacedName{Namespace: l.cfg.Namespace, Name: authSecretName}, &existing); err != nil {
				return "", err
			}
			existing.Data = secret.Data
			if err := l.cfg.Client.Update(ctx, &existing); err != nil {
				return "", fmt.Errorf("update auth secret: %w", err)
			}
		} else {
			return "", fmt.Errorf("store cluster token: %w", err)
		}
	}
	slog.Info("registered with hub", "cluster", l.cfg.ClusterName)
	return resp.ClusterToken, nil
}

// clearCredentialIfUnauthenticated deletes the stored hub-auth Secret when
// err is a definitive codes.Unauthenticated rejection from the hub — meaning
// the token itself was rejected (e.g. an admin rotated it via the documented
// re-registration recovery: delete the hub-side auth Secret). Clearing it
// lets the next retry's ensureRegistered fall through to the registration
// flow instead of looping forever presenting the same dead token. Other
// error types (network errors, context cancellation, etc.) don't imply an
// invalid token and are left untouched.
func (l *Link) clearCredentialIfUnauthenticated(ctx context.Context, err error) {
	if status.Code(err) != codes.Unauthenticated {
		return
	}
	if clearErr := l.clearStoredCredential(ctx); clearErr != nil {
		slog.Warn("failed to clear stale hub credential after unauthenticated rejection", "error", clearErr)
	}
}

// clearStoredCredential deletes the member-side auth Secret; already-gone is
// treated as success.
func (l *Link) clearStoredCredential(ctx context.Context) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: l.cfg.Namespace, Name: authSecretName},
	}
	if err := l.cfg.Client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete stale auth secret: %w", err)
	}
	slog.Info("cleared stale hub credential; will re-register on next retry", "cluster", l.cfg.ClusterName)
	return nil
}

// clusterUID is the kube-system namespace UID — the stable machine
// identity of this cluster (spec D6).
func (l *Link) clusterUID(ctx context.Context) (string, error) {
	var ns corev1.Namespace
	if err := l.cfg.Reader.Get(ctx, types.NamespacedName{Name: "kube-system"}, &ns); err != nil {
		return "", fmt.Errorf("read kube-system uid: %w", err)
	}
	return string(ns.UID), nil
}

// clusterInventory collects the member cluster's own version/address/node
// counts for StatusReport. Every source is optional and best-effort: a member
// that cannot read one leaves it empty/zero and reports the rest, because a
// missing inventory value must never hold back the status report itself.
func (l *Link) clusterInventory(ctx context.Context) (k8sVersion string, nodeCount, readyNodeCount int32) {
	if l.cfg.Discovery != nil {
		if info, err := l.cfg.Discovery.ServerVersion(); err != nil {
			slog.Warn("read kubernetes version", "error", err)
		} else if info != nil {
			k8sVersion = info.GitVersion
		}
	}
	if l.cfg.NodeLister != nil {
		if nodes, err := l.cfg.NodeLister.ListNodes(ctx); err != nil {
			slog.Warn("list nodes for inventory", "error", err)
		} else {
			nodeCount = int32(len(nodes))
			for _, node := range nodes {
				if nodeReady(&node) {
					readyNodeCount++
				}
			}
		}
	}
	return k8sVersion, nodeCount, readyNodeCount
}

func nodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
