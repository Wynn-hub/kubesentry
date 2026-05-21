//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestT4_EnforceToAudit: switch a policy from enforce to audit.
// After switching, a previously-blocked pod should be admitted.
// In the new model Policy is first-class; the operator no longer owns
// Policy objects, so we patch the Policy directly instead of via the group.
func TestT4_EnforceToAudit(t *testing.T) {
	ctx := context.Background()
	policyName := "run-as-privileged"

	failPath := "../builtin-rules/security/run-as-privileged-fail.yaml"
	out, err := helpers.KubectlApply(ctx, failPath)
	if err == nil {
		helpers.KubectlDelete(ctx, failPath)
		t.Fatal("T4-1 setup: expected deny fixture to be blocked in enforce mode, but it was admitted")
	}
	t.Logf("T4-1: confirmed enforce mode blocks pod. Output: %s", out)

	var pol v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &pol); err != nil {
		t.Fatalf("get policy %s: %v", policyName, err)
	}
	patch := client.MergeFrom(pol.DeepCopy())
	pol.Spec.EnforcementMode = v1alpha1.ModeAudit
	if err := k8sClient.Patch(ctx, &pol, patch); err != nil {
		t.Fatalf("patch policy to audit: %v", err)
	}
	t.Cleanup(func() {
		var pol2 v1alpha1.Policy
		if k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &pol2) != nil {
			return
		}
		pt := client.MergeFrom(pol2.DeepCopy())
		pol2.Spec.EnforcementMode = v1alpha1.ModeEnforce
		k8sClient.Patch(ctx, &pol2, pt) //nolint:errcheck
	})

	deadline := time.Now().Add(30 * time.Second)
	passed := false
	for time.Now().Before(deadline) {
		var p v1alpha1.Policy
		if k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &p) == nil {
			t.Logf("T4-1: policy enforcement mode = %s", p.Spec.EnforcementMode)
		}
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
	ctx := context.Background()
	policyName := "run-as-privileged"

	var pol v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &pol); err != nil {
		t.Fatalf("get policy %s: %v", policyName, err)
	}

	patch := client.MergeFrom(pol.DeepCopy())
	pol.Spec.EnforcementMode = v1alpha1.ModeEnforce
	if err := k8sClient.Patch(ctx, &pol, patch); err != nil {
		t.Fatalf("patch policy to enforce: %v", err)
	}

	time.Sleep(3 * time.Second)

	failPath := "../builtin-rules/security/run-as-privileged-fail.yaml"
	_, err := helpers.KubectlApply(ctx, failPath)
	if err == nil {
		helpers.KubectlDelete(ctx, failPath)
		t.Error("T4-2: after switch to enforce, deny fixture was not blocked")
	} else {
		t.Log("T4-2 PASS: audit→enforce — pod is blocked again")
	}
}

// TestT4_DisablePolicy: switch a policy to audit to stop enforcement.
// In the new model there is no per-Policy enabled toggle in PolicyGroup;
// the closest equivalent is switching the Policy's own enforcementMode to audit.
// After switching, the previously-blocked resource should be admitted.
func TestT4_DisablePolicy(t *testing.T) {
	ctx := context.Background()
	policyName := "host-ipc-set"

	var pol v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &pol); err != nil {
		t.Fatalf("get policy %s: %v", policyName, err)
	}
	originalMode := pol.Spec.EnforcementMode

	patch := client.MergeFrom(pol.DeepCopy())
	pol.Spec.EnforcementMode = v1alpha1.ModeAudit
	if err := k8sClient.Patch(ctx, &pol, patch); err != nil {
		t.Fatalf("patch policy to audit: %v", err)
	}
	t.Cleanup(func() {
		var pol2 v1alpha1.Policy
		if k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &pol2) != nil {
			return
		}
		pt := client.MergeFrom(pol2.DeepCopy())
		pol2.Spec.EnforcementMode = originalMode
		k8sClient.Patch(ctx, &pol2, pt) //nolint:errcheck
	})

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

