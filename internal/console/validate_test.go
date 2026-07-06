package console

import (
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func validReq() *policyRequest {
	return &policyRequest{
		Name:            "my-policy",
		EnforcementMode: "audit",
		Match: v1alpha1.PolicyMatch{
			Operations: []string{"CREATE"},
			Resources: []v1alpha1.MatchResource{
				{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			},
		},
		Rego: validRego,
	}
}

func TestValidatePolicyRequest(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*policyRequest)
		wantErr string // 空串表示期望通过
	}{
		{"valid", func(r *policyRequest) {}, ""},
		{"bad name", func(r *policyRequest) { r.Name = "Bad_Name" }, "name"},
		{"bad mode", func(r *policyRequest) { r.EnforcementMode = "block" }, "enforcementMode"},
		{"no operations", func(r *policyRequest) { r.Match.Operations = nil }, "operations"},
		{"no resources", func(r *policyRequest) { r.Match.Resources = nil }, "resources"},
		{"bad rego", func(r *policyRequest) { r.Rego = "package kubesentry\ndeny[msg] {" }, "rego"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validReq()
			tc.mutate(req)
			err := validatePolicyRequest(req, true)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tc.wantErr)
			}
		})
	}
}
