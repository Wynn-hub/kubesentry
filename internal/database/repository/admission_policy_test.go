package repository

import (
	"testing"

	"github.com/lib/pq"
)

func TestShouldStoreAdmissionEventEmptyRuleSetStoresEverything(t *testing.T) {
	event := AdmissionEvent{Verdict: "deny", ClusterName: "prod", Resource: "pods", Namespace: "app"}
	if !ShouldStoreAdmissionEvent(nil, event) {
		t.Error("an empty rule set must store everything")
	}
}

// The single rule a user naturally writes for "only keep default/deployments"
// must do exactly that: matching events are stored, everything else is not.
func TestShouldStoreAdmissionEventIsAnAllowList(t *testing.T) {
	rules := []AdmissionStoragePolicy{{
		Priority:   1,
		MatchMode:  AdmissionMatchModeInclude,
		Namespaces: pq.StringArray{"default"},
		GVKs:       pq.StringArray{"apps/deployments"},
	}}

	tests := []struct {
		name  string
		event AdmissionEvent
		want  bool
	}{
		{"matching namespace and resource", AdmissionEvent{Namespace: "default", APIGroup: "apps", Resource: "deployments"}, true},
		{"other namespace, same resource", AdmissionEvent{Namespace: "test-builtin-rules", APIGroup: "apps", Resource: "deployments"}, false},
		{"same namespace, other resource", AdmissionEvent{Namespace: "default", Resource: "pods"}, false},
		{"other namespace and resource", AdmissionEvent{Namespace: "test-builtin-rules", Resource: "pods"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldStoreAdmissionEvent(rules, tt.event); got != tt.want {
				t.Errorf("ShouldStoreAdmissionEvent = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldStoreAdmissionEventCombinesRulesWithOr(t *testing.T) {
	rules := []AdmissionStoragePolicy{
		{Priority: 1, Namespaces: pq.StringArray{"default"}},
		{Priority: 2, GVKs: pq.StringArray{"pods"}},
	}
	if !ShouldStoreAdmissionEvent(rules, AdmissionEvent{Namespace: "default", Resource: "configmaps"}) {
		t.Error("event matching the first rule must be stored")
	}
	if !ShouldStoreAdmissionEvent(rules, AdmissionEvent{Namespace: "other", Resource: "pods"}) {
		t.Error("event matching the second rule must be stored")
	}
	if ShouldStoreAdmissionEvent(rules, AdmissionEvent{Namespace: "other", Resource: "configmaps"}) {
		t.Error("event matching neither rule must not be stored")
	}
}

func TestShouldStoreAdmissionEventExcludeStoresOutsideItsConditions(t *testing.T) {
	rules := []AdmissionStoragePolicy{{
		Priority:   1,
		MatchMode:  AdmissionMatchModeExclude,
		Namespaces: pq.StringArray{"kube-system"},
		GVKs:       pq.StringArray{"pods"},
	}}
	tests := []struct {
		name      string
		namespace string
		resource  string
		want      bool
	}{
		{"complete condition is excluded", "kube-system", "pods", false},
		{"different namespace is stored", "payments", "pods", true},
		{"different resource is stored", "kube-system", "services", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := AdmissionEvent{Resource: tt.resource, Namespace: tt.namespace}
			if got := ShouldStoreAdmissionEvent(rules, event); got != tt.want {
				t.Errorf("ShouldStoreAdmissionEvent = %v, want %v", got, tt.want)
			}
		})
	}
}

// An exclude rule with no conditions matches nothing, so with it as the only
// rule nothing is stored.
func TestShouldStoreAdmissionEventEmptyExcludeRuleStoresNothing(t *testing.T) {
	rules := []AdmissionStoragePolicy{{Priority: 1, MatchMode: AdmissionMatchModeExclude}}
	if ShouldStoreAdmissionEvent(rules, AdmissionEvent{Namespace: "app", Resource: "pods"}) {
		t.Error("an empty exclude rule must not store anything")
	}
}

func TestShouldStoreAdmissionEventTreatsEmptyConditionsAsWildcard(t *testing.T) {
	rules := []AdmissionStoragePolicy{{Priority: 1, MatchMode: AdmissionMatchModeInclude}}
	if !ShouldStoreAdmissionEvent(rules, AdmissionEvent{}) {
		t.Error("an include rule with no conditions must store everything")
	}
}

func TestNormalizedAdmissionGVR(t *testing.T) {
	if got := NormalizedAdmissionGVR("", "pods"); got != "pods" {
		t.Fatalf("core GVR = %q, want pods", got)
	}
	if got := NormalizedAdmissionGVR("apps", "deployments"); got != "apps/deployments" {
		t.Fatalf("apps GVR = %q, want apps/deployments", got)
	}
}
