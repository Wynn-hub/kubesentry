// internal/console/handlers_exception_test.go
package console

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func validExceptionReq() *exceptionRequest {
	return &exceptionRequest{
		Name:       "my-exception",
		PolicyRefs: []string{"run-as-privileged"},
		Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{"dev"}},
		Duration:   "24h",
		Reason:     "temporary migration",
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
	h, c := newTestServer(t)

	rec, _ := doRequest(t, h, "POST", "/api/v1/exceptions", validExceptionReq())
	if rec.Code != 200 {
		t.Fatalf("create code = %d", rec.Code)
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/exceptions", nil)
	if rec.Code != 200 {
		t.Fatalf("list code = %d", rec.Code)
	}
	items := mustUnmarshal[[]exceptionListItem](t, env.Data)
	if len(items) != 1 || items[0].Name != "my-exception" {
		t.Fatalf("items = %+v", items)
	}

	req := validExceptionReq()
	req.Name = ""
	req.Reason = "extended migration"
	rec, _ = doRequest(t, h, "PUT", "/api/v1/exceptions/my-exception", req)
	if rec.Code != 200 {
		t.Fatalf("update code = %d", rec.Code)
	}
	var pex v1alpha1.PolicyException
	_ = c.Get(t.Context(), client.ObjectKey{Name: "my-exception"}, &pex)
	if pex.Spec.Reason != "extended migration" {
		t.Fatalf("reason = %q", pex.Spec.Reason)
	}

	rec, _ = doRequest(t, h, "DELETE", "/api/v1/exceptions/my-exception", nil)
	if rec.Code != 200 {
		t.Fatalf("delete code = %d", rec.Code)
	}
}

func TestGetExceptionDetail(t *testing.T) {
	pex := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: "e1"},
		Spec: v1alpha1.PolicyExceptionSpec{
			AllPolicies: true,
			Duration:    "1h",
			Reason:      "drill",
		},
		Status: v1alpha1.PolicyExceptionStatus{Phase: "Active"},
	}
	h, _ := newTestServer(t, pex)
	rec, env := doRequest(t, h, "GET", "/api/v1/exceptions/e1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[exceptionDetail](t, env.Data)
	if !d.Spec.AllPolicies || d.Status.Phase != "Active" {
		t.Fatalf("detail = %+v", d)
	}
}
