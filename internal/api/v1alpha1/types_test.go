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

func TestPolicyGroupDeepCopy(t *testing.T) {
	enabled := true
	orig := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "security"},
		Spec: v1alpha1.PolicyGroupSpec{
			DisplayName: "Security",
			Enabled:     true,
			Policies: []v1alpha1.PolicyInGroup{
				{Key: "runAsPrivileged", Enabled: &enabled, Mode: "enforce"},
			},
		},
		Status: v1alpha1.PolicyGroupStatus{
			Phase:           v1alpha1.PhaseReady,
			ActivePolicies:  1,
			SkippedPolicies: 0,
		},
	}
	cp := orig.DeepCopy()
	if cp.Name != "security" {
		t.Error("name not copied")
	}
	if len(cp.Spec.Policies) != 1 {
		t.Error("policies not copied")
	}
	cp.Spec.Policies[0].Key = "changed"
	if orig.Spec.Policies[0].Key == "changed" {
		t.Error("DeepCopy shares slice backing array")
	}
	if cp.Spec.Policies[0].Enabled == orig.Spec.Policies[0].Enabled {
		t.Error("DeepCopy shares Enabled pointer")
	}
}

func TestPolicySpecDescriptionField(t *testing.T) {
	spec := v1alpha1.PolicySpec{
		Description:     "test description",
		EnforcementMode: v1alpha1.ModeEnforce,
	}
	cp := new(v1alpha1.PolicySpec)
	spec.DeepCopyInto(cp)
	if cp.Description != "test description" {
		t.Errorf("Description not copied: %q", cp.Description)
	}
}

func TestLabelConstants(t *testing.T) {
	if v1alpha1.LabelKey == "" || v1alpha1.LabelGroup == "" || v1alpha1.LabelSource == "" {
		t.Error("label constants must not be empty")
	}
}
