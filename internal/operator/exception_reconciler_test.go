package operator_test

import (
	"context"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/operator"
)

// NOTE: keep using existing `buildScheme`, `validRego`, `newPolicy` declared in
// policy_reconciler_test.go — do NOT redeclare (per project CLAUDE.md).

func newException(name, duration, reason string) *v1alpha1.PolicyException {
	return &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Second)),
			UID:               "ex-uid",
		},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-privileged"},
			Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr-system"}},
			Duration:   duration,
			Reason:     reason,
		},
	}
}

func TestExceptionReconcilePendingToActive(t *testing.T) {
	pex := newException("ex1", "1h", "valid reason")
	policy := newPolicy("run-as-privileged", validRego)

	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(pex, policy).
		WithStatusSubresource(pex).
		Build()

	r := operator.NewExceptionReconciler(c)
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	if err := c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != v1alpha1.PhaseActive {
		t.Errorf("phase = %q want %q", got.Status.Phase, v1alpha1.PhaseActive)
	}
	if got.Status.ExpiresAt == nil {
		t.Fatal("ExpiresAt not set")
	}
	if got.Status.EffectiveAt == nil || !got.Status.EffectiveAt.Equal(&got.CreationTimestamp) {
		t.Errorf("EffectiveAt must equal CreationTimestamp")
	}
	if res.RequeueAfter <= 0 || res.RequeueAfter > time.Hour+time.Second {
		t.Errorf("RequeueAfter should approximate duration, got %v", res.RequeueAfter)
	}
}

func TestExceptionReconcileInvalidEmptyReason(t *testing.T) {
	pex := newException("ex1", "1h", "   ") // whitespace
	c := fake.NewClientBuilder().
		WithScheme(buildScheme()).
		WithObjects(pex).
		WithStatusSubresource(pex).
		Build()
	r := operator.NewExceptionReconciler(c)
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	_ = c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got)
	if got.Status.Phase != v1alpha1.PhaseInvalid {
		t.Errorf("phase=%q want Invalid", got.Status.Phase)
	}
	if !strings.Contains(got.Status.Message, "reason") {
		t.Errorf("Message should mention reason, got %q", got.Status.Message)
	}
}

func TestExceptionReconcileInvalidBadDuration(t *testing.T) {
	pex := newException("ex1", "abc", "ok")
	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(pex).WithStatusSubresource(pex).Build()
	if _, err := operator.NewExceptionReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	_ = c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got)
	if got.Status.Phase != v1alpha1.PhaseInvalid {
		t.Errorf("phase=%q want Invalid", got.Status.Phase)
	}
}

func TestExceptionReconcileOneOfViolation(t *testing.T) {
	pex := newException("ex1", "1h", "ok")
	pex.Spec.AllPolicies = true // both PolicyRefs and AllPolicies set
	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(pex).WithStatusSubresource(pex).Build()
	if _, err := operator.NewExceptionReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	_ = c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got)
	if got.Status.Phase != v1alpha1.PhaseInvalid {
		t.Errorf("phase=%q want Invalid", got.Status.Phase)
	}
}

func TestExceptionReconcileDanglingPolicyRef(t *testing.T) {
	pex := newException("ex1", "1h", "ok")
	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(pex).WithStatusSubresource(pex).Build()
	if _, err := operator.NewExceptionReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	_ = c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got)
	if got.Status.Phase != v1alpha1.PhaseInvalid {
		t.Errorf("phase=%q want Invalid for dangling ref", got.Status.Phase)
	}
}

func TestExceptionReconcileActiveToExpired(t *testing.T) {
	pex := newException("ex1", "1ns", "ok")
	policy := newPolicy("run-as-privileged", validRego)
	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(pex, policy).WithStatusSubresource(pex).Build()
	r := operator.NewExceptionReconciler(c)
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	_ = c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got)
	if got.Status.Phase != v1alpha1.PhaseExpired {
		t.Errorf("phase=%q want Expired", got.Status.Phase)
	}
}

func TestExceptionReconcileExpiredZeroRetainDeletes(t *testing.T) {
	pex := newException("ex1", "1ns", "ok")
	pex.Status.Phase = v1alpha1.PhaseExpired // already expired
	policy := newPolicy("run-as-privileged", validRego)
	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(pex, policy).WithStatusSubresource(pex).Build()

	if _, err := operator.NewExceptionReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	err := c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got)
	if err == nil {
		t.Error("expected object to be deleted (retainAfterExpiry=0)")
	}
}

func TestExceptionReconcileExpiredTerminalEvenIfDurationExtended(t *testing.T) {
	now := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	pex := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ex1",
			CreationTimestamp: now,
			UID:               "ex-uid",
		},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-privileged"},
			Duration:   "100h", // would put expiresAt far in future
			Reason:     "ok",
			Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr-system"}},
		},
		Status: v1alpha1.PolicyExceptionStatus{Phase: v1alpha1.PhaseExpired},
	}
	policy := newPolicy("run-as-privileged", validRego)
	c := fake.NewClientBuilder().WithScheme(buildScheme()).WithObjects(pex, policy).WithStatusSubresource(pex).Build()
	if _, err := operator.NewExceptionReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "ex1"}}); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("reconcile: %v", err)
	}
	var got v1alpha1.PolicyException
	if err := c.Get(context.Background(), types.NamespacedName{Name: "ex1"}, &got); err == nil {
		if got.Status.Phase != v1alpha1.PhaseExpired {
			t.Errorf("Expired must be terminal; got phase=%q", got.Status.Phase)
		}
	}
}

