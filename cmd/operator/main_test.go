package main

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
