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

// TestT6_BySelectorDynamicBinding asserts that creating a custom Policy
// labeled with kubesentry.io/category=security makes it appear in a
// bySelector-matching group's resolvedPolicies within 10s. Uses its own
// throwaway PolicyGroup rather than the built-in "default" group: the
// built-in catalogue's group membership has changed shape multiple times
// across this plan, so this test brings its own fixture instead of
// depending on catalogue content.
func TestT6_BySelectorDynamicBinding(t *testing.T) {
	ctx := context.Background()

	pol := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-byselector-pol",
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
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pol) })

	group := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-byselector-group"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				BySelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubesentry.io/category": "security"},
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, group); err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, group) })

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var pg v1alpha1.PolicyGroup
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-byselector-group"}, &pg); err == nil {
			for _, m := range pg.Status.ResolvedPolicies {
				if m.Name == "test-byselector-pol" {
					t.Log("T6 PASS: custom policy appeared in test-byselector-group.resolvedPolicies via bySelector")
					return
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("custom policy did not enter test-byselector-group.resolvedPolicies within 10s")
}
