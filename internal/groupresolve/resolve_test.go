package groupresolve

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
)

func pol(name, mode string, labels map[string]string) v1alpha1.Policy {
	return v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       v1alpha1.PolicySpec{EnforcementMode: mode},
	}
}

func TestResolveByNamePrecedenceAndModes(t *testing.T) {
	policies := []v1alpha1.Policy{
		pol("a", v1alpha1.ModeAudit, map[string]string{"team": "x"}),
		pol("b", v1alpha1.ModeAudit, map[string]string{"team": "x"}),
	}
	spec := v1alpha1.PolicyGroupSpec{
		Enabled: true,
		Policies: v1alpha1.PolicyGroupPolicies{
			ByName:     []v1alpha1.PolicyRef{{Name: "a", EnforcementMode: v1alpha1.ModeEnforce}},
			BySelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "x"}},
		},
		SelectorEnforcementMode: v1alpha1.ModeAudit,
	}
	members, conds := Resolve(spec, policies)
	if len(conds) != 0 {
		t.Fatalf("unexpected conditions: %+v", conds)
	}
	if len(members) != 2 || members[0].Name != "a" || members[1].Name != "b" {
		t.Fatalf("members = %+v", members)
	}
	if members[0].EnforcementMode != v1alpha1.ModeEnforce || members[0].Source != v1alpha1.SourceByName {
		t.Fatalf("byName precedence lost: %+v", members[0])
	}
	if members[1].EnforcementMode != v1alpha1.ModeAudit || members[1].Source != v1alpha1.SourceBySelector {
		t.Fatalf("selector member wrong: %+v", members[1])
	}
}

func TestResolveMissingByNameYieldsCondition(t *testing.T) {
	members, conds := Resolve(v1alpha1.PolicyGroupSpec{
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "ghost"}}},
	}, nil)
	if len(members) != 0 {
		t.Fatalf("members = %+v", members)
	}
	if len(conds) != 1 || conds[0].Type != CondTypeMissingMember || conds[0].Message != "ghost" {
		t.Fatalf("conds = %+v", conds)
	}
}

func TestResolveExcludeWinsOverBoth(t *testing.T) {
	policies := []v1alpha1.Policy{pol("a", v1alpha1.ModeAudit, map[string]string{"team": "x"})}
	spec := v1alpha1.PolicyGroupSpec{
		Policies: v1alpha1.PolicyGroupPolicies{
			ByName:     []v1alpha1.PolicyRef{{Name: "a"}},
			BySelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "x"}},
			Exclude:    []string{"a"},
		},
	}
	members, _ := Resolve(spec, policies)
	if len(members) != 0 {
		t.Fatalf("exclude ignored: %+v", members)
	}
}

func TestResolveMalformedSelectorYieldsCondition(t *testing.T) {
	spec := v1alpha1.PolicyGroupSpec{
		Policies: v1alpha1.PolicyGroupPolicies{
			BySelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "k", Operator: "Bogus"}},
			},
		},
	}
	_, conds := Resolve(spec, nil)
	if len(conds) != 1 || conds[0].Type != CondTypeInvalidSelector {
		t.Fatalf("conds = %+v", conds)
	}
}

func TestResolveEmptyModeFallsBackToPolicyDefault(t *testing.T) {
	policies := []v1alpha1.Policy{pol("a", v1alpha1.ModeEnforce, nil)}
	members, _ := Resolve(v1alpha1.PolicyGroupSpec{
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "a"}}},
	}, policies)
	if members[0].EnforcementMode != v1alpha1.ModeEnforce {
		t.Fatalf("mode = %q", members[0].EnforcementMode)
	}
}
