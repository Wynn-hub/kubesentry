package v1alpha1_test

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func TestPolicyMarshal(t *testing.T) {
	p := v1alpha1.Policy{
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: "enforce",
			Rego:            "package kubesentry\ndeny[msg]{ msg := \"test\" }",
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources: []v1alpha1.MatchResource{
					{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			},
		},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.Policy
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Spec.EnforcementMode != "enforce" {
		t.Errorf("enforcementMode = %q, want %q", got.Spec.EnforcementMode, "enforce")
	}
	if len(got.Spec.Match.Resources) != 1 {
		t.Errorf("resources len = %d, want 1", len(got.Spec.Match.Resources))
	}
}

func TestPolicyVersionMarshal(t *testing.T) {
	pv := v1alpha1.PolicyVersion{
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       "deny-privileged-pods",
			Version:         3,
			EnforcementMode: "enforce",
			CreatedAt:       metav1.Now(),
		},
	}

	data, err := json.Marshal(pv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got v1alpha1.PolicyVersion
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Spec.Version != 3 {
		t.Errorf("version = %d, want 3", got.Spec.Version)
	}
}

func TestPolicyRollbackTo(t *testing.T) {
	p := v1alpha1.Policy{
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: "enforce",
			Rego:            "package kubesentry",
			RollbackTo:      &v1alpha1.RollbackTo{Version: 2},
		},
	}
	data, _ := json.Marshal(p)
	var got v1alpha1.Policy
	json.Unmarshal(data, &got)
	if got.Spec.RollbackTo == nil || got.Spec.RollbackTo.Version != 2 {
		t.Error("rollbackTo not preserved")
	}
}

func TestPolicyGroupDeepCopy(t *testing.T) {
	orig := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "security"},
		Spec: v1alpha1.PolicyGroupSpec{
			DisplayName: "Security",
			Enabled:     true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{
					{Name: "run-as-privileged", EnforcementMode: "enforce"},
				},
			},
		},
		Status: v1alpha1.PolicyGroupStatus{
			Phase: v1alpha1.PhaseReady,
			ResolvedPolicies: []v1alpha1.EffectiveMember{
				{Name: "run-as-privileged", EnforcementMode: "enforce", Source: v1alpha1.SourceByName},
			},
		},
	}
	cp := orig.DeepCopy()
	if cp.Name != "security" {
		t.Error("name not copied")
	}
	if len(cp.Spec.Policies.ByName) != 1 {
		t.Error("policies not copied")
	}
	cp.Spec.Policies.ByName[0].Name = "changed"
	if orig.Spec.Policies.ByName[0].Name == "changed" {
		t.Error("DeepCopy shares slice backing array")
	}
	if len(cp.Status.ResolvedPolicies) != 1 {
		t.Error("resolvedPolicies not copied")
	}
}

func TestPolicySpecDescriptionField(t *testing.T) {
	spec := v1alpha1.PolicySpec{
		Description:     "test description",
		EnforcementMode: v1alpha1.ModeEnforce,
	}
	cp := new(v1alpha1.PolicySpec)
	spec.DeepCopyInto(cp)
	if cp.Description != "test description" {
		t.Errorf("Description not copied: %q", cp.Description)
	}
}

func TestLabelConstants(t *testing.T) {
	if v1alpha1.LabelSource == "" || v1alpha1.LabelCategory == "" {
		t.Error("label constants must not be empty")
	}
}

func TestGroupVersionValues(t *testing.T) {
	if v1alpha1.GroupVersion.Group != "kubesentry.io" {
		t.Errorf("GroupVersion.Group = %q, want %q", v1alpha1.GroupVersion.Group, "kubesentry.io")
	}
	if v1alpha1.GroupVersion.Version != "v1alpha1" {
		t.Errorf("GroupVersion.Version = %q, want %q", v1alpha1.GroupVersion.Version, "v1alpha1")
	}
}

func TestPhaseConstants(t *testing.T) {
	phases := []string{
		v1alpha1.PhaseReady,
		v1alpha1.PhaseInvalid,
		v1alpha1.PhaseSyncing,
		v1alpha1.PhaseDisabled,
		v1alpha1.PhaseDegraded,
	}
	seen := make(map[string]bool)
	for _, p := range phases {
		if p == "" {
			t.Fatal("phase constant must not be empty")
		}
		if seen[p] {
			t.Fatalf("duplicate phase: %s", p)
		}
		seen[p] = true
	}
	if v1alpha1.SourceBuiltin == v1alpha1.SourceCustom {
		t.Error("SourceBuiltin and SourceCustom must be different")
	}
	if v1alpha1.SourceBuiltin == "" || v1alpha1.SourceCustom == "" {
		t.Error("source constants must not be empty")
	}
}

