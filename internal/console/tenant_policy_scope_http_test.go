package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

func TestTenantPolicyResourceBoundaries(t *testing.T) {
	repos := repository.New(newTestDB(t))
	server, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	h := server.Handler
	password := "correct horse battery staple"
	request := func(method, path string, body any, cookie *http.Cookie, csrf bool) *httptest.ResponseRecorder {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(payload))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if csrf {
			req.Header.Set("X-KubeSentry-CSRF", "1")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(http.MethodPost, "/api/v1/setup/admin", dto.PasswordInput{Password: password}, nil, false); rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	admin := loginCookie(t, h, "admin", password)

	createTenant := func(name string) dto.TenantView {
		rec := request(http.MethodPost, "/api/v1/tenants", dto.CreateTenantRequest{Name: name}, admin, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("create tenant %s = %d %s", name, rec.Code, rec.Body.String())
		}
		return responseData[dto.TenantView](t, rec.Body.Bytes())
	}
	first, second := createTenant("policy-first"), createTenant("policy-second")
	createUser := func(username string, role dbrepo.Role, tenantID int64) *http.Cookie {
		rec := request(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{Username: username, Role: role, TenantID: &tenantID}, admin, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("create user %s = %d %s", username, rec.Code, rec.Body.String())
		}
		result := responseData[dto.CreateUserResult](t, rec.Body.Bytes())
		activated := request(http.MethodPost, "/api/v1/auth/activate", dto.ActivateInput{Code: result.ActivationCode, Password: password}, nil, false)
		if activated.Code != http.StatusOK {
			t.Fatalf("activate user %s = %d %s", username, activated.Code, activated.Body.String())
		}
		return loginCookie(t, h, username, password)
	}
	firstAdmin := createUser("policy-first-admin", dbrepo.RoleTenantAdmin, first.ID)
	secondAdmin := createUser("policy-second-admin", dbrepo.RoleTenantAdmin, second.ID)
	viewer := createUser("policy-first-viewer", dbrepo.RoleViewer, first.ID)

	for _, cluster := range []*dbrepo.Cluster{
		{Name: "policy-first-cluster", TenantID: &first.ID, Phase: gatewaymodel.ClusterPhaseOnline, Labels: datatypes.NewJSONType(map[string]string{})},
		{Name: "policy-second-cluster", TenantID: &second.ID, Phase: gatewaymodel.ClusterPhaseOnline, Labels: datatypes.NewJSONType(map[string]string{})},
	} {
		if err := repos.Clusters.Create(t.Context(), cluster); err != nil {
			t.Fatal(err)
		}
	}

	policy := validReq()
	policy.Name = "tenant-shared-policy"
	if rec := request(http.MethodPost, "/api/v1/policies", policy, firstAdmin, true); rec.Code != http.StatusOK {
		t.Fatalf("tenant policy create = %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, "/api/v1/policies/tenant-shared-policy", nil, secondAdmin, false); rec.Code != http.StatusOK {
		t.Fatalf("shared policy get = %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodPost, "/api/v1/policies", validReq(), viewer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer policy create = %d %s", rec.Code, rec.Body.String())
	}

	group := validGroupReq()
	group.Name = "second-private-group"
	group.Policies.ByName = []v1alpha1.PolicyRef{{Name: "tenant-shared-policy"}}
	if rec := request(http.MethodPost, "/api/v1/policygroups", group, secondAdmin, true); rec.Code != http.StatusOK {
		t.Fatalf("second tenant group create = %d %s", rec.Code, rec.Body.String())
	}
	listRec := request(http.MethodGet, "/api/v1/policygroups", nil, firstAdmin, false)
	groupPage := responseData[dto.PolicyGroupPageView](t, listRec.Body.Bytes())
	if _, found := findGroup(groupPage.Items, "second-private-group"); found {
		t.Fatalf("cross-tenant group leaked in list: %+v", groupPage.Items)
	}
	if rec := request(http.MethodPut, "/api/v1/policygroups/default", validGroupReq(), firstAdmin, true); rec.Code != http.StatusForbidden {
		t.Fatalf("tenant default group update = %d %s", rec.Code, rec.Body.String())
	}

	policy.Name = ""
	if rec := request(http.MethodPut, "/api/v1/policies/tenant-shared-policy", policy, firstAdmin, true); rec.Code != http.StatusConflict {
		t.Fatalf("cross-tenant referenced policy update = %d %s", rec.Code, rec.Body.String())
	}

	distribution := &testDistributionRequest{Name: "cross-tenant-target", Spec: gatewaymodel.PolicyDistributionSpec{Clusters: gatewaymodel.DistributionClusters{ByName: []string{"policy-second-cluster"}}, PolicyGroups: []string{"default"}}}
	if rec := request(http.MethodPost, "/api/v1/distributions", distribution, firstAdmin, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-tenant distribution target = %d %s", rec.Code, rec.Body.String())
	}
}
