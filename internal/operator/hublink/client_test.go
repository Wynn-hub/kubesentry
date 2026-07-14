package hublink

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hub"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
)

const (
	hubNS    = "kubesentry-hub-system"
	memberNS = "kubesentry-system"
)

func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// startHub 起 bufconn hub，返回 dial 函数与 hub 侧 fake client。
func startHub(t *testing.T, objs ...client.Object) (func(ctx context.Context) (*grpc.ClientConn, error), client.Client, *hub.HeartbeatTracker) {
	t.Helper()
	hc := fake.NewClientBuilder().WithScheme(buildScheme(t)).
		WithStatusSubresource(&v1alpha1.Cluster{}, &v1alpha1.PolicyDistribution{}).WithObjects(objs...).Build()
	tracker := hub.NewHeartbeatTracker()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	hubpb.RegisterHubServer(gs, hub.NewServer(hc, hubNS, tracker, hub.NewNotifier()))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	dial := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return dial, hc, tracker
}

func TestLinkRegistersAndHeartbeats(t *testing.T) {
	dial, hubClient, tracker := startHub(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: hubNS, Name: hub.RegTokenSecretName("prod"),
				Labels: map[string]string{"kubesentry.io/cluster": "prod"},
			},
			Data: map[string][]byte{hub.KeyToken: []byte("reg-tok")},
		},
	)

	// 成员集群侧：kube-system（提供 UID）+ 注册 token Secret
	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-register"},
			Data:       map[string][]byte{"token": []byte("reg-tok")},
		},
	).Build()

	l := New(Config{
		HubAddress: "bufnet", ClusterName: "prod", Namespace: memberNS,
		OperatorVersion: "v-test", Reader: member, Client: member,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = l.Start(ctx) }()

	// 注册成功：成员侧 auth Secret 落盘
	waitFor(t, 3*time.Second, func() bool {
		var sec corev1.Secret
		return member.Get(ctx, types.NamespacedName{Namespace: memberNS, Name: "kubesentry-hub-auth"}, &sec) == nil &&
			len(sec.Data["clusterToken"]) == 64
	}, "member auth secret not written")

	// hub 侧注册 token 被消费 + UID 记录
	waitFor(t, 3*time.Second, func() bool {
		var cl v1alpha1.Cluster
		_ = hubClient.Get(ctx, types.NamespacedName{Name: "prod"}, &cl)
		return cl.Status.ClusterUID == "uid-1"
	}, "hub did not record cluster uid")

	// 心跳送达 tracker
	waitFor(t, 3*time.Second, func() bool {
		info, ok := tracker.Snapshot()["prod"]
		return ok && info.OperatorVersion == "v-test"
	}, "heartbeat never reached hub tracker")
}

func TestLinkReusesStoredToken(t *testing.T) {
	// 成员侧 auth Secret 已存在（此前注册过）→ 不走 Register，直接用存量 token 心跳
	dial, _, tracker := startHub(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: hubNS, Name: hub.AuthSecretName("prod")},
		Data: map[string][]byte{
			hub.KeyTokenHash:  []byte(hub.HashToken("stored-token")),
			hub.KeyClusterUID: []byte("uid-1"),
		},
	})

	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-auth"},
			Data:       map[string][]byte{"clusterToken": []byte("stored-token")},
		},
	).Build()

	l := New(Config{
		HubAddress: "bufnet", ClusterName: "prod", Namespace: memberNS,
		OperatorVersion: "v2", Reader: member, Client: member,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = l.Start(ctx) }()

	waitFor(t, 3*time.Second, func() bool {
		info, ok := tracker.Snapshot()["prod"]
		return ok && info.OperatorVersion == "v2"
	}, "heartbeat with stored token failed")
}

