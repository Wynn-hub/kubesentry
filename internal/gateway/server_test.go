package gateway

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
