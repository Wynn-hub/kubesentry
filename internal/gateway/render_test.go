package gateway

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func rPolicy(name, mode string) v1alpha1.Policy {
	return v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"k": "v"}},
		Spec:       v1alpha1.PolicySpec{EnforcementMode: mode, Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}"},
	}
}

func rGroup(name string, refs ...v1alpha1.PolicyRef) v1alpha1.PolicyGroup {
	return v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1alpha1.PolicyGroupSpec{Enabled: true, Policies: v1alpha1.PolicyGroupPolicies{ByName: refs}},
	}
}

func rDist(name string, clusters []string, groups []string, overrides ...v1alpha1.DistributionOverride) v1alpha1.PolicyDistribution {
	return v1alpha1.PolicyDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ByName: clusters},
			PolicyGroups: groups,
			Overrides:    overrides,
		},
	}
}

func groupSpecOf(t *testing.T, res RenderResult, name string) v1alpha1.PolicyGroupSpec {
	t.Helper()
	for _, o := range res.Objects {
		if o.Kind == "PolicyGroup" && o.Name == name {
			var spec v1alpha1.PolicyGroupSpec
			if err := json.Unmarshal(o.Spec, &spec); err != nil {
				t.Fatal(err)
			}
			return spec
		}
	}
	t.Fatalf("group %s not in snapshot", name)
	return v1alpha1.PolicyGroupSpec{}
}

func TestRenderBasicSnapshot(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{rDist("d1", []string{"c1"}, []string{"g1"})},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeAudit)})

	if len(res.Errors) != 0 || len(res.Matched) != 1 || res.Matched[0] != "d1" {
		t.Fatalf("matched=%v errors=%v", res.Matched, res.Errors)
	}
	if len(res.Objects) != 2 || res.Objects[0].Kind != "Policy" || res.Objects[1].Kind != "PolicyGroup" {
		t.Fatalf("objects = %+v", res.Objects)
	}
	for _, o := range res.Objects {
		if o.Labels[v1alpha1.ManagedByLabel] != v1alpha1.ManagedByPlatform {
			t.Fatalf("missing managed-by label on %s", o.Name)
		}
	}
	spec := groupSpecOf(t, res, "g1")
	if len(spec.Policies.ByName) != 1 || spec.Policies.ByName[0].EnforcementMode != v1alpha1.ModeAudit {
		t.Fatalf("byName not materialized with explicit mode: %+v", spec.Policies)
	}
	if spec.Policies.BySelector != nil || spec.Policies.Exclude != nil || spec.SelectorEnforcementMode != "" {
		t.Fatal("snapshot group must be self-contained byName only")
	}
	if res.Hash == "" {
		t.Fatal("hash empty")
	}
}

func TestRenderSelectorAndOverride(t *testing.T) {
	cl := &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1"},
		Spec:       v1alpha1.ClusterSpec{Labels: map[string]string{"env": "prod"}},
	}
	d := rDist("d1", nil, []string{"g1"}, v1alpha1.DistributionOverride{Cluster: "c1", EnforcementMode: v1alpha1.ModeAudit})
	d.Spec.Clusters.ClusterSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{d},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1", EnforcementMode: v1alpha1.ModeEnforce})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeEnforce)})
	if len(res.Matched) != 1 {
		t.Fatalf("selector match failed: %+v", res.Matched)
	}
	if got := groupSpecOf(t, res, "g1").Policies.ByName[0].EnforcementMode; got != v1alpha1.ModeAudit {
		t.Fatalf("override not applied, mode=%q", got)
	}
}

func TestRenderStrictestAcrossDistributions(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{
			rDist("d-audit", []string{"c1"}, []string{"g1"}, v1alpha1.DistributionOverride{Cluster: "c1", EnforcementMode: v1alpha1.ModeAudit}),
			rDist("d-enforce", []string{"c1"}, []string{"g1"}),
		},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1", EnforcementMode: v1alpha1.ModeEnforce})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeEnforce)})
	if got := groupSpecOf(t, res, "g1").Policies.ByName[0].EnforcementMode; got != v1alpha1.ModeEnforce {
		t.Fatalf("strictest lost: %q", got)
	}
	if len(res.Objects) != 2 {
		t.Fatalf("group not deduped: %+v", res.Objects)
	}
}

