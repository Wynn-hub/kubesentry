package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

func authRequest(t *testing.T, h http.Handler, method, path string, body any, cookie *http.Cookie, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
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

func TestAuthSetupLoginMeAndLogout(t *testing.T) {
	repos := repository.New(newTestDB(t))
	srv, err := NewServer(&Handlers{Repos: repos, SessionAbsoluteDuration: 2 * time.Hour, SessionIdleDuration: time.Hour, SessionCookieSecure: true}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler

	if rec := authRequest(t, h, http.MethodGet, "/api/v1/setup/status", nil, nil, false); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"required":true`)) {
		t.Fatalf("initial setup status = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/setup/admin", map[string]string{"password": "correct horse battery staple"}, nil, false); rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/setup/admin", map[string]string{"password": "another secure password"}, nil, false); rec.Code != http.StatusConflict {
		t.Fatalf("repeated setup = %d %s", rec.Code, rec.Body.String())
	}
	login := authRequest(t, h, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "admin", "password": "correct horse battery staple"}, nil, false)
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 {
		t.Fatalf("login = %d %s cookies=%v", login.Code, login.Body.String(), login.Result().Cookies())
	}
	cookie := login.Result().Cookies()[0]
	if cookie.Name != sessionCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.MaxAge != 7200 || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %+v", cookie)
	}
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/me", nil, cookie, false); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"role":"system_admin"`)) {
		t.Fatalf("me = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/auth/logout", nil, cookie, false); rec.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/auth/logout", nil, cookie, true); rec.Code != http.StatusOK || rec.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatalf("logout = %d %s cookies=%v", rec.Code, rec.Body.String(), rec.Result().Cookies())
	}
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/me", nil, cookie, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedRouteRejectsMissingSessionAndSelfReportedHeaders(t *testing.T) {
	repos := repository.New(newTestDB(t))
	srv, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler
	req := httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	req.Header.Set("X-User-Role", "system_admin")
	req.Header.Set("Authorization", "Bearer self-reported")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected route = %d %s", rec.Code, rec.Body.String())
	}
}
