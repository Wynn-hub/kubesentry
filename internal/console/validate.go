package console

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

type policyRequest struct {
	Name            string               `json:"name,omitempty"`
	Description     string               `json:"description"`
	EnforcementMode string               `json:"enforcementMode"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	Rego            string               `json:"rego"`
	Labels          map[string]string    `json:"labels,omitempty"`
	ResourceVersion string               `json:"resourceVersion,omitempty"`
}

func validatePolicyRequest(req *policyRequest, isCreate bool) error {
	if isCreate {
		if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
			return fmt.Errorf("invalid name: %s", strings.Join(errs, "; "))
		}
	}
	if req.EnforcementMode != v1alpha1.ModeEnforce && req.EnforcementMode != v1alpha1.ModeAudit {
		return fmt.Errorf("enforcementMode must be %q or %q", v1alpha1.ModeEnforce, v1alpha1.ModeAudit)
	}
	if len(req.Match.Operations) == 0 {
		return fmt.Errorf("match.operations must not be empty")
	}
	if len(req.Match.Resources) == 0 {
		return fmt.Errorf("match.resources must not be empty")
	}
	if _, err := webhook.CompileRego(req.Rego); err != nil {
		return fmt.Errorf("rego compile failed: %w", err)
	}
	return nil
}

type groupRequest struct {
	Name                    string                       `json:"name,omitempty"`
	DisplayName             string                       `json:"displayName"`
	Description             string                       `json:"description"`
	Enabled                 bool                         `json:"enabled"`
	NamespaceSelector       *metav1.LabelSelector        `json:"namespaceSelector,omitempty"`
	Policies                v1alpha1.PolicyGroupPolicies `json:"policies"`
	SelectorEnforcementMode string                       `json:"selectorEnforcementMode,omitempty"`
	ResourceVersion         string                       `json:"resourceVersion,omitempty"`
}

func validMode(m string) bool {
	return m == "" || m == v1alpha1.ModeEnforce || m == v1alpha1.ModeAudit
}

func validateGroupRequest(req *groupRequest, isCreate bool) error {
	if isCreate {
		if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
			return fmt.Errorf("invalid name: %s", strings.Join(errs, "; "))
		}
	}
	for _, ref := range req.Policies.ByName {
		if ref.Name == "" {
			return fmt.Errorf("policies.byName entries must have a name")
		}
		if !validMode(ref.EnforcementMode) {
			return fmt.Errorf("policies.byName[%s]: invalid enforcementMode %q", ref.Name, ref.EnforcementMode)
		}
	}
	if !validMode(req.SelectorEnforcementMode) {
		return fmt.Errorf("invalid selectorEnforcementMode %q", req.SelectorEnforcementMode)
	}
	return nil
}
