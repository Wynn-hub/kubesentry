// Package admissionpolicy holds the canonical admission-event storage-rule
// matcher shared by the gateway plane (internal/dbrepo, internal/gateway) and
// the member plane (internal/operator/hublink). It deliberately has no database
// or protobuf dependency so both planes can call the exact same logic: two
// copies of this matching rule previously drifted apart (one trimmed candidate
// values, the other did not), which made the member side skip discards the
// gateway would perform.
package admissionpolicy

import "strings"

const (
	MatchModeInclude = "include"
	MatchModeExclude = "exclude"
)

// Rule is one storage rule: a set of conditions plus whether they describe what
// to store (include) or what to leave out (exclude).
//
// Rules carry no action and no retention: storing is the only outcome and the
// retention period is a single global setting. See ShouldStore for how a rule
// set is evaluated.
type Rule struct {
	MatchMode  string
	Verdicts   []string
	Clusters   []string
	GVKs       []string
	Namespaces []string
}

// Event is the subset of an admission event the matcher inspects.
type Event struct {
	Verdict     string
	ClusterName string
	APIGroup    string
	Resource    string
	Namespace   string
}

// ShouldStore reports whether an event is stored under the given rule set.
//
// The rule set is an unordered allow-list combined with OR: an event is stored
// when it matches ANY rule, and is not stored when it matches none. An empty
// rule set stores everything, so collection is on by default until an operator
// narrows it.
//
// Within one rule, conditions combine with AND and values inside a condition are
// alternatives (OR); an empty condition is a wildcard. MatchModeExclude inverts
// that rule's conditions, so it matches events *outside* what they describe.
//
// This makes the natural single rule do the obvious thing: include
// {namespace=default, gvk=apps/deployments} stores exactly those events and
// nothing else, with no ordering and no catch-all rule to remember.
func ShouldStore(rules []Rule, event Event) bool {
	if len(rules) == 0 {
		return true
	}
	return MatchIndex(rules, event) >= 0
}

// MatchIndex returns the index of the first rule matching the event, or -1 when
// none does. Rules are unordered for storage decisions (see ShouldStore); the
// index is returned only so callers can report which rule matched.
func MatchIndex(rules []Rule, event Event) int {
	gvr := NormalizedGVR(event.APIGroup, event.Resource)
	for i, rule := range rules {
		conditionsMatch := matchesAny(rule.Verdicts, event.Verdict) &&
			matchesAny(rule.Clusters, event.ClusterName) &&
			matchesAny(rule.GVKs, gvr) &&
			matchesAny(rule.Namespaces, event.Namespace)
		if rule.MatchMode == MatchModeExclude {
			conditionsMatch = !conditionsMatch
		}
		if conditionsMatch {
			return i
		}
	}
	return -1
}

// NormalizedGVR renders the version-less resource identifier used by storage
// rules and event filtering alike: a core-group resource is the bare plural
// name ("pods"), anything else is "<group>/<resource>" ("apps/deployments").
// Note the core group must NOT produce a leading slash.
func NormalizedGVR(apiGroup, resource string) string {
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