// TestRunOnceConnectedReporting drives runOnce directly (not through Start's
// retry loop) to assert the connected return value: false when the failure
// happens before the Sync stream opens (missing registration secret here),
// true once the stream has opened and the heartbeat loop has started
// (ctx cancellation while heartbeating still counts as "was connected").
func TestRunOnceConnectedReporting(t *testing.T) {
	dial, _, _ := startHub(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: hubNS, Name: hub.RegTokenSecretName("prod"),
				Labels: map[string]string{"kubesentry.io/cluster": "prod"},
			},
			Data: map[string][]byte{hub.KeyToken: []byte("reg-tok")},
		},
	)

	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"

	// First: no registration secret stored yet -> ensureRegistered fails
	// before the stream ever opens -> connected must be false.
	memberNoSecret := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(kubeSystem).Build()
	l := New(Config{
		HubAddress: "bufnet", ClusterName: "prod", Namespace: memberNS,
		OperatorVersion: "v-test", Reader: memberNoSecret, Client: memberNoSecret,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	connected, err := l.runOnce(context.Background())
	if connected {
		t.Fatal("expected connected=false when registration fails before stream opens")
	}
	if err == nil {
		t.Fatal("expected error when registration fails")
	}

	// Second: registration secret present -> stream opens successfully;
	// cancel ctx shortly after so the heartbeat loop exits cleanly.
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-register"},
			Data:       map[string][]byte{"token": []byte("reg-tok")},
		},
	).Build()
	l2 := New(Config{
		HubAddress: "bufnet", ClusterName: "prod", Namespace: memberNS,
		OperatorVersion: "v-test", Reader: member, Client: member,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	connected, err = l2.runOnce(ctx)
	if !connected {
		t.Fatalf("expected connected=true once the stream opened, got err=%v", err)
	}
}

// TestLinkSelfHealsAfterTokenRotation reproduces the documented admin
// recovery procedure: the member holds a stale kubesentry-hub-auth token the
// hub no longer recognizes (simulating the hub-side auth Secret having been
// deleted), while a fresh registration token has already been issued on both
// sides (simulating ClusterReconciler's half of the recovery). The stale
// token must get the member's Sync stream rejected as Unauthenticated,
// which should clear the local stored credential and let the next retry
// re-register with the fresh token — no manual intervention on the member.
func TestLinkSelfHealsAfterTokenRotation(t *testing.T) {
	dial, hubClient, tracker := startHub(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: hubNS, Name: hub.RegTokenSecretName("prod"),
				Labels: map[string]string{"kubesentry.io/cluster": "prod"},
			},
			Data: map[string][]byte{hub.KeyToken: []byte("fresh-reg-tok")},
		},
		// Deliberately no hub-side kubesentry-hub-auth-prod Secret: the
		// member's stored token below is orphaned/unrecognized by the hub.
	)

	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-auth"},
			Data:       map[string][]byte{"clusterToken": []byte("stale-orphaned-token")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-register"},
			Data:       map[string][]byte{"token": []byte("fresh-reg-tok")},
		},
	).Build()

	l := New(Config{
		HubAddress: "bufnet", ClusterName: "prod", Namespace: memberNS,
		OperatorVersion: "v-heal", Reader: member, Client: member,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = l.Start(ctx) }()

	// Self-healing: the stale token gets cleared and replaced with a
	// freshly issued one (never equal to the orphaned token).
	waitFor(t, 5*time.Second, func() bool {
		var sec corev1.Secret
		if err := member.Get(ctx, types.NamespacedName{Namespace: memberNS, Name: "kubesentry-hub-auth"}, &sec); err != nil {
			return false
		}
		tok := string(sec.Data["clusterToken"])
		return tok != "" && tok != "stale-orphaned-token" && len(tok) == 64
	}, "stale token was never cleared/replaced")

	// Re-registration completed on the hub side too.
	waitFor(t, 5*time.Second, func() bool {
		var cl v1alpha1.Cluster
		_ = hubClient.Get(ctx, types.NamespacedName{Name: "prod"}, &cl)
		return cl.Status.ClusterUID == "uid-1"
	}, "hub did not record cluster uid after re-registration")

	// Heartbeats flow again using the new token.
	waitFor(t, 5*time.Second, func() bool {
		info, ok := tracker.Snapshot()["prod"]
		return ok && info.OperatorVersion == "v-heal"
	}, "heartbeat never resumed after self-healing re-registration")
}

// TestLinkAppliesSnapshotAndReportsBack drives the real hub.Server (bufconn)
// against a member Link end to end: the hub renders a snapshot from a
// PolicyDistribution targeting this cluster, the member applies it (creating
// the managed Policy/PolicyGroup and pruning a pre-existing managed orphan),
// and reports back — which the hub folds into the distribution's per-cluster
// status.
func TestLinkAppliesSnapshotAndReportsBack(t *testing.T) {
	const clusterName = "prod"
	dial, hubClient, _ := startHub(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: hubNS, Name: hub.AuthSecretName(clusterName)},
			Data: map[string][]byte{
				hub.KeyTokenHash:  []byte(hub.HashToken("tok")),
				hub.KeyClusterUID: []byte("uid-1"),
			},
		},
		&v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: "p1"},
			Spec: v1alpha1.PolicySpec{
				EnforcementMode: v1alpha1.ModeAudit,
				Rego:            "package kubesentry\ndeny[msg]{false;msg:=\"x\"}",
			},
		},
		&v1alpha1.PolicyGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "g1"},
			Spec: v1alpha1.PolicyGroupSpec{
				Enabled:  true,
				Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1"}}},
			},
		},
		&v1alpha1.PolicyDistribution{
			ObjectMeta: metav1.ObjectMeta{Name: "d1"},
			Spec: v1alpha1.PolicyDistributionSpec{
				Clusters:     v1alpha1.DistributionClusters{ByName: []string{clusterName}},
				PolicyGroups: []string{"g1"},
			},
		},
	)

	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"
	orphan := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{
		Name:   "orphan",
		Labels: map[string]string{v1alpha1.ManagedByLabel: v1alpha1.ManagedByPlatform},
	}}
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		orphan,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-auth"},
			Data:       map[string][]byte{"clusterToken": []byte("tok")},
		},
	).Build()

	l := New(Config{
		HubAddress: "bufnet", ClusterName: clusterName, Namespace: memberNS,
		OperatorVersion: "v-apply", Reader: member, Client: member,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = l.Start(ctx) }()

	// Snapshot applied on the member: managed Policy p1 materialized.
	waitFor(t, 5*time.Second, func() bool {
		var p v1alpha1.Policy
		if err := member.Get(ctx, types.NamespacedName{Name: "p1"}, &p); err != nil {
			return false
		}
		return p.Labels[v1alpha1.ManagedByLabel] == v1alpha1.ManagedByPlatform
	}, "member never applied Policy p1")

	// Pre-existing managed orphan pruned since it's absent from the snapshot.
	waitFor(t, 5*time.Second, func() bool {
		var p v1alpha1.Policy
		return apierrors.IsNotFound(member.Get(ctx, types.NamespacedName{Name: "orphan"}, &p))
	}, "orphan managed policy not pruned on member")

	// Member's StatusReport flows back into the hub's distribution status.
	waitFor(t, 5*time.Second, func() bool {
		var d v1alpha1.PolicyDistribution
		if err := hubClient.Get(ctx, types.NamespacedName{Name: "d1"}, &d); err != nil {
			return false
		}
		for _, e := range d.Status.Clusters {
			if e.Name == clusterName && e.State == v1alpha1.DistributionSynced {
				return true
			}
		}
		return false
	}, "hub distribution status never reached Synced")
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}
