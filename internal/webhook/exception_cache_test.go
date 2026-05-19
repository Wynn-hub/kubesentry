package webhook_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func newPex(name string, spec v1alpha1.PolicyExceptionSpec, active bool) *v1alpha1.PolicyException {
	pex := &v1alpha1.PolicyException{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: spec}
	if active {
		pex.Status.Phase = v1alpha1.PhaseActive
	}
	// else: leave Phase empty (zero value). Cache must treat this as non-Active.
	return pex
}

func TestExceptionCacheOnlyCachesActive(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"run-as-privileged"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:     "x",
	}, false))
	if got := c.ExemptedKeys("hr", nil, nil, []*webhook.CompiledPolicy{{Name: "run-as-privileged"}}); len(got) != 0 {
		t.Errorf("non-Active exception should not exempt; got %v", got)
	}
}

func TestExceptionCacheExactNamespaceMatch(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"run-as-privileged"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:     "x",
	}, true))
	got := c.ExemptedKeys("hr", nil, nil, []*webhook.CompiledPolicy{{Name: "run-as-privileged"}, {Name: "host-net"}})
	if !got["run-as-privileged"] {
		t.Errorf("expected run-as-privileged exempted, got %v", got)
	}
	if got["host-net"] {
		t.Errorf("host-net should NOT be exempted")
	}
}

func TestExceptionCacheNamespaceMismatch(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"run-as-privileged"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:     "x",
	}, true))
	got := c.ExemptedKeys("finance", nil, nil, []*webhook.CompiledPolicy{{Name: "run-as-privileged"}})
	if len(got) != 0 {
		t.Errorf("ns mismatch — no exemption expected; got %v", got)
	}
}

func TestExceptionCacheResourceLabelSelector(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"run-as-privileged"},
		Match: v1alpha1.PolicyExceptionMatch{
			Namespaces:       []string{"hr"},
			ResourceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "legacy"}},
		},
		Reason: "x",
	}, true))
	pols := []*webhook.CompiledPolicy{{Name: "run-as-privileged"}}

	if got := c.ExemptedKeys("hr", nil, map[string]string{"app": "legacy"}, pols); !got["run-as-privileged"] {
		t.Error("matching resource labels should be exempted")
	}
	if got := c.ExemptedKeys("hr", nil, map[string]string{"app": "modern"}, pols); got["run-as-privileged"] {
		t.Error("non-matching resource labels should NOT be exempted")
	}
}

func TestExceptionCacheAllPoliciesSentinel(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		AllPolicies: true,
		Match:       v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:      "x",
	}, true))
	got := c.ExemptedKeys("hr", nil, nil, []*webhook.CompiledPolicy{{Name: "p1"}, {Name: "p2"}})
	if !got["p1"] || !got["p2"] {
		t.Errorf("allPolicies should exempt every matched policy, got %v", got)
	}
}

func TestExceptionCachePolicyGroupMatch(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyGroupRefs: []string{"baseline"},
		Match:           v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:          "x",
	}, true))
	pols := []*webhook.CompiledPolicy{
		{Name: "run-as-privileged", GroupName: "baseline"},
		{Name: "host-net", GroupName: "network"},
	}
	got := c.ExemptedKeys("hr", nil, nil, pols)
	if !got["run-as-privileged"] {
		t.Error("baseline-group policy should be exempted")
	}
	if got["host-net"] {
		t.Error("non-baseline policy should NOT be exempted")
	}
}

func TestExceptionCacheDeleteRemoves(t *testing.T) {
	c := webhook.NewExceptionCache()
	pex := newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"x"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:     "x",
	}, true)
	c.Upsert(pex)
	c.Delete(pex.Name)
	if got := c.ExemptedKeys("hr", nil, nil, []*webhook.CompiledPolicy{{Name: "x"}}); len(got) != 0 {
		t.Error("deleted exception should not match")
	}
}

func TestExceptionCacheNamespaceSelectorMatch(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"x"},
		Match: v1alpha1.PolicyExceptionMatch{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "legacy"}},
		},
		Reason: "x",
	}, true))
	pols := []*webhook.CompiledPolicy{{Name: "x"}}

	// Matching namespace labels → exempted.
	if got := c.ExemptedKeys("any-ns", map[string]string{"env": "legacy"}, nil, pols); !got["x"] {
		t.Error("matching namespace labels should exempt")
	}
	// Non-matching namespace labels → not exempted.
	if got := c.ExemptedKeys("any-ns", map[string]string{"env": "prod"}, nil, pols); got["x"] {
		t.Error("non-matching namespace labels should NOT exempt")
	}
	// Empty (but non-nil) namespace labels → not exempted (selector requires "env=legacy").
	if got := c.ExemptedKeys("any-ns", map[string]string{}, nil, pols); got["x"] {
		t.Error("empty labels should not match a selector requiring env=legacy")
	}
}

func TestExceptionCacheNamespaceSelectorFailClosed(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"x"},
		Match: v1alpha1.PolicyExceptionMatch{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "legacy"}},
		},
		Reason: "x",
	}, true))
	// nil namespaceLabels → caller could not resolve. Must fail-closed.
	got := c.ExemptedKeys("any-ns", nil, nil, []*webhook.CompiledPolicy{{Name: "x"}})
	if len(got) != 0 {
		t.Errorf("nil namespace labels must fail-closed, got %v", got)
	}
}

func TestExceptionCacheNamespaceSelectorCombinedWithNamespaces(t *testing.T) {
	c := webhook.NewExceptionCache()
	c.Upsert(newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"x"},
		Match: v1alpha1.PolicyExceptionMatch{
			Namespaces:        []string{"hr"},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "legacy"}},
		},
		Reason: "x",
	}, true))
	pols := []*webhook.CompiledPolicy{{Name: "x"}}

	// Right namespace + matching labels → exempt.
	if got := c.ExemptedKeys("hr", map[string]string{"env": "legacy"}, nil, pols); !got["x"] {
		t.Error("hr + legacy labels should exempt")
	}
	// Right namespace + wrong labels → not exempt (AND semantics).
	if got := c.ExemptedKeys("hr", map[string]string{"env": "prod"}, nil, pols); got["x"] {
		t.Error("wrong labels must reject even in matching namespace")
	}
	// Wrong namespace + matching labels → not exempt.
	if got := c.ExemptedKeys("finance", map[string]string{"env": "legacy"}, nil, pols); got["x"] {
		t.Error("wrong namespace must reject even with matching labels")
	}
}

func TestExceptionCacheUpdateOnPhaseTransition(t *testing.T) {
	c := webhook.NewExceptionCache()
	pex := newPex("p", v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"x"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr"}},
		Reason:     "x",
	}, true)
	c.Upsert(pex)
	// Mark expired and upsert again.
	pex.Status.Phase = v1alpha1.PhaseExpired
	c.Upsert(pex)
	if got := c.ExemptedKeys("hr", nil, nil, []*webhook.CompiledPolicy{{Name: "x"}}); len(got) != 0 {
		t.Error("expired exception must be removed from cache")
	}
}
