package dto

import (
	"strconv"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func policyFixture() *dbrepo.Policy {
	return &dbrepo.Policy{
		Base: dbmodel.Base{
			ID:        42,
			CreatedAt: time.Date(2026, 8, 3, 10, 15, 30, 0, time.UTC),
			UpdatedAt: time.Date(2026, 8, 3, 11, 20, 45, 0, time.UTC),
		},
		Name:            "deny-privileged",
		Description:     "no privileged containers",
		Rego:            "package kubesentry",
		EnforcementMode: "enforce",
		Match: datatypes.NewJSONType(v1alpha1.PolicyMatch{
			Operations: []string{"CREATE"},
			Resources: []v1alpha1.MatchResource{
				{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			},
		}),
		CompileOK:   true,
		Labels:      datatypes.NewJSONType(map[string]string{"kubesentry.io/builtin": "true"}),
		Annotations: datatypes.NewJSONType(map[string]string{"team": "platform"}),
	}
}

func TestPolicyToListItem(t *testing.T) {
	item := PolicyToListItem(policyFixture())
	if item.Name != "deny-privileged" {
		t.Fatalf("name = %q", item.Name)
	}
	if item.Source != PolicyListItemSourceBuiltin {
		t.Fatalf("source = %q, want builtin", item.Source)
	}
	if item.EnforcementMode != PolicyListItemEnforcementModeEnforce {
		t.Fatalf("enforcementMode = %q", item.EnforcementMode)
	}
	if item.Phase != PolicyListItemPhaseReady {
		t.Fatalf("phase = %q, want Ready", item.Phase)
	}
}

func TestPolicyToListItemInvalidPhase(t *testing.T) {
	p := policyFixture()
	p.CompileOK = false
	item := PolicyToListItem(p)
	if item.Phase != PolicyListItemPhaseInvalid {
		t.Fatalf("phase = %q, want Invalid", item.Phase)
	}
}

func TestPolicyToDetailCompileError(t *testing.T) {
	p := policyFixture()
	p.CompileOK = false
	p.CompileError = "rego_parse_error: eof"
	d := PolicyToDetail(p, []string{})
	if !d.CompileError.IsSet() || d.CompileError.Value != "rego_parse_error: eof" {
		t.Fatalf("compileError = %+v, want set", d.CompileError)
	}
	// CompileOK=true must leave compileError unset (omitempty semantics).
	ok := policyFixture()
	if d2 := PolicyToDetail(ok, []string{}); d2.CompileError.IsSet() {
		t.Fatalf("compileError should be unset, got %+v", d2.CompileError)
	}
}

// referencedBy must be non-nil: a nil slice serializes to null, violating the
// OpenAPI required-array contract (P2-CF1).
func TestPolicyToDetailReferencedByNeverNil(t *testing.T) {
	d := PolicyToDetail(policyFixture(), nil)
	if d.ReferencedBy == nil {
		t.Fatal("referencedBy is nil, want empty slice")
	}
	if len(d.ReferencedBy) != 0 {
		t.Fatalf("referencedBy = %v, want empty", d.ReferencedBy)
	}
}

func TestMatchRoundTrip(t *testing.T) {
	m := v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
		},
	}
	d := MatchToDTO(m)
	if len(d.Operations) != 2 || d.Operations[0] != PolicyMatchOperationsItemCREATE {
		t.Fatalf("operations = %v", d.Operations)
	}
	back := MatchFromDTO(d)
	if len(back.Operations) != 2 || back.Operations[0] != "CREATE" {
		t.Fatalf("round trip operations = %v", back.Operations)
	}
	if len(back.Resources) != 1 || back.Resources[0].APIGroups[0] != "apps" {
		t.Fatalf("round trip resources = %v", back.Resources)
	}
}

func TestPolicyResourceVersionMatchesHandlerBehavior(t *testing.T) {
	p := policyFixture()
	got := PolicyResourceVersion(p)
	want := strconv.FormatInt(p.UpdatedAt.UnixNano(), 10)
	if got != want {
		t.Fatalf("resourceVersion = %q, want %q", got, want)
	}
}
