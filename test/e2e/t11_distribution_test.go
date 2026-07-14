//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func distState(ctx context.Context, name string) (v1alpha1.DistributionClusterStatus, bool) {
	var d v1alpha1.PolicyDistribution
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &d); err != nil {
		return v1alpha1.DistributionClusterStatus{}, false
	}
	for _, e := range d.Status.Clusters {
		if e.Name == "local" {
			return e, true
		}
	}
	return v1alpha1.DistributionClusterStatus{}, false
}

func waitDistState(t *testing.T, ctx context.Context, name, state string, timeout time.Duration) v1alpha1.DistributionClusterStatus {
	t.Helper()
	var last v1alpha1.DistributionClusterStatus
	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		e, ok := distState(ctx, name)
		last = e
		return ok && e.State == state, nil
	}); err != nil {
		t.Fatalf("distribution %s never reached %s (last: %+v): %v", name, state, last, err)
	}
	return last
}

func TestT11DistributionPipeline(t *testing.T) {
	ctx := context.Background()
	fx := func(f string) string { return filepath.Join("fixtures", "distribution", f) }

	if err := helpers.RegisterLocalCluster(ctx, k8sClient); err != nil {
		t.Fatalf("self-registration failed: %v", err)
	}
	// Registration restarted the operator; make sure the policy plane has
	// recovered before continuing.
	if err := helpers.WaitForAllPoliciesReady(ctx, k8sClient, 2*time.Minute); err != nil {
		t.Fatalf("policies not ready after re-registration: %v", err)
	}

	// Orphaned managed object pre-exists the distribution → the first
	// snapshot must prune it.
	if _, err := helpers.KubectlApplyClusterScoped(ctx, fx("orphan.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := helpers.KubectlApplyClusterScoped(ctx, fx("policy.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := helpers.KubectlApplyClusterScoped(ctx, fx("group.yaml")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		helpers.KubectlDeleteClusterScoped(ctx, fx("distribution.yaml"))
		helpers.KubectlDeleteClusterScoped(ctx, fx("group.yaml"))
		helpers.KubectlDeleteClusterScoped(ctx, fx("policy.yaml"))
		helpers.KubectlDeleteClusterScoped(ctx, fx("orphan.yaml"))
	})
	if err := helpers.WaitForPolicyPhase(ctx, k8sClient, "e2e-dist-policy", v1alpha1.PhaseReady, time.Minute); err != nil {
		t.Fatal(err)
	}

	if _, err := helpers.KubectlApplyClusterScoped(ctx, fx("distribution.yaml")); err != nil {
		t.Fatal(err)
	}

	// 1) Synced + appliedVersion flows back.
	entry := waitDistState(t, ctx, "e2e-dist", v1alpha1.DistributionSynced, 3*time.Minute)
	if entry.AppliedVersion == "" || entry.LastSyncTime == nil {
		t.Fatalf("synced entry incomplete: %+v", entry)
	}

	// 2) Orphan gets cascade-pruned.
	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
		var p v1alpha1.Policy
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-orphan-managed"}, &p)
		return apierrors.IsNotFound(err), nil
	}); err != nil {
		t.Fatal("orphan managed policy was not pruned")
	}

	// 3) Library object is not labeled managed (identical-twin never takes ownership).
	var lib v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-dist-policy"}, &lib); err != nil {
		t.Fatal(err)
	}
	if _, labeled := lib.Labels[v1alpha1.ManagedByLabel]; labeled {
		t.Fatal("library policy must stay unmanaged in self-registration")
	}

	// 4) Cluster.status.policySummary flows back.
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
		var cl v1alpha1.Cluster
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "local"}, &cl); err != nil {
			return false, nil
		}
		return cl.Status.PolicySummary != nil && cl.Status.PolicySummary.Ready > 0, nil
	}); err != nil {
		t.Fatal("policySummary never flowed back to Cluster.status")
	}

	// 5) Override makes the rendered group diverge from the library group →
	// unmanaged conflict → Error flows back.
	if _, err := helpers.RunKubectl(ctx, "patch", "policydistribution", "e2e-dist", "--type", "merge",
		"-p", `{"spec":{"overrides":[{"cluster":"local","enforcementMode":"enforce"}]}}`); err != nil {
		t.Fatal(err)
	}
	errEntry := waitDistState(t, ctx, "e2e-dist", v1alpha1.DistributionError, 3*time.Minute)
	if errEntry.Message == "" {
		t.Fatalf("error entry has no message: %+v", errEntry)
	}

	// 6) Removing the override restores Synced.
	if _, err := helpers.RunKubectl(ctx, "patch", "policydistribution", "e2e-dist", "--type", "merge",
		"-p", `{"spec":{"overrides":null}}`); err != nil {
		t.Fatal(err)
	}
	waitDistState(t, ctx, "e2e-dist", v1alpha1.DistributionSynced, 3*time.Minute)

	// 7) Deleting the distribution leaves the library object untouched.
	helpers.KubectlDeleteClusterScoped(ctx, fx("distribution.yaml"))
	time.Sleep(10 * time.Second) // give push/cleanup a cycle
	var still v1alpha1.Policy
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-dist-policy"}, &still); err != nil {
		t.Fatal("library policy must survive distribution deletion")
	}
}
