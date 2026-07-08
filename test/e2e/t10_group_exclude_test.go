//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestT10_GroupExcludeOverridesBySelector reproduces the original bug report:
// a policy matched purely via bySelector must be excludable by name, and the
// exclusion must be reversible.
func TestT10_GroupExcludeOverridesBySelector(t *testing.T) {
	ctx := context.Background()

	pol := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-exclude-pol",
			Labels: map[string]string{"kubesentry.io/category": "security"},
		},
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: v1alpha1.ModeAudit,
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources: []v1alpha1.MatchResource{
					{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			},
			Rego: "package kubesentry\ndeny[m]{false;m:=\"x\"}",
		},
	}
	if err := k8sClient.Create(ctx, pol); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pol) })

	// Step 1: confirm bySelector auto-capture still works (regression guard).
	waitForMember(t, ctx, "security", "test-exclude-pol", true, 10*time.Second)

	// Step 2: add it to the group's exclude list.
	var pg v1alpha1.PolicyGroup
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "security"}, &pg); err != nil {
		t.Fatalf("get group: %v", err)
	}
	pg.Spec.Policies.Exclude = append(pg.Spec.Policies.Exclude, "test-exclude-pol")
	if err := k8sClient.Update(ctx, &pg); err != nil {
		t.Fatalf("update group exclude: %v", err)
	}
	waitForMember(t, ctx, "security", "test-exclude-pol", false, 10*time.Second)

	// Step 3: clear the exclude entry, confirm the member reappears.
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "security"}, &pg); err != nil {
		t.Fatalf("re-get group: %v", err)
	}
	pg.Spec.Policies.Exclude = nil
	if err := k8sClient.Update(ctx, &pg); err != nil {
		t.Fatalf("clear group exclude: %v", err)
	}
	waitForMember(t, ctx, "security", "test-exclude-pol", true, 10*time.Second)
}

// waitForMember polls the named PolicyGroup until the named member's presence
// in status.resolvedPolicies matches wantPresent, or fails after timeout.
func waitForMember(t *testing.T, ctx context.Context, group, member string, wantPresent bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var pg v1alpha1.PolicyGroup
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: group}, &pg); err == nil {
			present := false
			for _, m := range pg.Status.ResolvedPolicies {
				if m.Name == member {
					present = true
					break
				}
			}
			if present == wantPresent {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("member %q presence did not reach %v in group %q within %s", member, wantPresent, group, timeout)
}
