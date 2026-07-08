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

func newGroup(name string, enabled bool, byName []v1alpha1.PolicyRef, bySelector *metav1.LabelSelector) *v1alpha1.PolicyGroup {
	return &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name)},
		Spec: v1alpha1.PolicyGroupSpec{
			Enabled: enabled,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName:     byName,
				BySelector: bySelector,
			},
		},
	}
}

func newLabeledPolicy(name, mode string, lbls map[string]string) *v1alpha1.Policy {
	return &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls},
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: mode,
			Rego:            "package kubesentry\ndeny[m]{false;m:=\"x\"}",
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources: []v1alpha1.MatchResource{
					{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			},
		},
	}
}

func newGroupWithExclude(name string, byName []v1alpha1.PolicyRef, bySelector *metav1.LabelSelector, exclude []string) *v1alpha1.PolicyGroup {
	g := newGroup(name, true, byName, bySelector)
	g.Spec.Policies.Exclude = exclude
	return g
}

func TestPolicyGroupReconcileByNameResolvesMember(t *testing.T) {
	pg := newGroup("security", true,
		[]v1alpha1.PolicyRef{{Name: "run-as-privileged"}},
		nil,
	)
	pol := newLabeledPolicy("run-as-privileged", v1alpha1.ModeEnforce, nil)

	s := buildScheme()
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	r := operator.NewPolicyGroupReconciler(c)
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if got.Status.Phase != v1alpha1.PhaseReady {
		t.Errorf("phase = %q, want Ready", got.Status.Phase)
	}
	if len(got.Status.ResolvedPolicies) != 1 {
		t.Fatalf("resolvedPolicies len = %d, want 1", len(got.Status.ResolvedPolicies))
	}
	m := got.Status.ResolvedPolicies[0]
	if m.Name != "run-as-privileged" || m.EnforcementMode != v1alpha1.ModeEnforce || m.Source != v1alpha1.SourceByName {
		t.Errorf("member = %+v", m)
	}

	var gotPol v1alpha1.Policy
	_ = c.Get(context.Background(), types.NamespacedName{Name: "run-as-privileged"}, &gotPol)
	if len(gotPol.Status.ReferencedBy) != 1 || gotPol.Status.ReferencedBy[0] != "security" {
		t.Errorf("referencedBy = %v, want [security]", gotPol.Status.ReferencedBy)
	}
}

func TestPolicyGroupReconcileByNameModeOverride(t *testing.T) {
	pg := newGroup("security", true,
		[]v1alpha1.PolicyRef{{Name: "host-network-set", EnforcementMode: v1alpha1.ModeEnforce}},
		nil,
	)
	pol := newLabeledPolicy("host-network-set", v1alpha1.ModeAudit, nil)

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if got.Status.ResolvedPolicies[0].EnforcementMode != v1alpha1.ModeEnforce {
		t.Errorf("override failed: got mode %q", got.Status.ResolvedPolicies[0].EnforcementMode)
	}
}

func TestPolicyGroupReconcileBySelectorMatchesPolicies(t *testing.T) {
	pg := newGroup("security", true, nil,
		&metav1.LabelSelector{MatchLabels: map[string]string{v1alpha1.LabelCategory: "security"}},
	)
	polA := newLabeledPolicy("a", v1alpha1.ModeEnforce, map[string]string{v1alpha1.LabelCategory: "security"})
	polB := newLabeledPolicy("b", v1alpha1.ModeAudit, map[string]string{v1alpha1.LabelCategory: "other"})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, polA, polB).
		WithStatusSubresource(pg, polA, polB).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 1 {
		t.Fatalf("resolvedPolicies len = %d, want 1", len(got.Status.ResolvedPolicies))
	}
	if got.Status.ResolvedPolicies[0].Name != "a" || got.Status.ResolvedPolicies[0].Source != v1alpha1.SourceBySelector {
		t.Errorf("member = %+v", got.Status.ResolvedPolicies[0])
	}
}

