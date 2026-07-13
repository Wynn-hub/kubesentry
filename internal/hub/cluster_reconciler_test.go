package hub

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

const hubNS = "kubesentry-hub-system"

func buildHubScheme(t *testing.T) *runtime.Scheme {
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

func newHubClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(buildHubScheme(t)).
		WithStatusSubresource(&v1alpha1.Cluster{}).
		WithObjects(objs...).
		Build()
}

func reconcileCluster(t *testing.T, c client.Client, name string) {
	t.Helper()
	r := NewClusterReconciler(c, hubNS)
	_, err := r.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func TestReconcileNewClusterIssuesRegToken(t *testing.T) {
	c := newHubClient(t, &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}})
	reconcileCluster(t, c, "prod")

	var sec corev1.Secret
	if err := c.Get(context.Background(),
		types.NamespacedName{Namespace: hubNS, Name: RegTokenSecretName("prod")}, &sec); err != nil {
		t.Fatalf("reg-token secret not created: %v", err)
	}
	if len(sec.Data[KeyToken]) != 64 {
		t.Fatalf("token length = %d, want 64", len(sec.Data[KeyToken]))
	}
	if sec.Labels["kubesentry.io/cluster"] != "prod" {
		t.Fatalf("labels = %v", sec.Labels)
	}

	var cl v1alpha1.Cluster
	if err := c.Get(context.Background(), types.NamespacedName{Name: "prod"}, &cl); err != nil {
		t.Fatal(err)
	}
	if cl.Status.Phase != v1alpha1.ClusterPhaseRegistering {
		t.Fatalf("phase = %q, want Registering", cl.Status.Phase)
	}
}

func TestReconcileIdempotentWhenRegTokenExists(t *testing.T) {
	c := newHubClient(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
	)
	reconcileCluster(t, c, "prod")
	var first corev1.Secret
	_ = c.Get(context.Background(),
		types.NamespacedName{Namespace: hubNS, Name: RegTokenSecretName("prod")}, &first)

	reconcileCluster(t, c, "prod") // second run must not rotate the token
	var second corev1.Secret
	_ = c.Get(context.Background(),
		types.NamespacedName{Namespace: hubNS, Name: RegTokenSecretName("prod")}, &second)
	if string(first.Data[KeyToken]) != string(second.Data[KeyToken]) {
		t.Fatal("reconcile rotated an existing registration token")
	}
}

func TestReconcileSkipsWhenAuthSecretExists(t *testing.T) {
	c := newHubClient(t,
		&v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: hubNS, Name: AuthSecretName("prod")},
			Data:       map[string][]byte{KeyTokenHash: []byte("h"), KeyClusterUID: []byte("u")},
		},
	)
	reconcileCluster(t, c, "prod")

	var sec corev1.Secret
	err := c.Get(context.Background(),
		types.NamespacedName{Namespace: hubNS, Name: RegTokenSecretName("prod")}, &sec)
	if err == nil {
		t.Fatal("reg token issued although cluster already registered")
	}
}

func TestReconcileDeletedClusterNoError(t *testing.T) {
	c := newHubClient(t)
	reconcileCluster(t, c, "gone") // must not error on missing Cluster
}

func TestReconcilePreservesExistingPhaseOnReRegistration(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}}
	cl.Status.Phase = v1alpha1.ClusterPhaseOnline // simulates: was Online, admin deleted auth secret to re-register
	c := newHubClient(t, cl)
	reconcileCluster(t, c, "prod")

	var got v1alpha1.Cluster
	if err := c.Get(context.Background(), types.NamespacedName{Name: "prod"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != v1alpha1.ClusterPhaseOnline {
		t.Fatalf("phase = %q, want unchanged Online", got.Status.Phase)
	}
	// re-registration must still issue a fresh reg-token secret
	var sec corev1.Secret
	if err := c.Get(context.Background(),
		types.NamespacedName{Namespace: hubNS, Name: RegTokenSecretName("prod")}, &sec); err != nil {
		t.Fatalf("reg-token secret not issued on re-registration: %v", err)
	}
}
