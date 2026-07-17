package gateway

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func regTokenSecret(cluster, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: gatewayNS,
			Name:      RegTokenSecretName(cluster),
			Labels:    map[string]string{"kubesentry.io/cluster": cluster},
		},
		Data: map[string][]byte{KeyToken: []byte(token)},
	}
}

func doRegister(t *testing.T, c client.Client, req *gatewaypb.RegisterRequest) (*gatewaypb.RegisterResponse, error) {
	t.Helper()
	s := NewServer(c, gatewayNS, nil, NewNotifier())
	return s.Register(context.Background(), req)
}

func TestRegisterHappyPath(t *testing.T) {
	c := newGatewayClient(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		regTokenSecret("prod", "tok123"),
	)
	resp, err := doRegister(t, c, &gatewaypb.RegisterRequest{
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
	// 注册 token 一次性：Secret 必须被删除
	var reg corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: gatewayNS, Name: RegTokenSecretName("prod")}, &reg); !apierrors.IsNotFound(err) {
		t.Fatalf("reg token secret still exists (err=%v)", err)
	}
	// auth Secret 只存哈希 + 绑定 UID
	var auth corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: gatewayNS, Name: AuthSecretName("prod")}, &auth); err != nil {
		t.Fatalf("auth secret missing: %v", err)
	}
	if string(auth.Data[KeyTokenHash]) != HashToken(resp.ClusterToken) {
		t.Fatal("stored hash does not match issued token")
	}
	if string(auth.Data[KeyClusterUID]) != "uid-1" {
		t.Fatalf("bound uid = %q", auth.Data[KeyClusterUID])
	}
	// Cluster.status 记录 UID 与版本
	var cl v1alpha1.Cluster
	_ = c.Get(ctx, types.NamespacedName{Name: "prod"}, &cl)
	if cl.Status.ClusterUID != "uid-1" || cl.Status.OperatorVersion != "v1.2.0" {
		t.Fatalf("status = %+v", cl.Status)
	}
}

func TestRegisterWrongToken(t *testing.T) {
	c := newGatewayClient(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		regTokenSecret("prod", "tok123"),
	)
	_, err := doRegister(t, c, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "WRONG", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestRegisterConsumedToken(t *testing.T) {
	// No reg-token Secret present — simulates a token already consumed by
	// a prior successful registration (or never issued).
	c := newGatewayClient(t, &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}})
	_, err := doRegister(t, c, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "any-token", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestRegisterUnknownCluster(t *testing.T) {
	c := newGatewayClient(t)
	_, err := doRegister(t, c, &gatewaypb.RegisterRequest{
		ClusterName: "ghost", RegistrationToken: "x", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestRegisterUIDConflictAcrossClusters(t *testing.T) {
	other := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "other"}}
	other.Status.ClusterUID = "uid-1"
	c := newGatewayClient(t,
		other,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		regTokenSecret("prod", "tok123"),
	)
	_, err := doRegister(t, c, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok123", ClusterUid: "uid-1",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists", status.Code(err))
	}
}

func TestReRegisterSameUIDRotatesToken(t *testing.T) {
	// 已注册集群（auth 存在且绑定 uid-1）重新注册：管理员删 auth 后 reconciler
	// 会重发 reg token；此处模拟新一轮注册必须校验 UID 一致。
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}}
	cl.Status.ClusterUID = "uid-1"
	c := newGatewayClient(t, cl, regTokenSecret("prod", "tok456"))
	// UID 不一致 → 拒绝
	_, err := doRegister(t, c, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok456", ClusterUid: "uid-OTHER",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied for UID mismatch", status.Code(err))
	}
	// UID 一致 → 成功换新
	resp, err := doRegister(t, c, &gatewaypb.RegisterRequest{
		ClusterName: "prod", RegistrationToken: "tok456", ClusterUid: "uid-1",
	})
	if err != nil || len(resp.ClusterToken) != 64 {
		t.Fatalf("re-register failed: %v", err)
	}
}
