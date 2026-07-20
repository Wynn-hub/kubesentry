//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// groupPayload mirrors internal/console's private groupRequest — the fields
// a create/update PUT accepts. Kept local since the console type is
// unexported.
type groupPayload struct {
	DisplayName             string                       `json:"displayName"`
	Description             string                       `json:"description"`
	Enabled                 bool                         `json:"enabled"`
	NamespaceSelector       *metav1.LabelSelector        `json:"namespaceSelector,omitempty"`
	Policies                v1alpha1.PolicyGroupPolicies `json:"policies"`
	SelectorEnforcementMode string                       `json:"selectorEnforcementMode,omitempty"`
}

// groupDetailPayload mirrors internal/console's groupDetail response shape.
type groupDetailPayload struct {
	Name            string                   `json:"name"`
	ResourceVersion string                   `json:"resourceVersion"`
	Spec            v1alpha1.PolicyGroupSpec `json:"spec"`
}

// getGroupSpec fetches a PolicyGroup's current spec via console-api — the
// starting point for any read-modify-write update, since PUT replaces the
// whole spec rather than merging.
func getGroupSpec(t *testing.T, name string) v1alpha1.PolicyGroupSpec {
	t.Helper()
	code, env := consoleDo(t, http.MethodGet, "/api/v1/policygroups/"+name, nil)
	if code != http.StatusOK {
		t.Fatalf("get group %s via console-api: status %d env=%+v", name, code, env)
	}
	var d groupDetailPayload
	if err := json.Unmarshal(env.Data, &d); err != nil {
		t.Fatalf("decode group %s: %v", name, err)
	}
	return d.Spec
}

// putGroupSpec writes spec back for name via console-api.
func putGroupSpec(t *testing.T, name string, spec v1alpha1.PolicyGroupSpec) {
	t.Helper()
	body := groupPayload{
		DisplayName:             spec.DisplayName,
		Description:             spec.Description,
		Enabled:                 spec.Enabled,
		NamespaceSelector:       spec.NamespaceSelector,
		Policies:                spec.Policies,
		SelectorEnforcementMode: spec.SelectorEnforcementMode,
	}
	code, env := consoleDo(t, http.MethodPut, "/api/v1/policygroups/"+name, body)
	if code != http.StatusOK {
		t.Fatalf("update group %s via console-api: status %d env=%+v", name, code, env)
	}
}

