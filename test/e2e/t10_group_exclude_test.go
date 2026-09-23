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

// TestT10_GroupExcludeOverridesBySelector reproduces the original bug
// report: a policy matched purely via bySelector must be excludable by
// name, and the exclusion must be reversible.
//
// Uses the built-in "default" group (temporarily adding a throwaway
// bySelector and exclude entry, both restored on cleanup) rather than a
// brand-new PolicyGroup — see TestT6_BySelectorDynamicBinding's comment for
// why a throwaway PolicyGroup can never be pushed to the member cluster
// through console-api alone (no distribution CRUD route yet).
func TestT10_GroupExcludeOverridesBySelector(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	const policyName = "test-exclude-pol"
	const markerKey = "kubesentry.io/e2e-t10-marker"

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

	spec := getGroupSpec(t, "default")
	origSelector := spec.Policies.BySelector
	origExclude := spec.Policies.Exclude
	t.Cleanup(func() {
		spec := getGroupSpec(t, "default")
		spec.Policies.BySelector = origSelector
		spec.Policies.Exclude = origExclude
		putGroupSpec(t, "default", spec)
	})

	spec.Policies.BySelector = &metav1.LabelSelector{MatchLabels: map[string]string{markerKey: "yes"}}
	putGroupSpec(t, "default", spec)

	// Step 1: confirm bySelector auto-capture still works (regression guard).
	if err := helpers.WaitForGroupMemberPresence(ctx, k8sClient, "default", policyName, true, 10*time.Second); err != nil {
		t.Fatalf("step 1: bySelector auto-capture failed: %v", err)
	}

	// Step 2: add it to the group's exclude list.
	spec = getGroupSpec(t, "default")
	spec.Policies.Exclude = append(spec.Policies.Exclude, policyName)
	putGroupSpec(t, "default", spec)
	if err := helpers.WaitForGroupMemberPresence(ctx, k8sClient, "default", policyName, false, 10*time.Second); err != nil {
		t.Fatalf("step 2: exclude did not remove member: %v", err)
	}

	// Step 3: clear the exclude entry, confirm the member reappears.
	spec = getGroupSpec(t, "default")
	kept := spec.Policies.Exclude[:0]
	for _, name := range spec.Policies.Exclude {
		if name != policyName {
			kept = append(kept, name)
		}
	}
	spec.Policies.Exclude = kept
	putGroupSpec(t, "default", spec)
	if err := helpers.WaitForGroupMemberPresence(ctx, k8sClient, "default", policyName, true, 10*time.Second); err != nil {
		t.Fatalf("step 3: member did not reappear after clearing exclude: %v", err)
	}
}
