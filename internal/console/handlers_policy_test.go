package console

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
