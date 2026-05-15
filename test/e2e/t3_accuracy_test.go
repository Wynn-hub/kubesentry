//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
)

type policyCase struct {
	key           string
	mode          string // "enforce" or "audit"
	failFixture   string // relative to ../builtin-rules/
	passFixture   string // relative to ../builtin-rules/
	clusterScoped bool
}

var t3Cases = []policyCase{
	// ---- Security (enforce) ----
	{"runAsPrivileged", "enforce", "security/run-as-privileged-fail.yaml", "security/run-as-privileged-pass.yaml", false},
	{"privilegeEscalationAllowed", "enforce", "security/privilege-escalation-fail.yaml", "security/privilege-escalation-pass.yaml", false},
	{"linuxHardening", "enforce", "security/linux-hardening-fail.yaml", "security/linux-hardening-pass.yaml", false},
	{"dangerousCapabilities", "enforce", "security/dangerous-capabilities-fail.yaml", "security/dangerous-capabilities-pass.yaml", false},
	{"hostPIDSet", "enforce", "security/host-pid-fail.yaml", "security/host-pid-pass.yaml", false},
	{"hostIPCSet", "enforce", "security/host-ipc-fail.yaml", "security/host-ipc-pass.yaml", false},
	{"sensitiveContainerEnvVar", "enforce", "security/sensitive-env-var-fail.yaml", "security/sensitive-env-var-pass.yaml", false},
	{"sensitiveConfigmapContent", "enforce", "security/sensitive-configmap-fail.yaml", "security/sensitive-configmap-pass.yaml", false},
	{"clusterrolePodExecAttach", "enforce", "security/clusterrole-exec-fail.yaml", "security/clusterrole-exec-pass.yaml", true},
	{"rolePodExecAttach", "enforce", "security/role-exec-fail.yaml", "security/role-exec-pass.yaml", false},
	{"clusterrolebindingPodExecAttach", "enforce", "security/clusterrolebinding-exec-fail.yaml", "security/clusterrolebinding-exec-pass.yaml", true},
	{"rolebindingRolePodExecAttach", "enforce", "security/rolebinding-role-exec-fail.yaml", "security/rolebinding-role-exec-pass.yaml", false},
	{"rolebindingClusterRolePodExecAttach", "enforce", "security/rolebinding-clusterrole-exec-fail.yaml", "security/rolebinding-clusterrole-exec-pass.yaml", false},
	{"clusterrolebindingClusterAdmin", "enforce", "security/clusterrolebinding-admin-fail.yaml", "security/clusterrolebinding-admin-pass.yaml", true},
	{"rolebindingClusterAdminClusterRole", "enforce", "security/rolebinding-clusteradmin-clusterrole-fail.yaml", "security/rolebinding-clusteradmin-clusterrole-pass.yaml", false},
	{"rolebindingClusterAdminRole", "enforce", "security/rolebinding-clusteradmin-role-fail.yaml", "security/rolebinding-clusteradmin-role-pass.yaml", false},
	// ---- Security (audit) ----
	{"runAsRootAllowed", "audit", "security/run-as-root-fail.yaml", "security/run-as-root-pass.yaml", false},
	{"notReadOnlyRootFilesystem", "audit", "security/not-readonly-filesystem-fail.yaml", "security/not-readonly-filesystem-pass.yaml", false},
	{"insecureCapabilities", "audit", "security/insecure-capabilities-fail.yaml", "security/insecure-capabilities-pass.yaml", false},
	{"hostNetworkSet", "audit", "security/host-network-fail.yaml", "security/host-network-pass.yaml", false},
	{"hostPortSet", "audit", "security/host-port-fail.yaml", "security/host-port-pass.yaml", false},
	{"automountServiceAccountToken", "audit", "security/automount-token-fail.yaml", "security/automount-token-pass.yaml", false},
	{"tlsSettingsMissing", "audit", "security/tls-settings-missing-fail.yaml", "security/tls-settings-missing-pass.yaml", false},
	// ---- Efficiency (audit) ----
	{"cpuRequestsMissing", "audit", "efficiency/cpu-requests-missing-fail.yaml", "efficiency/cpu-requests-missing-pass.yaml", false},
	{"memoryRequestsMissing", "audit", "efficiency/memory-requests-missing-fail.yaml", "efficiency/memory-requests-missing-pass.yaml", false},
	{"cpuLimitsMissing", "audit", "efficiency/cpu-limits-missing-fail.yaml", "efficiency/cpu-limits-missing-pass.yaml", false},
	{"memoryLimitsMissing", "audit", "efficiency/memory-limits-missing-fail.yaml", "efficiency/memory-limits-missing-pass.yaml", false},
	// ---- Reliability (enforce) ----
	{"tagNotSpecified", "enforce", "reliability/tag-not-specified-fail.yaml", "reliability/tag-not-specified-pass.yaml", false},
	// ---- Reliability (audit) ----
	{"readinessProbeMissing", "audit", "reliability/readiness-probe-missing-fail.yaml", "reliability/readiness-probe-missing-pass.yaml", false},
	{"livenessProbeMissing", "audit", "reliability/liveness-probe-missing-fail.yaml", "reliability/liveness-probe-missing-pass.yaml", false},
	{"pullPolicyNotAlways", "audit", "reliability/pull-policy-not-always-fail.yaml", "reliability/pull-policy-not-always-pass.yaml", false},
	{"priorityClassNotSet", "audit", "reliability/priority-class-not-set-fail.yaml", "reliability/priority-class-not-set-pass.yaml", false},
	{"deploymentMissingReplicas", "audit", "reliability/deployment-single-replica-fail.yaml", "reliability/deployment-single-replica-pass.yaml", false},
	{"metadataAndInstanceMismatched", "audit", "reliability/metadata-instance-mismatch-fail.yaml", "reliability/metadata-instance-mismatch-pass.yaml", false},
	{"topologySpreadConstraint", "audit", "reliability/topology-spread-missing-fail.yaml", "reliability/topology-spread-missing-pass.yaml", false},
	{"hpaMaxAvailability", "audit", "reliability/hpa-max-availability-fail.yaml", "reliability/hpa-max-availability-pass.yaml", false},
	{"hpaMinAvailability", "audit", "reliability/hpa-min-availability-fail.yaml", "reliability/hpa-min-availability-pass.yaml", false},
}

