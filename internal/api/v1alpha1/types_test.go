package v1alpha1_test

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func TestPolicyMarshal(t *testing.T) {
	p := v1alpha1.Policy{
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: "enforce",
			Rego:            "package kubesentry\ndeny[msg]{ msg := \"test\" }",
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources: []v1alpha1.MatchResource{
					{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			},
		},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.Policy
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Spec.EnforcementMode != "enforce" {
		t.Errorf("enforcementMode = %q, want %q", got.Spec.EnforcementMode, "enforce")
	}
	if len(got.Spec.Match.Resources) != 1 {
		t.Errorf("resources len = %d, want 1", len(got.Spec.Match.Resources))
	}
}

func TestPolicyVersionMarshal(t *testing.T) {
	pv := v1alpha1.PolicyVersion{
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       "deny-privileged-pods",
			Version:         3,
			EnforcementMode: "enforce",
			CreatedAt:       metav1.Now(),
		},
	}

	data, err := json.Marshal(pv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.PolicyVersion
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Spec.Version != 3 {
		t.Errorf("version = %d, want 3", got.Spec.Version)
	}
}

func TestPolicyRollbackTo(t *testing.T) {
	p := v1alpha1.Policy{
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: "enforce",
			Rego:            "package kubesentry",
			RollbackTo:      &v1alpha1.RollbackTo{Version: 2},
		},
	}
	data, _ := json.Marshal(p)
	var got v1alpha1.Policy
	json.Unmarshal(data, &got)
	if got.Spec.RollbackTo == nil || got.Spec.RollbackTo.Version != 2 {
		t.Error("rollbackTo not preserved")
	}
}
