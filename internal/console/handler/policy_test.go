package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/handler"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// ---------------------------------------------------------------------------
// Fakes. Minimal copies of the service package's test fakes — Go test helpers
// cannot cross package boundaries, and these tests exercise the HTTP layer, so
// only the repository behaviour the handlers reach through is reproduced.
// ---------------------------------------------------------------------------

type fakePolicyRepo struct {
	byName   map[string]*dbrepo.Policy
	versions map[int64][]dbrepo.PolicyVersion
	nextID   int64
	// listErr forces the ListPage path to fail so the 500 mapping is reachable.
	listErr error
	// prunedVersions drops snapshots from ListVersions while keeping the
	// version counter high, which is how a pruned rollback target (410) looks.
	prunedVersions bool
}

func newFakePolicyRepo() *fakePolicyRepo {
	return &fakePolicyRepo{
		byName:   map[string]*dbrepo.Policy{},
		versions: map[int64][]dbrepo.PolicyVersion{},
		nextID:   1,
	}
}

func (f *fakePolicyRepo) ListPage(ctx context.Context, opts dbrepo.PolicyPageOptions) ([]dbrepo.Policy, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	all := make([]dbrepo.Policy, 0, len(f.byName))
	for _, p := range f.byName {
		all = append(all, *p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all, int64(len(all)), nil
}

func (f *fakePolicyRepo) List(ctx context.Context) ([]dbrepo.Policy, error) {
	all, _, err := f.ListPage(ctx, dbrepo.PolicyPageOptions{})
	return all, err
}

func (f *fakePolicyRepo) GetByName(ctx context.Context, name string) (*dbrepo.Policy, error) {
	p, ok := f.byName[name]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return p, nil
}

func (f *fakePolicyRepo) CreateWithVersion(ctx context.Context, p *dbrepo.Policy, v *dbrepo.PolicyVersion) error {
	if _, dup := f.byName[p.Name]; dup {
		return &pgconn.PgError{
			Code:    "23505",
			Message: `duplicate key value violates unique constraint "idx_policies_name"`,
		}
	}
	p.ID = f.nextID
	f.nextID++
	p.UpdatedAt = time.Now()
	f.byName[p.Name] = p
	v.PolicyID = p.ID
	f.versions[p.ID] = append(f.versions[p.ID], *v)
	return nil
}

func (f *fakePolicyRepo) UpdateWithVersion(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any, v *dbrepo.PolicyVersion) error {
	for _, p := range f.byName {
		if p.ID != id {
			continue
		}
		if !p.UpdatedAt.Equal(expectedUpdatedAt) {
			return dbrepo.ErrConflict
		}
		p.UpdatedAt = time.Now()
		for k, val := range fields {
			switch k {
			case "match":
				p.Match = val.(datatypes.JSONType[v1alpha1.PolicyMatch])
			case "enforcement_mode":
				p.EnforcementMode = val.(string)
			case "rego":
				p.Rego = val.(string)
			case "description":
				p.Description = val.(string)
			case "compile_ok":
				p.CompileOK = val.(bool)
			case "compile_error":
				p.CompileError = val.(string)
			case "labels":
				p.Labels = val.(datatypes.JSONType[map[string]string])
			case "annotations":
				p.Annotations = val.(datatypes.JSONType[map[string]string])
			}
		}
		v.PolicyID = id
		f.versions[id] = append(f.versions[id], *v)
		return nil
	}
	return gorm.ErrRecordNotFound
}

func (f *fakePolicyRepo) Delete(ctx context.Context, id int64) error {
	for name, p := range f.byName {
		if p.ID == id {
			delete(f.byName, name)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

// ListVersions returns newest-first, matching dbrepo.PolicyRepo's ordering.
func (f *fakePolicyRepo) ListVersions(ctx context.Context, policyID int64) ([]dbrepo.PolicyVersion, error) {
	out := append([]dbrepo.PolicyVersion(nil), f.versions[policyID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	if f.prunedVersions && len(out) > 1 {
		// Keep only the newest so `current-1` has no snapshot left.
		out = out[:1]
	}
	return out, nil
}

type fakeGroupRepo struct{ groups []dbrepo.PolicyGroup }

func (f *fakeGroupRepo) List(ctx context.Context) ([]dbrepo.PolicyGroup, error) {
	return f.groups, nil
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

const testRego = "package kubesentry\n\ndeny[msg] {\n\tfalse\n\tmsg := \"test\"\n}"

// apiPrefix must be passed explicitly: ogen does NOT derive the router prefix
// from the spec's servers[0].url — serverConfig.Prefix defaults to "" and the
// generated router matches bare "/policies". Task 7's real wiring needs the
// same dto.WithPathPrefix call for /api/v1 URLs to resolve.
const apiPrefix = "/api/v1"

func newServerWith(t *testing.T, repo *fakePolicyRepo, groups *fakeGroupRepo) http.Handler {
	t.Helper()
	svc := service.NewPolicyService(repo, groups, nil)
	srv, err := dto.NewServer(
		handler.New(service.Services{Policy: svc}),
		dto.WithPathPrefix(apiPrefix),
	)
	if err != nil {
		t.Fatalf("dto.NewServer: %v", err)
	}
	return srv
}

func newPolicyTestServer(t *testing.T) http.Handler {
	t.Helper()
	return newServerWith(t, newFakePolicyRepo(), &fakeGroupRepo{})
}

func doJSON(t *testing.T, srv http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Details []string        `json:"details"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope from %q: %v", rec.Body.String(), err)
	}
	return env
}

func policyBody(name, rego string, operations []string) map[string]any {
	return map[string]any{
		"name":            name,
		"enforcementMode": "audit",
		"rego":            rego,
		"match": map[string]any{
			"operations": operations,
			"resources": []map[string]any{
				{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
			},
		},
	}
}

// seedPolicy creates one policy through the API so the repo state matches what
// the service would produce (version snapshot included).
func seedPolicy(t *testing.T, srv http.Handler, name string) {
	t.Helper()
	rec := doJSON(t, srv, "POST", "/api/v1/policies", policyBody(name, testRego, []string{"CREATE"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed %s: status = %d, body = %s", name, rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Success paths — one per handler method
// ---------------------------------------------------------------------------

func TestListPoliciesEmpty(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "GET", "/api/v1/policies?page=1&pageSize=20", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
	if env.Message != "success" {
		t.Fatalf("message = %q, want success", env.Message)
	}
}

func TestCreatePolicySuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "POST", "/api/v1/policies", policyBody("ok-policy", testRego, []string{"CREATE"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
	var data struct{ Name string }
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.Name != "ok-policy" {
		t.Fatalf("data.name = %q, want ok-policy", data.Name)
	}
}

func TestGetPolicySuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "get-me")

	rec := doJSON(t, srv, "GET", "/api/v1/policies/get-me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
	var detail struct {
		Name         string   `json:"name"`
		ReferencedBy []string `json:"referencedBy"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if detail.Name != "get-me" {
		t.Fatalf("data.name = %q, want get-me", detail.Name)
	}
	if detail.ReferencedBy == nil {
		t.Fatal("referencedBy serialized as null, want []")
	}
}

func TestUpdatePolicySuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "update-me")

	rec := doJSON(t, srv, "PUT", "/api/v1/policies/update-me", policyBody("update-me", testRego, []string{"CREATE", "UPDATE"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
}

func TestDeletePolicySuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "delete-me")

	rec := doJSON(t, srv, "DELETE", "/api/v1/policies/delete-me", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
	// Gone from the store.
	if rec := doJSON(t, srv, "GET", "/api/v1/policies/delete-me", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("after delete: status = %d, want 404", rec.Code)
	}
}

func TestListPolicyVersionsSuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "versioned")

	rec := doJSON(t, srv, "GET", "/api/v1/policies/versioned/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	var data struct {
		CurrentVersion int `json:"currentVersion"`
		Versions       []struct {
			Version   int  `json:"version"`
			IsCurrent bool `json:"isCurrent"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.CurrentVersion != 1 {
		t.Fatalf("currentVersion = %d, want 1", data.CurrentVersion)
	}
	if len(data.Versions) != 1 || !data.Versions[0].IsCurrent {
		t.Fatalf("versions = %+v, want one current entry", data.Versions)
	}
}

func TestRollbackPolicySuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "rollback-me")
	// A second version so "prev" has somewhere to go.
	if rec := doJSON(t, srv, "PUT", "/api/v1/policies/rollback-me", policyBody("rollback-me", testRego, []string{"UPDATE"})); rec.Code != http.StatusOK {
		t.Fatalf("seed v2: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec := doJSON(t, srv, "POST", "/api/v1/policies/rollback-me/rollback", map[string]any{"direction": "prev"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	var data struct {
		RestoredFromVersion int `json:"restoredFromVersion"`
		NewVersion          int `json:"newVersion"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.RestoredFromVersion != 1 || data.NewVersion != 3 {
		t.Fatalf("rollback result = %+v, want restoredFrom 1 / new 3", data)
	}
}

func TestValidatePolicyRegoSuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "POST", "/api/v1/policies/validate", map[string]any{"rego": testRego})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); env.Code != 0 {
		t.Fatalf("code = %d, want 0", env.Code)
	}
}

func TestGetResourceSuggestionsSuccess(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "suggest-source")

	rec := doJSON(t, srv, "GET", "/api/v1/policies/resource-suggestions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	var data struct {
		Resources []string `json:"resources"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if len(data.Resources) != 1 || data.Resources[0] != "pods" {
		t.Fatalf("resources = %v, want [pods]", data.Resources)
	}
}

// ---------------------------------------------------------------------------
// 404
// ---------------------------------------------------------------------------

func TestGetPolicyNotFound(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "GET", "/api/v1/policies/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 40400 {
		t.Fatalf("code = %d, want 40400", env.Code)
	}
	// Message fidelity: the service's sentinel text, not a handler rewrite.
	if env.Message != "not found" {
		t.Fatalf("message = %q, want %q", env.Message, "not found")
	}
}

func TestDeletePolicyNotFound(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "DELETE", "/api/v1/policies/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Code != 40400 {
		t.Fatalf("code = %d, want 40400", env.Code)
	}
}

func TestListPolicyVersionsNotFound(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "GET", "/api/v1/policies/nope/versions", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if env := decodeEnvelope(t, rec); env.Code != 40400 {
		t.Fatalf("code = %d, want 40400", env.Code)
	}
}

func TestUpdatePolicyNotFound(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "PUT", "/api/v1/policies/nope", policyBody("nope", testRego, []string{"CREATE"}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); env.Code != 40400 {
		t.Fatalf("code = %d, want 40400", env.Code)
	}
}

func TestRollbackPolicyNotFound(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "POST", "/api/v1/policies/nope/rollback", map[string]any{"direction": "prev"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); env.Code != 40400 {
		t.Fatalf("code = %d, want 40400", env.Code)
	}
}

// ---------------------------------------------------------------------------
// 400
// ---------------------------------------------------------------------------

func TestCreatePolicyInvalidRego(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "POST", "/api/v1/policies", policyBody("bad-rego", "[[[", []string{"CREATE"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 40000 {
		t.Fatalf("code = %d, want 40000", env.Code)
	}
}

func TestValidatePolicyRegoInvalid(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "POST", "/api/v1/policies/validate", map[string]any{"rego": "not rego at all ["})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); env.Code != 40000 {
		t.Fatalf("code = %d, want 40000", env.Code)
	}
}

// direction=next is always rejected by the service (roll-forward only).
func TestRollbackPolicyNextRejected(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "no-next")

	rec := doJSON(t, srv, "POST", "/api/v1/policies/no-next/rollback", map[string]any{"direction": "next"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 40000 {
		t.Fatalf("code = %d, want 40000", env.Code)
	}
	if env.Message != "already at the latest version" {
		t.Fatalf("message = %q", env.Message)
	}
}

func TestRollbackPolicyOldestRejected(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "only-v1")

	rec := doJSON(t, srv, "POST", "/api/v1/policies/only-v1/rollback", map[string]any{"direction": "prev"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Message != "already at the oldest version" {
		t.Fatalf("message = %q", env.Message)
	}
}

// Pagination is protocol semantics: pageSize outside {10,20,50,100} is a 400.
func TestListPoliciesBadPageSize(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "GET", "/api/v1/policies?pageSize=7", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPoliciesBadPage(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "GET", "/api/v1/policies?page=0", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListPoliciesBadSortBy(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "GET", "/api/v1/policies?sortBy=droptable", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// D1: the operations enum was tightened to the four verbs, so "*" must be
// rejected during ogen's decode — before any handler or service code runs.
func TestCreatePolicyRejectsWildcardOperation(t *testing.T) {
	srv := newPolicyTestServer(t)
	rec := doJSON(t, srv, "POST", "/api/v1/policies", policyBody("star-ops", testRego, []string{"*"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — wildcard must be rejected at decode time (D1), body = %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 409
// ---------------------------------------------------------------------------

func TestCreatePolicyDuplicateConflict(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "dup")

	rec := doJSON(t, srv, "POST", "/api/v1/policies", policyBody("dup", testRego, []string{"CREATE"}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 40900 {
		t.Fatalf("code = %d, want 40900", env.Code)
	}
	// The dbrepo sentinel's internal text must not leak into the API message.
	if env.Message != "policy dup already exists" {
		t.Fatalf("message = %q, want %q", env.Message, "policy dup already exists")
	}
}

func TestUpdatePolicyStaleResourceVersionConflict(t *testing.T) {
	srv := newPolicyTestServer(t)
	seedPolicy(t, srv, "cas")

	body := policyBody("cas", testRego, []string{"CREATE"})
	body["resourceVersion"] = "999999999999"
	rec := doJSON(t, srv, "PUT", "/api/v1/policies/cas", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 40900 {
		t.Fatalf("code = %d, want 40900", env.Code)
	}
	if env.Message != "policy has been modified, refresh and retry" {
		t.Fatalf("message = %q", env.Message)
	}
}

// H3: a blocked delete returns 409 with the referencing groups in data.
func TestDeletePolicyReferencedByConflict(t *testing.T) {
	repo := newFakePolicyRepo()
	groups := &fakeGroupRepo{groups: []dbrepo.PolicyGroup{{
		Name: "group-a",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "referenced"}},
			},
		}),
	}}}
	srv := newServerWith(t, repo, groups)
	seedPolicy(t, srv, "referenced")

	rec := doJSON(t, srv, "DELETE", "/api/v1/policies/referenced", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 40900 {
		t.Fatalf("code = %d, want 40900", env.Code)
	}
	var data struct {
		ReferencedBy []string `json:"referencedBy"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode data from %s: %v", env.Data, err)
	}
	if len(data.ReferencedBy) != 1 || data.ReferencedBy[0] != "group-a" {
		t.Fatalf("data.referencedBy = %v, want [group-a]", data.ReferencedBy)
	}
}

// force=true skips the reference check.
func TestDeletePolicyForceSkipsReferenceCheck(t *testing.T) {
	repo := newFakePolicyRepo()
	groups := &fakeGroupRepo{groups: []dbrepo.PolicyGroup{{
		Name: "group-a",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: "referenced"}},
			},
		}),
	}}}
	srv := newServerWith(t, repo, groups)
	seedPolicy(t, srv, "referenced")

	rec := doJSON(t, srv, "DELETE", "/api/v1/policies/referenced?force=true", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 410
// ---------------------------------------------------------------------------

// A rollback whose target snapshot has been pruned is 410, not 404.
func TestRollbackPolicyGone(t *testing.T) {
	repo := newFakePolicyRepo()
	srv := newServerWith(t, repo, &fakeGroupRepo{})
	seedPolicy(t, srv, "pruned")
	if rec := doJSON(t, srv, "PUT", "/api/v1/policies/pruned", policyBody("pruned", testRego, []string{"UPDATE"})); rec.Code != http.StatusOK {
		t.Fatalf("seed v2: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// v1's snapshot disappears while the counter stays at 2.
	repo.prunedVersions = true

	rec := doJSON(t, srv, "POST", "/api/v1/policies/pruned/rollback", map[string]any{"direction": "prev"})
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 41000 {
		t.Fatalf("code = %d, want 41000", env.Code)
	}
}

// ---------------------------------------------------------------------------
// 500
// ---------------------------------------------------------------------------

func TestListPoliciesInternalError(t *testing.T) {
	repo := newFakePolicyRepo()
	repo.listErr = errors.New("database is on fire")
	srv := newServerWith(t, repo, &fakeGroupRepo{})

	rec := doJSON(t, srv, "GET", "/api/v1/policies", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Code != 50000 {
		t.Fatalf("code = %d, want 50000", env.Code)
	}
}
