//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestT8_MultiGroupStrictestMode: same Policy referenced by group-A (enforce)
// and group-B (audit), both matching the request namespace. Request must be
// denied (strictest wins).
func TestT8_MultiGroupStrictestMode(t *testing.T) {
	ctx := context.Background()

	// Custom policy that denies configmaps with a specific label.
	pol := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "always-deny-test"},
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: v1alpha1.ModeAudit, // default audit
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources: []v1alpha1.MatchResource{
					{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"configmaps"}},
				},
			},
			Rego: `package kubesentry
deny[msg] {
  input.request.object.metadata.labels["test-strict-deny"] == "yes"
  msg := "denied by test policy"
}`,
		},
	}
	if err := k8sClient.Create(ctx, pol); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, pol) })

	// Group A overrides to enforce.
	gA := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-strict-a"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "always-deny-test", EnforcementMode: v1alpha1.ModeEnforce}},
			},
		},
	}
	// Group B inherits Policy default (audit).
	gB := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-strict-b"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "always-deny-test"}},
			},
		},
	}
	if err := k8sClient.Create(ctx, gA); err != nil {
		t.Fatalf("create gA: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, gA) })
	if err := k8sClient.Create(ctx, gB); err != nil {
		t.Fatalf("create gB: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, gB) })

	// Wait for both groups to reflect the resolved member.
	time.Sleep(5 * time.Second)

	// Issue a CREATE that should be denied (strictest = enforce).
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trigger-deny",
			Namespace: "default",
			Labels:    map[string]string{"test-strict-deny": "yes"},
		},
	}
	err := k8sClient.Create(ctx, cm)
	if err == nil {
		_ = k8sClient.Delete(ctx, cm)
		t.Fatalf("T8: expected denial when enforce group present, request was allowed")
	}
	if !apierrors.IsForbidden(err) {
		t.Fatalf("T8: expected 403 Forbidden, got %v", err)
	}
	t.Log("T8 step 1 PASS: enforce group causes denial (strictest mode wins)")

	// Delete gA → only audit group remains → same request should be allowed.
	if err := k8sClient.Delete(ctx, gA); err != nil {
		t.Fatalf("delete gA: %v", err)
	}
	time.Sleep(5 * time.Second)

	cm.ResourceVersion = ""
	if err := k8sClient.Create(ctx, cm); err != nil {
		t.Fatalf("T8: with only audit group, expected allow but got %v", err)
	}
	_ = k8sClient.Delete(ctx, cm)
	t.Log("T8 PASS: after removing enforce group, audit-only request is admitted")
}
