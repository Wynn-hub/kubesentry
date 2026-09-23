package gateway

import (
	"testing"

	"gorm.io/datatypes"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

// TestToV1alpha1PoliciesCarriesLabels covers migration 0003 (Policy.Labels):
// groupresolve.Resolve's bySelector matching reads v1alpha1.Policy.Labels,
// so the DB->CRD projection this render path uses must actually carry them
// through, not just the Match/EnforcementMode/Rego fields it always had.
func TestToV1alpha1PoliciesCarriesLabels(t *testing.T) {
	in := []dbrepo.Policy{{
		Name:        "p1",
		Labels:      datatypes.NewJSONType(map[string]string{"team": "security"}),
		Annotations: datatypes.NewJSONType(map[string]string{"note": "hello"}),
	}}
	out := toV1alpha1Policies(in)
	if len(out) != 1 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].Labels["team"] != "security" {
		t.Fatalf("labels = %+v", out[0].Labels)
	}
	if out[0].Annotations["note"] != "hello" {
		t.Fatalf("annotations = %+v", out[0].Annotations)
	}
}

func TestToV1alpha1GroupsCarriesLabels(t *testing.T) {
	in := []dbrepo.PolicyGroup{{
		Name:   "g1",
		Labels: datatypes.NewJSONType(map[string]string{"kubesentry.io/source": "custom"}),
	}}
	out := toV1alpha1Groups(in)
	if len(out) != 1 || out[0].Labels["kubesentry.io/source"] != "custom" {
		t.Fatalf("out = %+v", out)
	}
}
