package webhook

import (
	"sync"

	"github.com/open-policy-agent/opa/rego"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// CompiledPolicy holds a pre-compiled OPA query and its match criteria.
type CompiledPolicy struct {
	Name            string
	EnforcementMode string
	Match           v1alpha1.PolicyMatch
	Query           rego.PreparedEvalQuery
}

// PolicyCache is a thread-safe in-memory store of compiled policies.
type PolicyCache struct {
	mu       sync.RWMutex
	policies map[string]*CompiledPolicy
	ready    bool
}

func NewPolicyCache() *PolicyCache {
	return &PolicyCache{policies: make(map[string]*CompiledPolicy)}
}

func (c *PolicyCache) Set(name string, p *CompiledPolicy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.policies[name] = p
}

func (c *PolicyCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.policies, name)
}

func (c *PolicyCache) SetReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = true
}

func (c *PolicyCache) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// MatchingPolicies returns all policies whose match rules cover the given request.
func (c *PolicyCache) MatchingPolicies(resource, apiGroup, operation, namespace string) []*CompiledPolicy {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var out []*CompiledPolicy
	for _, p := range c.policies {
		if matchesPolicy(p.Match, resource, apiGroup, operation) {
			out = append(out, p)
		}
	}
	return out
}

func matchesPolicy(m v1alpha1.PolicyMatch, resource, apiGroup, operation string) bool {
	if !containsStr(m.Operations, operation) && !containsStr(m.Operations, "*") {
		return false
	}
	for _, r := range m.Resources {
		if matchesResource(r, resource, apiGroup) {
			return true
		}
	}
	return false
}

func matchesResource(r v1alpha1.MatchResource, resource, apiGroup string) bool {
	return (containsStr(r.Resources, resource) || containsStr(r.Resources, "*")) &&
		(containsStr(r.APIGroups, apiGroup) || containsStr(r.APIGroups, "*"))
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
