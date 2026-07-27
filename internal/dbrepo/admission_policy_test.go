package dbrepo

import (
	"testing"

	"github.com/lib/pq"
)

func TestMatchAdmissionStoragePolicyUsesFirstMatchingRule(t *testing.T) {
	event := AdmissionEvent{Verdict: "deny", ClusterName: "prod", APIGroup: "", Resource: "pods", Namespace: "payments"}
	policies := []AdmissionStoragePolicy{
		{Priority: 1, Verdicts: pq.StringArray{"audit"}, Action: AdmissionStorageActionDiscard},
		{Priority: 2, Clusters: pq.StringArray{"prod"}, GVKs: pq.StringArray{"/pods"}, Action: AdmissionStorageActionDiscard},
		{Priority: 3, Action: AdmissionStorageActionRetain},
	}

	got := MatchAdmissionStoragePolicy(policies, event)
	if got == nil || got.Priority != 2 {
		t.Fatalf("matched policy = %#v, want priority 2", got)
	}
}

func TestMatchAdmissionStoragePolicyTreatsEmptyConditionsAsWildcard(t *testing.T) {
	got := MatchAdmissionStoragePolicy([]AdmissionStoragePolicy{{Priority: 1, Action: AdmissionStorageActionRetain}}, AdmissionEvent{})
	if got == nil || got.Priority != 1 {
		t.Fatalf("matched policy = %#v, want wildcard policy", got)
	}
}
