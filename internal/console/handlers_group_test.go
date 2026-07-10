package console

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func testGroup(name string, enabled bool) *v1alpha1.PolicyGroup {
	return &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{v1alpha1.LabelSource: "builtin"},
		},
		Spec: v1alpha1.PolicyGroupSpec{
			DisplayName: "DN " + name,
			Enabled:     enabled,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "p1"}},
			},
		},
		Status: v1alpha1.PolicyGroupStatus{
			Phase:         "Ready",
			ResolvedCount: 1,
			ResolvedPolicies: []v1alpha1.EffectiveMember{
				{Name: "p1", EnforcementMode: "enforce", Source: "byName"},
			},
		},
	}
}

func validGroupReq() *groupRequest {
	return &groupRequest{
		Name:        "my-group",
		DisplayName: "My Group",
		Enabled:     true,
		Policies: v1alpha1.PolicyGroupPolicies{
			ByName: []v1alpha1.PolicyRef{{Name: "p1", EnforcementMode: "audit"}},
		},
	}
}

func TestListGroups(t *testing.T) {
	h, _ := newTestServer(t, testGroup("g1", true), testGroup("g2", false))
	rec, env := doRequest(t, h, "GET", "/api/v1/policygroups", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	items := mustUnmarshal[[]groupListItem](t, env.Data)
	if len(items) != 2 || items[0].Name != "g1" || items[0].ResolvedCount != 1 {
		t.Fatalf("items = %+v", items)
	}
}

func TestGetGroupDetail(t *testing.T) {
	h, _ := newTestServer(t, testGroup("g1", true))
	rec, env := doRequest(t, h, "GET", "/api/v1/policygroups/g1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[groupDetail](t, env.Data)
	if len(d.Status.ResolvedPolicies) != 1 || d.Status.ResolvedPolicies[0].Name != "p1" {
		t.Fatalf("detail = %+v", d)
	}
}

func TestCreateGroup(t *testing.T) {
	h, c := newTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", validGroupReq())
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var g v1alpha1.PolicyGroup
	if err := c.Get(t.Context(), client.ObjectKey{Name: "my-group"}, &g); err != nil {
		t.Fatal(err)
	}
	if !g.Spec.Enabled || g.Labels[v1alpha1.LabelSource] != v1alpha1.SourceCustom {
		t.Fatalf("group = %+v", g)
	}
}

func TestCreateGroupBadMode(t *testing.T) {
	h, _ := newTestServer(t)
	req := validGroupReq()
	req.Policies.ByName[0].EnforcementMode = "block"
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateGroup(t *testing.T) {
	h, c := newTestServer(t, testGroup("g1", true))
	req := validGroupReq()
	req.Name = ""
	req.Enabled = false
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var g v1alpha1.PolicyGroup
	_ = c.Get(t.Context(), client.ObjectKey{Name: "g1"}, &g)
	if g.Spec.Enabled {
		t.Fatal("enabled should be false after update")
	}
}

func TestSetGroupEnabled(t *testing.T) {
	h, c := newTestServer(t, testGroup("g1", true))
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1/enabled", map[string]bool{"enabled": false})
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var g v1alpha1.PolicyGroup
	_ = c.Get(t.Context(), client.ObjectKey{Name: "g1"}, &g)
	if g.Spec.Enabled {
		t.Fatal("enabled should be false after toggle")
	}

	// Idempotent repeat.
	rec, _ = doRequest(t, h, "PUT", "/api/v1/policygroups/g1/enabled", map[string]bool{"enabled": false})
	if rec.Code != 200 {
		t.Fatalf("repeat code = %d", rec.Code)
	}

	// Missing field → 400.
	rec, _ = doRequest(t, h, "PUT", "/api/v1/policygroups/g1/enabled", map[string]string{})
	if rec.Code != 400 {
		t.Fatalf("missing enabled code = %d", rec.Code)
	}

	// Unknown group → 404.
	rec, _ = doRequest(t, h, "PUT", "/api/v1/policygroups/nope/enabled", map[string]bool{"enabled": true})
	if rec.Code != 404 {
		t.Fatalf("unknown group code = %d", rec.Code)
	}
}

func TestDeleteGroup(t *testing.T) {
	h, c := newTestServer(t, testGroup("g1", true))
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policygroups/g1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var g v1alpha1.PolicyGroup
	if err := c.Get(t.Context(), client.ObjectKey{Name: "g1"}, &g); err == nil {
		t.Fatal("group should be deleted")
	}
}

func TestCreateGroupWithExclude(t *testing.T) {
	h, c := newTestServer(t)
	req := validGroupReq()
	req.Policies.Exclude = []string{"host-ipc-set"}
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var g v1alpha1.PolicyGroup
	if err := c.Get(t.Context(), client.ObjectKey{Name: "my-group"}, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Spec.Policies.Exclude) != 1 || g.Spec.Policies.Exclude[0] != "host-ipc-set" {
		t.Fatalf("exclude = %+v", g.Spec.Policies.Exclude)
	}
}

func TestCreateGroupExcludeEmptyEntryRejected(t *testing.T) {
	h, _ := newTestServer(t)
	req := validGroupReq()
	req.Policies.Exclude = []string{""}
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateGroupClearsExclude(t *testing.T) {
	g := testGroup("g1", true)
	g.Spec.Policies.Exclude = []string{"old-excluded"}
	h, c := newTestServer(t, g)
	req := validGroupReq()
	req.Name = ""
	req.Policies.Exclude = nil
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	var got v1alpha1.PolicyGroup
	_ = c.Get(t.Context(), client.ObjectKey{Name: "g1"}, &got)
	if len(got.Spec.Policies.Exclude) != 0 {
		t.Fatalf("exclude = %+v, want empty after clearing", got.Spec.Policies.Exclude)
	}
}