func TestPolicyDeepCopyObject(t *testing.T) {
	orig := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
		Spec: v1alpha1.PolicySpec{
			EnforcementMode: v1alpha1.ModeEnforce,
			Rego:            "package kubesentry\ndeny[msg] { msg := \"test\" }",
		},
	}

	obj := orig.DeepCopyObject()
	p, ok := obj.(*v1alpha1.Policy)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *Policy", obj)
	}

	if p.Name != "test-policy" {
		t.Errorf("name = %q, want %q", p.Name, "test-policy")
	}
	if p.Spec.Rego != orig.Spec.Rego {
		t.Error("Rego not preserved")
	}

	// Verify no shared memory
	orig.Spec.Rego = "changed"
	if p.Spec.Rego == "changed" {
		t.Error("DeepCopyObject shares state with original")
	}
}

func TestPolicyGroupDeepCopyObject(t *testing.T) {
	orig := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-group"},
		Spec: v1alpha1.PolicyGroupSpec{
			DisplayName: "Test Group",
			Enabled:     true,
		},
	}

	obj := orig.DeepCopyObject()
	pg, ok := obj.(*v1alpha1.PolicyGroup)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *PolicyGroup", obj)
	}

	if pg.Name != "test-group" {
		t.Errorf("name = %q, want %q", pg.Name, "test-group")
	}
	if pg.Spec.DisplayName != "Test Group" {
		t.Error("DisplayName not preserved")
	}

	orig.Spec.DisplayName = "Changed"
	if pg.Spec.DisplayName == "Changed" {
		t.Error("DeepCopyObject shares state with original")
	}
}

func TestPolicyListDeepCopyObject(t *testing.T) {
	orig := &v1alpha1.PolicyList{
		Items: []v1alpha1.Policy{
			{ObjectMeta: metav1.ObjectMeta{Name: "policy1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "policy2"}},
		},
	}

	obj := orig.DeepCopyObject()
	pl, ok := obj.(*v1alpha1.PolicyList)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *PolicyList", obj)
	}

	if len(pl.Items) != 2 {
		t.Errorf("items count = %d, want 2", len(pl.Items))
	}
	if pl.Items[0].Name != "policy1" {
		t.Errorf("first item name = %q, want %q", pl.Items[0].Name, "policy1")
	}

	orig.Items[0].Name = "changed"
	if pl.Items[0].Name == "changed" {
		t.Error("DeepCopyObject shares items array with original")
	}
}

func TestPolicyVersionListDeepCopyObject(t *testing.T) {
	orig := &v1alpha1.PolicyVersionList{
		Items: []v1alpha1.PolicyVersion{
			{ObjectMeta: metav1.ObjectMeta{Name: "pv1"}},
		},
	}

	obj := orig.DeepCopyObject()
	pvl, ok := obj.(*v1alpha1.PolicyVersionList)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *PolicyVersionList", obj)
	}

	if len(pvl.Items) != 1 {
		t.Errorf("items count = %d, want 1", len(pvl.Items))
	}
}

func TestPolicyGroupListDeepCopyObject(t *testing.T) {
	orig := &v1alpha1.PolicyGroupList{
		Items: []v1alpha1.PolicyGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "pg1"}},
		},
	}

	obj := orig.DeepCopyObject()
	pgl, ok := obj.(*v1alpha1.PolicyGroupList)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *PolicyGroupList", obj)
	}

	if len(pgl.Items) != 1 {
		t.Errorf("items count = %d, want 1", len(pgl.Items))
	}
}

func TestMatchResourceDeepCopy(t *testing.T) {
	orig := &v1alpha1.MatchResource{
		APIGroups:   []string{"apps", "batch"},
		APIVersions: []string{"v1", "v1beta1"},
		Resources:   []string{"deployments", "jobs"},
	}

	cp := new(v1alpha1.MatchResource)
	orig.DeepCopyInto(cp)

	if len(cp.APIGroups) != 2 || cp.APIGroups[0] != "apps" {
		t.Error("APIGroups not copied correctly")
	}
	if len(cp.APIVersions) != 2 || cp.APIVersions[0] != "v1" {
		t.Error("APIVersions not copied correctly")
	}
	if len(cp.Resources) != 2 || cp.Resources[0] != "deployments" {
		t.Error("Resources not copied correctly")
	}

	// Verify no shared memory
	orig.APIGroups[0] = "changed"
	if cp.APIGroups[0] == "changed" {
		t.Error("DeepCopyInto shares APIGroups slice")
	}
}

func TestPolicyMatchDeepCopy(t *testing.T) {
	orig := &v1alpha1.PolicyMatch{
		Operations: []string{"CREATE", "UPDATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
			{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
		},
	}

	cp := new(v1alpha1.PolicyMatch)
	orig.DeepCopyInto(cp)

	if len(cp.Operations) != 2 {
		t.Errorf("operations count = %d, want 2", len(cp.Operations))
	}
	if len(cp.Resources) != 2 {
		t.Errorf("resources count = %d, want 2", len(cp.Resources))
	}

	orig.Operations[0] = "DELETE"
	if cp.Operations[0] == "DELETE" {
		t.Error("DeepCopyInto shares operations slice")
	}
}

func TestAddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	// Verify all three types are registered
	testCases := []runtime.Object{
		&v1alpha1.Policy{},
		&v1alpha1.PolicyList{},
		&v1alpha1.PolicyVersion{},
		&v1alpha1.PolicyVersionList{},
		&v1alpha1.PolicyGroup{},
		&v1alpha1.PolicyGroupList{},
	}

	for _, obj := range testCases {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err != nil || len(gvks) == 0 {
			t.Errorf("%T not registered in scheme", obj)
		}
	}
}

