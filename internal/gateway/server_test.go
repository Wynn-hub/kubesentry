package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewayauth"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func mustCreateCluster(t *testing.T, repos Repos, c *dbrepo.Cluster) {
	t.Helper()
	if err := repos.Clusters.Create(context.Background(), c); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
}

func doRegister(t *testing.T, repos Repos, req *gatewaypb.RegisterRequest) (*gatewaypb.RegisterResponse, error) {
	t.Helper()
	s := NewServer(repos, gatewayNS, nil, NewNotifier(), "test-token")
	return s.Register(context.Background(), req)
}

func TestRegisterHappyPath(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("tok123")})

	resp, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok123",
		ClusterUid: "uid-1", OperatorVersion: "v1.2.0",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(resp.ClusterToken) != 64 {
		t.Fatalf("cluster token length = %d", len(resp.ClusterToken))
	}

	ctx := context.Background()
	cl, err := repos.Clusters.GetByName(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	// 注册 token 一次性：哈希必须被清空
	if cl.RegTokenHash != "" {
		t.Fatal("reg token hash still present")
	}
	// auth 只存哈希 + 绑定 UID
	if cl.AuthTokenHash != gatewayauth.HashToken(resp.ClusterToken) {
		t.Fatal("stored hash does not match issued token")
	}
	if cl.ClusterUID != "uid-1" || cl.OperatorVersion != "v1.2.0" {
		t.Fatalf("cluster = %+v", cl)
	}
}

func TestRegisterWrongToken(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("tok123")})

	_, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "WRONG", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestRegisterConsumedToken(t *testing.T) {
	// RegTokenHash empty — simulates a token already consumed by a prior
	// successful registration (or never issued).
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod"})

	_, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "any-token", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestRegisterUnknownCluster(t *testing.T) {
	repos := newTestRepos(t)
	_, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "ghost", RegistrationToken: "x", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestRegisterUIDConflictAcrossClusters(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "other", ClusterUID: "uid-1"})
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("tok123")})

	_, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok123", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists", status.Code(err))
	}
}

// TestPushLoopIncludesTraceIDFromNotifyChange proves the trace_id chain:
// NotifyChange's TraceId reaches the next actually-sent DesiredState, while
// the plain 60s ticker resync path (which never stashes a trace_id) sends
// an empty one. Split into two independent connections so the ticker phase
// (deliberately shortened) can't race with the NotifyChange phase (which
// relies on the default, effectively-infinite ticker never firing first).
func TestPushLoopIncludesTraceIDFromNotifyChange(t *testing.T) {
	t.Run("NotifyChange trace_id reaches the push", func(t *testing.T) {
		repos := newTestRepos(t)
		ctx := context.Background()
		mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: gatewayauth.HashToken("tok"), ClusterUID: "u"})
		if err := repos.Policies.Create(ctx, &dbrepo.Policy{Name: "p1", EnforcementMode: v1alpha1.ModeAudit}); err != nil {
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

		client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token"))
		sctx := metadata.AppendToOutgoingContext(context.Background(),
			gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
		stream, err := client.Sync(sctx)
		if err != nil {
			t.Fatal(err)
		}
		sendLeaderIdentity(t, stream, "operator-0")

		first := recvDesiredState(t, stream, 3*time.Second)
		if first.TraceId != "" {
			t.Fatalf("initial push trace_id = %q, want empty", first.TraceId)
		}

		// Pair a real content change with the NotifyChange call so pushLoop's
		// hash-gated send actually fires (an unchanged-hash NotifyChange has
		// nothing new to push, trace_id included).
		dist := &dbrepo.PolicyDistribution{
			Name: "d1",
			Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{
				Clusters:     gatewaymodel.DistributionClusters{ByName: []string{"prod"}},
				PolicyGroups: []string{"g1"},
			}),
		}
		if err := repos.Distributions.Create(ctx, dist); err != nil {
			t.Fatal(err)
		}
		if _, err := client.NotifyChange(ctx, &gatewaypb.NotifyChangeRequest{TraceId: "trace-abc"}); err != nil {
			t.Fatal(err)
		}
		second := recvDesiredState(t, stream, 3*time.Second)
		if second.TraceId != "trace-abc" {
			t.Fatalf("NotifyChange-triggered push trace_id = %q, want trace-abc", second.TraceId)
		}
	})

	t.Run("ticker resync never carries a trace_id", func(t *testing.T) {
		restore := SetPushResyncForTest(30 * time.Millisecond)
		defer restore()

		repos := newTestRepos(t)
		mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: gatewayauth.HashToken("tok"), ClusterUID: "u"})
		client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token"))

		ctx := metadata.AppendToOutgoingContext(context.Background(),
			gatewaypb.MDClusterKey, "prod", gatewaypb.MDAuthKey, "Bearer tok")
		stream, err := client.Sync(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sendLeaderIdentity(t, stream, "operator-0")

		_ = recvDesiredState(t, stream, 3*time.Second) // initial push
		tick := recvDesiredState(t, stream, 3*time.Second)
		if tick.TraceId != "" {
			t.Fatalf("ticker-driven resync trace_id = %q, want empty", tick.TraceId)
		}
	})
}