// setDefaultGroupMemberMode patches the built-in "default" PolicyGroup's
// byName entry for policyName to mode via console-api (the gateway's own
// source of truth — the periodic push resync keeps reasserting this
// group's DB-stored spec over any direct member-CR edit, so a direct
// k8sClient patch no longer has a durable effect), returning its previous
// mode.
func setDefaultGroupMemberMode(t *testing.T, policyName, mode string) string {
	t.Helper()
	spec := getGroupSpec(t, "default")
	var original string
	found := false
	for i := range spec.Policies.ByName {
		if spec.Policies.ByName[i].Name == policyName {
			original = spec.Policies.ByName[i].EnforcementMode
			spec.Policies.ByName[i].EnforcementMode = mode
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("policy %s not found in default group byName", policyName)
	}
	putGroupSpec(t, "default", spec)
	return original
}

// TestT4_EnforceToAudit: switch a policy from enforce to audit.
// After switching, a previously-blocked pod should be admitted.
func TestT4_EnforceToAudit(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	policyName := "run-as-privileged"

	failPath := "../builtin-rules/security/run-as-privileged-fail.yaml"
	out, err := helpers.KubectlApply(ctx, failPath)
	if err == nil {
		helpers.KubectlDelete(ctx, failPath)
		t.Fatal("T4-1 setup: expected deny fixture to be blocked in enforce mode, but it was admitted")
	}
	t.Logf("T4-1: confirmed enforce mode blocks pod. Output: %s", out)

	// ---- API layer ----
	original := setDefaultGroupMemberMode(t, policyName, v1alpha1.ModeAudit)
	t.Cleanup(func() {
		setDefaultGroupMemberMode(t, policyName, original)
	})

	// ---- propagation layer ----
	if err := helpers.WaitForGroupMemberMode(ctx, k8sClient, "default", policyName, v1alpha1.ModeAudit, 30*time.Second); err != nil {
		t.Fatalf("T4-1: mode change did not propagate to member group: %v", err)
	}

	// ---- enforcement layer ----
	deadline := time.Now().Add(30 * time.Second)
	passed := false
	for time.Now().Before(deadline) {
		applyOut, applyErr := helpers.KubectlApply(ctx, failPath)
		if applyErr == nil {
			helpers.KubectlDelete(ctx, failPath)
			t.Log("T4-1 PASS: enforce→audit — previously-blocked pod now admitted")
			passed = true
			break
		}
		t.Logf("T4-1: pod still blocked (cache lag), retrying in 2s... output: %s", applyOut)
		time.Sleep(2 * time.Second)
	}
	if !passed {
		t.Errorf("T4-1: after switch to audit, deny fixture still blocked after 30s")
	}
}

// TestT4_AuditToEnforce: switch back to enforce.
func TestT4_AuditToEnforce(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	policyName := "run-as-privileged"

	setDefaultGroupMemberMode(t, policyName, v1alpha1.ModeEnforce)

	if err := helpers.WaitForGroupMemberMode(ctx, k8sClient, "default", policyName, v1alpha1.ModeEnforce, 30*time.Second); err != nil {
		t.Fatalf("T4-2: mode change did not propagate to member group: %v", err)
	}

	failPath := "../builtin-rules/security/run-as-privileged-fail.yaml"
	deadline := time.Now().Add(30 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		_, err := helpers.KubectlApply(ctx, failPath)
		if err != nil {
			blocked = true
			break
		}
		helpers.KubectlDelete(ctx, failPath)
		time.Sleep(2 * time.Second)
	}
	if !blocked {
		t.Error("T4-2: after switch to enforce, deny fixture was not blocked")
	} else {
		t.Log("T4-2 PASS: audit→enforce — pod is blocked again")
	}
}

// TestT4_DisablePolicy: switch a policy to audit to stop enforcement.
// In the new model there is no per-Policy enabled toggle in PolicyGroup;
// the closest equivalent is switching the default group's per-entry mode to audit.
// After switching, the previously-blocked resource should be admitted.
func TestT4_DisablePolicy(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	policyName := "host-ipc-set"

	original := setDefaultGroupMemberMode(t, policyName, v1alpha1.ModeAudit)
	t.Cleanup(func() {
		setDefaultGroupMemberMode(t, policyName, original)
	})

	if err := helpers.WaitForGroupMemberMode(ctx, k8sClient, "default", policyName, v1alpha1.ModeAudit, 30*time.Second); err != nil {
		t.Fatalf("T4-3: mode change did not propagate to member group: %v", err)
	}

	failPath := "../builtin-rules/security/host-ipc-fail.yaml"
	deadline := time.Now().Add(30 * time.Second)
	passed := false
	for time.Now().Before(deadline) {
		applyOut, applyErr := helpers.KubectlApply(ctx, failPath)
		if applyErr == nil {
			helpers.KubectlDelete(ctx, failPath)
			t.Log("T4-3 PASS: policy switched to audit — previously-blocked resource now admitted")
			passed = true
			break
		}
		t.Logf("T4-3: pod still blocked (cache lag), retrying in 2s... output: %s", applyOut)
		time.Sleep(2 * time.Second)
	}
	if !passed {
		t.Errorf("T4-3: after switching to audit, deny fixture still blocked after 30s")
	}
}

const t4CustomPolicyName = "e2e-forbidden-label"

var t4CustomPolicyMatch = map[string]any{
	"operations": []string{"CREATE", "UPDATE"},
	"resources": []map[string]any{
		{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
	},
}

const t4InitialRego = `package kubesentry
deny[msg] {
  input.request.object.metadata.labels["app"] == "forbidden-app"
  msg := "pods with app=forbidden-app label are not allowed"
}`

const t4UpdatedRego = `package kubesentry
deny[msg] {
  input.request.object.metadata.labels["app"] == "forbidden-app"
  msg := "pods with app=forbidden-app label are not allowed"
}
deny[msg] {
  input.request.object.metadata.labels["app"] == "also-forbidden"
  msg := "pods with app=also-forbidden label are not allowed"
}`

// TestT4_AddCustomPolicy: create a new custom Policy via console-api and add
// it to the "default" group (so the gateway actually pushes it to the
// member cluster — a brand-new PolicyGroup created directly on the member
// would never be targeted by the seeded "default" PolicyDistribution, since
// console-api has no distribution CRUD route yet), then verify it blocks
// matching pods.
// NOTE: does NOT clean up the policy or group entry — TestT4_RegoHotUpdate
// and TestT4_VersionRollback depend on them being present; the last test in
// this sequence (TestT4_VersionRollback) cleans up at the end.
func TestT4_AddCustomPolicy(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()

	// ---- API layer ----
	body := map[string]any{
		"name":            t4CustomPolicyName,
		"enforcementMode": v1alpha1.ModeEnforce,
		"description":     "E2E test: denies pods with app=forbidden-app label",
		"match":           t4CustomPolicyMatch,
		"rego":            t4InitialRego,
	}
	if code, env := consoleDo(t, http.MethodPost, "/api/v1/policies", body); code != http.StatusOK {
		t.Fatalf("T4-4: create policy via console-api: status %d env=%+v", code, env)
	}
	waitCurrentVersion(t, t4CustomPolicyName, 1)

	spec := getGroupSpec(t, "default")
	spec.Policies.ByName = append(spec.Policies.ByName, v1alpha1.PolicyRef{Name: t4CustomPolicyName})
	putGroupSpec(t, "default", spec)

	// ---- propagation layer ----
	if err := helpers.WaitForGroupMemberMode(ctx, k8sClient, "default", t4CustomPolicyName, v1alpha1.ModeEnforce, 30*time.Second); err != nil {
		t.Fatalf("T4-4: custom policy did not propagate to member group: %v", err)
	}
	if err := helpers.WaitForPolicyPhase(ctx, k8sClient, t4CustomPolicyName, v1alpha1.PhaseReady, 30*time.Second); err != nil {
		t.Fatalf("T4-4: custom policy did not reach Ready on member: %v", err)
	}

	// ---- enforcement layer ----
	forbiddenPod := `apiVersion: v1
kind: Pod
metadata:
  name: e2e-forbidden-pod
  namespace: test-builtin-rules
  labels:
    app: forbidden-app
spec:
  automountServiceAccountToken: false
  containers:
  - name: app
    image: nginx:1.25.0
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
`
	podFile := fmt.Sprintf("/tmp/e2e-forbidden-pod-%d.yaml", time.Now().UnixNano())
	if err := os.WriteFile(podFile, []byte(forbiddenPod), 0o600); err != nil {
		t.Fatalf("write temp pod file: %v", err)
	}
	defer os.Remove(podFile)

	deadline := time.Now().Add(30 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		_, err := helpers.KubectlApply(ctx, podFile)
		if err != nil {
			blocked = true
			break
		}
		helpers.KubectlDelete(ctx, podFile)
		time.Sleep(2 * time.Second)
	}
	if !blocked {
		t.Error("T4-4: pod with forbidden label was admitted, expected rejection")
	} else {
		t.Log("T4-4 PASS: custom policy active — forbidden pod blocked")
	}
}

// TestT4_RegoHotUpdate: update Policy Rego via console-api, verify new rule
// takes effect.
func TestT4_RegoHotUpdate(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()

	if code, _ := consoleDo(t, http.MethodGet, "/api/v1/policies/"+t4CustomPolicyName, nil); code != http.StatusOK {
		t.Skip("T4-5: custom policy e2e-forbidden-label not found (run T4-4 first)")
	}

	countBefore, err := helpers.CountPolicyVersions(ctx, k8sClient, t4CustomPolicyName)
	if err != nil {
		t.Fatalf("count policy versions before: %v", err)
	}

	// ---- API layer ----
	body := map[string]any{
		"name":            t4CustomPolicyName,
		"enforcementMode": v1alpha1.ModeEnforce,
		"description":     "E2E test: denies pods with forbidden app labels (updated)",
		"match":           t4CustomPolicyMatch,
		"rego":            t4UpdatedRego,
	}
	if code, env := consoleDo(t, http.MethodPut, "/api/v1/policies/"+t4CustomPolicyName, body); code != http.StatusOK {
		t.Fatalf("update policy via console-api: status %d env=%+v", code, env)
	}
	waitCurrentVersion(t, t4CustomPolicyName, 2)

	// ---- propagation layer: member's own PolicyReconciler creates a new
	// PolicyVersion CR whenever it observes the pushed Policy spec change,
	// regardless of push origin. ----
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		count, _ := helpers.CountPolicyVersions(ctx, k8sClient, t4CustomPolicyName)
		if count > countBefore {
			break
		}
		time.Sleep(2 * time.Second)
	}
	countAfter, _ := helpers.CountPolicyVersions(ctx, k8sClient, t4CustomPolicyName)
	if countAfter <= countBefore {
		t.Errorf("T4-5: expected PolicyVersion count to increase (was %d, still %d)", countBefore, countAfter)
	}

	// ---- enforcement layer ----
	alsoForbiddenPod := `apiVersion: v1
kind: Pod
metadata:
  name: e2e-also-forbidden-pod
  namespace: test-builtin-rules
  labels:
    app: also-forbidden
spec:
  automountServiceAccountToken: false
  containers:
  - name: app
    image: nginx:1.25.0
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
`
	podFile := fmt.Sprintf("/tmp/e2e-also-forbidden-%d.yaml", time.Now().UnixNano())
	if err := os.WriteFile(podFile, []byte(alsoForbiddenPod), 0o600); err != nil {
		t.Fatalf("write temp pod file: %v", err)
	}
	defer os.Remove(podFile)

	deadline = time.Now().Add(30 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		_, err := helpers.KubectlApply(ctx, podFile)
		if err != nil {
			blocked = true
			break
		}
		helpers.KubectlDelete(ctx, podFile)
		time.Sleep(2 * time.Second)
	}
	if !blocked {
		t.Error("T4-5: pod with also-forbidden label was admitted after Rego update, expected rejection")
	} else {
		t.Log("T4-5 PASS: Rego hot-update active — new rule blocks updated label")
	}
}

// TestT4_VersionRollback: rollback a policy to a previous version via
// console-api, verify old rule is restored. Also cleans up the T4-4/T4-5
// test artifacts (policy + default-group byName entry) since this is the
// last test in the sequence that depends on them.
func TestT4_VersionRollback(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	t.Cleanup(func() {
		spec := getGroupSpec(t, "default")
		kept := spec.Policies.ByName[:0]
		for _, ref := range spec.Policies.ByName {
			if ref.Name != t4CustomPolicyName {
				kept = append(kept, ref)
			}
		}
		spec.Policies.ByName = kept
		putGroupSpec(t, "default", spec)
		consoleDo(t, http.MethodDelete, "/api/v1/policies/"+t4CustomPolicyName+"?force=true", nil)
	})

	if code, _ := consoleDo(t, http.MethodGet, "/api/v1/policies/"+t4CustomPolicyName, nil); code != http.StatusOK {
		t.Skip("T4-6: custom policy not found (run T4-4 first)")
	}

	vCode, vEnv := consoleDo(t, http.MethodGet, "/api/v1/policies/"+t4CustomPolicyName+"/versions", nil)
	if vCode != http.StatusOK {
		t.Fatalf("list versions: status %d", vCode)
	}
	var tl struct {
		CurrentVersion int64 `json:"currentVersion"`
	}
	if err := json.Unmarshal(vEnv.Data, &tl); err != nil {
		t.Fatal(err)
	}
	if tl.CurrentVersion <= 1 {
		t.Skip("T4-6: policy has only 1 version, cannot roll back")
	}

	// ---- API layer ----
	if code, env := consoleDo(t, http.MethodPost, "/api/v1/policies/"+t4CustomPolicyName+"/rollback",
		map[string]string{"direction": "prev"}); code != http.StatusOK {
		t.Fatalf("rollback: status %d env=%+v", code, env)
	}
	waitCurrentVersion(t, t4CustomPolicyName, tl.CurrentVersion+1)

	// ---- propagation layer ----
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var pol v1alpha1.Policy
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: t4CustomPolicyName}, &pol); err == nil &&
			!strings.Contains(pol.Spec.Rego, "also-forbidden") {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// ---- enforcement layer ----
	alsoForbiddenPod := `apiVersion: v1
kind: Pod
metadata:
  name: e2e-rollback-test-pod
  namespace: test-builtin-rules
  labels:
    app: also-forbidden
spec:
  automountServiceAccountToken: false
  containers:
  - name: app
    image: nginx:1.25.0
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
`
	podFile := fmt.Sprintf("/tmp/e2e-rollback-%d.yaml", time.Now().UnixNano())
	if err := os.WriteFile(podFile, []byte(alsoForbiddenPod), 0o600); err != nil {
		t.Fatalf("write temp pod file: %v", err)
	}
	defer os.Remove(podFile)

	_, err := helpers.KubectlApply(ctx, podFile)
	if err != nil {
		helpers.KubectlDelete(ctx, podFile)
		t.Error("T4-6: after rollback, pod with also-forbidden was still blocked — rollback may not have taken effect")
	} else {
		helpers.KubectlDelete(ctx, podFile)
		t.Log("T4-6 PASS: rollback restored old Rego — also-forbidden pod now admitted")
	}
}
