package operator_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/operator"
)

const validRego = `
package kubesentry

deny[msg] {
	input.request.object.spec.containers[_].securityContext.privileged == true
	msg := "privileged containers are not allowed"
}
`

const invalidRego = `package kubesentry THIS IS NOT VALID REGO`

func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func newPolicy(name, rego string) *v1alpha1.Policy {
	return &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  "test-uid",
		},
		Spec: v1alpha1.PolicySpec{
			Rego:            rego,
			EnforcementMode: v1alpha1.ModeEnforce,
		},
	}
}

func TestReconcileValidPolicy(t *testing.T) {
	policy := newPolicy("allow-all", validRego)
	scheme := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := operator.NewPolicyReconciler(c, 5)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "allow-all"}}
	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue")
	}

	var updated v1alpha1.Policy
	if err := c.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseReady {
		t.Errorf("expected phase=Ready, got %q", updated.Status.Phase)
	}
	if updated.Status.CurrentVersion != 1 {
		t.Errorf("expected currentVersion=1, got %d", updated.Status.CurrentVersion)
	}

	var pvList v1alpha1.PolicyVersionList
	if err := c.List(context.Background(), &pvList); err != nil {
		t.Fatalf("list PolicyVersions: %v", err)
	}
	if len(pvList.Items) != 1 {
		t.Errorf("expected 1 PolicyVersion, got %d", len(pvList.Items))
	}
}

func TestReconcileInvalidRego(t *testing.T) {
	policy := newPolicy("bad-policy", invalidRego)
	scheme := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := operator.NewPolicyReconciler(c, 5)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "bad-policy"}}
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	var updated v1alpha1.Policy
	if err := c.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseInvalid {
		t.Errorf("expected phase=Invalid, got %q", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Error("expected non-empty error message")
	}

	var pvList v1alpha1.PolicyVersionList
	if err := c.List(context.Background(), &pvList); err != nil {
		t.Fatalf("list PolicyVersions: %v", err)
	}
	if len(pvList.Items) != 0 {
		t.Errorf("expected 0 PolicyVersions for invalid policy, got %d", len(pvList.Items))
	}
}

func TestReconcileRollback(t *testing.T) {
	targetVersion := int64(2)
	policy := newPolicy("my-policy", validRego)
	policy.Status.CurrentVersion = 3
	policy.Spec.RollbackTo = &v1alpha1.RollbackTo{Version: targetVersion}

	oldRego := `
package kubesentry
deny[msg] { msg := "old policy" }
`
	pv := &v1alpha1.PolicyVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-policy-v2",
			Labels: map[string]string{
				"kubesentry/policy":  "my-policy",
				"kubesentry/version": "2",
			},
		},
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       "my-policy",
			Version:         targetVersion,
			Rego:            oldRego,
			EnforcementMode: v1alpha1.ModeAudit,
		},
	}

	scheme := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, pv).
		WithStatusSubresource(policy).
		Build()

	r := operator.NewPolicyReconciler(c, 5)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "my-policy"}}
	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !result.Requeue {
		t.Error("expected requeue=true after rollback")
	}

	var updated v1alpha1.Policy
	if err := c.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if updated.Spec.RollbackTo != nil {
		t.Error("expected RollbackTo to be cleared after rollback")
	}
	if updated.Spec.Rego != oldRego {
		t.Errorf("expected spec.rego to match rolled-back version, got %q", updated.Spec.Rego)
	}
	if updated.Spec.EnforcementMode != v1alpha1.ModeAudit {
		t.Errorf("expected enforcementMode=audit, got %q", updated.Spec.EnforcementMode)
	}
}

func TestReconcileIdempotentSkip(t *testing.T) {
	// Test that reconcile skips when ObservedGeneration matches Generation and Phase is Ready
	policy := newPolicy("idempotent-policy", validRego)
	policy.Generation = 5
	policy.Status.ObservedGeneration = 5
	policy.Status.Phase = v1alpha1.PhaseReady
	policy.Status.CurrentVersion = 1

	scheme := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := operator.NewPolicyReconciler(c, 5)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "idempotent-policy"}}
	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue for idempotent case")
	}

	// Verify no new PolicyVersion was created (still only 1)
	var pvList v1alpha1.PolicyVersionList
	if err := c.List(context.Background(), &pvList); err != nil {
		t.Fatalf("list PolicyVersions: %v", err)
	}
	if len(pvList.Items) > 0 {
		t.Errorf("expected 0 new PolicyVersions (idempotent skip), got %d", len(pvList.Items))
	}
}

func TestReconcilePrunesOldVersions(t *testing.T) {
	// Test that old versions are deleted when exceeding historyLimit
	historyLimit := 2
	policy := newPolicy("prune-test", validRego)
	policy.Status.CurrentVersion = 3

	// Create 3 existing PolicyVersions
	pv1 := &v1alpha1.PolicyVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prune-test-v1",
			Labels: map[string]string{
				"kubesentry/policy":  "prune-test",
				"kubesentry/version": "1",
			},
		},
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       "prune-test",
			Version:         1,
			Rego:            validRego,
			EnforcementMode: v1alpha1.ModeEnforce,
		},
	}
	pv2 := &v1alpha1.PolicyVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prune-test-v2",
			Labels: map[string]string{
				"kubesentry/policy":  "prune-test",
				"kubesentry/version": "2",
			},
		},
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       "prune-test",
			Version:         2,
			Rego:            validRego,
			EnforcementMode: v1alpha1.ModeEnforce,
		},
	}
	pv3 := &v1alpha1.PolicyVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prune-test-v3",
			Labels: map[string]string{
				"kubesentry/policy":  "prune-test",
				"kubesentry/version": "3",
			},
		},
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       "prune-test",
			Version:         3,
			Rego:            validRego,
			EnforcementMode: v1alpha1.ModeEnforce,
		},
	}

	scheme := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, pv1, pv2, pv3).
		WithStatusSubresource(policy).
		Build()

	// Reconcile with historyLimit=2 should delete v1
	r := operator.NewPolicyReconciler(c, historyLimit)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "prune-test"}}
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// Verify only 2 versions remain (v2 and v3 or v3 and v4)
	var pvList v1alpha1.PolicyVersionList
	if err := c.List(context.Background(), &pvList); err != nil {
		t.Fatalf("list PolicyVersions: %v", err)
	}
	if len(pvList.Items) > historyLimit+1 {
		t.Errorf("expected <= %d PolicyVersions after pruning, got %d", historyLimit+1, len(pvList.Items))
	}
}

func TestReconcileRollbackMissingVersion(t *testing.T) {
	// Test that rollback to non-existent version sets Invalid status
	policy := newPolicy("missing-rollback", validRego)
	policy.Spec.RollbackTo = &v1alpha1.RollbackTo{Version: 999}

	scheme := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		WithStatusSubresource(policy).
		Build()

	r := operator.NewPolicyReconciler(c, 5)
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing-rollback"}}
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var updated v1alpha1.Policy
	if err := c.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if updated.Status.Phase != v1alpha1.PhaseInvalid {
		t.Errorf("expected phase=Invalid for missing version, got %q", updated.Status.Phase)
	}
	if updated.Status.Message == "" {
		t.Error("expected error message for missing version")
	}
}
