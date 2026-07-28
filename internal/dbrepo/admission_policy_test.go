package dbrepo

import (
	"testing"

	"github.com/lib/pq"
)

func TestMatchAdmissionStoragePolicyUsesFirstMatchingRule(t *testing.T) {
	event := AdmissionEvent{Verdict: "deny", ClusterName: "prod", APIGroup: "", Resource: "pods", Namespace: "payments"}
	policies := []AdmissionStoragePolicy{
		{Priority: 1, Verdicts: pq.StringArray{"audit"}, Action: AdmissionStorageActionDiscard},
		{Priority: 2, Clusters: pq.StringArray{"prod"}, GVKs: pq.StringArray{"pods"}, Action: AdmissionStorageActionDiscard},
		{Priority: 3, Action: AdmissionStorageActionRetain},
	}

	got := MatchAdmissionStoragePolicy(policies, event)
	if got == nil || got.Priority != 2 {
		t.Fatalf("matched policy = %#v, want priority 2", got)
	}
}

func TestMatchAdmissionStoragePolicyExcludeNegatesWholeCondition(t *testing.T) {
	policy := AdmissionStoragePolicy{
		Priority:   1,
		MatchMode:  AdmissionMatchModeExclude,
		Namespaces: pq.StringArray{"kube-system"},
		GVKs:       pq.StringArray{"pods"},
		Action:     AdmissionStorageActionRetain,
	}
	tests := []struct {
		name      string
		namespace string
		resource  string
		wantMatch bool
	}{
		{name: "complete condition is excluded", namespace: "kube-system", resource: "pods", wantMatch: false},
		{name: "different namespace matches", namespace: "payments", resource: "pods", wantMatch: true},
		{name: "different resource matches", namespace: "kube-system", resource: "services", wantMatch: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := AdmissionEvent{APIGroup: "", Resource: tt.resource, Namespace: tt.namespace}
			got := MatchAdmissionStoragePolicy([]AdmissionStoragePolicy{policy}, event)
			if (got != nil) != tt.wantMatch {
				t.Fatalf("matched = %v, want %v", got != nil, tt.wantMatch)
			}
		})
	}
}

func TestMatchAdmissionStoragePolicyExcludeEmptyConditionNeverMatches(t *testing.T) {
	policy := AdmissionStoragePolicy{Priority: 1, MatchMode: AdmissionMatchModeExclude, Action: AdmissionStorageActionDiscard}
	if got := MatchAdmissionStoragePolicy([]AdmissionStoragePolicy{policy}, AdmissionEvent{}); got != nil {
		t.Fatalf("matched policy = %#v, want nil", got)
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

func TestMatchAdmissionStoragePolicyTreatsEmptyConditionsAsWildcard(t *testing.T) {
	got := MatchAdmissionStoragePolicy([]AdmissionStoragePolicy{{Priority: 1, Action: AdmissionStorageActionRetain}}, AdmissionEvent{})
	if got == nil || got.Priority != 1 {
		t.Fatalf("matched policy = %#v, want wildcard policy", got)
	}
}
