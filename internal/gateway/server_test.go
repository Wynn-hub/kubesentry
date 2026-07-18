package gateway

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
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
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: HashToken("tok123")})

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
	if cl.AuthTokenHash != HashToken(resp.ClusterToken) {
		t.Fatal("stored hash does not match issued token")
	}
	if cl.ClusterUID != "uid-1" || cl.OperatorVersion != "v1.2.0" {
		t.Fatalf("cluster = %+v", cl)
	}
}

func TestRegisterWrongToken(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: HashToken("tok123")})

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
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: HashToken("tok123")})

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
		mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: HashToken("tok"), ClusterUID: "u"})
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
			MDClusterKey, "prod", MDAuthKey, "Bearer tok")
		stream, err := client.Sync(sctx)
		if err != nil {
			t.Fatal(err)
		}

		first := recvDesiredState(t, stream, 3*time.Second)
		if first.TraceId != "" {
			t.Fatalf("initial push trace_id = %q, want empty", first.TraceId)
		}

		// Pair a real content change with the NotifyChange call so pushLoop's
		// hash-gated send actually fires (an unchanged-hash NotifyChange has
		// nothing new to push, trace_id included).
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
		mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", AuthTokenHash: HashToken("tok"), ClusterUID: "u"})
		client := startBufconnGateway(t, NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token"))

		ctx := metadata.AppendToOutgoingContext(context.Background(),
			MDClusterKey, "prod", MDAuthKey, "Bearer tok")
		stream, err := client.Sync(ctx)
		if err != nil {
			t.Fatal(err)
		}

		_ = recvDesiredState(t, stream, 3*time.Second) // initial push
		tick := recvDesiredState(t, stream, 3*time.Second)
		if tick.TraceId != "" {
			t.Fatalf("ticker-driven resync trace_id = %q, want empty", tick.TraceId)
		}
	})
}

func TestReRegisterSameUIDRotatesToken(t *testing.T) {
	// 已注册集群（绑定 uid-1）重新注册：管理员触发重发 reg token 后，此处模拟
	// 新一轮注册必须校验 UID 一致。
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", ClusterUID: "uid-1", RegTokenHash: HashToken("tok456")})

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
