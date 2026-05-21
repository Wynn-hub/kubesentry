package webhook_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func newCompiledPolicy(t *testing.T, name, defaultMode string, ops []string, group, version, resource string) *webhook.CompiledPolicy {
	t.Helper()
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatalf("compile rego: %v", err)
	}
	return &webhook.CompiledPolicy{
		Name:        name,
		DefaultMode: defaultMode,
		Match: v1alpha1.PolicyMatch{
			Operations: ops,
			Resources: []v1alpha1.MatchResource{
				{APIGroups: []string{group}, APIVersions: []string{version}, Resources: []string{resource}},
			},
		},
		Query: q,
	}
}

func TestCacheMatchingForRequestSingleGroup(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.SetPolicy("p1", newCompiledPolicy(t, "p1", v1alpha1.ModeEnforce, []string{"CREATE"}, "", "v1", "pods"))
	c.SetGroup("security", &webhook.CompiledGroup{
		Name:    "security",
		Enabled: true,
		Members: map[string]string{"p1": v1alpha1.ModeEnforce},
	})

	got := c.MatchingForRequest("pods", "", "CREATE", "prod", nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Policy.Name != "p1" || got[0].Mode != v1alpha1.ModeEnforce {
		t.Errorf("got = %+v", got[0])
	}
}

func TestCacheMatchingForRequestDisabledGroupSkipped(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.SetPolicy("p1", newCompiledPolicy(t, "p1", v1alpha1.ModeEnforce, []string{"CREATE"}, "", "v1", "pods"))
	c.SetGroup("security", &webhook.CompiledGroup{
		Name:    "security",
		Enabled: false, // disabled
		Members: map[string]string{"p1": v1alpha1.ModeEnforce},
	})

	got := c.MatchingForRequest("pods", "", "CREATE", "prod", nil)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestCacheMatchingForRequestStrictestModeAcrossGroups(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.SetPolicy("p1", newCompiledPolicy(t, "p1", v1alpha1.ModeAudit, []string{"CREATE"}, "", "v1", "pods"))
	// Two groups both match the same request, with conflicting modes.
	c.SetGroup("g-strict", &webhook.CompiledGroup{
		Name: "g-strict", Enabled: true,
		Members: map[string]string{"p1": v1alpha1.ModeEnforce},
	})
	c.SetGroup("g-lax", &webhook.CompiledGroup{
		Name: "g-lax", Enabled: true,
		Members: map[string]string{"p1": v1alpha1.ModeAudit},
	})

	got := c.MatchingForRequest("pods", "", "CREATE", "prod", nil)
	if len(got) != 1 || got[0].Mode != v1alpha1.ModeEnforce {
		t.Errorf("strictest mode failed: %+v", got)
	}
}

func TestCacheMatchingForRequestNamespaceSelectorFiltersOut(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.SetPolicy("p1", newCompiledPolicy(t, "p1", v1alpha1.ModeEnforce, []string{"CREATE"}, "", "v1", "pods"))
	cg, err := webhook.CompileGroup(&v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "g"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: true,
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
		},
		Status: v1alpha1.PolicyGroupStatus{
			ResolvedPolicies: []v1alpha1.EffectiveMember{
				{Name: "p1", EnforcementMode: v1alpha1.ModeEnforce, Source: v1alpha1.SourceByName},
			},
		},
	})
	if err != nil {
		t.Fatalf("compile group: %v", err)
	}
	c.SetGroup("g", cg)

	// Request from a staging ns labels — selector should reject.
	got := c.MatchingForRequest("pods", "", "CREATE", "staging", map[string]string{"env": "staging"})
	if len(got) != 0 {
		t.Errorf("staging should not match, got %+v", got)
	}

	// Request from a prod ns labels — selector should match.
	got = c.MatchingForRequest("pods", "", "CREATE", "prod", map[string]string{"env": "prod"})
	if len(got) != 1 {
		t.Errorf("prod should match, got %+v", got)
	}
}

func TestCacheMatchingForRequestMissingNsLabelsSkipsGroupWithSelector(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.SetPolicy("p1", newCompiledPolicy(t, "p1", v1alpha1.ModeEnforce, []string{"CREATE"}, "", "v1", "pods"))
	cg, _ := webhook.CompileGroup(&v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "g"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled:           true,
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"x": "y"}},
		},
		Status: v1alpha1.PolicyGroupStatus{
			ResolvedPolicies: []v1alpha1.EffectiveMember{
				{Name: "p1", EnforcementMode: v1alpha1.ModeEnforce, Source: v1alpha1.SourceByName},
			},
		},
	})
	c.SetGroup("g", cg)

	got := c.MatchingForRequest("pods", "", "CREATE", "ns-no-cache", nil) // nsLabels=nil → fail-open skip
	if len(got) != 0 {
		t.Errorf("fail-open expected, got %+v", got)
	}
}

func TestCacheMatchingForRequestUnknownPolicyInGroupIsIgnored(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.SetGroup("g", &webhook.CompiledGroup{
		Name:    "g",
		Enabled: true,
		Members: map[string]string{"absent": v1alpha1.ModeEnforce},
	})
	got := c.MatchingForRequest("pods", "", "CREATE", "prod", nil)
	if len(got) != 0 {
		t.Errorf("unknown policy must be skipped, got %+v", got)
	}
}

func TestCacheReadiness(t *testing.T) {
	c := webhook.NewPolicyCache()
	if c.IsReady() {
		t.Error("cache should not be ready before SetReady()")
	}
	c.SetReady()
	if !c.IsReady() {
		t.Error("cache should be ready after SetReady()")
	}
}
