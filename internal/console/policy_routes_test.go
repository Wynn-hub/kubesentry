package console

import (
	"context"
	"reflect"
	"testing"

	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

// These aliases keep the assertions focused on the JSON contract.
type (
	policyListItem              = dto.PolicyItem
	policyDetail                = dto.PolicyView
	versionList                 = dto.PolicyVersionListView
	resourceSuggestionsResponse = dto.ResourceSuggestionsView
)

type testPolicyRequest struct {
	Name            string               `json:"name,omitempty"`
	Description     string               `json:"description"`
	EnforcementMode string               `json:"enforcementMode"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	Rego            string               `json:"rego"`
	Labels          map[string]string    `json:"labels,omitempty"`
	Annotations     map[string]string    `json:"annotations,omitempty"`
	ResourceVersion string               `json:"resourceVersion,omitempty"`
}

func validReq() *testPolicyRequest {
	return &testPolicyRequest{
		Name:            "my-policy",
		EnforcementMode: "audit",
		Match:           testMatch(),
		Rego:            validRego,
	}
}

func testMatch() v1alpha1.PolicyMatch {
	return v1alpha1.PolicyMatch{
		Operations: []string{"CREATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
		},
	}
}

// seedPolicyBare inserts a Policy row without any policy_versions rows —
// used by the "pruned snapshot" tests, which need a policy whose early
// version history has already been garbage-collected.
func seedPolicyBare(t *testing.T, repos repository.Repositories, name, mode string) *dbrepo.Policy {
	t.Helper()
	p := &dbrepo.Policy{
		Name:            name,
		Description:     "desc of " + name,
		Rego:            validRego,
		EnforcementMode: mode,
		Match:           datatypes.NewJSONType(testMatch()),
		CompileOK:       true,
	}
	if err := repos.Policies.Create(context.Background(), p); err != nil {
		t.Fatalf("seed policy %s: %v", name, err)
	}
	return p
}

func seedPolicyVersion(t *testing.T, repos repository.Repositories, p *dbrepo.Policy, version int, mode string) {
	t.Helper()
	if err := repos.Policies.CreateVersion(context.Background(), &dbrepo.PolicyVersion{
		PolicyID: p.ID,
		Version:  version,
		Rego:     validRego,
		SpecSnapshot: datatypes.NewJSONType(v1alpha1.PolicySpec{
			Match: p.Match.Data(), EnforcementMode: mode, Rego: validRego, Description: p.Description,
		}),
	}); err != nil {
		t.Fatalf("seed policy version %s/v%d: %v", p.Name, version, err)
	}
}

// seedPolicy inserts a Policy row plus its matching v1 policy_versions row —
// the shape every policy created through the console API has.
func seedPolicy(t *testing.T, repos repository.Repositories, name, mode string) *dbrepo.Policy {
	t.Helper()
	p := seedPolicyBare(t, repos, name, mode)
	seedPolicyVersion(t, repos, p, 1, mode)
	return p
}

func findPolicy(items []policyListItem, name string) (policyListItem, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return policyListItem{}, false
}

// zzTestKeyword prefixes fixture names in list-filter tests. The migration
// seeds 37 real builtin policies alongside whatever a test creates (see
// internal/dbmigrate/seed_test.go), so a plain mode/phase filter also
// matches builtin rows — these tests narrow with an always-included keyword
// filter unlikely to collide with any real policy name, then assert on the
// narrowed set instead of the full list.
const zzTestKeyword = "zz-test-"

func TestListPoliciesFiltered(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, zzTestKeyword+"a-ready", "enforce")
	bad := seedPolicy(t, repos, zzTestKeyword+"b-invalid", "audit")
	bad.CompileOK = false
	bad.CompileError = "boom"
	if err := repos.Policies.Update(t.Context(), bad); err != nil {
		t.Fatal(err)
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/policies?mode=audit&keyword="+zzTestKeyword, nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	page := mustUnmarshal[testPageResult[policyListItem]](t, env.Data)
	items := page.Items
	if page.Total != 1 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page metadata = %+v", page)
	}
	if len(items) != 1 || items[0].Name != zzTestKeyword+"b-invalid" {
		t.Fatalf("mode filter items = %+v", items)
	}
	if items[0].UpdatedAt.IsZero() {
		t.Fatalf("updatedAt not populated: %+v", items[0])
	}

	_, env = doRequest(t, h, "GET", "/api/v1/policies?phase=Invalid&keyword="+zzTestKeyword, nil)
	items = mustUnmarshal[testPageResult[policyListItem]](t, env.Data).Items
	if len(items) != 1 || items[0].Name != zzTestKeyword+"b-invalid" {
		t.Fatalf("phase filter items = %+v", items)
	}

	_, env = doRequest(t, h, "GET", "/api/v1/policies?keyword="+zzTestKeyword+"a-rea", nil)
	items = mustUnmarshal[testPageResult[policyListItem]](t, env.Data).Items
	if len(items) != 1 || items[0].Name != zzTestKeyword+"a-ready" {
		t.Fatalf("keyword filter items = %+v", items)
	}
}

func TestListPoliciesMultiValueFilters(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, zzTestKeyword+"a-enforce", "enforce")
	seedPolicy(t, repos, zzTestKeyword+"b-audit", "audit")
	seedPolicy(t, repos, zzTestKeyword+"c-enforce", "enforce")

	_, env := doRequest(t, h, "GET", "/api/v1/policies?mode=enforce,audit&keyword="+zzTestKeyword, nil)
	items := mustUnmarshal[testPageResult[policyListItem]](t, env.Data).Items
	if len(items) != 3 {
		t.Fatalf("multi mode items = %+v", items)
	}

	_, env = doRequest(t, h, "GET", "/api/v1/policies?mode=enforce&keyword="+zzTestKeyword, nil)
	items = mustUnmarshal[testPageResult[policyListItem]](t, env.Data).Items
	if len(items) != 2 {
		t.Fatalf("mode=enforce items = %+v", items)
	}
	if _, ok := findPolicy(items, zzTestKeyword+"a-enforce"); !ok {
		t.Fatalf("missing a-enforce: %+v", items)
	}
	if _, ok := findPolicy(items, zzTestKeyword+"c-enforce"); !ok {
		t.Fatalf("missing c-enforce: %+v", items)
	}

	_, env = doRequest(t, h, "GET", "/api/v1/policies?mode=enforce&keyword="+zzTestKeyword+"c-en", nil)
	items = mustUnmarshal[testPageResult[policyListItem]](t, env.Data).Items
	if len(items) != 1 || items[0].Name != zzTestKeyword+"c-enforce" {
		t.Fatalf("mode+keyword items = %+v", items)
	}
}

func TestGetPolicy(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "enforce")
	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[policyDetail](t, env.Data)
	if d.Name != "p1" || d.EnforcementMode != "enforce" || d.ResourceVersion == "" {
		t.Fatalf("detail = %+v", d)
	}
}

func TestGetPolicyNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "GET", "/api/v1/policies/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestCreatePolicy(t *testing.T) {
	h, repos := newDBTestServer(t)
	rec, env := doRequest(t, h, "POST", "/api/v1/policies", validReq())
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	p, err := repos.Policies.GetByName(t.Context(), "my-policy")
	if err != nil {
		t.Fatal(err)
	}
	if !p.CompileOK {
		t.Fatalf("compileOK = %v, want true", p.CompileOK)
	}
	versions, err := repos.Policies.ListVersions(t.Context(), p.ID)
	if err != nil || len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("versions = %+v, err = %v", versions, err)
	}
}

// TestCreatePolicyNotifiesGatewayOnSuccess verifies the write handler calls
// NotifyChange with the "policy" topic after a successful commit (Task 11).
func TestCreatePolicyNotifiesGatewayOnSuccess(t *testing.T) {
	h, _, handlers := newDBTestServerWithHandlers(t)
	fake := &fakeHubClient{}
	handlers.Gateway = fake

	rec, env := doRequestWithHeaders(t, h, "POST", "/api/v1/policies", validReq(), map[string]string{"X-Request-Id": "policy-trace"})
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	topics, traces := fake.calls()
	if len(topics) != 1 || topics[0] != "policy" || len(traces) != 1 || traces[0] != "policy-trace" {
		t.Fatalf("NotifyChange topics=%v traces=%v, want policy with request trace", topics, traces)
	}
}

// TestCreatePolicySucceedsWhenGatewayNotifyFails is the core best-effort
// guarantee: a failing gRPC NotifyChange call must never fail the HTTP
// request or leave the DB write unrolled-back — the 60s gateway resync is
// the documented fallback (spec D12).
func TestCreatePolicySucceedsWhenGatewayNotifyFails(t *testing.T) {
	h, repos, handlers := newDBTestServerWithHandlers(t)
	handlers.Gateway = &fakeHubClient{notifyChangeErr: context.DeadlineExceeded}

	rec, env := doRequest(t, h, "POST", "/api/v1/policies", validReq())
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v, want a successful response despite gateway failure", rec.Code, env)
	}
	if _, err := repos.Policies.GetByName(t.Context(), "my-policy"); err != nil {
		t.Fatalf("policy row missing despite successful response: %v", err)
	}
}

func TestCreatePolicyDuplicate(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "my-policy", "audit")
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies", validReq())
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreatePolicyBadRego(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validReq()
	req.Rego = "package kubesentry\ndeny[msg] {"
	rec, env := doRequest(t, h, "POST", "/api/v1/policies", req)
	if rec.Code != 400 || env.Code == 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
}

func TestUpdatePolicy(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "audit")
	req := validReq()
	req.Name = ""
	req.EnforcementMode = "enforce"
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policies/p1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	got, _ := repos.Policies.GetByName(t.Context(), "p1")
	if got.EnforcementMode != "enforce" {
		t.Fatalf("mode = %q", got.EnforcementMode)
	}
	versions, _ := repos.Policies.ListVersions(t.Context(), got.ID)
	if len(versions) != 2 || versions[0].Version != 2 {
		t.Fatalf("versions = %+v, want a new v2 snapshot", versions)
	}
}

func TestUpdatePolicyStaleResourceVersion(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "audit")
	req := validReq()
	req.Name = ""
	req.ResourceVersion = "stale-does-not-match"
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policies/p1", req)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestUpdatePolicyNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validReq()
	req.Name = ""
	rec, _ := doRequest(t, h, "PUT", "/api/v1/policies/nope", req)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestUpdatePolicyBadRego(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "audit")
	req := validReq()
	req.Name = ""
	req.Rego = "package kubesentry\ndeny[msg] {"
	rec, env := doRequest(t, h, "PUT", "/api/v1/policies/p1", req)
	if rec.Code != 400 || env.Code == 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
}

func TestValidateEndpoint(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/validate", map[string]string{"rego": validRego})
	if rec.Code != 200 {
		t.Fatalf("valid rego: code = %d", rec.Code)
	}
	rec, env := doRequest(t, h, "POST", "/api/v1/policies/validate", map[string]string{"rego": "package kubesentry\ndeny[msg] {"})
	if rec.Code != 400 || env.Code == 0 {
		t.Fatalf("bad rego: code=%d env=%+v", rec.Code, env)
	}
}

func TestDeletePolicyReferenced(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "audit")
	group := &dbrepo.PolicyGroup{
		Name: "security",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled:  true,
			Policies: v1alpha1.PolicyGroupPolicies{ByName: []v1alpha1.PolicyRef{{Name: "p1"}}},
		}),
	}
	if err := repos.Groups.Create(t.Context(), group); err != nil {
		t.Fatal(err)
	}

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

func TestDeletePolicyExcludedNotReferenced(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "audit")
	group := &dbrepo.PolicyGroup{
		Name: "security",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName:  []v1alpha1.PolicyRef{{Name: "p1"}},
				Exclude: []string{"p1"},
			},
		}),
	}
	if err := repos.Groups.Create(t.Context(), group); err != nil {
		t.Fatal(err)
	}
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policies/p1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200 (excluded members are not references)", rec.Code)
	}
}

// TestDeletePolicyReferencedBySelector exercises the labels-column fix
// (migration 0003): a PolicyGroup that only references a policy via
// policies.bySelector now blocks deletion too, since referencingGroups uses
// groupresolve.Resolve against real Policy.Labels instead of only checking
// policies.byName.
func TestDeletePolicyReferencedBySelector(t *testing.T) {
	h, repos := newDBTestServer(t)
	p := seedPolicyBare(t, repos, "p1", "audit")
	p.Labels = datatypes.NewJSONType(map[string]string{"team": "security"})
	if err := repos.Policies.Update(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	group := &dbrepo.PolicyGroup{
		Name: "security",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				BySelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "security"}},
			},
		}),
	}
	if err := repos.Groups.Create(t.Context(), group); err != nil {
		t.Fatal(err)
	}

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
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "audit")
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policies/p1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if _, err := repos.Policies.GetByName(t.Context(), "p1"); err == nil {
		t.Fatal("policy should be deleted")
	}
}

func TestDeletePolicyNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/policies/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestListPolicyVersions(t *testing.T) {
	h, repos := newDBTestServer(t)
	p := seedPolicyBare(t, repos, "p1", "enforce")
	seedPolicyVersion(t, repos, p, 1, "audit")
	seedPolicyVersion(t, repos, p, 2, "enforce")

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p1/versions", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	vl := mustUnmarshal[versionList](t, env.Data)
	if vl.CurrentVersion != 2 || len(vl.Versions) != 2 {
		t.Fatalf("versions = %+v", vl)
	}
	if vl.Cursor != 2 || vl.Head != 2 || vl.InFlight || !vl.PrevEnabled || vl.NextEnabled {
		t.Fatalf("timeline controls = %+v", vl)
	}
	if vl.Versions[0].Version != 2 || !vl.Versions[0].IsCurrent {
		t.Fatalf("newest entry = %+v, want v2 current", vl.Versions[0])
	}
	if vl.Versions[0].Phase != "Ready" {
		t.Fatalf("newest phase = %q, want Ready", vl.Versions[0].Phase)
	}
	if vl.Versions[1].Version != 1 || vl.Versions[1].IsCurrent {
		t.Fatalf("oldest entry = %+v, want v1 not current", vl.Versions[1])
	}
}

func TestListPolicyVersionsPruned(t *testing.T) {
	h, repos := newDBTestServer(t)
	p := seedPolicyBare(t, repos, "p1", "enforce")
	seedPolicyVersion(t, repos, p, 5, "enforce") // v1-v4 已被剪枝

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p1/versions", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	vl := mustUnmarshal[versionList](t, env.Data)
	if vl.CurrentVersion != 5 || len(vl.Versions) != 1 {
		t.Fatalf("versions = %+v", vl)
	}
}

func TestRollbackPrev(t *testing.T) {
	h, repos := newDBTestServer(t)
	p := seedPolicyBare(t, repos, "p1", "enforce")
	seedPolicyVersion(t, repos, p, 1, "audit")
	seedPolicyVersion(t, repos, p, 2, "enforce")

	rec, env := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback", map[string]string{"direction": "prev"})
	if rec.Code != 200 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	got, err := repos.Policies.GetByName(t.Context(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.EnforcementMode != "audit" {
		t.Fatalf("mode = %q, want audit (restored from v1)", got.EnforcementMode)
	}
	versions, err := repos.Policies.ListVersions(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 || versions[0].Version != 3 {
		t.Fatalf("versions = %+v, want a new v3 appended", versions)
	}
}

func TestRollbackNextAlwaysRejected(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "enforce")
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback", map[string]string{"direction": "next"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestRollbackAtOldestRejected(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "enforce") // only v1 exists
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback", map[string]string{"direction": "prev"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestRollbackTargetPruned(t *testing.T) {
	h, repos := newDBTestServer(t)
	p := seedPolicyBare(t, repos, "p1", "enforce")
	seedPolicyVersion(t, repos, p, 5, "enforce") // v4 已被剪枝
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback", map[string]string{"direction": "prev"})
	if rec.Code != 410 {
		t.Fatalf("code = %d, want 410", rec.Code)
	}
}

func TestRollbackBadDirection(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedPolicy(t, repos, "p1", "enforce")
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/p1/rollback", map[string]string{"direction": "jump"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestRollbackNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/policies/nope/rollback", map[string]string{"direction": "prev"})
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func containsStr(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}

func TestResourceSuggestions(t *testing.T) {
	// The migration seeds 37 real builtin policies (see
	// internal/dbmigrate/seed_test.go), which contribute their own
	// apiGroups/resources to this aggregate — assert our fixtures' values
	// are present (and the intra-policy duplicate "pods" entry collapses to
	// one), rather than asserting the aggregate equals only our fixtures.
	h, repos := newDBTestServer(t)
	p1 := seedPolicyBare(t, repos, "zz-test-p1", "audit")
	p1.Match = datatypes.NewJSONType(v1alpha1.PolicyMatch{
		Operations: []string{"CREATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods", "zz-test-services"}},
		},
	})
	if err := repos.Policies.Update(t.Context(), p1); err != nil {
		t.Fatal(err)
	}
	p2 := seedPolicyBare(t, repos, "zz-test-p2", "audit")
	p2.Match = datatypes.NewJSONType(v1alpha1.PolicyMatch{
		Operations: []string{"CREATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{"zz-test-apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}, // 与 p1 重复，验证去重
		},
	})
	if err := repos.Policies.Update(t.Context(), p2); err != nil {
		t.Fatal(err)
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/resource-suggestions", nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
	got := mustUnmarshal[resourceSuggestionsResponse](t, env.Data)

	if !containsStr(got.APIGroups, "") || !containsStr(got.APIGroups, "zz-test-apps") {
		t.Fatalf("apiGroups = %v, want to contain \"\" and zz-test-apps", got.APIGroups)
	}
	if !containsStr(got.APIVersions, "v1") {
		t.Fatalf("apiVersions = %v, want to contain v1", got.APIVersions)
	}
	if !containsStr(got.Resources, "deployments") || !containsStr(got.Resources, "pods") || !containsStr(got.Resources, "zz-test-services") {
		t.Fatalf("resources = %v, want to contain deployments/pods/zz-test-services", got.Resources)
	}

	// Group "" (core) is shared with builtin policies, so only assert our
	// fixtures' entries are present (deduped) — not that the group is
	// exclusively ours.
	core := got.ResourcesByGroup[""]
	if !containsStr(core, "pods") || !containsStr(core, "zz-test-services") {
		t.Fatalf("resourcesByGroup[\"\"] = %v, want to contain pods and zz-test-services", core)
	}
	if want := []string{"deployments"}; !reflect.DeepEqual(got.ResourcesByGroup["zz-test-apps"], want) {
		t.Fatalf("resourcesByGroup[\"zz-test-apps\"] = %v, want %v", got.ResourcesByGroup["zz-test-apps"], want)
	}
}

// TestPolicyAnnotationsRoundTrip restores the CRD-era annotations round-trip
// test (dropped when Task 9 first converted these handlers, since
// dbrepo.Policy had no annotations column at the time) now that migration
// 0003 added Policy.Annotations. Mirrors the pre-Task-9 test at
// git show 761e4b6:internal/console/handlers_policy_test.go.
func TestPolicyAnnotationsRoundTrip(t *testing.T) {
	h, _ := newDBTestServer(t)

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

// TestCreatePolicyPersistsLabels covers the other half of migration 0003:
// Labels round-trip through create/get, which is what makes
// TestDeletePolicyReferencedBySelector (and real bySelector group
// resolution via internal/groupresolve) possible again.
func TestCreatePolicyPersistsLabels(t *testing.T) {
	h, repos := newDBTestServer(t)
	create := map[string]any{
		"name":            "p-lbl",
		"enforcementMode": "audit",
		"rego":            validRego,
		"labels":          map[string]string{"team": "security"},
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
	p, err := repos.Policies.GetByName(t.Context(), "p-lbl")
	if err != nil {
		t.Fatal(err)
	}
	if p.Labels.Data()["team"] != "security" {
		t.Fatalf("labels = %+v", p.Labels.Data())
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/policies/p-lbl", nil)
	if rec.Code != 200 {
		t.Fatalf("get: code=%d", rec.Code)
	}
	d := mustUnmarshal[policyDetail](t, env.Data)
	if d.Labels["team"] != "security" {
		t.Fatalf("response labels = %+v", d.Labels)
	}
}
