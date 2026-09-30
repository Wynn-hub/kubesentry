package dto

import (
	"strconv"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

// This file holds the mapping between database entities and Console response
// DTOs so schema or model changes have one conversion point.

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
		Match:           NormalizePolicyMatch(p.Match.Data()),
		EnforcementMode: p.EnforcementMode,
		Rego:            p.Rego,
		Description:     p.Description,
	}
}

func PolicyToListItem(p *dbrepo.Policy) PolicyItem {
	return PolicyItem{
		Name:            p.Name,
		Source:          policySource(p.Labels.Data()),
		EnforcementMode: p.EnforcementMode,
		Phase:           policyPhase(p.CompileOK),
		Description:     p.Description,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		OwnerType:       p.OwnerType,
		TenantID:        p.TenantID,
	}
}

// PolicyToDetail builds the GET response. referencedBy is normalized to a
// non-nil slice: a nil slice would serialize as null, violating the spec's
// required-array contract (incident P2-CF1).
func PolicyToDetail(p *dbrepo.Policy, referencedBy []string) PolicyView {
	if referencedBy == nil {
		referencedBy = []string{}
	}
	labels, annotations := p.Labels.Data(), p.Annotations.Data()
	if labels == nil {
		labels = map[string]string{}
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	d := PolicyView{
		Name:            p.Name,
		Source:          policySource(p.Labels.Data()),
		Description:     p.Description,
		Labels:          labels,
		Annotations:     annotations,
		EnforcementMode: p.EnforcementMode,
		Match:           NormalizePolicyMatch(p.Match.Data()),
		Rego:            p.Rego,
		Phase:           policyPhase(p.CompileOK),
		ReferencedBy:    referencedBy,
		ResourceVersion: PolicyResourceVersion(p),
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
		OwnerType:       p.OwnerType,
		TenantID:        p.TenantID,
	}
	if p.CompileError != "" {
		d.CompileError = &p.CompileError
	}
	// Labels/Annotations: dbrepo declares the columns JSONB NOT NULL DEFAULT
	// '{}' (0001_init_schema.up.sql:36-37) so .Data() never yields nil here.
	return d
}

func NormalizePolicyMatch(match v1alpha1.PolicyMatch) v1alpha1.PolicyMatch {
	result := v1alpha1.PolicyMatch{Operations: append([]string{}, match.Operations...), Resources: make([]v1alpha1.MatchResource, 0, len(match.Resources))}
	for _, resource := range match.Resources {
		result.Resources = append(result.Resources, v1alpha1.MatchResource{
			APIGroups: append([]string{}, resource.APIGroups...), APIVersions: append([]string{}, resource.APIVersions...), Resources: append([]string{}, resource.Resources...),
		})
	}
	return result
}
