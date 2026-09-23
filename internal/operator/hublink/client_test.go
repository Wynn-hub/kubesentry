package hublink

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/datatypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewayauth"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

type clientWithoutGet struct {
	client.Client
}

func (clientWithoutGet) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("cached client must not read pods")
}

const (
	gatewayNS = "kubesentry-gateway-system"
	memberNS  = "kubesentry-system"
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

func TestSetEventLeaderLabelReadsPodThroughAPIReader(t *testing.T) {
	t.Setenv("POD_NAME", "kubesentry-operator-0")
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-operator-0"}}
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(pod).Build()
	link := New(Config{Namespace: memberNS, Reader: member, Client: clientWithoutGet{Client: member}})

	if err := link.setEventLeaderLabel(context.Background(), true); err != nil {
		t.Fatalf("set event leader label: %v", err)
	}

	var actual corev1.Pod
	if err := member.Get(context.Background(), types.NamespacedName{Namespace: memberNS, Name: pod.Name}, &actual); err != nil {
		t.Fatalf("get patched pod: %v", err)
	}
	if got := actual.Labels["kubesentry.io/event-leader"]; got != "true" {
		t.Fatalf("event leader label = %q, want true", got)
	}
}

func mustCreateCluster(t *testing.T, repos gateway.Repos, c *dbrepo.Cluster) {
	t.Helper()
	if err := repos.Clusters.Create(context.Background(), c); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
}

// startHub starts a real gateway.Server (bufconn) backed by repos — the hub
// side of these end-to-end tests. Callers seed repos with whatever
// Cluster/Policy/PolicyGroup/PolicyDistribution rows the scenario needs
// before calling this.
func startHub(t *testing.T, repos gateway.Repos) (dial func(ctx context.Context) (*grpc.ClientConn, error), tracker *gateway.HeartbeatTracker) {
	t.Helper()
	tracker = gateway.NewHeartbeatTracker()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	gatewaypb.RegisterHubServer(gs, gateway.NewServer(repos, gatewayNS, tracker, gateway.NewNotifier(), "test-token"))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	dial = func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	return dial, tracker
}

func TestLinkRegistersAndHeartbeats(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("reg-tok")})
	dial, tracker := startHub(t, repos)

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
		cl, err := repos.Clusters.GetByName(context.Background(), "prod")
		return err == nil && cl.ClusterUID == "uid-1"
	}, "hub did not record cluster uid")

	// 心跳送达 tracker
	waitFor(t, 3*time.Second, func() bool {
		info, ok := tracker.Snapshot()["prod"]
		return ok && info.OperatorVersion == "v-test"
	}, "heartbeat never reached hub tracker")
}

func TestOnlyLeaderConsumesRegistrationTokenInHA(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("reg-tok")})
	dial, _ := startHub(t, repos)

	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: registerSecretName},
			Data:       map[string][]byte{keyToken: []byte("reg-tok")},
		},
	).Build()
	ctx := context.Background()
	conn, err := dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	hub := gatewaypb.NewHubClient(conn)

	neverElected := make(chan struct{})
	standby := New(Config{
		ClusterName: "prod", Namespace: memberNS, Reader: member, Client: member,
		LeaderElected: neverElected,
	})
	if _, err := standby.ensureRegistered(ctx, hub); !errors.Is(err, errWaitingForLeaderCredential) {
		t.Fatalf("standby registration error = %v, want waiting-for-leader", err)
	}
	cluster, err := repos.Clusters.GetByName(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if cluster.RegTokenHash == "" {
		t.Fatal("standby consumed the one-time registration token")
	}

	elected := make(chan struct{})
	close(elected)
	leader := New(Config{
		ClusterName: "prod", Namespace: memberNS, Reader: member, Client: member,
		LeaderElected: elected,
	})
	leader.leader.Store(true)
	leaderToken, err := leader.ensureRegistered(ctx, hub)
	if err != nil {
		t.Fatalf("leader register: %v", err)
	}
	standbyToken, err := standby.ensureRegistered(ctx, hub)
	if err != nil {
		t.Fatalf("standby read shared credential: %v", err)
	}
	if standbyToken != leaderToken {
		t.Fatalf("standby token differs from leader token")
	}
}

