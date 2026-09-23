package dto

import (
	"strconv"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/database/model"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func policyFixture() *dbrepo.Policy {
	return &dbrepo.Policy{
		Base: model.Base{
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
	if item.Source != "builtin" {
		t.Fatalf("source = %q, want builtin", item.Source)
	}
	if item.EnforcementMode != "enforce" {
		t.Fatalf("enforcementMode = %q", item.EnforcementMode)
	}
	if item.Phase != "Ready" {
		t.Fatalf("phase = %q, want Ready", item.Phase)
	}
}

func TestPolicyToListItemInvalidPhase(t *testing.T) {
	p := policyFixture()
	p.CompileOK = false
	item := PolicyToListItem(p)
	if item.Phase != "Invalid" {
		t.Fatalf("phase = %q, want Invalid", item.Phase)
	}
}

func TestPolicyToDetailCompileError(t *testing.T) {
	p := policyFixture()
	p.CompileOK = false
	p.CompileError = "rego_parse_error: eof"
	d := PolicyToDetail(p, []string{})
	if d.CompileError == nil || *d.CompileError != "rego_parse_error: eof" {
		t.Fatalf("compileError = %+v, want set", d.CompileError)
	}
	// CompileOK=true must leave compileError unset (omitempty semantics).
	ok := policyFixture()
	ok.CompileOK = true // explicit — don't rely on fixture default
	if d2 := PolicyToDetail(ok, []string{}); d2.CompileError != nil {
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

func TestPolicyToDetailNormalizesMatchArrays(t *testing.T) {
	p := policyFixture()
	p.Match = datatypes.NewJSONType(v1alpha1.PolicyMatch{Resources: []v1alpha1.MatchResource{{}}})
	d := PolicyToDetail(p, nil)
	if d.Match.Operations == nil || d.Match.Resources == nil || d.Match.Resources[0].APIGroups == nil || d.Match.Resources[0].APIVersions == nil || d.Match.Resources[0].Resources == nil {
		t.Fatalf("match arrays were not normalized: %+v", d.Match)
	}
}

// The enforcement mode is the only enum that reaches the DTO as a bare cast of
// a database column — Source and Phase both funnel through helpers that can
// only emit two values. So both modes need a case here, not just the fixture's.
func TestPolicyToListItemModeAndSource(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		labels   map[string]string
		wantMode string
		wantSrc  string
	}{
		{
			name:     "builtin enforce",
			mode:     "enforce",
			labels:   map[string]string{builtinLabel: "true"},
			wantMode: "enforce",
			wantSrc:  "builtin",
		},
		{
			name:     "custom audit",
			mode:     "audit",
			labels:   map[string]string{"team": "platform"},
			wantMode: "audit",
			wantSrc:  "custom",
		},
		{
			name:     "custom when builtin label is not exactly true",
			mode:     "audit",
			labels:   map[string]string{builtinLabel: "false"},
			wantMode: "audit",
			wantSrc:  "custom",
		},
		{
			name:     "custom when labels are empty",
			mode:     "enforce",
			labels:   map[string]string{},
			wantMode: "enforce",
			wantSrc:  "custom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := policyFixture()
			p.EnforcementMode = tt.mode
			p.Labels = datatypes.NewJSONType(tt.labels)
			item := PolicyToListItem(p)
			if item.EnforcementMode != tt.wantMode {
				t.Errorf("enforcementMode = %q, want %q", item.EnforcementMode, tt.wantMode)
			}
			if item.Source != tt.wantSrc {
				t.Errorf("source = %q, want %q", item.Source, tt.wantSrc)
			}
		})
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
