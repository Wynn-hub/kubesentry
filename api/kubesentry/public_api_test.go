package kubesentry_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
)

func TestV1Alpha1IsPubliclyImportable(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	gvk := v1alpha1.GroupVersion.WithKind("Policy")
	if _, err := scheme.New(gvk); err != nil {
		t.Fatalf("scheme does not know %s: %v", gvk, err)
	}
}
