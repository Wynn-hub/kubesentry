package admissionpolicy_test

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/admissionpolicy"
)

func TestNormalizedGVRLeavesCoreGroupUnprefixed(t *testing.T) {
	if got := admissionpolicy.NormalizedGVR("", "pods"); got != "pods" {
		t.Errorf("core group GVR = %q, want pods", got)
	}
	if got := admissionpolicy.NormalizedGVR("apps", "deployments"); got != "apps/deployments" {
		t.Errorf("apps GVR = %q, want apps/deployments", got)
	}
}

func TestMatchIndexIncludeAndExclude(t *testing.T) {
	event := admissionpolicy.Event{Verdict: "deny", ClusterName: "prod", APIGroup: "", Resource: "pods", Namespace: "app"}

	tests := []struct {
		name  string
		rules []admissionpolicy.Rule
		want  int
	}{
		{
			name:  "empty include rule is a catch-all",
			rules: []admissionpolicy.Rule{{MatchMode: admissionpolicy.MatchModeInclude}},
			want:  0,
		},
		{
			name:  "empty exclude rule never matches",
			rules: []admissionpolicy.Rule{{MatchMode: admissionpolicy.MatchModeExclude}},
			want:  -1,
		},
		{
			name:  "include matches on normalized core-group resource",
			rules: []admissionpolicy.Rule{{GVKs: []string{"pods"}}},
			want:  0,
		},
		{
			name:  "include does not match a different resource",
			rules: []admissionpolicy.Rule{{GVKs: []string{"apps/deployments"}}},
			want:  -1,
		},
		{
			name:  "exclude matches events outside its conditions",
			rules: []admissionpolicy.Rule{{MatchMode: admissionpolicy.MatchModeExclude, Namespaces: []string{"kube-system"}}},
			want:  0,
		},
		{
			name:  "exclude skips events its conditions describe",
			rules: []admissionpolicy.Rule{{MatchMode: admissionpolicy.MatchModeExclude, Namespaces: []string{"app"}}},
			want:  -1,
		},
		{
			name: "reports the first matching rule",
			rules: []admissionpolicy.Rule{
				{GVKs: []string{"apps/deployments"}},
				{Verdicts: []string{"deny"}},
				{MatchMode: admissionpolicy.MatchModeInclude},
			},
			want: 1,
		},
		{
			name:  "conditions combine with AND",
			rules: []admissionpolicy.Rule{{Verdicts: []string{"deny"}, Namespaces: []string{"other"}}},
			want:  -1,
		},
		{
			name:  "values within a condition are alternatives",
			rules: []admissionpolicy.Rule{{Verdicts: []string{"audit", "deny"}}},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := admissionpolicy.MatchIndex(tt.rules, event); got != tt.want {
				t.Errorf("MatchIndex = %d, want %d", got, tt.want)
			}
		})
	}
}

// ShouldStore is the decision the whole system relies on: an empty rule set
// stores everything, otherwise the set acts as an allow-list.
func TestShouldStore(t *testing.T) {
	wanted := admissionpolicy.Event{Namespace: "default", APIGroup: "apps", Resource: "deployments"}
	otherNS := admissionpolicy.Event{Namespace: "test-builtin-rules", APIGroup: "apps", Resource: "deployments"}
	otherRes := admissionpolicy.Event{Namespace: "default", Resource: "pods"}

	if !admissionpolicy.ShouldStore(nil, otherRes) {
		t.Error("empty rule set must store everything")
	}

	only := []admissionpolicy.Rule{{Namespaces: []string{"default"}, GVKs: []string{"apps/deployments"}}}
	if !admissionpolicy.ShouldStore(only, wanted) {
		t.Error("matching event must be stored")
	}
	if admissionpolicy.ShouldStore(only, otherNS) {
		t.Error("event in another namespace must not be stored")
	}
	if admissionpolicy.ShouldStore(only, otherRes) {
		t.Error("event with another resource must not be stored")
	}

	either := []admissionpolicy.Rule{
		{Namespaces: []string{"default"}},
		{GVKs: []string{"pods"}},
	}
	if !admissionpolicy.ShouldStore(either, otherRes) {
		t.Error("rules combine with OR, so a pods event must be stored")
	}
	if admissionpolicy.ShouldStore(either, admissionpolicy.Event{Namespace: "other", Resource: "configmaps"}) {
		t.Error("event matching no rule must not be stored")
	}
}

// Both planes must agree on padded rule values. The member-side copy of this
// matcher used to compare without trimming, so " pods" matched on the gateway
// (which discarded the event) but not on the member (which forwarded it).
func TestMatchIndexTrimsRuleValues(t *testing.T) {
	event := admissionpolicy.Event{Verdict: "deny", Resource: "pods"}
	for _, value := range []string{" pods", "pods ", " pods "} {
		rules := []admissionpolicy.Rule{{GVKs: []string{value}}}
		if got := admissionpolicy.MatchIndex(rules, event); got != 0 {
			t.Errorf("MatchIndex with rule value %q = %d, want 0", value, got)
		}
	}
}
