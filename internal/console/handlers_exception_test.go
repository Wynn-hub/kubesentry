// internal/console/handlers_exception_test.go
package console

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
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

func TestExceptionMutationsNotifyGateway(t *testing.T) {
	h, _, handlers := newDBTestServerWithHandlers(t)
	fake := &fakeHubClient{}
	handlers.Gateway = fake
	headers := map[string]string{"X-Request-Id": "exception-trace"}

	create := validExceptionReq()
	update := validExceptionReq()
	update.Name = ""
	update.Reason = "updated"
	for _, mutation := range []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodPost, path: "/api/v1/exceptions", body: create},
		{method: http.MethodPut, path: "/api/v1/exceptions/my-exception", body: update},
		{method: http.MethodDelete, path: "/api/v1/exceptions/my-exception"},
	} {
		rec, env := doRequestWithHeaders(t, h, mutation.method, mutation.path, mutation.body, headers)
		if rec.Code != http.StatusOK || env.Code != 0 {
			t.Fatalf("%s %s: code=%d body=%s", mutation.method, mutation.path, rec.Code, rec.Body.String())
		}
	}

	topics, traces := fake.calls()
	if len(topics) != 3 || topics[0] != "exception" || topics[1] != "exception" || topics[2] != "exception" {
		t.Fatalf("topics = %v, want three exception notifications", topics)
	}
	for _, trace := range traces {
		if trace != "exception-trace" {
			t.Fatalf("traces = %v, want request trace preserved", traces)
		}
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

func TestCreateExceptionRejectsInvalidTargetBeforeMutation(t *testing.T) {
	h, repos := newDBTestServer(t)
	req := validExceptionReq()
	req.AllClusters = false
	req.ClusterNames = []string{"missing-cluster"}
	rec, env := doRequest(t, h, "POST", "/api/v1/exceptions", req)
	if rec.Code != 400 || !strings.Contains(env.Message, "target cluster") {
		t.Fatalf("code=%d message=%q, want target cluster validation", rec.Code, env.Message)
	}
	if _, err := repos.Exceptions.GetByName(t.Context(), req.Name); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("invalid request must not create exception, got err=%v", err)
	}
}

func TestCreateExceptionRollsBackWhenScopeWriteFails(t *testing.T) {
	h, repos := newDBTestServer(t)
	if err := repos.Distributions.Create(t.Context(), &dbrepo.PolicyDistribution{
		Name: "scope-exception-my-exception",
		Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{}),
	}); err != nil {
		t.Fatal(err)
	}
	rec, _ := doRequest(t, h, "POST", "/api/v1/exceptions", validExceptionReq())
	if rec.Code != 500 {
		t.Fatalf("code=%d body=%s, want scope write failure", rec.Code, rec.Body.String())
	}
	if _, err := repos.Exceptions.GetByName(t.Context(), "my-exception"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("exception must roll back with failed scope write, got err=%v", err)
	}
}

func TestListExceptionsHonorsSortFieldsAndStableNameTieBreak(t *testing.T) {
	h, repos := newDBTestServer(t)
	now := time.Now().UTC()
	rows := []dbrepo.PolicyException{
		{Name: "sort-alpha", Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{AllPolicies: true, Duration: "24h", Reason: "a"})},
		{Name: "sort-beta", Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{PolicyRefs: []string{"p1"}, Duration: "2h", Reason: "b"}), ExpiresAt: func() *time.Time { v := now.Add(time.Hour); return &v }()},
		{Name: "sort-gamma", Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{PolicyGroupRefs: []string{"g1"}, Duration: "1h", Reason: "c"}), ExpiresAt: func() *time.Time { v := now.Add(-time.Hour); return &v }()},
	}
	for i := range rows {
		if err := repos.Exceptions.Create(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "name", path: "/api/v1/exceptions?pageSize=100&sortBy=name&sortOrder=desc", want: []string{"sort-gamma", "sort-beta", "sort-alpha"}},
		{name: "phase stable name", path: "/api/v1/exceptions?pageSize=100&sortBy=phase&sortOrder=asc", want: []string{"sort-alpha", "sort-beta", "sort-gamma"}},
		{name: "target summary", path: "/api/v1/exceptions?pageSize=100&sortBy=targetSummary&sortOrder=asc", want: []string{"sort-alpha", "sort-gamma", "sort-beta"}},
		{name: "duration numeric", path: "/api/v1/exceptions?pageSize=100&sortBy=duration&sortOrder=asc", want: []string{"sort-gamma", "sort-beta", "sort-alpha"}},
		{name: "expires at empty first", path: "/api/v1/exceptions?pageSize=100&sortBy=expiresAt&sortOrder=asc", want: []string{"sort-alpha", "sort-gamma", "sort-beta"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec, env := doRequest(t, h, "GET", test.path, nil)
			if rec.Code != 200 {
				t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
			}
			items := mustUnmarshal[testPageResult[dto.ExceptionListItem]](t, env.Data).Items
			got := make([]string, 0, len(test.want))
			for _, item := range items {
				if strings.HasPrefix(item.Name, "sort-") {
					got = append(got, item.Name)
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("order=%v, want %v", got, test.want)
			}
		})
	}
}

func TestListExceptionsRendersAndSortsMultiReferenceTargetSummary(t *testing.T) {
	h, repos := newDBTestServer(t)
	rows := []dbrepo.PolicyException{
		{Name: "multi-a", Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{PolicyRefs: []string{"alpha", "zulu", "zulu2", "zulu3"}, Duration: "1h", Reason: "a"})},
		{Name: "multi-b", Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{PolicyRefs: []string{"alpha", "aardvark"}, Duration: "1h", Reason: "b"})},
		{Name: "multi-c", Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{PolicyGroupRefs: []string{"blue", "green", "orange", "purple", "red"}, Duration: "1h", Reason: "c"})},
	}
	for i := range rows {
		if err := repos.Exceptions.Create(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/exceptions?pageSize=100&sortBy=targetSummary&sortOrder=asc", nil)
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	items := mustUnmarshal[testPageResult[dto.ExceptionListItem]](t, env.Data).Items
	gotOrder := make([]string, 0, len(rows))
	gotSummaries := make(map[string]string, len(rows))
	for _, item := range items {
		if strings.HasPrefix(item.Name, "multi-") {
			gotOrder = append(gotOrder, item.Name)
			gotSummaries[item.Name] = item.TargetSummary
		}
	}
	wantOrder := []string{"multi-c", "multi-b", "multi-a"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("targetSummary order=%v, want %v", gotOrder, wantOrder)
	}
	wantSummaries := map[string]string{
		"multi-a": "policies: alpha, zulu, zulu2 (+1 more)",
		"multi-b": "policies: alpha, aardvark",
		"multi-c": "groups: blue, green, orange (+2 more)",
	}
	if !reflect.DeepEqual(gotSummaries, wantSummaries) {
		t.Fatalf("target summaries=%v, want %v", gotSummaries, wantSummaries)
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
