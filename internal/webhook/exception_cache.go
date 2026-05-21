package webhook

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// exemptEntry is the materialized form an Exception takes inside the cache.
type exemptEntry struct {
	name            string
	policyRefs      map[string]struct{}
	policyGroupRefs map[string]struct{}
	allPolicies     bool
	namespaces      map[string]struct{}
	namespaceSel    labels.Selector
	resourceSel     labels.Selector
	hasNsSelector   bool
	hasResourceSel  bool
}

// ExceptionCache holds active PolicyExceptions in memory, indexed for fast lookup.
type ExceptionCache struct {
	mu      sync.RWMutex
	all     map[string]*exemptEntry
	nsIndex map[string][]*exemptEntry
	global  []*exemptEntry
	ready   bool
}

func NewExceptionCache() *ExceptionCache {
	return &ExceptionCache{
		all:     map[string]*exemptEntry{},
		nsIndex: map[string][]*exemptEntry{},
	}
}

func (c *ExceptionCache) SetReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = true
}

func (c *ExceptionCache) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Upsert inserts/updates an exception. Non-Active phases trigger Delete().
func (c *ExceptionCache) Upsert(pex *v1alpha1.PolicyException) {
	if pex.Status.Phase != v1alpha1.PhaseActive {
		c.Delete(pex.Name)
		return
	}
	entry := compile(pex)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeUnlocked(pex.Name)
	c.all[pex.Name] = entry
	if len(entry.namespaces) == 0 {
		c.global = append(c.global, entry)
	} else {
		for ns := range entry.namespaces {
			c.nsIndex[ns] = append(c.nsIndex[ns], entry)
		}
	}
}

func (c *ExceptionCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeUnlocked(name)
}

func (c *ExceptionCache) removeUnlocked(name string) {
	old, ok := c.all[name]
	if !ok {
		return
	}
	delete(c.all, name)
	if len(old.namespaces) == 0 {
		c.global = removeEntry(c.global, old)
		return
	}
	for ns := range old.namespaces {
		c.nsIndex[ns] = removeEntry(c.nsIndex[ns], old)
		if len(c.nsIndex[ns]) == 0 {
			delete(c.nsIndex, ns)
		}
	}
}

func removeEntry(list []*exemptEntry, target *exemptEntry) []*exemptEntry {
	out := make([]*exemptEntry, 0, len(list))
	for _, e := range list {
		if e != target {
			out = append(out, e)
		}
	}
	return out
}

// ExemptedKeys returns a set of policy names that are exempted for the given
// admission context. `namespaceLabels` is the labels on the Namespace object
// (may be nil if the caller cannot resolve them). `resourceLabels` is the
// admitted object's metadata.labels (may be nil). `policies` is the candidate
// list already filtered by PolicyCache.
//
// Caller treats a returned set as: "skip Rego evaluation for each policy whose
// .Name is present in the set".
func (c *ExceptionCache) ExemptedKeys(namespace string, namespaceLabels, resourceLabels map[string]string, policies []*CompiledPolicy) map[string]bool {
	c.mu.RLock()
	candidates := append([]*exemptEntry(nil), c.global...)
	candidates = append(candidates, c.nsIndex[namespace]...)
	c.mu.RUnlock()

	if len(candidates) == 0 {
		return nil
	}
	out := make(map[string]bool, len(policies))
	nsLabelSet := labels.Set(namespaceLabels)
	resLabels := labels.Set(resourceLabels)

	for _, entry := range candidates {
		if entry.hasResourceSel && !entry.resourceSel.Matches(resLabels) {
			continue
		}
		// Fail-closed: if a namespaceSelector is set but the caller couldn't
		// resolve namespace labels (namespaceLabels == nil), the entry does
		// not match. An explicit empty map (len==0, non-nil) means "namespace
		// has no labels" and is evaluated normally.
		if entry.hasNsSelector {
			if namespaceLabels == nil {
				continue
			}
			if !entry.namespaceSel.Matches(nsLabelSet) {
				continue
			}
		}
		if entry.allPolicies {
			for _, p := range policies {
				out[p.Name] = true
			}
			continue
		}
		for _, p := range policies {
			if _, ok := entry.policyRefs[p.Name]; ok {
				out[p.Name] = true
				continue
			}
			for _, group := range p.ReferencedBy {
				if _, ok := entry.policyGroupRefs[group]; ok {
					out[p.Name] = true
					break
				}
			}
		}
	}
	return out
}

func compile(pex *v1alpha1.PolicyException) *exemptEntry {
	e := &exemptEntry{
		name:            pex.Name,
		policyRefs:      stringSliceToSet(pex.Spec.PolicyRefs),
		policyGroupRefs: stringSliceToSet(pex.Spec.PolicyGroupRefs),
		allPolicies:     pex.Spec.AllPolicies,
		namespaces:      stringSliceToSet(pex.Spec.Match.Namespaces),
	}
	if pex.Spec.Match.NamespaceSelector != nil {
		if sel, err := metav1.LabelSelectorAsSelector(pex.Spec.Match.NamespaceSelector); err == nil {
			e.namespaceSel = sel
			e.hasNsSelector = true
		}
	}
	if pex.Spec.Match.ResourceSelector != nil {
		if sel, err := metav1.LabelSelectorAsSelector(pex.Spec.Match.ResourceSelector); err == nil {
			e.resourceSel = sel
			e.hasResourceSel = true
		}
	}
	return e
}

func stringSliceToSet(s []string) map[string]struct{} {
	if len(s) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(s))
	for _, v := range s {
		out[v] = struct{}{}
	}
	return out
}
