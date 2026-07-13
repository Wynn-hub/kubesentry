package v1alpha1_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func TestClusterAddToScheme(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, kind := range []string{"Cluster", "ClusterList"} {
		gvk := v1alpha1.GroupVersion.WithKind(kind)
		if _, err := s.New(gvk); err != nil {
			t.Fatalf("scheme cannot create %s: %v", kind, err)
		}
	}
}

func TestClusterDeepCopy(t *testing.T) {
	orig := &v1alpha1.Cluster{
		Spec: v1alpha1.ClusterSpec{
			DisplayName: "prod",
			Labels:      map[string]string{"env": "prod"},
		},
		Status: v1alpha1.ClusterStatus{Phase: v1alpha1.ClusterPhaseOnline},
	}
	cp := orig.DeepCopy()
	cp.Spec.Labels["env"] = "staging"
	if orig.Spec.Labels["env"] != "prod" {
		t.Fatal("DeepCopy shares Labels map with original")
	}
}