func TestRenderMissingGroupIsLocalizedError(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{
			rDist("bad", []string{"c1"}, []string{"ghost"}),
			rDist("good", []string{"c1"}, []string{"g1"}),
		},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeAudit)})
	if res.Errors["bad"] == "" {
		t.Fatal("missing group error not recorded")
	}
	if len(res.Objects) != 2 {
		t.Fatal("good distribution must still render")
	}
}

func TestRenderNoMatchIsEmptySnapshotWithHash(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "other"}}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{rDist("d1", []string{"c1"}, []string{"g1"})},
		nil, nil)
	if len(res.Objects) != 0 || len(res.Matched) != 0 || res.Hash == "" {
		t.Fatalf("res = %+v", res)
	}
}

func TestRenderDeterministicHash(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	args := func() ([]v1alpha1.PolicyDistribution, []v1alpha1.PolicyGroup, []v1alpha1.Policy) {
		return []v1alpha1.PolicyDistribution{rDist("d1", []string{"c1"}, []string{"g1"})},
			[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"}, v1alpha1.PolicyRef{Name: "p2"})},
			[]v1alpha1.Policy{rPolicy("p2", v1alpha1.ModeAudit), rPolicy("p1", v1alpha1.ModeAudit)}
	}
	d1, g1, p1 := args()
	d2, g2, p2 := args()
	if Render(cl, d1, g1, p1).Hash != Render(cl, d2, g2, p2).Hash {
		t.Fatal("hash not deterministic")
	}
}

func TestRenderDisabledGroupContributesNoMembers(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	g := rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})
	g.Spec.Enabled = false
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{rDist("d1", []string{"c1"}, []string{"g1"})},
		[]v1alpha1.PolicyGroup{g},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeAudit)})
	if res.Errors["d1"] != "" {
		t.Fatalf("disabled group must not be an error: %v", res.Errors)
	}
	for _, o := range res.Objects {
		if o.Name == "g1" || o.Name == "p1" {
			t.Fatalf("disabled group must contribute nothing: %+v", res.Objects)
		}
	}
	if len(res.Objects) != 0 {
		t.Fatalf("expected empty snapshot: %+v", res.Objects)
	}
}

func TestDistMatchesMalformedSelectorRecordsErrorButStaysMatched(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	d := rDist("bad-selector", nil, []string{"g1"})
	d.Spec.Clusters.ClusterSelector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "k", Operator: "Bogus"}},
	}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{d},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeAudit)})
	if res.Errors["bad-selector"] == "" {
		t.Fatal("expected error for malformed clusterSelector")
	}
	found := false
	for _, m := range res.Matched {
		if m == "bad-selector" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errored distribution must still appear in Matched: %+v", res.Matched)
	}
}

func TestRenderHashChangesWithContent(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	base := func(mode string) RenderResult {
		return Render(cl,
			[]v1alpha1.PolicyDistribution{rDist("d1", []string{"c1"}, []string{"g1"})},
			[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})},
			[]v1alpha1.Policy{rPolicy("p1", mode)})
	}
	if base(v1alpha1.ModeAudit).Hash == base(v1alpha1.ModeEnforce).Hash {
		t.Fatal("hash did not change when EnforcementMode changed")
	}
}

func TestRenderSkipsDeletingDistribution(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	d := rDist("d1", []string{"c1"}, []string{"g1"})
	now := metav1.Now()
	d.DeletionTimestamp = &now
	d.Finalizers = []string{"test"}
	res := Render(cl, []v1alpha1.PolicyDistribution{d},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeAudit)})
	if len(res.Objects) != 0 || len(res.Matched) != 0 {
		t.Fatalf("deleting distribution still rendered: %+v", res)
	}
}

func TestRenderEmptyClusterSelectorMatchesEveryCluster(t *testing.T) {
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "any-cluster-no-labels"}}
	dist := v1alpha1.PolicyDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ClusterSelector: &metav1.LabelSelector{}},
			PolicyGroups: []string{"g1"},
		},
	}
	res := Render(cl,
		[]v1alpha1.PolicyDistribution{dist},
		[]v1alpha1.PolicyGroup{rGroup("g1", v1alpha1.PolicyRef{Name: "p1"})},
		[]v1alpha1.Policy{rPolicy("p1", v1alpha1.ModeAudit)})

	if len(res.Errors) != 0 || len(res.Matched) != 1 || res.Matched[0] != "default" {
		t.Fatalf("expected empty-selector distribution to match a label-less cluster: matched=%v errors=%v", res.Matched, res.Errors)
	}
}