func TestOnlyLeaderClearsSharedRejectedCredential(t *testing.T) {
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: authSecretName},
			Data:       map[string][]byte{keyClusterToken: []byte("stale-token")},
		},
	).Build()
	leaderElection := make(chan struct{})
	standby := New(Config{
		Namespace: memberNS, Reader: member, Client: member, LeaderElected: leaderElection,
	})
	rejected := status.Error(codes.Unauthenticated, "invalid cluster token")
	standby.clearCredentialIfUnauthenticated(context.Background(), rejected)
	var secret corev1.Secret
	key := types.NamespacedName{Namespace: memberNS, Name: authSecretName}
	if err := member.Get(context.Background(), key, &secret); err != nil {
		t.Fatalf("standby deleted shared credential: %v", err)
	}

	standby.leader.Store(true)
	standby.clearCredentialIfUnauthenticated(context.Background(), rejected)
	if err := member.Get(context.Background(), key, &secret); !apierrors.IsNotFound(err) {
		t.Fatalf("leader did not delete rejected credential: %v", err)
	}
}

func TestLinkReusesStoredToken(t *testing.T) {
	// 成员侧 auth Secret 已存在（此前注册过）→ 不走 Register，直接用存量 token 心跳
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{
		Name: "prod", AuthTokenHash: gatewayauth.HashToken("stored-token"), ClusterUID: "uid-1",
	})
	dial, tracker := startHub(t, repos)

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
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("reg-tok")})
	dial, _ := startHub(t, repos)

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
// hub no longer recognizes (simulating the hub-side auth having been
// cleared), while a fresh registration token has already been issued on both
// sides (simulating the reconciler's half of the recovery). The stale token
// must get the member's Sync stream rejected as Unauthenticated, which
// should clear the local stored credential and let the next retry
// re-register with the fresh token — no manual intervention on the member.
func TestLinkSelfHealsAfterTokenRotation(t *testing.T) {
	repos := newTestRepos(t)
	// Deliberately no AuthTokenHash set: the member's stored token below is
	// orphaned/unrecognized by the hub.
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", RegTokenHash: gatewayauth.HashToken("fresh-reg-tok")})
	dial, tracker := startHub(t, repos)

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
		cl, err := repos.Clusters.GetByName(context.Background(), "prod")
		return err == nil && cl.ClusterUID == "uid-1"
	}, "hub did not record cluster uid after re-registration")

	// Heartbeats flow again using the new token.
	waitFor(t, 5*time.Second, func() bool {
		info, ok := tracker.Snapshot()["prod"]
		return ok && info.OperatorVersion == "v-heal"
	}, "heartbeat never resumed after self-healing re-registration")
}

