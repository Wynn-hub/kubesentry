package service

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	"gorm.io/datatypes"
)

func TestPolicyListPreservesFieldsUsedByGroupResolution(t *testing.T) {
	policies := policyList([]dbrepo.Policy{{
		Name:            "require-labels",
		Description:     "Require labels",
		Rego:            "package kubesentry",
		EnforcementMode: v1alpha1.ModeAudit,
		Match:           datatypes.NewJSONType(v1alpha1.PolicyMatch{Operations: []string{"CREATE"}}),
		Labels:          datatypes.NewJSONType(map[string]string{"kubesentry.io/category": "best-practices"}),
	}})

	resolved, conflicts := groupresolve.Resolve(v1alpha1.PolicyGroupSpec{
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "require-labels"}}},
	}, policies)

	if len(conflicts) != 0 || len(resolved) != 1 {
		t.Fatalf("resolved/conflicts = %v/%v, want one resolved policy and no conflicts", resolved, conflicts)
	}
	got := policies[0]
	if got.Name != "require-labels" || got.Spec.Description != "Require labels" || got.Spec.EnforcementMode != v1alpha1.ModeAudit || got.Spec.Rego == "" {
		t.Fatalf("converted policy = %#v", got)
	}
	if got.Labels["kubesentry.io/category"] != "best-practices" || len(got.Spec.Match.Operations) != 1 {
		t.Fatalf("converted metadata/spec = %#v/%#v", got.Labels, got.Spec.Match)
	}
}
