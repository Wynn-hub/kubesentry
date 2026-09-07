package gateway

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func startBufconnGateway(t *testing.T, s *Server) gatewaypb.HubClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	gatewaypb.RegisterHubServer(gs, s)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return gatewaypb.NewHubClient(conn)
}

func TestSyncHeartbeatFlow(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{
		Name: "prod", AuthTokenHash: HashToken("secret-token"), ClusterUID: "uid-1",
	})
	tr := NewHeartbeatTracker()
	client := startBufconnGateway(t, NewServer(repos, gatewayNS, tr, NewNotifier(), "test-token"))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-kubesentry-cluster", "prod",
		"authorization", "Bearer secret-token")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_Heartbeat{
		Heartbeat: &gatewaypb.Heartbeat{OperatorVersion: "v9"},
	}}); err != nil {
		t.Fatal(err)
	}
	// The pushLoop goroutine may deliver a DesiredState snapshot before or
	// interleaved with the recv loop's Ack — skip any DesiredState frames
	// and assert only on the Ack this test actually cares about.
	var reply *gatewaypb.HubMessage
	for {
		reply, err = stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if reply.GetDesiredState() != nil {
			continue
		}
		break
	}
	if reply.GetAck() == nil {
		t.Fatalf("expected Ack, got %+v", reply)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if info, ok := tr.Snapshot()["prod"]; ok && info.OperatorVersion == "v9" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("heartbeat not recorded in tracker")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSyncRejectsBadToken(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: HashToken("right")})
	client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token"))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-kubesentry-cluster", "prod",
		"authorization", "Bearer wrong")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestSyncAdmissionStorageFailureReturnsRetryableWithoutClosingStream(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: HashToken("tok"), ClusterUID: "uid-1"})
	// Force the same handler error path as a database write failure without
	// destroying the repositories needed by authentication and pushLoop.
	repos.Admission = nil
	client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token"))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_AdmissionEventBatch{
		AdmissionEventBatch: &gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{{
			EventId: "event-1", Verdict: "deny", DeliveryAttempt: 7,
		}}},
	}}); err != nil {
		t.Fatal(err)
	}

	var eventAck *gatewaypb.AdmissionEventBatchAck
	for eventAck == nil {
		reply, err := stream.Recv()
		if err != nil {
			t.Fatalf("stream closed after admission storage failure: %v", err)
		}
		eventAck = reply.GetAdmissionEventBatchAck()
	}
	if len(eventAck.Results) != 1 ||
		eventAck.Results[0].Status != gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE ||
		eventAck.Results[0].DeliveryAttempt != 7 {
		t.Fatalf("event ack = %+v, want RETRYABLE attempt 7", eventAck)
	}

	if err := stream.Send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_Heartbeat{
		Heartbeat: &gatewaypb.Heartbeat{OperatorVersion: "v1"},
	}}); err != nil {
		t.Fatalf("send heartbeat after admission failure: %v", err)
	}
	for {
		reply, err := stream.Recv()
		if err != nil {
			t.Fatalf("stream closed before heartbeat ack: %v", err)
		}
		if reply.GetAck() != nil {
			break
		}
	}
}

// recvDesiredState drains the stream in a background goroutine until a
// DesiredState frame arrives (skipping Acks), or times out.
func recvDesiredState(t *testing.T, stream gatewaypb.Hub_SyncClient, timeout time.Duration) *gatewaypb.DesiredState {
	t.Helper()
	type item struct {
		ds  *gatewaypb.DesiredState
		err error
	}
	ch := make(chan item, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				ch <- item{err: err}
				return
			}
			if ds := msg.GetDesiredState(); ds != nil {
				ch <- item{ds: ds}
				return
			}
		}
	}()
	select {
	case it := <-ch:
		if it.err != nil {
			t.Fatalf("recv: %v", it.err)
		}
		return it.ds
	case <-time.After(timeout):
		t.Fatal("no DesiredState received")
		return nil
	}
}

func sendLeaderIdentity(t *testing.T, stream gatewaypb.Hub_SyncClient, instance string) {
	t.Helper()
	if err := stream.Send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_Identity{
		Identity: &gatewaypb.MemberIdentity{Instance: instance, Role: "leader"},
	}}); err != nil {
		t.Fatalf("send member identity: %v", err)
	}
}

func TestSyncPushesSnapshotOnConnectAndChange(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: HashToken("tok"), ClusterUID: "u"})
	if err := repos.Policies.Create(ctx, &dbrepo.Policy{
		Name: "p1", EnforcementMode: v1alpha1.ModeAudit,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Groups.Create(ctx, &dbrepo.PolicyGroup{
		Name: "g1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled:  true,
			Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1"}}},
		}),
	}); err != nil {
		t.Fatal(err)
	}

	notifier := NewNotifier()
	client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), notifier, "test-token"))

	sctx := metadata.AppendToOutgoingContext(context.Background(),
		gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
	stream, err := client.Sync(sctx)
	if err != nil {
		t.Fatal(err)
	}
	sendLeaderIdentity(t, stream, "operator-0")

	// 连接即收到初始快照。The migrated schema seeds a built-in "default"
	// PolicyGroup/PolicyDistribution matching every cluster (Task 3, mirrors
	// the Helm chart's built-in catalogue) — a freshly migrated test DB is
	// never actually empty, so assert against a baseline object count
	// instead of zero.
	first := recvDesiredState(t, stream, 3*time.Second)
	if first.VersionHash == "" {
		t.Fatalf("expected non-empty initial snapshot hash, got %+v", first)
	}
	baseline := len(first.Objects)

	// 建分发 → Notify → 收到多 2 对象的快照。
	dist := &dbrepo.PolicyDistribution{
		Name: "d1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ByName: []string{"prod"}},
			PolicyGroups: []string{"g1"},
		}),
	}
	if err := repos.Distributions.Create(ctx, dist); err != nil {
		t.Fatal(err)
	}
	notifier.Notify()
	second := recvDesiredState(t, stream, 3*time.Second)
	if len(second.Objects) != baseline+2 || second.VersionHash == first.VersionHash {
		t.Fatalf("expected baseline+2-object snapshot with new hash, got %+v (baseline=%d)", second, baseline)
	}

	// 删分发 → 回到基线快照（级联删除的协议表达）。
	if err := repos.Distributions.Delete(ctx, dist.ID); err != nil {
		t.Fatal(err)
	}
	notifier.Notify()
	third := recvDesiredState(t, stream, 3*time.Second)
	if len(third.Objects) != baseline || third.VersionHash != first.VersionHash {
		t.Fatalf("expected snapshot to return to baseline, got %+v", third)
	}
}

func TestSyncResendsOnResyncTickEvenWithUnchangedHash(t *testing.T) {
	orig := pushResync
	pushResync = 30 * time.Millisecond
	defer func() { pushResync = orig }()

	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: HashToken("tok"), ClusterUID: "u"})
	notifier := NewNotifier()
	client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), notifier, "test-token"))

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
	stream, err := client.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sendLeaderIdentity(t, stream, "operator-0")

	first := recvDesiredState(t, stream, 3*time.Second)
	second := recvDesiredState(t, stream, 3*time.Second)
	if first.VersionHash != second.VersionHash {
		t.Fatalf("expected identical hash across resync tick, got %q vs %q", first.VersionHash, second.VersionHash)
	}
}
