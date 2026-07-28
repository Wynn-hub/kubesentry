package dbrepo

import "strings"

const (
	AdmissionStorageActionRetain  = "retain"
	AdmissionStorageActionDiscard = "discard"
	AdmissionMatchModeInclude     = "include"
	AdmissionMatchModeExclude     = "exclude"
)

// MatchAdmissionStoragePolicy returns the first matching rule. Empty fields
// are wildcards; values in one field are alternatives while fields combine
// with AND. Both Gateway and member-side code use this canonical matcher.
func MatchAdmissionStoragePolicy(policies []AdmissionStoragePolicy, event AdmissionEvent) *AdmissionStoragePolicy {
	gvr := NormalizedAdmissionGVR(event.APIGroup, event.Resource)
	for i := range policies {
		p := &policies[i]
		conditionsMatch := matchesAny(p.Verdicts, event.Verdict) &&
			matchesAny(p.Clusters, event.ClusterName) &&
			matchesAny(p.GVKs, gvr) &&
			matchesAny(p.Namespaces, event.Namespace)
		ruleMatches := conditionsMatch
		if p.MatchMode == AdmissionMatchModeExclude {
			ruleMatches = !conditionsMatch
		}
		if ruleMatches {
			return p
		}
	}
	return nil
}

func NormalizedAdmissionGVR(apiGroup, resource string) string {
	if apiGroup == "" {
		return resource
	}
	return apiGroup + "/" + resource
}

func matchesAny(values []string, value string) bool {
	if len(values) == 0 {
		return true
	}
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}
