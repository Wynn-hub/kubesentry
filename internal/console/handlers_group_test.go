package console

import (
	"context"
	"errors"
	"testing"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

func seedGroup(t *testing.T, repos gateway.Repos, name string, enabled bool) *dbrepo.PolicyGroup {
	t.Helper()
	g := &dbrepo.PolicyGroup{
		Name: name,
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			DisplayName: "DN " + name,
			Enabled:     enabled,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "p1"}},
			},
		}),
	}
	if err := repos.Groups.Create(context.Background(), g); err != nil {
		t.Fatalf("seed group %s: %v", name, err)
	}
	return g
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

func findGroup(items []groupListItem, name string) (groupListItem, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return groupListItem{}, false
}

func TestListGroups(t *testing.T) {
	// The migration seeds a "default" policy group alongside the 37 builtin
	// policies (see internal/dbmigrate/seed_test.go), so the list is never
	// empty even in a fresh DB — assert our two seeded groups are present
	// rather than asserting an exact total count.
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	seedGroup(t, repos, "g2", false)
	rec, env := doRequest(t, h, "GET", "/api/v1/policygroups", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	page := mustUnmarshal[pageResult[groupListItem]](t, env.Data)
	items := page.Items
	if page.Total < 2 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page metadata = %+v", page)
	}
	g1, ok := findGroup(items, "g1")
	if !ok || !g1.Enabled {
		t.Fatalf("g1 = %+v, ok=%v, items = %+v", g1, ok, items)
	}
	g2, ok := findGroup(items, "g2")
	if !ok || g2.Enabled {
		t.Fatalf("g2 = %+v, ok=%v, items = %+v", g2, ok, items)
	}
}

func TestGetGroupDetail(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	rec, env := doRequest(t, h, "GET", "/api/v1/policygroups/g1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[groupDetail](t, env.Data)
	if len(d.Spec.Policies.ByName) != 1 || d.Spec.Policies.ByName[0].Name != "p1" {
		t.Fatalf("detail = %+v", d)
	}
}

func TestCreateGroup(t *testing.T) {
	h, repos := newDBTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", validGroupReq())
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	g, err := repos.Groups.GetByName(t.Context(), "my-group")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Spec.Data().Enabled {
		t.Fatalf("group = %+v", g.Spec.Data())
	}
}

func TestCreateGroupDuplicate(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "my-group", true)
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", validGroupReq())
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreateGroupBadMode(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.Policies.ByName[0].EnforcementMode = "block"
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateGroup(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	req := validGroupReq()
	req.Name = ""
	req.Enabled = false
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	g, _ := repos.Groups.GetByName(t.Context(), "g1")
	if g.Spec.Data().Enabled {
		t.Fatal("enabled should be false after update")
	}
}

func TestUpdateGroupStaleResourceVersion(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	req := validGroupReq()
	req.Name = ""
	req.ResourceVersion = "stale-does-not-match"
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1", req)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestUpdateGroupNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.Name = ""
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/nope", req)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestSetGroupEnabled(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1/enabled", map[string]bool{"enabled": false})
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	g, _ := repos.Groups.GetByName(t.Context(), "g1")
	if g.Spec.Data().Enabled {
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
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policygroups/g1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if _, err := repos.Groups.GetByName(t.Context(), "g1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("group should be deleted, got err=%v", err)
	}
}

func TestDeleteGroupNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policygroups/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteGroupRejectsDefaultName(t *testing.T) {
	// The migration already seeds a "default" policy group (it backs the
	// reserved "default" PolicyDistribution) — no need to seed our own, and
	// seeding one would collide with it on the unique name constraint.
	h, repos := newDBTestServer(t)
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policygroups/default", nil)
	if rec.Code != 403 {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
	if _, err := repos.Groups.GetByName(t.Context(), "default"); err != nil {
		t.Fatalf("default group must survive the rejected delete: %v", err)
	}
}

func TestCreateGroupWithExclude(t *testing.T) {
	h, repos := newDBTestServer(t)
	req := validGroupReq()
	req.Policies.Exclude = []string{"host-ipc-set"}
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	g, err := repos.Groups.GetByName(t.Context(), "my-group")
	if err != nil {
		t.Fatal(err)
	}
	exclude := g.Spec.Data().Policies.Exclude
	if len(exclude) != 1 || exclude[0] != "host-ipc-set" {
		t.Fatalf("exclude = %+v", exclude)
	}
}

func TestCreateGroupExcludeEmptyEntryRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.Policies.Exclude = []string{""}
	rec, _ := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestUpdateGroupClearsExclude(t *testing.T) {
	h, repos := newDBTestServer(t)
	g := seedGroup(t, repos, "g1", true)
	spec := g.Spec.Data()
	spec.Policies.Exclude = []string{"old-excluded"}
	g.Spec = datatypes.NewJSONType(spec)
	if err := repos.Groups.Update(t.Context(), g); err != nil {
		t.Fatal(err)
	}

	req := validGroupReq()
	req.Name = ""
	req.Policies.Exclude = nil
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/g1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	got, _ := repos.Groups.GetByName(t.Context(), "g1")
	if len(got.Spec.Data().Policies.Exclude) != 0 {
		t.Fatalf("exclude = %+v, want empty after clearing", got.Spec.Data().Policies.Exclude)
	}
}
