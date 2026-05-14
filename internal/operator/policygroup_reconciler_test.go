package operator_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/operator"
)

func newPolicyGroup(name string, enabled bool, policies []v1alpha1.PolicyInGroup) *v1alpha1.PolicyGroup {
	return &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: "pg-uid"},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled:  enabled,
			Policies: policies,
		},
	}
}

func boolPtr(b bool) *bool { return &b }

func TestPolicyGroupReconcileCreatesPolicy(t *testing.T) {
	enabled := true
	pg := newPolicyGroup("security", true, []v1alpha1.PolicyInGroup{
		{Key: "runAsPrivileged", Enabled: &enabled},
	})
	s := buildScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pg).WithStatusSubresource(pg).Build()

	r := operator.NewPolicyGroupReconciler(c)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "security"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var pList v1alpha1.PolicyList
	if err := c.List(context.Background(), &pList); err != nil {
		t.Fatal(err)
	}
	if len(pList.Items) != 1 {
		t.Fatalf("expected 1 Policy, got %d", len(pList.Items))
	}
	p := pList.Items[0]
	if p.Name != "run-as-privileged" {
		t.Errorf("expected Policy name run-as-privileged, got %q", p.Name)
	}
	if p.Labels[v1alpha1.LabelKey] != "runAsPrivileged" {
		t.Errorf("expected label kubesentry.io/key=runAsPrivileged")
	}
	if p.Labels[v1alpha1.LabelGroup] != "security" {
		t.Errorf("expected label kubesentry.io/group=security")
	}
	if p.Spec.EnforcementMode != v1alpha1.ModeEnforce {
		t.Errorf("expected enforce mode")
	}
	if len(p.OwnerReferences) != 1 {
		t.Errorf("expected ownerRef")
	}
}

func TestPolicyGroupReconcileDisabledGroupDeletesPolicies(t *testing.T) {
	enabled := true
	pg := newPolicyGroup("security", false, []v1alpha1.PolicyInGroup{
		{Key: "runAsPrivileged", Enabled: &enabled},
	})
	// Pre-create a Policy owned by this group
	existingPolicy := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "run-as-privileged",
			Labels: map[string]string{
				v1alpha1.LabelKey:   "runAsPrivileged",
				v1alpha1.LabelGroup: "security",
			},
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "kubesentry.io/v1alpha1", Kind: "PolicyGroup", Name: "security", UID: "pg-uid"},
			},
		},
		Spec: v1alpha1.PolicySpec{EnforcementMode: v1alpha1.ModeEnforce, Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}"},
	}
	s := buildScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pg, existingPolicy).WithStatusSubresource(pg).Build()

	r := operator.NewPolicyGroupReconciler(c)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "security"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var pList v1alpha1.PolicyList
	c.List(context.Background(), &pList)
	if len(pList.Items) != 0 {
		t.Errorf("expected 0 Policies after group disabled, got %d", len(pList.Items))
	}
}

func TestPolicyGroupReconcileCustomOverrideSkipped(t *testing.T) {
	enabled := true
	pg := newPolicyGroup("security", true, []v1alpha1.PolicyInGroup{
		{Key: "runAsPrivileged", Enabled: &enabled},
	})
	// Custom Policy with same key, NO ownerRef to any PolicyGroup
	customPolicy := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "run-as-privileged",
			Labels: map[string]string{v1alpha1.LabelKey: "runAsPrivileged"},
			// No OwnerReferences — this is a user-created custom policy
		},
		Spec: v1alpha1.PolicySpec{EnforcementMode: v1alpha1.ModeAudit, Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}"},
	}
	s := buildScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pg, customPolicy).WithStatusSubresource(pg).Build()

	r := operator.NewPolicyGroupReconciler(c)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "security"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	// Custom policy should still be audit mode (not overwritten)
	var updated v1alpha1.Policy
	c.Get(context.Background(), types.NamespacedName{Name: "run-as-privileged"}, &updated)
	if updated.Spec.EnforcementMode != v1alpha1.ModeAudit {
		t.Errorf("expected custom policy to remain audit mode")
	}

	var pg2 v1alpha1.PolicyGroup
	c.Get(context.Background(), types.NamespacedName{Name: "security"}, &pg2)
	if pg2.Status.SkippedPolicies != 1 {
		t.Errorf("expected skippedPolicies=1, got %d", pg2.Status.SkippedPolicies)
	}
}

func TestPolicyGroupReconcileInvalidKeyWritesCondition(t *testing.T) {
	enabled := true
	pg := newPolicyGroup("custom-group", true, []v1alpha1.PolicyInGroup{
		{Key: "nonExistentKey", Enabled: &enabled}, // not in library, no rego provided
	})
	s := buildScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pg).WithStatusSubresource(pg).Build()

	r := operator.NewPolicyGroupReconciler(c)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "custom-group"},
	})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var pg2 v1alpha1.PolicyGroup
	c.Get(context.Background(), types.NamespacedName{Name: "custom-group"}, &pg2)
	found := false
	for _, cond := range pg2.Status.Conditions {
		if cond.Type == "InvalidPolicy" {
			found = true
		}
	}
	if !found {
		t.Error("expected InvalidPolicy condition")
	}
}

func TestCamelToKebab(t *testing.T) {
	cases := []struct{ in, want string }{
		{"runAsPrivileged", "run-as-privileged"},
		{"hostPIDSet", "host-p-i-d-set"},
		{"cpuRequestsMissing", "cpu-requests-missing"},
		{"hpaMaxAvailability", "hpa-max-availability"},
	}
	for _, tc := range cases {
		got := operator.CamelToKebab(tc.in)
		if got != tc.want {
			t.Errorf("CamelToKebab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
