package uninstall

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func TestRunnerDeletesAllowlistedResourcesAndKeepsNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := admissionv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubesentry-system"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "kubesentry-operator", Namespace: "kubesentry-system"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "kubesentry-webhook", Namespace: "kubesentry-system"}},
		&admissionv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "kubesentry"}},
		&v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "managed-policy"}},
	).Build()
	if err := (Runner{Client: c, Namespace: "kubesentry-system"}).Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, obj := range []clientObject{
		{&corev1.Service{}, types.NamespacedName{Name: "kubesentry-webhook", Namespace: "kubesentry-system"}},
		{&admissionv1.ValidatingWebhookConfiguration{}, types.NamespacedName{Name: "kubesentry"}},
		{&v1alpha1.Policy{}, types.NamespacedName{Name: "managed-policy"}},
	} {
		if err := c.Get(context.Background(), obj.key, obj.obj); err == nil {
			t.Fatalf("%T %s still exists", obj.obj, obj.key)
		}
	}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "kubesentry-operator", Namespace: "kubesentry-system"}, &appsv1.Deployment{}); err != nil {
		t.Fatalf("operator must remain until completion is acknowledged: %v", err)
	}
	if err := (Runner{Client: c, Namespace: "kubesentry-system"}).Finalize(context.Background()); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "kubesentry-operator", Namespace: "kubesentry-system"}, &appsv1.Deployment{}); err == nil {
		t.Fatal("operator still exists after finalize")
	}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "kubesentry-system"}, &corev1.Namespace{}); err != nil {
		t.Fatalf("namespace was deleted: %v", err)
	}
}

type clientObject struct {
	obj client.Object
	key types.NamespacedName
}
