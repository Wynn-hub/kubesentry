// internal/console/handlers_exception_test.go
package console

import (
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func validExceptionReq() *exceptionRequest {
	return &exceptionRequest{
		AllClusters: true,
		Name:        "my-exception",
		PolicyRefs:  []string{"run-as-privileged"},
		Match:       v1alpha1.PolicyExceptionMatch{Namespaces: []string{"dev"}},
		Duration:    "24h",
		Reason:      "temporary migration",
	}
}

func TestValidateExceptionRequest(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*exceptionRequest)
		wantErr string
	}{
		{"valid", func(r *exceptionRequest) {}, ""},
		{"no target", func(r *exceptionRequest) { r.PolicyRefs = nil }, "exactly one"},
		{"two targets", func(r *exceptionRequest) { r.AllPolicies = true }, "exactly one"},
		{"bad duration", func(r *exceptionRequest) { r.Duration = "soon" }, "duration"},
		{"zero duration", func(r *exceptionRequest) { r.Duration = "0s" }, "duration"},
		{"blank reason", func(r *exceptionRequest) { r.Reason = "  " }, "reason"},
		{"bad retain", func(r *exceptionRequest) { r.RetainAfterExpiry = "forever" }, "retainAfterExpiry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validExceptionReq()
			tc.mutate(req)
			err := validateExceptionRequest(req, true)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tc.wantErr)
			}
		})
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
	page := mustUnmarshal[pageResult[exceptionListItem]](t, env.Data)
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
	if rec.Code != 400 || env.Error == nil {
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
	d := mustUnmarshal[exceptionDetail](t, env.Data)
	if !d.Spec.AllPolicies || d.Status.Phase != "Active" {
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
	d := mustUnmarshal[exceptionDetail](t, env.Data)
	if d.Status.Phase != "Expired" {
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
