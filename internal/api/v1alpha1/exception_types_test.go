package v1alpha1_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func TestPolicyExceptionDeepCopy(t *testing.T) {
	now := metav1.Now()
	in := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: "ex1"},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-privileged"},
			Match: v1alpha1.PolicyExceptionMatch{
				Namespaces: []string{"hr-system"},
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "legacy"},
				},
				ResourceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "billing"},
				},
			},
			Duration:          "24h",
			RetainAfterExpiry: "0s",
			Reason:            "migration",
		},
		Status: v1alpha1.PolicyExceptionStatus{
			Phase:       v1alpha1.PhaseActive,
			EffectiveAt: &now,
			ExpiresAt:   &now,
		},
	}
	out := in.DeepCopy()
	if out == in {
		t.Fatal("DeepCopy returned same pointer")
	}
	if &out.Spec.PolicyRefs[0] == &in.Spec.PolicyRefs[0] {
		t.Error("Spec.PolicyRefs slice not deep-copied")
	}
	if out.Spec.Match.NamespaceSelector == in.Spec.Match.NamespaceSelector {
		t.Error("Match.NamespaceSelector not deep-copied")
	}
}

func TestPolicyExceptionListDeepCopy(t *testing.T) {
	in := &v1alpha1.PolicyExceptionList{
		Items: []v1alpha1.PolicyException{
			{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
		},
	}
	out := in.DeepCopy()
	if &out.Items[0] == &in.Items[0] {
		t.Error("PolicyExceptionList items not deep-copied")
	}
	_, ok := interface{}(out).(runtime.Object)
	if !ok {
		t.Error("PolicyExceptionList does not implement runtime.Object")
	}
}

func TestPolicyExceptionAddToScheme(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	gvk := v1alpha1.GroupVersion.WithKind("PolicyException")
	if _, err := s.New(gvk); err != nil {
		t.Fatalf("scheme does not know PolicyException: %v", err)
	}
	gvkList := v1alpha1.GroupVersion.WithKind("PolicyExceptionList")
	if _, err := s.New(gvkList); err != nil {
		t.Fatalf("scheme does not know PolicyExceptionList: %v", err)
	}
}