func TestPolicyStatusDeepCopyInto(t *testing.T) {
	now := metav1.Now()
	orig := &v1alpha1.PolicyStatus{
		Phase:              v1alpha1.PhaseReady,
		ObservedGeneration: 42,
		CurrentVersion:     3,
		LastSyncTime:       &now,
		Message:            "test message",
	}

	cp := new(v1alpha1.PolicyStatus)
	orig.DeepCopyInto(cp)
	if cp.Phase != v1alpha1.PhaseReady {
		t.Error("Phase not copied")
	}
	if cp.ObservedGeneration != 42 {
		t.Error("ObservedGeneration not copied")
	}
	if cp.CurrentVersion != 3 {
		t.Error("CurrentVersion not copied")
	}
	if cp.LastSyncTime == nil {
		t.Error("LastSyncTime not copied")
	}
	if cp.Message != "test message" {
		t.Error("Message not copied")
	}
}

func TestPolicyGroupStatusDeepCopyInto(t *testing.T) {
	orig := &v1alpha1.PolicyGroupStatus{
		Phase:              v1alpha1.PhaseReady,
		ObservedGeneration: 7,
		ResolvedPolicies: []v1alpha1.EffectiveMember{
			{Name: "pol-a", EnforcementMode: v1alpha1.ModeEnforce, Source: v1alpha1.SourceByName},
			{Name: "pol-b", EnforcementMode: v1alpha1.ModeAudit, Source: v1alpha1.SourceBySelector},
		},
	}

	cp := new(v1alpha1.PolicyGroupStatus)
	orig.DeepCopyInto(cp)
	if cp.Phase != v1alpha1.PhaseReady {
		t.Error("Phase not copied")
	}
	if cp.ObservedGeneration != 7 {
		t.Error("ObservedGeneration not copied")
	}
	if len(cp.ResolvedPolicies) != 2 {
		t.Errorf("ResolvedPolicies len = %d, want 2", len(cp.ResolvedPolicies))
	}
	// Verify no shared memory for the slice
	orig.ResolvedPolicies[0].Name = "changed"
	if cp.ResolvedPolicies[0].Name == "changed" {
		t.Error("DeepCopyInto shares ResolvedPolicies backing array")
	}
}

func TestPolicyVersionSpecDeepCopyInto(t *testing.T) {
	createdAt := metav1.Now()
	orig := &v1alpha1.PolicyVersionSpec{
		PolicyRef:       "my-policy",
		Version:         3,
		Rego:            "package kubesentry\ndeny[msg] { msg := \"test\" }",
		EnforcementMode: v1alpha1.ModeAudit,
		CreatedAt:       createdAt,
	}

	cp := new(v1alpha1.PolicyVersionSpec)
	orig.DeepCopyInto(cp)
	if cp.PolicyRef != "my-policy" {
		t.Error("PolicyRef not copied")
	}
	if cp.Version != 3 {
		t.Error("Version not copied")
	}
	if cp.Rego != orig.Rego {
		t.Error("Rego not copied")
	}
	if cp.EnforcementMode != v1alpha1.ModeAudit {
		t.Error("EnforcementMode not copied")
	}
}

func TestPolicyRefDeepCopyInto(t *testing.T) {
	orig := &v1alpha1.PolicyRef{
		Name:            "test-policy",
		EnforcementMode: v1alpha1.ModeEnforce,
	}

	cp := new(v1alpha1.PolicyRef)
	orig.DeepCopyInto(cp)
	if cp.Name != "test-policy" {
		t.Error("Name not copied")
	}
	if cp.EnforcementMode != v1alpha1.ModeEnforce {
		t.Error("EnforcementMode not copied")
	}
}

func TestPolicySpecDeepCopyInto(t *testing.T) {
	orig := &v1alpha1.PolicySpec{
		Description:     "test policy",
		EnforcementMode: v1alpha1.ModeEnforce,
		Rego:            "package kubesentry",
		Match: v1alpha1.PolicyMatch{
			Operations: []string{"CREATE"},
			Resources:  []v1alpha1.MatchResource{{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}},
		},
		RollbackTo: &v1alpha1.RollbackTo{Version: 5},
	}

	cp := new(v1alpha1.PolicySpec)
	orig.DeepCopyInto(cp)
	if cp.Description != "test policy" {
		t.Error("Description not copied")
	}
	if cp.EnforcementMode != v1alpha1.ModeEnforce {
		t.Error("EnforcementMode not copied")
	}
	if cp.Rego != "package kubesentry" {
		t.Error("Rego not copied")
	}
	if cp.RollbackTo == nil || cp.RollbackTo.Version != 5 {
		t.Error("RollbackTo not copied correctly")
	}
	if cp.RollbackTo == orig.RollbackTo {
		t.Error("RollbackTo shares reference with original")
	}
}