// TestLinkAppliesSnapshotAndReportsBack drives the real gateway.Server (bufconn)
// against a member Link end to end: the hub renders a snapshot from a
// PolicyDistribution targeting this cluster, the member applies it (creating
// the managed Policy/PolicyGroup and pruning a pre-existing managed orphan),
// and reports back — which the hub folds into the distribution's per-cluster
// status.
func TestLinkAppliesSnapshotAndReportsBack(t *testing.T) {
	const clusterName = "prod"
	repos := newTestRepos(t)
	ctx0 := context.Background()
	mustCreateCluster(t, repos, &dbrepo.Cluster{
		Name: clusterName, AuthTokenHash: gatewayauth.HashToken("tok"), ClusterUID: "uid-1",
	})
	if err := repos.Policies.Create(ctx0, &dbrepo.Policy{
		Name: "p1", EnforcementMode: v1alpha1.ModeAudit,
		Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Groups.Create(ctx0, &dbrepo.PolicyGroup{
		Name: "g1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled:  true,
			Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1"}}},
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Distributions.Create(ctx0, &dbrepo.PolicyDistribution{
		Name: "d1",
		Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{
			Clusters:     gatewaymodel.DistributionClusters{ByName: []string{clusterName}},
			PolicyGroups: []string{"g1"},
		}),
	}); err != nil {
		t.Fatal(err)
	}
	dial, _ := startHub(t, repos)

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
		d, err := repos.Distributions.GetByName(context.Background(), "d1")
		if err != nil {
			return false
		}
		cl, err := repos.Clusters.GetByName(context.Background(), clusterName)
		if err != nil {
			return false
		}
		e, err := repos.Distributions.GetClusterStatus(context.Background(), d.ID, cl.ID)
		return err == nil && e != nil && e.Status == gatewaymodel.DistributionSynced
	}, "hub distribution status never reached Synced")
}

func TestLinkReappliesOnResendEvenWithUnchangedHash(t *testing.T) {
	const clusterName = "prod"
	repos := newTestRepos(t)
	ctx0 := context.Background()
	mustCreateCluster(t, repos, &dbrepo.Cluster{
		Name: clusterName, AuthTokenHash: gatewayauth.HashToken("tok"), ClusterUID: "uid-1",
	})
	if err := repos.Policies.Create(ctx0, &dbrepo.Policy{
		Name: "p1", EnforcementMode: v1alpha1.ModeAudit,
		Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Groups.Create(ctx0, &dbrepo.PolicyGroup{
		Name: "g1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled:  true,
			Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1"}}},
		}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Distributions.Create(ctx0, &dbrepo.PolicyDistribution{
		Name: "d1",
		Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{
			Clusters:     gatewaymodel.DistributionClusters{ByName: []string{clusterName}},
			PolicyGroups: []string{"g1"},
		}),
	}); err != nil {
		t.Fatal(err)
	}
	dial, _ := startHub(t, repos)

	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeSystem.UID = "uid-1"
	member := fake.NewClientBuilder().WithScheme(buildScheme(t)).WithObjects(
		kubeSystem,
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: memberNS, Name: "kubesentry-hub-auth"},
			Data:       map[string][]byte{"clusterToken": []byte("tok")},
		},
	).Build()

	l := New(Config{
		HubAddress: "bufnet", ClusterName: clusterName, Namespace: memberNS,
		OperatorVersion: "v-drift", Reader: member, Client: member,
		HeartbeatEvery: 50 * time.Millisecond, DialOverride: dial,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer gateway.SetPushResyncForTest(30 * time.Millisecond)()
	go func() { _ = l.Start(ctx) }()

	waitFor(t, 5*time.Second, func() bool {
		var p v1alpha1.Policy
		if err := member.Get(ctx, types.NamespacedName{Name: "p1"}, &p); err != nil {
			return false
		}
		return p.Labels[v1alpha1.ManagedByLabel] == v1alpha1.ManagedByPlatform
	}, "member never applied Policy p1")

	// Simulate local drift on the managed object: flip its mode directly.
	var drifted v1alpha1.Policy
	if err := member.Get(ctx, types.NamespacedName{Name: "p1"}, &drifted); err != nil {
		t.Fatal(err)
	}
	drifted.Spec.EnforcementMode = v1alpha1.ModeEnforce
	if err := member.Update(ctx, &drifted); err != nil {
		t.Fatal(err)
	}

	// A second, identical-hash DesiredState arriving over the same connection
	// must still correct the drift back to audit (SP2's per-connection
	// dedup no longer blanket-skips applyDesiredState).
	waitFor(t, 5*time.Second, func() bool {
		var p v1alpha1.Policy
		if err := member.Get(ctx, types.NamespacedName{Name: "p1"}, &p); err != nil {
			return false
		}
		return p.Spec.EnforcementMode == v1alpha1.ModeAudit
	}, "drifted policy was never re-corrected by a resent identical-hash snapshot")
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
