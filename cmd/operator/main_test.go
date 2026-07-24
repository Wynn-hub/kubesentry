package main

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuildSchemeSupportsUninstallResources(t *testing.T) {
	scheme := buildScheme()
	for _, obj := range []runtime.Object{
		&appsv1.Deployment{},
		&batchv1.Job{},
		&coordinationv1.Lease{},
		&rbacv1.Role{},
		&rbacv1.ClusterRole{},
		&apiextensionsv1.CustomResourceDefinition{},
	} {
		if _, _, err := scheme.ObjectKinds(obj); err != nil {
			t.Fatalf("scheme does not register %T: %v", obj, err)
		}
	}
}

func TestApplyCRDCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	first := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "policies.kubesentry.io"},
		Spec:       apiextensionsv1.CustomResourceDefinitionSpec{Group: "kubesentry.io", Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "policies", Kind: "Policy"}, Scope: apiextensionsv1.ClusterScoped, Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1alpha1", Served: true, Storage: true}}},
	}
	c := fake.NewClientBuilder().WithScheme(buildScheme()).Build()
	if err := applyCRD(ctx, c, first); err != nil {
		t.Fatalf("create CRD: %v", err)
	}
	updated := first.DeepCopy()
	updated.Spec.Names.Singular = "policy"
	if err := applyCRD(ctx, c, updated); err != nil {
		t.Fatalf("update CRD: %v", err)
	}
	var stored apiextensionsv1.CustomResourceDefinition
	if err := c.Get(ctx, client.ObjectKey{Name: first.Name}, &stored); err != nil {
		t.Fatalf("get CRD: %v", err)
	}
	if stored.Spec.Names.Singular != "policy" {
		t.Fatalf("singular = %q, want policy", stored.Spec.Names.Singular)
	}
}
