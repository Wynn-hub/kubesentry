package service

import (
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestValidatePolicyGroupMembersRejectsResolutionErrors(t *testing.T) {
	policies := policyList([]dbrepo.Policy{{Name: "existing"}})
	tests := []struct {
		name string
		spec v1alpha1.PolicyGroupSpec
		want string
	}{
		{
			name: "missing named policy",
			spec: v1alpha1.PolicyGroupSpec{Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "missing"}}}},
			want: "missing",
		},
		{
			name: "invalid selector",
			spec: v1alpha1.PolicyGroupSpec{Policies: v1alpha1.PolicyGroupPolicies{BySelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "tier", Operator: "Unknown"}}}}},
			want: "selector",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePolicyGroupMembers(tt.spec, policies)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("error = %v, want message containing %q", err, tt.want)
			}
		})
	}
}

func TestValidatePolicyGroupMembersAllowsEmptySelectorResult(t *testing.T) {
	spec := v1alpha1.PolicyGroupSpec{Policies: v1alpha1.PolicyGroupPolicies{BySelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "missing"}}}}
	if err := validatePolicyGroupMembers(spec, policyList([]dbrepo.Policy{{Name: "existing"}})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyGroupDetailExplainsResolutionError(t *testing.T) {
	group := &dbrepo.PolicyGroup{Name: "broken", Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "missing"}}},
	})}
	got := detail(group, nil, dto.PolicyGroupScopeView{})
	if got.Status.Phase != v1alpha1.PhaseInvalid || len(got.Status.Issues) != 1 || !strings.Contains(got.Status.Issues[0], "missing") {
		t.Fatalf("status = %#v, want Invalid with missing-policy explanation", got.Status)
	}
}
