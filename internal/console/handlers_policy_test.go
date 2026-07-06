package console

import (
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
