package operator_test

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/operator"
)

func TestExceptionImmutableFieldsCheck(t *testing.T) {
	old := &v1alpha1.PolicyExceptionSpec{
		PolicyRefs: []string{"run-as-privileged"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"hr-system"}},
		Duration:   "1h",
		Reason:     "ok",
	}
	// Mutating mutable fields → allowed.
	mut := *old
	mut.Duration = "2h"
	mut.RetainAfterExpiry = "5m"
	mut.Reason = "extended"
	if err := operator.CheckExceptionSpecMutation(old, &mut); err != nil {
		t.Errorf("mutable-only change rejected: %v", err)
	}

	// Mutating an immutable field → rejected.
	bad := *old
	bad.Match.Namespaces = []string{"finance"}
	if err := operator.CheckExceptionSpecMutation(old, &bad); err == nil {
		t.Error("expected error when match changes")
	}

	// Switching one-of target → rejected.
	swap := *old
	swap.PolicyRefs = nil
	swap.AllPolicies = true
	if err := operator.CheckExceptionSpecMutation(old, &swap); err == nil {
		t.Error("expected error when one-of target changes")
	}
}

func TestExceptionExpiredFreeze(t *testing.T) {
	old := &v1alpha1.PolicyException{
		Spec:   v1alpha1.PolicyExceptionSpec{Duration: "1h", Reason: "ok"},
		Status: v1alpha1.PolicyExceptionStatus{Phase: v1alpha1.PhaseExpired},
	}
	upd := old.DeepCopy()
	upd.Spec.Duration = "2h"
	if err := operator.CheckExceptionExpiredFreeze(old, upd); err == nil {
		t.Error("expected error patching spec on Expired exception")
	}
}
