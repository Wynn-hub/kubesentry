package webhook_test

import (
	"testing"

	"github.com/open-policy-agent/opa/rego"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func makeCompiledPolicy(name, mode string, apiGroup, resource, op string) *webhook.CompiledPolicy {
	return &webhook.CompiledPolicy{
		Name:            name,
		EnforcementMode: mode,
		Match: v1alpha1.PolicyMatch{
			Operations: []string{op},
			Resources: []v1alpha1.MatchResource{
				{APIGroups: []string{apiGroup}, APIVersions: []string{"v1"}, Resources: []string{resource}},
			},
		},
		Query: rego.PreparedEvalQuery{},
	}
}

func TestCacheSetAndMatch(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.Set("p1", makeCompiledPolicy("p1", "enforce", "", "pods", "CREATE"))
	c.Set("p2", makeCompiledPolicy("p2", "audit", "apps", "deployments", "CREATE"))

	matched := c.MatchingPolicies("pods", "", "CREATE", "default")
	if len(matched) != 1 || matched[0].Name != "p1" {
		t.Errorf("expected p1 only, got %v", matched)
	}

	matched2 := c.MatchingPolicies("deployments", "apps", "UPDATE", "default")
	if len(matched2) != 0 {
		t.Errorf("expected no match for UPDATE, got %v", matched2)
	}
}

func TestCacheDelete(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.Set("p1", makeCompiledPolicy("p1", "enforce", "", "pods", "CREATE"))
	c.Delete("p1")

	matched := c.MatchingPolicies("pods", "", "CREATE", "default")
	if len(matched) != 0 {
		t.Errorf("expected empty after delete, got %v", matched)
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

func TestCacheWildcardAPIGroup(t *testing.T) {
	c := webhook.NewPolicyCache()
	c.Set("p1", makeCompiledPolicy("p1", "enforce", "*", "pods", "CREATE"))

	matched := c.MatchingPolicies("pods", "apps", "CREATE", "default")
	if len(matched) != 1 {
		t.Errorf("wildcard apiGroup should match, got %v", matched)
	}
}
