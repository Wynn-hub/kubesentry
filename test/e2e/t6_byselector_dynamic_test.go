//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestT6_BySelectorDynamicBinding asserts that creating a custom Policy via
// console-api, labeled with a unique marker, makes it appear in the
// "default" PolicyGroup's resolvedPolicies on the member cluster once a
// matching bySelector is added — within 10s of the console-api write.
//
// Uses the built-in "default" group (temporarily adding a throwaway
// bySelector, restored on cleanup) rather than a brand-new PolicyGroup: a
// PolicyGroup created directly on the member cluster would never be
// targeted by the seeded "default" PolicyDistribution (console-api has no
// distribution CRUD route yet), so it would never receive a gateway push to
// prove propagation against. The unique marker label avoids matching any
// other (builtin or test) policy that might carry a real
// kubesentry.io/category label.
func TestT6_BySelectorDynamicBinding(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	const policyName = "test-byselector-pol"
	const markerKey = "kubesentry.io/e2e-t6-marker"

	// ---- API layer: create the policy ----
	body := map[string]any{
		"name":            policyName,
		"enforcementMode": v1alpha1.ModeAudit,
		"labels":          map[string]string{markerKey: "yes"},
		"match": map[string]any{
			"operations": []string{"CREATE"},
			"resources": []map[string]any{
				{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
			},
		},
		"rego": "package kubesentry\ndeny[m]{false;m:=\"x\"}",
	}
	if code, env := consoleDo(t, http.MethodPost, "/api/v1/policies", body); code != http.StatusOK {
		t.Fatalf("create policy via console-api: status %d env=%+v", code, env)
	}
	t.Cleanup(func() { consoleDo(t, http.MethodDelete, "/api/v1/policies/"+policyName+"?force=true", nil) })

	// ---- API layer: bind it into "default" via a throwaway bySelector ----
	spec := getGroupSpec(t, "default")
	origSelector := spec.Policies.BySelector
	spec.Policies.BySelector = &metav1.LabelSelector{MatchLabels: map[string]string{markerKey: "yes"}}
	putGroupSpec(t, "default", spec)
	t.Cleanup(func() {
		spec := getGroupSpec(t, "default")
		spec.Policies.BySelector = origSelector
		putGroupSpec(t, "default", spec)
	})

	// ---- propagation layer ----
	if err := helpers.WaitForGroupMemberPresence(ctx, k8sClient, "default", policyName, true, 10*time.Second); err != nil {
		t.Fatalf("custom policy did not enter default group's resolvedPolicies via bySelector within 10s: %v", err)
	}
	t.Log("T6 PASS: custom policy appeared in default group's resolvedPolicies via bySelector")
}
