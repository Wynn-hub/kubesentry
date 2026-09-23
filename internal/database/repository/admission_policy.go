package repository

import admissionpolicy "github.com/Wynn-hub/kubesentry/internal/admission/policy"

const (
	AdmissionMatchModeInclude = admissionpolicy.MatchModeInclude
	AdmissionMatchModeExclude = admissionpolicy.MatchModeExclude
)

func admissionRulesOf(policies []AdmissionStoragePolicy) []admissionpolicy.Rule {
	rules := make([]admissionpolicy.Rule, len(policies))
	for i, p := range policies {
		rules[i] = admissionpolicy.Rule{
			MatchMode:  p.MatchMode,
			Verdicts:   p.Verdicts,
			Clusters:   p.Clusters,
			GVKs:       p.GVKs,
			Namespaces: p.Namespaces,
		}
	}
	return rules
}

// ShouldStoreAdmissionEvent reports whether the rule set stores this event.
// Matching any rule stores it; an empty rule set stores everything. Retention
// is not decided here — it is the single global default retention period.
func ShouldStoreAdmissionEvent(policies []AdmissionStoragePolicy, event AdmissionEvent) bool {
	return admissionpolicy.ShouldStore(admissionRulesOf(policies), admissionEventOf(event))
}

func admissionEventOf(event AdmissionEvent) admissionpolicy.Event {
	return admissionpolicy.Event{
		Verdict:     event.Verdict,
		ClusterName: event.ClusterName,
		APIGroup:    event.APIGroup,
		Resource:    event.Resource,
		Namespace:   event.Namespace,
	}
}

// NormalizedAdmissionGVR is kept as the dbrepo-facing name for the canonical
// version-less resource identifier.
func NormalizedAdmissionGVR(apiGroup, resource string) string {
	return admissionpolicy.NormalizedGVR(apiGroup, resource)
}