// dsReader forwards every DesiredState frame off a Sync stream into a
// buffered channel via a single dedicated background reader goroutine.
// Needed here (rather than reusing recvDesiredState per-call) because the
// multi-cluster test below must repeatedly check "no frame arrived" on one
// stream while frames are actively expected on another — spawning a fresh
// goroutine calling stream.Recv() per check would leave prior calls' Recv
// still blocked, and grpc-go does not allow concurrent RecvMsg calls on the
// same stream.
type dsReader struct {
	ch chan *gatewaypb.DesiredState
}

func newDSReader(stream gatewaypb.Hub_SyncClient) *dsReader {
	r := &dsReader{ch: make(chan *gatewaypb.DesiredState, 16)}
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				close(r.ch)
				return
			}
			if ds := msg.GetDesiredState(); ds != nil {
				r.ch <- ds
			}
		}
	}()
	return r
}

func (r *dsReader) recv(t *testing.T, timeout time.Duration) *gatewaypb.DesiredState {
	t.Helper()
	select {
	case ds, ok := <-r.ch:
		if !ok {
			t.Fatal("stream closed while waiting for DesiredState")
		}
		return ds
	case <-time.After(timeout):
		t.Fatal("no DesiredState received")
		return nil
	}
}

func (r *dsReader) expectNone(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case ds, ok := <-r.ch:
		if ok {
			t.Fatalf("unexpected DesiredState received on unaffected cluster's stream: %+v", ds)
		}
	case <-time.After(timeout):
	}
}

