package webhook

import (
	"sync"

	"github.com/open-policy-agent/opa/rego"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// CompiledPolicy holds a pre-compiled OPA query and its match criteria.
// Note: Key + GroupName labels are no longer used (the old owner→child model
// is gone). Description still surfaces in violation messages.
type CompiledPolicy struct {
	Name        string
	Description string
	DefaultMode string // spec.enforcementMode — falls back here if no group overrides
	Match       v1alpha1.PolicyMatch
	Query       rego.PreparedEvalQuery
}

// CompiledGroup is the cached view of a PolicyGroup resolved by the operator.
// Members map: policyName → effective mode (from group.status.resolvedPolicies).
type CompiledGroup struct {
	Name              string
	Enabled           bool
	NamespaceSelector labels.Selector   // nil = match all
	Members           map[string]string // policyName → enforcementMode (already resolved)
}

// PolicyCache holds compiled Policies + compiled Groups; cache reads use RLock.
type PolicyCache struct {
	mu       sync.RWMutex
	policies map[string]*CompiledPolicy
	groups   map[string]*CompiledGroup
	ready    bool
}

func NewPolicyCache() *PolicyCache {
	return &PolicyCache{
		policies: map[string]*CompiledPolicy{},
		groups:   map[string]*CompiledGroup{},
	}
}

func (c *PolicyCache) SetPolicy(name string, p *CompiledPolicy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.policies[name] = p
}

func (c *PolicyCache) DeletePolicy(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.policies, name)
}

func (c *PolicyCache) SetGroup(name string, g *CompiledGroup) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.groups[name] = g
}

func (c *PolicyCache) DeleteGroup(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.groups, name)
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

// Resolved is the per-request output of the cache: the set of policies that
// must be evaluated, each with its effective mode for this admission context.
type Resolved struct {
	Policy *CompiledPolicy
	Mode   string   // "enforce" | "audit" — strictest of all groups matching this request
	Groups []string // names of groups contributing to this match (for diagnostics)
}

// MatchingForRequest applies group-level filtering (namespaceSelector) then
// per-policy filtering (match.resources / match.operations), and computes
// the strictest mode across all matching enabled groups.
//
// namespaceLabels may be nil if the caller cannot resolve them; in that case
// groups whose namespaceSelector is non-nil are skipped (fail-open).
func (c *PolicyCache) MatchingForRequest(
	resource, apiGroup, operation, namespace string,
	namespaceLabels map[string]string,
) []*Resolved {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Step 1: find groups whose namespaceSelector matches this request.
	type groupHit struct {
		name    string
		members map[string]string
	}
	var matchingGroups []groupHit
	nsLabelSet := labels.Set(namespaceLabels)
	for _, g := range c.groups {
		if !g.Enabled {
			continue
		}
		if g.NamespaceSelector != nil {
			if namespaceLabels == nil {
				// fail-open per spec: cannot resolve labels → skip this group.
				continue
			}
			if !g.NamespaceSelector.Matches(nsLabelSet) {
				continue
			}
		}
		matchingGroups = append(matchingGroups, groupHit{name: g.Name, members: g.Members})
	}
	if len(matchingGroups) == 0 {
		return nil
	}

	// Step 2: gather candidate policies (per-policy mode = strictest across groups).
	type accum struct {
		mode   string
		groups []string
	}
	byPolicy := map[string]*accum{}
	for _, g := range matchingGroups {
		for polName, mode := range g.members {
			pol, ok := c.policies[polName]
			if !ok {
				continue // dangling: policy not (yet) in cache
			}
			if !matchesPolicy(pol.Match, resource, apiGroup, operation) {
				continue
			}
			a, exists := byPolicy[polName]
			if !exists {
				byPolicy[polName] = &accum{mode: mode, groups: []string{g.name}}
				continue
			}
			a.mode = strictestOf(a.mode, mode)
			a.groups = append(a.groups, g.name)
		}
	}

	out := make([]*Resolved, 0, len(byPolicy))
	for polName, a := range byPolicy {
		out = append(out, &Resolved{
			Policy: c.policies[polName],
			Mode:   a.mode,
			Groups: a.groups,
		})
	}
	return out
}

// strictestOf returns the more restrictive mode. enforce > audit.
func strictestOf(a, b string) string {
	if a == v1alpha1.ModeEnforce || b == v1alpha1.ModeEnforce {
		return v1alpha1.ModeEnforce
	}
	return v1alpha1.ModeAudit
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

// CompileGroup translates a PolicyGroup into its cache form.
// Returns error if NamespaceSelector parse fails.
func CompileGroup(g *v1alpha1.PolicyGroup) (*CompiledGroup, error) {
	var sel labels.Selector
	if g.Spec.NamespaceSelector != nil {
		var err error
		sel, err = metav1.LabelSelectorAsSelector(g.Spec.NamespaceSelector)
		if err != nil {
			return nil, err
		}
	}
	members := make(map[string]string, len(g.Status.ResolvedPolicies))
	for _, m := range g.Status.ResolvedPolicies {
		members[m.Name] = m.EnforcementMode
	}
	return &CompiledGroup{
		Name:              g.Name,
		Enabled:           g.Spec.Enabled,
		NamespaceSelector: sel,
		Members:           members,
	}, nil
}