// TestT4_AddCustomPolicy: apply a new custom Policy CRD and create a PolicyGroup
// that references it, then verify it blocks matching pods.
// NOTE: does NOT clean up the policy or group — TestT4_RegoHotUpdate and
// TestT4_VersionRollback depend on them being present.
func TestT4_AddCustomPolicy(t *testing.T) {
	ctx := context.Background()
	policyFile := "fixtures/custom/test-policy.yaml"

	out, err := helpers.KubectlApplyClusterScoped(ctx, policyFile)
	if err != nil {
		t.Fatalf("T4-4: apply custom policy: %v\nOutput: %s", err, out)
	}

	if err := helpers.WaitForPolicyPhase(ctx, k8sClient, "e2e-forbidden-label", v1alpha1.PhaseReady, 30*time.Second); err != nil {
		t.Fatalf("T4-4: custom policy did not reach Ready: %v", err)
	}

	// Policy must belong to at least one enabled PolicyGroup to be evaluated.
	grp := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-custom-group"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "e2e-forbidden-label"}},
			},
		},
	}
	if err := k8sClient.Create(ctx, grp); err != nil {
		t.Fatalf("T4-4: create PolicyGroup: %v", err)
	}
	// NOTE: not cleaning up grp — subsequent tests depend on it.

	time.Sleep(5 * time.Second) // let operator sync + webhook cache update

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

	_, err = helpers.KubectlApply(ctx, podFile)
	if err == nil {
		helpers.KubectlDelete(ctx, podFile)
		t.Error("T4-4: pod with forbidden label was admitted, expected rejection")
	} else {
		t.Log("T4-4 PASS: custom policy active — forbidden pod blocked")
	}
}

// TestT4_RegoHotUpdate: update Policy Rego, verify new rule takes effect within 10s.
func TestT4_RegoHotUpdate(t *testing.T) {
	ctx := context.Background()

	var pol v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-forbidden-label"}, &pol); err != nil {
		t.Skip("T4-5: custom policy e2e-forbidden-label not found (run T4-4 first)")
	}

	countBefore, err := helpers.CountPolicyVersions(ctx, k8sClient, "e2e-forbidden-label")
	if err != nil {
		t.Fatalf("count policy versions before: %v", err)
	}

	out, err := helpers.KubectlApplyClusterScoped(ctx, "fixtures/custom/test-policy-updated.yaml")
	if err != nil {
		t.Fatalf("apply updated policy: %v\nOutput: %s", err, out)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		count, _ := helpers.CountPolicyVersions(ctx, k8sClient, "e2e-forbidden-label")
		if count > countBefore {
			break
		}
		time.Sleep(2 * time.Second)
	}

	countAfter, _ := helpers.CountPolicyVersions(ctx, k8sClient, "e2e-forbidden-label")
	if countAfter <= countBefore {
		t.Errorf("T4-5: expected PolicyVersion count to increase (was %d, still %d)", countBefore, countAfter)
	}

	time.Sleep(3 * time.Second) // let webhook cache sync the updated policy

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

	_, err = helpers.KubectlApply(ctx, podFile)
	if err == nil {
		helpers.KubectlDelete(ctx, podFile)
		t.Error("T4-5: pod with also-forbidden label was admitted after Rego update, expected rejection")
	} else {
		t.Log("T4-5 PASS: Rego hot-update active — new rule blocks updated label")
	}
}

// TestT4_VersionRollback: rollback a policy to a previous version, verify old rule is restored.
func TestT4_VersionRollback(t *testing.T) {
	ctx := context.Background()

	var pol v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-forbidden-label"}, &pol); err != nil {
		t.Skip("T4-6: custom policy not found (run T4-4 first)")
	}

	currentVersion := pol.Status.CurrentVersion
	if currentVersion <= 1 {
		t.Skip("T4-6: policy has only 1 version, cannot roll back")
	}
	rollbackTo := currentVersion - 1

	patch := client.MergeFrom(pol.DeepCopy())
	pol.Spec.RollbackTo = &v1alpha1.RollbackTo{Version: rollbackTo}
	if err := k8sClient.Patch(ctx, &pol, patch); err != nil {
		t.Fatalf("patch rollbackTo %d: %v", rollbackTo, err)
	}

	time.Sleep(5 * time.Second)

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
