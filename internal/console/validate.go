package console

import (
	"fmt"
	"strings"

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
