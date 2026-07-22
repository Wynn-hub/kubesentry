//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func waitForManagedDistributionObjects(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, false, func(ctx context.Context) (bool, error) {
		var policy v1alpha1.Policy
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-dist-policy"}, &policy); err != nil {
			return false, nil
		}
		if policy.Labels[v1alpha1.ManagedByLabel] != v1alpha1.ManagedByPlatform {
			return false, nil
		}

		var group v1alpha1.PolicyGroup
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-dist-group"}, &group); err != nil {
			return false, nil
		}
		return group.Labels[v1alpha1.ManagedByLabel] == v1alpha1.ManagedByPlatform, nil
	}); err != nil {
		t.Fatalf("distribution objects were not materialized on the member cluster: %v", err)
	}
}

func waitForDistributionObjectsPruned(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
		var policy v1alpha1.Policy
		policyErr := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-dist-policy"}, &policy)
		var group v1alpha1.PolicyGroup
		groupErr := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-dist-group"}, &group)
		return apierrors.IsNotFound(policyErr) && apierrors.IsNotFound(groupErr), nil
	}); err != nil {
		t.Fatalf("distribution objects were not pruned from the member cluster: %v", err)
	}
}

// TestT11DistributionPipeline exercises the database-backed distribution
// pipeline end to end. Gateway-side library objects are created through the
// console API; only the rendered Policy and PolicyGroup are materialized as
// managed CRs on the member cluster.
func TestT11DistributionPipeline(t *testing.T) {
	stop := startPortForward(t)
	t.Cleanup(stop)
	ctx := context.Background()
	fx := func(f string) string { return filepath.Join("fixtures", "distribution", f) }

	// Simulate a stale object from an earlier desired-state snapshot. The
	// next snapshot must prune it because it carries the platform-managed
	// label but is absent from the rendered object set.
	if _, err := helpers.KubectlApplyClusterScoped(ctx, fx("orphan.yaml")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { helpers.KubectlDeleteClusterScoped(ctx, fx("orphan.yaml")) })

	policyBody := map[string]any{
		"name":            "e2e-dist-policy",
		"enforcementMode": v1alpha1.ModeAudit,
		"description":     "E2E distribution pipeline test policy",
		"match": map[string]any{
			"operations": []string{"CREATE"},
			"resources": []map[string]any{
				{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
			},
		},
		"rego": "package kubesentry\ndeny[msg] {\n  input.request.object.metadata.labels[\"e2e-dist-deny\"] == \"true\"\n  msg := \"e2e distribution test denial\"\n}",
	}
	if code, env := consoleDo(t, http.MethodPost, "/api/v1/policies", policyBody); code != http.StatusOK {
		t.Fatalf("create policy via console-api: status %d env=%+v", code, env)
	}
	t.Cleanup(func() { consoleDo(t, http.MethodDelete, "/api/v1/policies/e2e-dist-policy?force=true", nil) })

	groupBody := map[string]any{
		"name":    "e2e-dist-group",
		"enabled": true,
		"policies": map[string]any{
			"byName": []map[string]any{{"name": "e2e-dist-policy", "enforcementMode": "audit"}},
		},
	}
	if code, env := consoleDo(t, http.MethodPost, "/api/v1/policygroups", groupBody); code != http.StatusOK {
		t.Fatalf("create group via console-api: status %d env=%+v", code, env)
	}
	t.Cleanup(func() { consoleDo(t, http.MethodDelete, "/api/v1/policygroups/e2e-dist-group", nil) })

	distBody := map[string]any{
		"name": "e2e-dist",
		"spec": map[string]any{
			"clusters":     map[string]any{"byName": []string{"local"}},
			"policyGroups": []string{"e2e-dist-group"},
		},
	}
	if code, env := consoleDo(t, http.MethodPost, "/api/v1/distributions", distBody); code != http.StatusOK {
		t.Fatalf("create distribution via console-api: status %d env=%+v", code, env)
	}
	distDeleted := false
	t.Cleanup(func() {
		if !distDeleted {
			consoleDo(t, http.MethodDelete, "/api/v1/distributions/e2e-dist", nil)
		}
	})

	// The gateway renders the DB rows and hublink materializes managed CRs.
	waitForManagedDistributionObjects(t, ctx)

	// The same desired-state apply prunes stale managed objects.
	if err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
		var p v1alpha1.Policy
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "e2e-orphan-managed"}, &p)
		return apierrors.IsNotFound(err), nil
	}); err != nil {
		t.Fatal("orphan managed policy was not pruned")
	}

	// The gateway-side library policy remains available through console-api.
	if code, env := consoleDo(t, http.MethodGet, "/api/v1/policies/e2e-dist-policy", nil); code != http.StatusOK {
		t.Fatalf("get library policy via console-api: status %d env=%+v", code, env)
	}

	// Confirm the target cluster is live and has heartbeated.
	if code, env := consoleDo(t, http.MethodGet, "/api/v1/clusters/local", nil); code != http.StatusOK {
		t.Fatalf("get cluster via console-api: status %d env=%+v", code, env)
	} else {
		var cluster struct {
			Phase           string  `json:"phase"`
			LastHeartbeatAt *string `json:"lastHeartbeatAt"`
		}
		if err := json.Unmarshal(env.Data, &cluster); err != nil {
			t.Fatalf("decode cluster: %v", err)
		}
		if cluster.Phase != "Online" || cluster.LastHeartbeatAt == nil {
			t.Fatalf("cluster is not live: %+v", cluster)
		}
	}

	// Removing the only distribution reference prunes its member-side
	// materializations, while the gateway library rows remain intact.
	if code, env := consoleDo(t, http.MethodDelete, "/api/v1/distributions/e2e-dist", nil); code != http.StatusOK {
		t.Fatalf("delete distribution via console-api: status %d env=%+v", code, env)
	}
	distDeleted = true
	waitForDistributionObjectsPruned(t, ctx)
	if code, env := consoleDo(t, http.MethodGet, "/api/v1/policies/e2e-dist-policy", nil); code != http.StatusOK {
		t.Fatalf("library policy did not survive distribution deletion: status %d env=%+v", code, env)
	}

	if code, _ := consoleDo(t, http.MethodDelete, "/api/v1/distributions/default", nil); code != http.StatusForbidden {
		t.Fatalf("delete default distribution: status %d, want 403", code)
	}
}
