package console

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func seedGroup(t *testing.T, repos repository.Repositories, name string, enabled bool) *dbrepo.PolicyGroup {
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

type testGroupRequest struct {
	AllClusters     bool                         `json:"allClusters"`
	Name            string                       `json:"name,omitempty"`
	DisplayName     string                       `json:"displayName"`
	Enabled         bool                         `json:"enabled"`
	Policies        v1alpha1.PolicyGroupPolicies `json:"policies"`
	ResourceVersion string                       `json:"resourceVersion,omitempty"`
	ClusterNames    []string                     `json:"clusterNames,omitempty"`
}

func validGroupReq() *testGroupRequest {
	return &testGroupRequest{
		AllClusters: true,
		Name:        "my-group",
		DisplayName: "My Group",
		Enabled:     true,
		Policies: v1alpha1.PolicyGroupPolicies{
			ByName: []v1alpha1.PolicyRef{{Name: "host-ipc-set", EnforcementMode: "audit"}},
		},
	}
}

func findGroup(items []dto.PolicyGroupItem, name string) (dto.PolicyGroupItem, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return dto.PolicyGroupItem{}, false
}

func TestListGroups(t *testing.T) {
	// The migration seeds a "default" policy group alongside the 37 builtin
	// policies (see internal/database/migrate/seed_test.go), so the list is never
	// empty even in a fresh DB — assert our two seeded groups are present
	// rather than asserting an exact total count.
	h, repos := newDBTestServer(t)
	seedGroup(t, repos, "g1", true)
	seedGroup(t, repos, "g2", false)
	rec, env := doRequest(t, h, "GET", "/api/v1/policygroups", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	page := mustUnmarshal[testPageResult[dto.PolicyGroupItem]](t, env.Data)
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
	d := mustUnmarshal[dto.PolicyGroupView](t, env.Data)
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

func TestCreateGroupRejectsMissingPolicy(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.Policies.ByName[0].Name = "does-not-exist"
	rec, env := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 || !strings.Contains(env.Message, "does-not-exist") {
		t.Fatalf("code=%d message=%q, want missing policy validation", rec.Code, env.Message)
	}
}

func TestCreateGroupRejectsInvalidDNSName(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.Name = "Not_Valid!"
	rec, env := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 || !strings.Contains(env.Message, "invalid name") {
		t.Fatalf("code=%d message=%q, want specific DNS name validation", rec.Code, env.Message)
	}
}

func TestCreateGroupRejectsAllClustersWithClusterNames(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.ClusterNames = []string{"local"}
	rec, env := doRequest(t, h, "POST", "/api/v1/policygroups", req)
	if rec.Code != 400 || !strings.Contains(env.Message, "allClusters") {
		t.Fatalf("code=%d message=%q, want conflicting scope validation", rec.Code, env.Message)
	}
}

func TestPolicyGroupMutationsNotifyGateway(t *testing.T) {
	h, repos, handlers := newDBTestServerWithHandlers(t)
	seedGroup(t, repos, "notify-group", true)
	fake := &fakeHubClient{}
	handlers.Gateway = fake
	headers := map[string]string{"X-Request-Id": "group-trace"}

	req := validGroupReq()
	req.Name = ""
	for _, mutation := range []struct {
		method string
		path   string
		body   any
	}{
		{method: "PUT", path: "/api/v1/policygroups/notify-group", body: req},
		{method: "PUT", path: "/api/v1/policygroups/notify-group/enabled", body: map[string]bool{"enabled": false}},
		{method: "DELETE", path: "/api/v1/policygroups/notify-group"},
	} {
		rec, env := doRequestWithHeaders(t, h, mutation.method, mutation.path, mutation.body, headers)
		if rec.Code != 200 || env.Code != 0 {
			t.Fatalf("%s %s: code=%d body=%s", mutation.method, mutation.path, rec.Code, rec.Body.String())
		}
	}

	topics, traces := fake.calls()
	if len(topics) != 3 || topics[0] != "group" || topics[1] != "group" || topics[2] != "group" {
		t.Fatalf("topics = %v, want three group notifications", topics)
	}
	for _, trace := range traces {
		if trace != "group-trace" {
			t.Fatalf("traces = %v, want request trace preserved", traces)
		}
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

func TestSetGroupEnabledRepeatIsNoOp(t *testing.T) {
	h, repos, handlers := newDBTestServerWithHandlers(t)
	seedGroup(t, repos, "idempotent-group", true)
	fake := &fakeHubClient{}
	handlers.Gateway = fake

	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/idempotent-group/enabled", map[string]bool{"enabled": false})
	if rec.Code != 200 {
		t.Fatalf("first request code=%d body=%s", rec.Code, rec.Body.String())
	}
	afterFirst, err := repos.Groups.GetByName(t.Context(), "idempotent-group")
	if err != nil {
		t.Fatal(err)
	}
	rec, env := doRequest(t, h, "GET", "/api/v1/policygroups/idempotent-group", nil)
	if rec.Code != 200 {
		t.Fatalf("first detail code=%d body=%s", rec.Code, rec.Body.String())
	}
	firstVersion := mustUnmarshal[dto.PolicyGroupView](t, env.Data).ResourceVersion
	firstTopics, _ := fake.calls()
	if len(firstTopics) != 1 || firstTopics[0] != "group" {
		t.Fatalf("first notification topics=%v, want [group]", firstTopics)
	}

	rec, _ = doRequest(t, h, "PUT", "/api/v1/policygroups/idempotent-group/enabled", map[string]bool{"enabled": false})
	if rec.Code != 200 {
		t.Fatalf("repeat request code=%d body=%s", rec.Code, rec.Body.String())
	}
	afterRepeat, err := repos.Groups.GetByName(t.Context(), "idempotent-group")
	if err != nil {
		t.Fatal(err)
	}
	rec, env = doRequest(t, h, "GET", "/api/v1/policygroups/idempotent-group", nil)
	if rec.Code != 200 {
		t.Fatalf("repeat detail code=%d body=%s", rec.Code, rec.Body.String())
	}
	repeatVersion := mustUnmarshal[dto.PolicyGroupView](t, env.Data).ResourceVersion
	repeatTopics, _ := fake.calls()
	if !afterRepeat.UpdatedAt.Equal(afterFirst.UpdatedAt) {
		t.Fatalf("UpdatedAt changed on no-op: first=%s repeat=%s", afterFirst.UpdatedAt, afterRepeat.UpdatedAt)
	}
	if repeatVersion != firstVersion {
		t.Fatalf("resourceVersion changed on no-op: first=%q repeat=%q", firstVersion, repeatVersion)
	}
	if len(repeatTopics) != len(firstTopics) {
		t.Fatalf("notification count changed on no-op: first=%v repeat=%v", firstTopics, repeatTopics)
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

func TestUpdateGroupRejectsDefaultManagedScope(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validGroupReq()
	req.Name = ""
	req.AllClusters = false
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policygroups/default", req)
	if rec.Code != 403 {
		t.Fatalf("code = %d, want 403", rec.Code)
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