func TestPolicyGroupReconcileByNamePrecedenceOverBySelector(t *testing.T) {
	pg := newGroup("security", true,
		[]v1alpha1.PolicyRef{{Name: "p", EnforcementMode: v1alpha1.ModeEnforce}},
		&metav1.LabelSelector{MatchLabels: map[string]string{v1alpha1.LabelCategory: "security"}},
	)
	pol := newLabeledPolicy("p", v1alpha1.ModeAudit, map[string]string{v1alpha1.LabelCategory: "security"})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 1 {
		t.Fatalf("want exactly one member, got %d", len(got.Status.ResolvedPolicies))
	}
	m := got.Status.ResolvedPolicies[0]
	if m.Source != v1alpha1.SourceByName || m.EnforcementMode != v1alpha1.ModeEnforce {
		t.Errorf("byName precedence failed: %+v", m)
	}
}

func TestPolicyGroupReconcileMissingByNameAddsCondition(t *testing.T) {
	pg := newGroup("security", true,
		[]v1alpha1.PolicyRef{{Name: "absent"}},
		nil,
	)
	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg).
		WithStatusSubresource(pg).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if got.Status.Phase != v1alpha1.PhaseDegraded {
		t.Errorf("phase = %q, want Degraded", got.Status.Phase)
	}
	if len(got.Status.Conditions) != 1 || got.Status.Conditions[0].Type != "MissingMember" {
		t.Errorf("conditions = %+v", got.Status.Conditions)
	}
}

func TestPolicyGroupReconcileDisabledClearsResolvedAndReferencedBy(t *testing.T) {
	pg := newGroup("security", false,
		[]v1alpha1.PolicyRef{{Name: "p"}},
		nil,
	)
	pg.Status.ResolvedPolicies = []v1alpha1.EffectiveMember{
		{Name: "p", EnforcementMode: v1alpha1.ModeEnforce, Source: v1alpha1.SourceByName},
	}
	pol := newLabeledPolicy("p", v1alpha1.ModeEnforce, nil)
	pol.Status.ReferencedBy = []string{"security"}

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if got.Status.Phase != v1alpha1.PhaseDisabled {
		t.Errorf("phase = %q, want Disabled", got.Status.Phase)
	}
	if got.Status.ResolvedPolicies != nil {
		t.Errorf("resolvedPolicies = %v, want nil", got.Status.ResolvedPolicies)
	}

	var gotPol v1alpha1.Policy
	_ = c.Get(context.Background(), types.NamespacedName{Name: "p"}, &gotPol)
	if len(gotPol.Status.ReferencedBy) != 0 {
		t.Errorf("referencedBy = %v, want empty", gotPol.Status.ReferencedBy)
	}
}

func TestPolicyGroupReconcileSelectorEnforcementModeOverride(t *testing.T) {
	pg := newGroup("security", true, nil,
		&metav1.LabelSelector{MatchLabels: map[string]string{v1alpha1.LabelCategory: "security"}},
	)
	pg.Spec.SelectorEnforcementMode = v1alpha1.ModeEnforce
	pol := newLabeledPolicy("p", v1alpha1.ModeAudit, map[string]string{v1alpha1.LabelCategory: "security"})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if got.Status.ResolvedPolicies[0].EnforcementMode != v1alpha1.ModeEnforce {
		t.Errorf("selectorEnforcementMode override failed")
	}
}

func TestPolicyGroupReconcileExcludeRemovesByNameMember(t *testing.T) {
	pg := newGroupWithExclude("security",
		[]v1alpha1.PolicyRef{{Name: "p", EnforcementMode: v1alpha1.ModeEnforce}},
		nil, []string{"p"},
	)
	pol := newLabeledPolicy("p", v1alpha1.ModeAudit, nil)

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 0 {
		t.Fatalf("resolvedPolicies = %+v, want empty (excluded)", got.Status.ResolvedPolicies)
	}
	if got.Status.ResolvedCount != 0 {
		t.Fatalf("resolvedCount = %d, want 0", got.Status.ResolvedCount)
	}
}

