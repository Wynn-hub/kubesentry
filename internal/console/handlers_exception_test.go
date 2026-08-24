// internal/console/handlers_exception_test.go
package console

import (
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type testExceptionRequest struct {
	Name              string                        `json:"name,omitempty"`
	PolicyRefs        []string                      `json:"policyRefs,omitempty"`
	PolicyGroupRefs   []string                      `json:"policyGroupRefs,omitempty"`
	AllPolicies       bool                          `json:"allPolicies,omitempty"`
	Match             v1alpha1.PolicyExceptionMatch `json:"match"`
	Duration          string                        `json:"duration"`
	RetainAfterExpiry string                        `json:"retainAfterExpiry,omitempty"`
	Reason            string                        `json:"reason"`
	ResourceVersion   string                        `json:"resourceVersion,omitempty"`
	AllClusters       bool                          `json:"allClusters,omitempty"`
	ClusterNames      []string                      `json:"clusterNames,omitempty"`
}

func validExceptionReq() *testExceptionRequest {
	return &testExceptionRequest{
		AllClusters: true,
		Name:        "my-exception",
		PolicyRefs:  []string{"run-as-privileged"},
		Match:       v1alpha1.PolicyExceptionMatch{Namespaces: []string{"dev"}},
		Duration:    "24h",
		Reason:      "temporary migration",
	}
}

func TestExceptionCRUD(t *testing.T) {
	h, repos := newDBTestServer(t)

	before := time.Now()
	rec, _ := doRequest(t, h, "POST", "/api/v1/exceptions", validExceptionReq())
	if rec.Code != 200 {
		t.Fatalf("create code = %d", rec.Code)
	}

	created, err := repos.Exceptions.GetByName(t.Context(), "my-exception")
	if err != nil {
		t.Fatal(err)
	}
	if created.ExpiresAt == nil || created.ExpiresAt.Before(before.Add(23*time.Hour)) {
		t.Fatalf("expiresAt = %v, want ~24h from now", created.ExpiresAt)
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/exceptions", nil)
	if rec.Code != 200 {
		t.Fatalf("list code = %d", rec.Code)
	}
	// The DB is seeded with a permanent "default-system-namespaces"
	// exception (migration 0004), so assert containment rather than an
	// exact count of 1.
	page := mustUnmarshal[testPageResult[dto.ExceptionListItem]](t, env.Data)
	items := page.Items
	if page.Total < 1 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page metadata = %+v", page)
	}
	found := false
	for _, item := range items {
		found = found || item.Name == "my-exception"
	}
	if !found {
		t.Fatalf("items = %+v, want to contain my-exception", items)
	}

	req := validExceptionReq()
	req.Name = ""
	req.Reason = "extended migration"
	rec, _ = doRequest(t, h, "PUT", "/api/v1/exceptions/my-exception", req)
	if rec.Code != 200 {
		t.Fatalf("update code = %d", rec.Code)
	}
	updated, _ := repos.Exceptions.GetByName(t.Context(), "my-exception")
	if updated.Spec.Data().Reason != "extended migration" {
		t.Fatalf("reason = %q", updated.Spec.Data().Reason)
	}

	rec, _ = doRequest(t, h, "DELETE", "/api/v1/exceptions/my-exception", nil)
	if rec.Code != 200 {
		t.Fatalf("delete code = %d", rec.Code)
	}
	if _, err := repos.Exceptions.GetByName(t.Context(), "my-exception"); err == nil {
		t.Fatal("exception should be deleted")
	}
}

func TestCreateExceptionDuplicate(t *testing.T) {
	h, repos := newDBTestServer(t)
	expiresAt := time.Now().Add(time.Hour)
	if err := repos.Exceptions.Create(t.Context(), &dbrepo.PolicyException{
		Name:      "my-exception",
		Spec:      datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{AllPolicies: true, Duration: "1h", Reason: "seed"}),
		ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	rec, _ := doRequest(t, h, "POST", "/api/v1/exceptions", validExceptionReq())
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreateExceptionBadDurationRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validExceptionReq()
	req.Duration = "not-a-duration"
	rec, env := doRequest(t, h, "POST", "/api/v1/exceptions", req)
	if rec.Code != 400 || env.Code == 0 {
		t.Fatalf("code=%d env=%+v", rec.Code, env)
	}
}

func TestGetExceptionDetail(t *testing.T) {
	h, repos := newDBTestServer(t)
	expiresAt := time.Now().Add(time.Hour)
	if err := repos.Exceptions.Create(t.Context(), &dbrepo.PolicyException{
		Name: "e1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{
			AllPolicies: true,
			Duration:    "1h",
			Reason:      "drill",
		}),
		ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	rec, env := doRequest(t, h, "GET", "/api/v1/exceptions/e1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[dto.ExceptionDetail](t, env.Data)
	if !d.Spec.AllPolicies.Or(false) || d.Status.Phase != dto.ExceptionStatusPhaseActive {
		t.Fatalf("detail = %+v", d)
	}
}

func TestGetExceptionDetailExpired(t *testing.T) {
	h, repos := newDBTestServer(t)
	expiresAt := time.Now().Add(-time.Hour)
	if err := repos.Exceptions.Create(t.Context(), &dbrepo.PolicyException{
		Name:      "e2",
		Spec:      datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{AllPolicies: true, Duration: "1h", Reason: "drill"}),
		ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	rec, env := doRequest(t, h, "GET", "/api/v1/exceptions/e2", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[dto.ExceptionDetail](t, env.Data)
	if d.Status.Phase != dto.ExceptionStatusPhaseExpired {
		t.Fatalf("phase = %q, want Expired", d.Status.Phase)
	}
}

func TestGetExceptionNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "GET", "/api/v1/exceptions/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}
