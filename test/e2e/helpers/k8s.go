package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const TestNamespace = "test-builtin-rules"

// KubectlApply applies a namespaced YAML file. Returns combined stdout+stderr.
func KubectlApply(ctx context.Context, path string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", path, "-n", TestNamespace)
	cmd.Stdout = &out
	cmd.Stderr = &out
	return out.String(), cmd.Run()
}

// KubectlDelete deletes namespaced resources from a YAML file (best-effort).
func KubectlDelete(ctx context.Context, path string) {
	_ = exec.CommandContext(ctx, "kubectl", "delete", "-f", path, "-n", TestNamespace, "--ignore-not-found").Run()
}

// KubectlApplyClusterScoped applies a cluster-scoped YAML file (no namespace flag).
func KubectlApplyClusterScoped(ctx context.Context, path string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", path)
	cmd.Stdout = &out
	cmd.Stderr = &out
	return out.String(), cmd.Run()
}

// KubectlDeleteClusterScoped deletes cluster-scoped resources (best-effort).
func KubectlDeleteClusterScoped(ctx context.Context, path string) {
	_ = exec.CommandContext(ctx, "kubectl", "delete", "-f", path, "--ignore-not-found").Run()
}

// RunKubectl runs an arbitrary kubectl command and returns combined output.
func RunKubectl(ctx context.Context, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	return out.String(), cmd.Run()
}

// WaitForAllPoliciesReady polls until every Policy object is Phase=Ready.
func WaitForAllPoliciesReady(ctx context.Context, c client.Client, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		var list v1alpha1.PolicyList
		if err := c.List(ctx, &list); err != nil {
			return false, err
		}
		if len(list.Items) == 0 {
			return false, nil
		}
		for _, p := range list.Items {
			if p.Status.Phase != v1alpha1.PhaseReady {
				return false, nil
			}
		}
		return true, nil
	})
}

// WaitForPolicyPhase polls until the named Policy reaches the given phase.
func WaitForPolicyPhase(ctx context.Context, c client.Client, name, phase string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		var p v1alpha1.Policy
		if err := c.Get(ctx, types.NamespacedName{Name: name}, &p); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return p.Status.Phase == phase, nil
	})
}

// GetPolicy retrieves a Policy by name.
func GetPolicy(ctx context.Context, c client.Client, name string) (*v1alpha1.Policy, error) {
	var p v1alpha1.Policy
	if err := c.Get(ctx, types.NamespacedName{Name: name}, &p); err != nil {
		return nil, fmt.Errorf("get policy %s: %w", name, err)
	}
	return &p, nil
}

// CountPolicyVersions returns the number of PolicyVersion objects for a policy.
func CountPolicyVersions(ctx context.Context, c client.Client, policyName string) (int, error) {
	var list v1alpha1.PolicyVersionList
	if err := c.List(ctx, &list, client.MatchingLabels{"kubesentry/policy": policyName}); err != nil {
		return 0, fmt.Errorf("list policy versions for %s: %w", policyName, err)
	}
	return len(list.Items), nil
}