func TestPolicyGroupReconcileExcludeRemovesBySelectorMember(t *testing.T) {
	// Reproduces the original bug report: host-ipc-set-equivalent policy
	// matched only via bySelector, removed from byName, must be excludable.
	pg := newGroupWithExclude("security", nil,
		&metav1.LabelSelector{MatchLabels: map[string]string{v1alpha1.LabelCategory: "security"}},
		[]string{"host-ipc-set"},
	)
	pol := newLabeledPolicy("host-ipc-set", v1alpha1.ModeEnforce, map[string]string{v1alpha1.LabelCategory: "security"})
	other := newLabeledPolicy("other-pol", v1alpha1.ModeAudit, map[string]string{v1alpha1.LabelCategory: "security"})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol, other).
		WithStatusSubresource(pg, pol, other).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 1 || got.Status.ResolvedPolicies[0].Name != "other-pol" {
		t.Fatalf("resolvedPolicies = %+v, want only other-pol (host-ipc-set excluded)", got.Status.ResolvedPolicies)
	}
	if got.Status.ResolvedCount != 1 {
		t.Fatalf("resolvedCount = %d, want 1", got.Status.ResolvedCount)
	}
}

func TestPolicyGroupReconcileExcludeWinsOverByNameModeOverride(t *testing.T) {
	pg := newGroupWithExclude("security",
		[]v1alpha1.PolicyRef{{Name: "p", EnforcementMode: v1alpha1.ModeEnforce}},
		&metav1.LabelSelector{MatchLabels: map[string]string{v1alpha1.LabelCategory: "security"}},
		[]string{"p"},
	)
	pol := newLabeledPolicy("p", v1alpha1.ModeAudit, map[string]string{v1alpha1.LabelCategory: "security"})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 0 {
		t.Fatalf("resolvedPolicies = %+v, want empty (exclude wins over byName+bySelector)", got.Status.ResolvedPolicies)
	}
}

func TestPolicyGroupReconcileExcludeUnknownNameIgnored(t *testing.T) {
	pg := newGroupWithExclude("security",
		[]v1alpha1.PolicyRef{{Name: "p", EnforcementMode: v1alpha1.ModeEnforce}},
		nil, []string{"does-not-exist"},
	)
	pol := newLabeledPolicy("p", v1alpha1.ModeAudit, nil)

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()

	if _, err := operator.NewPolicyGroupReconciler(c).Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.PolicyGroup
	_ = c.Get(context.Background(), types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 1 || got.Status.ResolvedPolicies[0].Name != "p" {
		t.Fatalf("resolvedPolicies = %+v, want [p] (unknown exclude entry ignored)", got.Status.ResolvedPolicies)
	}
}

func TestPolicyGroupReconcileExcludeClearedRestoresMember(t *testing.T) {
	pg := newGroupWithExclude("security", nil,
		&metav1.LabelSelector{MatchLabels: map[string]string{v1alpha1.LabelCategory: "security"}},
		[]string{"p"},
	)
	pol := newLabeledPolicy("p", v1alpha1.ModeEnforce, map[string]string{v1alpha1.LabelCategory: "security"})

	c := fake.NewClientBuilder().WithScheme(buildScheme()).
		WithObjects(pg, pol).
		WithStatusSubresource(pg, pol).
		Build()
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "security"}}
	r := operator.NewPolicyGroupReconciler(c)

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	var got v1alpha1.PolicyGroup
	_ = c.Get(ctx, types.NamespacedName{Name: "security"}, &got)
	if len(got.Status.ResolvedPolicies) != 0 {
		t.Fatalf("after exclude, resolvedPolicies = %+v, want empty", got.Status.ResolvedPolicies)
	}

	// Clear the exclude list and reconcile again.
	got.Spec.Policies.Exclude = nil
	if err := c.Update(ctx, &got); err != nil {
		t.Fatalf("clear exclude: %v", err)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	var after v1alpha1.PolicyGroup
	_ = c.Get(ctx, types.NamespacedName{Name: "security"}, &after)
	if len(after.Status.ResolvedPolicies) != 1 || after.Status.ResolvedPolicies[0].Name != "p" {
		t.Fatalf("after clearing exclude, resolvedPolicies = %+v, want [p]", after.Status.ResolvedPolicies)
	}
}
