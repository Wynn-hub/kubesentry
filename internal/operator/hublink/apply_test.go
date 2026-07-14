package hublink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
)

func newApplyClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func mObj(t *testing.T, kind, name string, spec any) *hubpb.ManagedObject {
	t.Helper()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return &hubpb.ManagedObject{
		Kind: kind, Name: name,
		Labels:   map[string]string{v1alpha1.ManagedByLabel: v1alpha1.ManagedByPlatform},
		SpecJson: raw,
	}
}

func policySpec(mode string) v1alpha1.PolicySpec {
	return v1alpha1.PolicySpec{EnforcementMode: mode, Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}"}
}

func TestApplyCreatesAndPrunes(t *testing.T) {
	orphan := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{
		Name:   "orphan",
		Labels: map[string]string{v1alpha1.ManagedByLabel: v1alpha1.ManagedByPlatform},
	}}
	c := newApplyClient(t, orphan)

	ds := &hubpb.DesiredState{VersionHash: "h1", Objects: []*hubpb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeAudit)),
		mObj(t, "PolicyGroup", "g1", v1alpha1.PolicyGroupSpec{
			Enabled:  true,
			Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1", EnforcementMode: v1alpha1.ModeAudit}}},
		}),
	}}
	if err := applyDesiredState(context.Background(), c, ds); err != nil {
		t.Fatal(err)
	}

	var p v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &p); err != nil {
		t.Fatal(err)
	}
	if p.Labels[v1alpha1.ManagedByLabel] != v1alpha1.ManagedByPlatform {
		t.Fatal("managed label missing")
	}
	var g v1alpha1.PolicyGroup
	if err := c.Get(context.Background(), types.NamespacedName{Name: "g1"}, &g); err != nil {
		t.Fatal(err)
	}
	var gone v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "orphan"}, &gone); err == nil {
		t.Fatal("orphan managed policy not pruned")
	}
}

func TestApplyEmptySnapshotCascades(t *testing.T) {
	managed := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{
		Name: "p1", Labels: map[string]string{v1alpha1.ManagedByLabel: v1alpha1.ManagedByPlatform},
	}}
	local := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "user-own"}}
	c := newApplyClient(t, managed, local)

	if err := applyDesiredState(context.Background(), c, &hubpb.DesiredState{VersionHash: "h"}); err != nil {
		t.Fatal(err)
	}
	var gone v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &gone); err == nil {
		t.Fatal("managed object survived empty snapshot")
	}
	var kept v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "user-own"}, &kept); err != nil {
		t.Fatal("unmanaged object must never be pruned")
	}
}

func TestApplyUnmanagedTwinIdenticalIsSatisfied(t *testing.T) {
	spec := policySpec(v1alpha1.ModeAudit)
	twin := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, Spec: spec}
	c := newApplyClient(t, twin)

	ds := &hubpb.DesiredState{VersionHash: "h", Objects: []*hubpb.ManagedObject{mObj(t, "Policy", "p1", spec)}}
	if err := applyDesiredState(context.Background(), c, ds); err != nil {
		t.Fatalf("identical twin must satisfy silently: %v", err)
	}
	var got v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &got); err != nil {
		t.Fatal(err)
	}
	if _, labeled := got.Labels[v1alpha1.ManagedByLabel]; labeled {
		t.Fatal("must not take ownership of an unmanaged twin")
	}
}

func TestApplyUnmanagedConflictReportsError(t *testing.T) {
	twin := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, Spec: policySpec(v1alpha1.ModeAudit)}
	c := newApplyClient(t, twin)

	ds := &hubpb.DesiredState{VersionHash: "h", Objects: []*hubpb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeEnforce)),
	}}
	err := applyDesiredState(context.Background(), c, ds)
	if err == nil || !strings.Contains(err.Error(), "p1") {
		t.Fatalf("conflict not reported: %v", err)
	}
	var got v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.EnforcementMode != v1alpha1.ModeAudit {
		t.Fatal("unmanaged object was overwritten")
	}
}

func TestApplyUpdatesManagedObjectInPlace(t *testing.T) {
	existing := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "p1",
			Labels: map[string]string{v1alpha1.ManagedByLabel: v1alpha1.ManagedByPlatform},
		},
		Spec: policySpec(v1alpha1.ModeAudit),
	}
	c := newApplyClient(t, existing)

	ds := &hubpb.DesiredState{VersionHash: "h", Objects: []*hubpb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeEnforce)),
	}}
	if err := applyDesiredState(context.Background(), c, ds); err != nil {
		t.Fatal(err)
	}
	var got v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.EnforcementMode != v1alpha1.ModeEnforce {
		t.Fatal("managed object not updated")
	}
}

func TestApplyUnknownKindReportsErrorAndSkipsOnlyThatObject(t *testing.T) {
	c := newApplyClient(t)

	ds := &hubpb.DesiredState{VersionHash: "h", Objects: []*hubpb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeAudit)),
		mObj(t, "Widget", "w1", policySpec(v1alpha1.ModeAudit)),
	}}
	err := applyDesiredState(context.Background(), c, ds)
	if err == nil || !strings.Contains(err.Error(), "w1") {
		t.Fatalf("unknown kind not reported: %v", err)
	}

	var p v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &p); err != nil {
		t.Fatalf("valid object must still be created despite unknown-kind sibling: %v", err)
	}
}

func TestPolicyCounts(t *testing.T) {
	ready := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "a"}, Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady}}
	invalid := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "b"}, Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseInvalid}}
	other := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
	c := newApplyClient(t, ready, invalid, other)
	r, i, err := policyCounts(context.Background(), c)
	if err != nil || r != 1 || i != 1 {
		t.Fatalf("r=%d i=%d err=%v", r, i, err)
	}
}