// fixtureBase is relative to the test working directory (test/e2e/).
const fixtureBase = "../builtin-rules"

func TestT3_AllPolicyAccuracy(t *testing.T) {
	ctx := context.Background()

	for _, tc := range t3Cases {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			failPath := fmt.Sprintf("%s/%s", fixtureBase, tc.failFixture)
			passPath := fmt.Sprintf("%s/%s", fixtureBase, tc.passFixture)

			if tc.mode == "enforce" {
				testEnforce(t, ctx, tc.key, failPath, passPath, tc.clusterScoped)
			} else {
				testAudit(t, ctx, tc.key, failPath, passPath, tc.clusterScoped)
			}
		})
	}
}

func testEnforce(t *testing.T, ctx context.Context, key, failPath, passPath string, clusterScoped bool) {
	t.Helper()
	apply := helpers.KubectlApply
	del := helpers.KubectlDelete
	if clusterScoped {
		apply = helpers.KubectlApplyClusterScoped
		del = helpers.KubectlDeleteClusterScoped
	}

	out, err := apply(ctx, failPath)
	if err == nil {
		del(ctx, failPath)
		t.Errorf("[%s] deny fixture was admitted, expected rejection", key)
	} else if !strings.Contains(out, key) {
		t.Logf("[%s] rejection output (key not in message): %s", key, out)
	}

	out, err = apply(ctx, passPath)
	if err != nil {
		t.Errorf("[%s] pass fixture was rejected: %v\nOutput: %s", key, err, out)
		return
	}
	del(ctx, passPath)
}

func testAudit(t *testing.T, ctx context.Context, key, failPath, passPath string, clusterScoped bool) {
	t.Helper()
	apply := helpers.KubectlApply
	del := helpers.KubectlDelete
	if clusterScoped {
		apply = helpers.KubectlApplyClusterScoped
		del = helpers.KubectlDeleteClusterScoped
	}

	before := time.Now()
	out, err := apply(ctx, failPath)
	if err != nil {
		t.Errorf("[%s] audit fail fixture was rejected (expected admission): %v\nOutput: %s", key, err, out)
		return
	}
	del(ctx, failPath)

	logCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := helpers.WaitForLogEntry(logCtx, k8sClientset, webhookPodName, key, before, 5*time.Second); err != nil {
		t.Errorf("[%s] expected webhook log entry containing %q within 5s, not found", key, key)
	}

	before = time.Now()
	out, err = apply(ctx, passPath)
	if err != nil {
		t.Errorf("[%s] pass fixture was rejected: %v\nOutput: %s", key, err, out)
		return
	}
	del(ctx, passPath)

	if err := helpers.AssertNoLogEntry(ctx, k8sClientset, webhookPodName, key, before, 2*time.Second); err != nil {
		t.Errorf("[%s] unexpected warning logged for pass fixture: %v", key, err)
	}
}
