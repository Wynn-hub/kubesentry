package console

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func testPolicy(name, source, mode, phase string) *v1alpha1.Policy {
	return &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{v1alpha1.LabelSource: source},
		},
		Spec: v1alpha1.PolicySpec{
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources: []v1alpha1.MatchResource{
					{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				},
			},
			EnforcementMode: mode,
			Rego:            validRego,
			Description:     "desc of " + name,
		},
		Status: v1alpha1.PolicyStatus{Phase: phase, CurrentVersion: 1},
	}
}

func TestListPoliciesFiltered(t *testing.T) {
	h, _ := newTestServer(t,
		testPolicy("a-builtin", "builtin", "enforce", "Ready"),
		testPolicy("b-custom", "custom", "audit", "Invalid"),
	)
	rec, env := doRequest(t, h, "GET", "/api/v1/policies?source=custom", nil)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	items := mustUnmarshal[[]policyListItem](t, env.Data)
	if len(items) != 1 || items[0].Name != "b-custom" {
		t.Fatalf("items = %+v", items)
	}

	_, env = doRequest(t, h, "GET", "/api/v1/policies?phase=Ready", nil)
	items = mustUnmarshal[[]policyListItem](t, env.Data)
	if len(items) != 1 || items[0].Name != "a-builtin" {
		t.Fatalf("phase filter items = %+v", items)
	}

	_, env = doRequest(t, h, "GET", "/api/v1/policies?keyword=b-cus", nil)
	items = mustUnmarshal[[]policyListItem](t, env.Data)
	if len(items) != 1 || items[0].Name != "b-custom" {
		t.Fatalf("keyword filter items = %+v", items)
	}
}

func TestGetPolicy(t *testing.T) {
	h, _ := newTestServer(t, testPolicy("p1", "custom", "enforce", "Ready"))
	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[policyDetail](t, env.Data)
	if d.Name != "p1" || d.Spec.EnforcementMode != "enforce" || d.ResourceVersion == "" {
		t.Fatalf("detail = %+v", d)
	}
}