// TestNotifyChangeTraceIDGoesToTheClusterThatActuallyChanged reproduces the
// multi-cluster fan-out race: Notifier.Notify() broadcasts to every
// connected cluster's pushLoop goroutine at once, but only ONE of them
// (cluster-a or cluster-b, alternating each iteration) actually has a
// content change to send. Before the fix, a single read-then-clear
// consumeTraceID() meant whichever goroutine won the mutex race got the
// trace_id — sometimes the unaffected cluster (which then had nothing to
// send, silently dropping the trace_id), sometimes correctly the affected
// one, entirely up to goroutine scheduling. The fix makes this deterministic:
// every goroutine peeks the same stashed value and only attaches it when
// its OWN render hash changed. Alternating which cluster is "the changed
// one" across iterations means any scheduler bias toward one cluster's
// goroutine winning a would-be race is still caught — on the iterations
// where the biased winner isn't the actually-changed cluster, the old
// behavior would have misattributed or dropped the trace_id.
func TestNotifyChangeTraceIDGoesToTheClusterThatActuallyChanged(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "cluster-a", AuthTokenHash: gatewayauth.HashToken("tok-a"), ClusterUID: "ua"})
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "cluster-b", AuthTokenHash: gatewayauth.HashToken("tok-b"), ClusterUID: "ub"})

	client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token"))

	connect := func(cluster, tok string) gatewaypb.Hub_SyncClient {
		sctx := metadata.AppendToOutgoingContext(context.Background(),
			gatewaypb.MDClusterKey, cluster, gatewaypb.MDAuthKey, "Bearer "+tok)
		stream, err := client.Sync(sctx)
		if err != nil {
			t.Fatalf("connect %s: %v", cluster, err)
		}
		sendLeaderIdentity(t, stream, cluster+"-operator-0")
		return stream
	}
	streamA := connect("cluster-a", "tok-a")
	streamB := connect("cluster-b", "tok-b")
	readerA := newDSReader(streamA)
	readerB := newDSReader(streamB)

	// Drain the initial connect-triggered push on both — never carries a
	// trace_id and isn't part of what this test is checking.
	readerA.recv(t, 3*time.Second)
	readerB.recv(t, 3*time.Second)

	readers := map[string]*dsReader{"cluster-a": readerA, "cluster-b": readerB}
	targets := []string{"cluster-a", "cluster-b"}

	const iterations = 8
	for i := 0; i < iterations; i++ {
		target := targets[i%2]
		other := targets[(i+1)%2]

		// Unique policy/group/distribution names per iteration: Render dedupes
		// objects by policy/group name, so re-targeting the same names would
		// leave the target's hash unchanged and the test wouldn't exercise a
		// real content change on this iteration.
		pname := fmt.Sprintf("p-%d", i)
		gname := fmt.Sprintf("g-%d", i)
		dname := fmt.Sprintf("d-%d", i)
		if err := repos.Policies.Create(ctx, &dbrepo.Policy{Name: pname, EnforcementMode: v1alpha1.ModeAudit}); err != nil {
			t.Fatal(err)
		}
		if err := repos.Groups.Create(ctx, &dbrepo.PolicyGroup{
			Name: gname,
			Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
				Enabled:  true,
				Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: pname}}},
			}),
		}); err != nil {
			t.Fatal(err)
		}
		if err := repos.Distributions.Create(ctx, &dbrepo.PolicyDistribution{
			Name: dname,
			Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{
				Clusters:     gatewaymodel.DistributionClusters{ByName: []string{target}},
				PolicyGroups: []string{gname},
			}),
		}); err != nil {
			t.Fatal(err)
		}

		traceID := fmt.Sprintf("trace-%d", i)
		if _, err := client.NotifyChange(ctx, &gatewaypb.NotifyChangeRequest{TraceId: traceID}); err != nil {
			t.Fatal(err)
		}

		ds := readers[target].recv(t, 3*time.Second)
		if ds.TraceId != traceID {
			t.Fatalf("iter %d: target %s trace_id = %q, want %q", i, target, ds.TraceId, traceID)
		}
		// The unaffected cluster's hash didn't change, so it must not send
		// a frame at all — and critically, it must not have stolen the
		// trace_id in the process (verified above: the target got it).
		readers[other].expectNone(t, 500*time.Millisecond)
	}
}

func TestReRegisterSameUIDRotatesToken(t *testing.T) {
	// 已注册集群（绑定 uid-1）重新注册：管理员触发重发 reg token 后，此处模拟
	// 新一轮注册必须校验 UID 一致。
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", ClusterUID: "uid-1", RegTokenHash: gatewayauth.HashToken("tok456")})

	// UID 不一致 → 拒绝
	_, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok456", ClusterUid: "uid-OTHER",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied for UID mismatch", status.Code(err))
	}
	// UID 一致 → 成功换新
	resp, err := doRegister(t, repos, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok456", ClusterUid: "uid-1",
	})
	if err != nil || len(resp.ClusterToken) != 64 {
		t.Fatalf("re-register failed: %v", err)
	}
}
