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
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func newApplyClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func mObj(t *testing.T, kind, name string, spec any) *gatewaypb.ManagedObject {
	t.Helper()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return &gatewaypb.ManagedObject{
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

	ds := &gatewaypb.DesiredState{VersionHash: "h1", Objects: []*gatewaypb.ManagedObject{
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

	if err := applyDesiredState(context.Background(), c, &gatewaypb.DesiredState{VersionHash: "h"}); err != nil {
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

	ds := &gatewaypb.DesiredState{VersionHash: "h", Objects: []*gatewaypb.ManagedObject{mObj(t, "Policy", "p1", spec)}}
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

func TestApplyUnmanagedTwinGroupSameMembersDifferentOrderIsSatisfied(t *testing.T) {
	existingSpec := v1alpha1.PolicyGroupSpec{
		Enabled: true,
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{
			{Name: "c", EnforcementMode: v1alpha1.ModeAudit},
			{Name: "a", EnforcementMode: v1alpha1.ModeAudit},
			{Name: "b", EnforcementMode: v1alpha1.ModeEnforce},
		}},
	}
	twin := &v1alpha1.PolicyGroup{ObjectMeta: metav1.ObjectMeta{Name: "g1"}, Spec: existingSpec}
	c := newApplyClient(t, twin)

	// render.go always emits ByName alphabetically sorted — same members and
	// modes, different order than the chart-authored source object above.
	desiredSpec := v1alpha1.PolicyGroupSpec{
		Enabled: true,
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{
			{Name: "a", EnforcementMode: v1alpha1.ModeAudit},
			{Name: "b", EnforcementMode: v1alpha1.ModeEnforce},
			{Name: "c", EnforcementMode: v1alpha1.ModeAudit},
		}},
	}
	ds := &gatewaypb.DesiredState{VersionHash: "h", Objects: []*gatewaypb.ManagedObject{mObj(t, "PolicyGroup", "g1", desiredSpec)}}
	if err := applyDesiredState(context.Background(), c, ds); err != nil {
		t.Fatalf("same members/modes in a different order must satisfy silently: %v", err)
	}
	var got v1alpha1.PolicyGroup
	if err := c.Get(context.Background(), types.NamespacedName{Name: "g1"}, &got); err != nil {
		t.Fatal(err)
	}
	if _, labeled := got.Labels[v1alpha1.ManagedByLabel]; labeled {
		t.Fatal("must not take ownership of an unmanaged twin")
	}
}

func TestApplyUnmanagedTwinGroupDifferentModeStillConflicts(t *testing.T) {
	twin := &v1alpha1.PolicyGroup{ObjectMeta: metav1.ObjectMeta{Name: "g1"}, Spec: v1alpha1.PolicyGroupSpec{
		Enabled: true,
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{
			{Name: "a", EnforcementMode: v1alpha1.ModeAudit},
			{Name: "b", EnforcementMode: v1alpha1.ModeEnforce},
		}},
	}}
	c := newApplyClient(t, twin)

	// Different order AND a genuinely different mode for "b" — order
	// tolerance must not mask a real content difference.
	ds := &gatewaypb.DesiredState{VersionHash: "h", Objects: []*gatewaypb.ManagedObject{mObj(t, "PolicyGroup", "g1", v1alpha1.PolicyGroupSpec{
		Enabled: true,
		Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{
			{Name: "b", EnforcementMode: v1alpha1.ModeAudit},
			{Name: "a", EnforcementMode: v1alpha1.ModeAudit},
		}},
	})}}
	err := applyDesiredState(context.Background(), c, ds)
	if err == nil || !strings.Contains(err.Error(), "g1") {
		t.Fatalf("conflict not reported: %v", err)
	}
}

func TestApplyUnmanagedConflictReportsError(t *testing.T) {
	twin := &v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, Spec: policySpec(v1alpha1.ModeAudit)}
	c := newApplyClient(t, twin)

	ds := &gatewaypb.DesiredState{VersionHash: "h", Objects: []*gatewaypb.ManagedObject{
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

	ds := &gatewaypb.DesiredState{VersionHash: "h", Objects: []*gatewaypb.ManagedObject{
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

	ds := &gatewaypb.DesiredState{VersionHash: "h", Objects: []*gatewaypb.ManagedObject{
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

func TestApplyWritesLastTraceIDAnnotation(t *testing.T) {
	c := newApplyClient(t)

	ds := &gatewaypb.DesiredState{VersionHash: "h1", TraceId: "trace-abc", Objects: []*gatewaypb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeAudit)),
	}}
	if err := applyDesiredState(context.Background(), c, ds); err != nil {
		t.Fatal(err)
	}
	var p v1alpha1.Policy
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Annotations[v1alpha1.LastTraceIDAnnotation]; got != "trace-abc" {
		t.Fatalf("last-trace-id annotation = %q, want trace-abc", got)
	}

	// Simulated 60s resync fallback: same spec, but no trace_id this time —
	// must not clobber the previously recorded value.
	ds2 := &gatewaypb.DesiredState{VersionHash: "h1", TraceId: "", Objects: []*gatewaypb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeAudit)),
	}}
	if err := applyDesiredState(context.Background(), c, ds2); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Annotations[v1alpha1.LastTraceIDAnnotation]; got != "trace-abc" {
		t.Fatalf("empty-trace resync clobbered annotation: got %q, want trace-abc", got)
	}

	// A resync that also carries a genuine spec change but still an empty
	// trace_id must still preserve the previously recorded trace_id.
	ds3 := &gatewaypb.DesiredState{VersionHash: "h2", TraceId: "", Objects: []*gatewaypb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeEnforce)),
	}}
	if err := applyDesiredState(context.Background(), c, ds3); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &p); err != nil {
		t.Fatal(err)
	}
	if p.Spec.EnforcementMode != v1alpha1.ModeEnforce {
		t.Fatal("spec change from resync was not applied")
	}
	if got := p.Annotations[v1alpha1.LastTraceIDAnnotation]; got != "trace-abc" {
		t.Fatalf("spec-changing empty-trace resync clobbered annotation: got %q, want trace-abc", got)
	}

	// A later NotifyChange-triggered push with a new trace_id does overwrite.
	ds4 := &gatewaypb.DesiredState{VersionHash: "h3", TraceId: "trace-xyz", Objects: []*gatewaypb.ManagedObject{
		mObj(t, "Policy", "p1", policySpec(v1alpha1.ModeEnforce)),
	}}
	if err := applyDesiredState(context.Background(), c, ds4); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "p1"}, &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Annotations[v1alpha1.LastTraceIDAnnotation]; got != "trace-xyz" {
		t.Fatalf("last-trace-id annotation = %q, want trace-xyz", got)
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