func TestGetPolicyNotFound(t *testing.T) {
	h, _ := newTestServer(t)
	rec, _ := doRequest(t, h, "GET", "/api/v1/policies/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestCreatePolicy(t *testing.T) {
	h, c := newTestServer(t)
	rec, env := doRequest(t, h, "POST", "/api/v1/policies", validReq())
	if rec.Code != 200 || !env.Success {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	var p v1alpha1.Policy
	if err := c.Get(t.Context(), client.ObjectKey{Name: "my-policy"}, &p); err != nil {
		t.Fatal(err)
	}
	if p.Labels[v1alpha1.LabelSource] != v1alpha1.SourceCustom {
		t.Fatalf("source label = %q", p.Labels[v1alpha1.LabelSource])
	}
}

func TestCreatePolicyDuplicate(t *testing.T) {
	h, _ := newTestServer(t, testPolicy("my-policy", "custom", "audit", "Ready"))
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies", validReq())
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreatePolicyBadRego(t *testing.T) {
	h, _ := newTestServer(t)
	req := validReq()
	req.Rego = "package kubesentry\ndeny[msg] {"
	rec, env := doRequest(t, h, "POST", "/api/v1/policies", req)
	if rec.Code != 400 || env.Error == nil {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
}

func TestUpdatePolicyStaleResourceVersion(t *testing.T) {
	h, _ := newTestServer(t, testPolicy("p1", "custom", "audit", "Ready"))
	req := validReq()
	req.Name = ""
	req.ResourceVersion = "stale-does-not-match"
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policies/p1", req)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestUpdatePolicyClearsCursor(t *testing.T) {
	p := testPolicy("p1", "custom", "audit", "Ready")
	p.Annotations = map[string]string{cursorAnnotation: `{"cursor":1,"atVersion":2,"head":2}`}
	h, c := newTestServer(t, p)
	req := validReq()
	req.Name = ""
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policies/p1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var got v1alpha1.Policy
	if err := c.Get(t.Context(), client.ObjectKey{Name: "p1"}, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Annotations[cursorAnnotation]; ok {
		t.Fatal("cursor annotation should be cleared on update")
	}
	if got.Spec.EnforcementMode != "audit" || got.Spec.Description != "" {
		t.Fatalf("spec = %+v", got.Spec)
	}
}

func TestUpdatePolicyRejectedDuringRollback(t *testing.T) {
	p := testPolicy("p1", "custom", "audit", "Ready")
	p.Spec.RollbackTo = &v1alpha1.RollbackTo{Version: 1}
	h, _ := newTestServer(t, p)
	req := validReq()
	req.Name = ""
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policies/p1", req)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestValidateEndpoint(t *testing.T) {
	h, _ := newTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/validate", map[string]string{"rego": validRego})
	if rec.Code != 200 {
		t.Fatalf("valid rego: code = %d", rec.Code)
	}
	rec, env := doRequest(t, h, "POST", "/api/v1/policies/validate", map[string]string{"rego": "package kubesentry\ndeny[msg] {"})
	if rec.Code != 400 || env.Error == nil {
		t.Fatalf("bad rego: code=%d env=%+v", rec.Code, env)
	}
}

func TestDeletePolicyReferenced(t *testing.T) {
	p := testPolicy("p1", "custom", "audit", "Ready")
	p.Status.ReferencedBy = []string{"security"}
	h, _ := newTestServer(t, p)

	rec, env := doRequest(t, h, "DELETE", "/api/v1/policies/p1", nil)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
	d := mustUnmarshal[map[string][]string](t, env.Data)
	if len(d["referencedBy"]) != 1 || d["referencedBy"][0] != "security" {
		t.Fatalf("data = %+v", d)
	}

	rec, _ = doRequest(t, h, "DELETE", "/api/v1/policies/p1?force=true", nil)
	if rec.Code != 200 {
		t.Fatalf("force delete code = %d", rec.Code)
	}
}

func TestDeletePolicyUnreferenced(t *testing.T) {
	h, c := newTestServer(t, testPolicy("p1", "custom", "audit", "Ready"))
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policies/p1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var got v1alpha1.Policy
	err := c.Get(t.Context(), client.ObjectKey{Name: "p1"}, &got)
	if err == nil {
		t.Fatal("policy should be deleted")
	}
}

func testVersion(policy string, version int64, mode string) *v1alpha1.PolicyVersion {
	return &v1alpha1.PolicyVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-v%d", policy, version),
			Labels: map[string]string{
				"kubesentry/policy":  policy,
				"kubesentry/version": strconv.FormatInt(version, 10),
			},
		},
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       policy,
			Version:         version,
			Rego:            validRego,
			EnforcementMode: mode,
		},
	}
}

func TestListVersionsTimeline(t *testing.T) {
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 2
	p.Status.VersionHistory = []v1alpha1.PolicyVersionSummary{
		{Version: 1, Phase: "Ready"}, {Version: 2, Phase: "Ready"},
	}
	h, _ := newTestServer(t, p, testVersion("p1", 1, "audit"), testVersion("p1", 2, "enforce"))

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p1/versions", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	tl := mustUnmarshal[versionTimeline](t, env.Data)
	if tl.Cursor != 2 || tl.Head != 2 || tl.InFlight {
		t.Fatalf("timeline = %+v", tl)
	}
	if !tl.PrevEnabled || tl.NextEnabled {
		t.Fatalf("prev/next = %v/%v, want true/false", tl.PrevEnabled, tl.NextEnabled)
	}
	if len(tl.Versions) != 2 || tl.Versions[0].Version != 2 || !tl.Versions[0].IsCurrent {
		t.Fatalf("versions = %+v", tl.Versions)
	}
}

func TestListVersionsPrunedPrevDisabled(t *testing.T) {
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 5
	h, _ := newTestServer(t, p, testVersion("p1", 5, "enforce")) // v4 已被剪枝
	_, env := doRequest(t, h, "GET", "/api/v1/policies/p1/versions", nil)
	tl := mustUnmarshal[versionTimeline](t, env.Data)
	if tl.PrevEnabled {
		t.Fatal("prev should be disabled when v4 snapshot is pruned")
	}
}

func TestListVersionsRollbackToForcesInFlight(t *testing.T) {
	// settled cursor (cur=3, atVersion=2) but RollbackTo still set —
	// the handler must report in-flight and disable navigation.
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 3
	p.Annotations = map[string]string{cursorAnnotation: `{"cursor":1,"atVersion":2,"head":2}`}
	p.Spec.RollbackTo = &v1alpha1.RollbackTo{Version: 1}
	h, _ := newTestServer(t, p,
		testVersion("p1", 1, "audit"), testVersion("p1", 2, "enforce"), testVersion("p1", 3, "audit"))
	_, env := doRequest(t, h, "GET", "/api/v1/policies/p1/versions", nil)
	tl := mustUnmarshal[versionTimeline](t, env.Data)
	if !tl.InFlight || tl.PrevEnabled || tl.NextEnabled {
		t.Fatalf("timeline = %+v, want inFlight=true and navigation disabled", tl)
	}
}

func TestRollbackPrevFromHead(t *testing.T) {
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 2
	h, c := newTestServer(t, p, testVersion("p1", 1, "audit"), testVersion("p1", 2, "enforce"))

	rec, env := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback",
		map[string]string{"direction": "prev"})
	if rec.Code != 200 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	var got v1alpha1.Policy
	if err := c.Get(t.Context(), client.ObjectKey{Name: "p1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.RollbackTo == nil || got.Spec.RollbackTo.Version != 1 {
		t.Fatalf("rollbackTo = %+v", got.Spec.RollbackTo)
	}
	var cur logicalCursor
	if err := json.Unmarshal([]byte(got.Annotations[cursorAnnotation]), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.Cursor != 1 || cur.AtVersion != 2 || cur.Head != 2 {
		t.Fatalf("cursor annotation = %+v", cur)
	}
}

func TestRollbackNextAtHead(t *testing.T) {
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 2
	h, _ := newTestServer(t, p, testVersion("p1", 1, "audit"), testVersion("p1", 2, "enforce"))
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback",
		map[string]string{"direction": "next"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestRollbackNextAfterSettled(t *testing.T) {
	// 已回滚到 v1（settled：cur=3, annotation {1,2,2}），next 应指向 v2
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 3
	p.Annotations = map[string]string{cursorAnnotation: `{"cursor":1,"atVersion":2,"head":2}`}
	h, c := newTestServer(t, p,
		testVersion("p1", 1, "audit"), testVersion("p1", 2, "enforce"), testVersion("p1", 3, "audit"))

	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback",
		map[string]string{"direction": "next"})
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var got v1alpha1.Policy
	_ = c.Get(t.Context(), client.ObjectKey{Name: "p1"}, &got)
	if got.Spec.RollbackTo.Version != 2 {
		t.Fatalf("rollbackTo = %+v", got.Spec.RollbackTo)
	}
	var cur logicalCursor
	_ = json.Unmarshal([]byte(got.Annotations[cursorAnnotation]), &cur)
	if cur.Cursor != 2 || cur.AtVersion != 3 || cur.Head != 2 {
		t.Fatalf("cursor = %+v", cur)
	}
}

func TestRollbackInFlightRejected(t *testing.T) {
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 2
	p.Annotations = map[string]string{cursorAnnotation: `{"cursor":1,"atVersion":2,"head":2}`}
	h, _ := newTestServer(t, p, testVersion("p1", 1, "audit"), testVersion("p1", 2, "enforce"))
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback",
		map[string]string{"direction": "prev"})
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestRollbackTargetPruned(t *testing.T) {
	p := testPolicy("p1", "custom", "enforce", "Ready")
	p.Status.CurrentVersion = 5
	h, _ := newTestServer(t, p, testVersion("p1", 5, "enforce")) // v4 被剪枝
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback",
		map[string]string{"direction": "prev"})
	if rec.Code != 410 {
		t.Fatalf("code = %d, want 410", rec.Code)
	}
}

func TestRollbackBadDirection(t *testing.T) {
	h, _ := newTestServer(t, testPolicy("p1", "custom", "enforce", "Ready"))
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback",
		map[string]string{"direction": "jump"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestResourceSuggestions(t *testing.T) {
	p1 := testPolicy("p1", "custom", "audit", "Ready")
	p1.Spec.Match.Resources = []v1alpha1.MatchResource{
		{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods", "services"}},
	}
	p2 := testPolicy("p2", "custom", "audit", "Ready")
	p2.Spec.Match.Resources = []v1alpha1.MatchResource{
		{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
		{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}, // 与 p1 重复，验证去重
	}
	h, _ := newTestServer(t, p1, p2)

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/resource-suggestions", nil)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	got := mustUnmarshal[resourceSuggestionsResponse](t, env.Data)

	sort.Strings(got.APIGroups)
	sort.Strings(got.APIVersions)
	sort.Strings(got.Resources)
	if want := []string{"", "apps"}; !reflect.DeepEqual(got.APIGroups, want) {
		t.Fatalf("apiGroups = %v, want %v", got.APIGroups, want)
	}
	if want := []string{"v1"}; !reflect.DeepEqual(got.APIVersions, want) {
		t.Fatalf("apiVersions = %v, want %v", got.APIVersions, want)
	}
	if want := []string{"deployments", "pods", "services"}; !reflect.DeepEqual(got.Resources, want) {
		t.Fatalf("resources = %v, want %v", got.Resources, want)
	}
}

func TestPolicyAnnotationsRoundTrip(t *testing.T) {
	h, _ := newTestServer(t)

	create := map[string]any{
		"name":            "p-ann",
		"enforcementMode": "audit",
		"rego":            validRego,
		"annotations":     map[string]string{"kubesentry.io/visual-builder-spec": `{"groups":[]}`},
		"match": map[string]any{
			"operations": []string{"CREATE"},
			"resources": []map[string]any{
				{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
			},
		},
	}
	if rec, env := doRequest(t, h, "POST", "/api/v1/policies", create); rec.Code != 200 {
		t.Fatalf("create: code=%d env=%+v", rec.Code, env)
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p-ann", nil)
	if rec.Code != 200 {
		t.Fatalf("get: code=%d", rec.Code)
	}
	d := mustUnmarshal[policyDetail](t, env.Data)
	if d.Annotations["kubesentry.io/visual-builder-spec"] != `{"groups":[]}` {
		t.Fatalf("annotations = %+v", d.Annotations)
	}

	// 更新时把该 annotation 的值传空字符串 → 应被清除
	update := map[string]any{
		"enforcementMode": "audit",
		"rego":            validRego,
		"annotations":     map[string]string{"kubesentry.io/visual-builder-spec": ""},
		"resourceVersion": d.ResourceVersion,
		"match": map[string]any{
			"operations": []string{"CREATE"},
			"resources": []map[string]any{
				{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
			},
		},
	}
	if rec, env := doRequest(t, h, "PUT", "/api/v1/policies/p-ann", update); rec.Code != 200 {
		t.Fatalf("update: code=%d env=%+v", rec.Code, env)
	}
	_, env = doRequest(t, h, "GET", "/api/v1/policies/p-ann", nil)
	d = mustUnmarshal[policyDetail](t, env.Data)
	if v, ok := d.Annotations["kubesentry.io/visual-builder-spec"]; ok {
		t.Fatalf("annotation should have been cleared, got %q", v)
	}
}
