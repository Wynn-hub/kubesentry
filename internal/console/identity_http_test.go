package console

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func authTestServer(t *testing.T) http.Handler {
	t.Helper()
	repos := repository.New(newTestDB(t))
	srv, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler
}

func loginCookie(t *testing.T, h http.Handler, username, password string) *http.Cookie {
	t.Helper()
	rec := authRequest(t, h, http.MethodPost, "/api/v1/auth/login", dto.LoginInput{Username: username, Password: password}, nil, false)
	if rec.Code != http.StatusOK || len(rec.Result().Cookies()) == 0 {
		t.Fatalf("login %s = %d %s", username, rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}

func responseData[T any](t *testing.T, recBody []byte) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(recBody, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func TestIdentityHTTPRoleBoundariesAndSessionRevocation(t *testing.T) {
	h := authTestServer(t)
	password := "correct horse battery staple"
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/setup/admin", dto.PasswordInput{Password: password}, nil, false); rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	adminCookie := loginCookie(t, h, "admin", password)

	createTenant := func(name string) dto.TenantView {
		rec := authRequest(t, h, http.MethodPost, "/api/v1/tenants", dto.CreateTenantRequest{Name: name}, adminCookie, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("create tenant %s = %d %s", name, rec.Code, rec.Body.String())
		}
		return responseData[dto.TenantView](t, rec.Body.Bytes())
	}
	first, second := createTenant("first"), createTenant("second")

	createUser := func(cookie *http.Cookie, request dto.CreateUserRequest, want int) dto.UserView {
		rec := authRequest(t, h, http.MethodPost, "/api/v1/users", request, cookie, true)
		if rec.Code != want {
			t.Fatalf("create user %s = %d %s, want %d", request.Username, rec.Code, rec.Body.String(), want)
		}
		if want != http.StatusOK {
			return dto.UserView{}
		}
		return responseData[dto.UserView](t, rec.Body.Bytes())
	}
	tenantAdmin := createUser(adminCookie, dto.CreateUserRequest{Username: "tenant-admin", Password: password, Role: dbrepo.RoleTenantAdmin, TenantID: &first.ID}, http.StatusOK)
	_ = tenantAdmin
	viewer := createUser(adminCookie, dto.CreateUserRequest{Username: "viewer", Password: password, Role: dbrepo.RoleViewer, TenantID: &first.ID}, http.StatusOK)
	otherViewer := createUser(adminCookie, dto.CreateUserRequest{Username: "other-viewer", Password: password, Role: dbrepo.RoleViewer, TenantID: &second.ID}, http.StatusOK)

	tenantCookie := loginCookie(t, h, "tenant-admin", password)
	list := authRequest(t, h, http.MethodGet, "/api/v1/users", nil, tenantCookie, false)
	users := responseData[[]dto.UserView](t, list.Body.Bytes())
	if list.Code != http.StatusOK || len(users) != 2 {
		t.Fatalf("tenant user list = %d %+v", list.Code, users)
	}
	createUser(tenantCookie, dto.CreateUserRequest{Username: "tenant-viewer", Password: password, Role: dbrepo.RoleViewer, TenantID: &first.ID}, http.StatusOK)
	createUser(tenantCookie, dto.CreateUserRequest{Username: "forbidden-admin", Password: password, Role: dbrepo.RoleTenantAdmin, TenantID: &first.ID}, http.StatusForbidden)
	if rec := authRequest(t, h, http.MethodPut, "/api/v1/users/"+itoa(otherViewer.ID)+"/status", dto.UserStatusRequest{Status: dbrepo.UserStatusDisabled}, tenantCookie, true); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant status = %d %s", rec.Code, rec.Body.String())
	}

	viewerCookie := loginCookie(t, h, "viewer", password)
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/users", nil, viewerCookie, false); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/users/"+itoa(viewer.ID)+"/reset-password", dto.ResetPasswordRequest{Password: "another secure password"}, adminCookie, true); rec.Code != http.StatusOK {
		t.Fatalf("reset password = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/me", nil, viewerCookie, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old viewer session = %d %s", rec.Code, rec.Body.String())
	}

	adminID := responseData[dto.MeView](t, authRequest(t, h, http.MethodGet, "/api/v1/me", nil, adminCookie, false).Body.Bytes()).ID
	if rec := authRequest(t, h, http.MethodPut, "/api/v1/users/"+itoa(adminID)+"/status", dto.UserStatusRequest{Status: dbrepo.UserStatusDisabled}, adminCookie, true); rec.Code != http.StatusConflict {
		t.Fatalf("disable last admin = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodDelete, "/api/v1/users/"+itoa(adminID), nil, adminCookie, true); rec.Code != http.StatusForbidden {
		t.Fatalf("delete default admin = %d %s", rec.Code, rec.Body.String())
	}
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }
