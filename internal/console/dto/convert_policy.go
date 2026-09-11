package dto

import (
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// This file holds the ONLY mapping between database entities and the
// generated ogen types. Keeping every conversion here means a schema or
// model change has exactly one place to update — and the named enum types
// turn an invalid value into a compile error at the conversion site rather
// than a runtime 500 downstream.

const builtinLabel = "kubesentry.io/builtin"

// policySource derives the builtin/custom badge from the label — mirrors
// handlers_policy.go's policySource. There is no dedicated source column.
func policySource(labels map[string]string) string {
	if labels[builtinLabel] == "true" {
		return "builtin"
	}
	return "custom"
}

// policyPhase derives Ready/Invalid from CompileOK at read time.
func policyPhase(compileOK bool) string {
	if compileOK {
		return v1alpha1.PhaseReady
	}
	return v1alpha1.PhaseInvalid
}

// PolicyResourceVersion derives an opaque optimistic-concurrency token from
// the row's UpdatedAt, mirroring the CRD-era resourceVersion contract.
func PolicyResourceVersion(p *dbrepo.Policy) string {
	return strconv.FormatInt(p.UpdatedAt.UnixNano(), 10)
}

// PolicySpecSnapshot is the content stored on every version row — identical
// shape to what handlers_policy.go's policySpecSnapshot produces.
func PolicySpecSnapshot(p *dbrepo.Policy) v1alpha1.PolicySpec {
	return v1alpha1.PolicySpec{
		Match:           p.Match.Data(),
		EnforcementMode: p.EnforcementMode,
		Rego:            p.Rego,
		Description:     p.Description,
	}
}

func PolicyToListItem(p *dbrepo.Policy) PolicyListItem {
	return PolicyListItem{
		Name:            p.Name,
		Source:          PolicyListItemSource(policySource(p.Labels.Data())),
		EnforcementMode: PolicyListItemEnforcementMode(p.EnforcementMode),
		Phase:           PolicyListItemPhase(policyPhase(p.CompileOK)),
		Description:     p.Description,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

// PolicyToDetail builds the GET response. referencedBy is normalized to a
// non-nil slice: a nil slice would serialize as null, violating the spec's
// required-array contract (incident P2-CF1).
func PolicyToDetail(p *dbrepo.Policy, referencedBy []string) PolicyDetail {
	if referencedBy == nil {
		referencedBy = []string{}
	}
	d := PolicyDetail{
		Name:            p.Name,
		Source:          PolicyDetailSource(policySource(p.Labels.Data())),
		Description:     p.Description,
		Labels:          PolicyDetailLabels(p.Labels.Data()),
		Annotations:     PolicyDetailAnnotations(p.Annotations.Data()),
		EnforcementMode: PolicyDetailEnforcementMode(p.EnforcementMode),
		Match:           MatchToDTO(p.Match.Data()),
		Rego:            p.Rego,
		Phase:           PolicyDetailPhase(policyPhase(p.CompileOK)),
		ReferencedBy:    referencedBy,
		ResourceVersion: PolicyResourceVersion(p),
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
	if p.CompileError != "" {
		d.CompileError.SetTo(p.CompileError)
	}
	// Labels/Annotations: dbrepo declares the columns JSONB NOT NULL DEFAULT
	// '{}' (0001_init_schema.up.sql:36-37) so .Data() never yields nil here.
	return d
}

func MatchToDTO(m v1alpha1.PolicyMatch) PolicyMatch {
	ops := make([]PolicyMatchOperationsItem, 0, len(m.Operations))
	for _, op := range m.Operations {
		ops = append(ops, PolicyMatchOperationsItem(op))
	}
	res := make([]MatchResource, 0, len(m.Resources))
	for _, r := range m.Resources {
		res = append(res, MatchResource{
			ApiGroups:   append([]string(nil), r.APIGroups...),
			ApiVersions: append([]string(nil), r.APIVersions...),
			Resources:   append([]string(nil), r.Resources...),
		})
	}
	return PolicyMatch{Operations: ops, Resources: res}
}

func MatchFromDTO(m PolicyMatch) v1alpha1.PolicyMatch {
	ops := make([]string, 0, len(m.Operations))
	for _, op := range m.Operations {
		ops = append(ops, string(op))
	}
	res := make([]v1alpha1.MatchResource, 0, len(m.Resources))
	for _, r := range m.Resources {
		res = append(res, v1alpha1.MatchResource{
			APIGroups:   append([]string(nil), r.ApiGroups...),
			APIVersions: append([]string(nil), r.ApiVersions...),
			Resources:   append([]string(nil), r.Resources...),
		})
	}
	return v1alpha1.PolicyMatch{Operations: ops, Resources: res}
}
