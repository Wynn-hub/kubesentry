package console

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

const sessionCookieName = "kubesentry_session"

func PrincipalFromContext(ctx context.Context) (dbrepo.Principal, bool) {
	return dbrepo.PrincipalFromContext(ctx)
}

type loginLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type authHTTP struct {
	service  *service.AuthService
	lifetime time.Duration
	secure   bool
	mu       sync.Mutex
	limits   map[string]*loginLimiterEntry
}

func newAuthHTTP(auth *service.AuthService, lifetime time.Duration, secure bool) *authHTTP {
	if lifetime <= 0 {
		lifetime = 24 * time.Hour
	}
	return &authHTTP{service: auth, lifetime: lifetime, secure: secure, limits: make(map[string]*loginLimiterEntry)}
}

func (a *authHTTP) setupStatus(w http.ResponseWriter, r *http.Request) {
	required, err := a.service.SetupRequired(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "request failed")
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success", Data: dto.SetupStatusView{Required: required}})
}

func (a *authHTTP) setupAdmin(w http.ResponseWriter, r *http.Request) {
	var input dto.PasswordInput
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	err := a.service.InitializeAdmin(r.Context(), input.Password)
	switch {
	case errors.Is(err, service.ErrInvalidPassword):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrAlreadyInitialized):
		writeErr(w, http.StatusConflict, err.Error())
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "request failed")
	default:
		slog.Info("administrator initialized", "remote_ip", remoteIP(r), "trace_id", TraceIDFromContext(r.Context()))
		writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
	}
}

func (a *authHTTP) login(w http.ResponseWriter, r *http.Request) {
	var input dto.LoginInput
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if !a.allowLogin(username + "\x00" + remoteIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	token, _, err := a.service.Login(r.Context(), username, input.Password, remoteIP(r))
	if err != nil {
		slog.Warn("login failed", "username", username, "remote_ip", remoteIP(r), "trace_id", TraceIDFromContext(r.Context()))
		if errors.Is(err, service.ErrAccountLocked) {
			writeCodedErr(w, http.StatusLocked, "ACCOUNT_LOCKED", err.Error())
			return
		}
		writeErr(w, http.StatusUnauthorized, service.ErrInvalidCredentials.Error())
		return
	}
	a.setSessionCookie(w, r, token, a.service.SessionAbsoluteDuration(r.Context()))
	slog.Info("login succeeded", "username", username, "remote_ip", remoteIP(r), "trace_id", TraceIDFromContext(r.Context()))
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
}

func (a *authHTTP) activate(w http.ResponseWriter, r *http.Request) {
	var input dto.ActivateInput
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	err := a.service.Activate(r.Context(), input.Code, input.Password)
	switch {
	case errors.Is(err, service.ErrInvalidPassword):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrActivationInvalid):
		writeCodedErr(w, http.StatusBadRequest, "ACTIVATION_INVALID", err.Error())
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "request failed")
	default:
		writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
	}
}

func (a *authHTTP) changePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeCodedErr(w, http.StatusUnauthorized, "SESSION_INVALID", "authentication required")
		return
	}
	var input dto.ChangePasswordInput
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeCodedErr(w, http.StatusUnauthorized, "SESSION_INVALID", "authentication required")
		return
	}
	err = a.service.ChangePassword(r.Context(), principal, cookie.Value, input.Password)
	switch {
	case errors.Is(err, service.ErrInvalidPassword):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrPasswordReused):
		writeCodedErr(w, http.StatusConflict, "PASSWORD_REUSED", err.Error())
	case errors.Is(err, service.ErrAuthentication):
		writeCodedErr(w, http.StatusUnauthorized, "SESSION_INVALID", "authentication required")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "request failed")
	default:
		writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
	}
}

func (a *authHTTP) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.service.Logout(r.Context(), cookie.Value)
	}
	a.setSessionCookie(w, r, "", -time.Hour)
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
}

func (a *authHTTP) me(w http.ResponseWriter, r *http.Request) {
	p, ok := PrincipalFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success", Data: dto.MeView{ID: p.UserID, Username: p.Username, Role: p.Role, TenantID: p.TenantID, LastLoginAt: p.LastLoginAt, PasswordExpiresAt: p.PasswordExpiresAt, MustChangePassword: p.MustChangePassword}})
}

func (a *authHTTP) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		principal, err := a.service.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			writeCodedErr(w, http.StatusUnauthorized, "SESSION_INVALID", "authentication required")
			return
		}
		if !isSafeMethod(r.Method) && (r.Header.Get("X-KubeSentry-CSRF") != "1" || !sameOrigin(r)) {
			writeErr(w, http.StatusForbidden, "csrf validation failed")
			return
		}
		if systemAdminOnlyRoute(r) && principal.Role != dbrepo.RoleSystemAdmin {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		if principal.MustChangePassword && !restrictedSessionRoute(r) {
			writeCodedErr(w, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "password change required")
			return
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
	})
}

func restrictedSessionRoute(r *http.Request) bool {
	if r.URL.Path == "/api/v1/me" && r.Method == http.MethodGet {
		return true
	}
	if r.URL.Path == "/api/v1/auth/change-password" && r.Method == http.MethodPost {
		return true
	}
	return r.URL.Path == "/api/v1/auth/logout" && r.Method == http.MethodPost
}

func (a *authHTTP) allowLogin(key string) bool {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for existing, entry := range a.limits {
		if now.Sub(entry.lastSeen) > 10*time.Minute {
			delete(a.limits, existing)
		}
	}
	entry := a.limits[key]
	if entry == nil {
		if len(a.limits) >= 10000 {
			return false
		}
		entry = &loginLimiterEntry{limiter: rate.NewLimiter(rate.Every(12*time.Second), 5)}
		a.limits[key] = entry
	}
	entry.lastSeen = now
	return entry.limiter.Allow()
}

func withPrincipal(ctx context.Context, principal dbrepo.Principal) context.Context {
	return dbrepo.WithPrincipal(ctx, principal)
}

func decodeAuthJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (a *authHTTP) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: value, Path: "/", HttpOnly: true, Secure: a.secure || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode, MaxAge: int(lifetime.Seconds())})
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func systemAdminOnlyRoute(r *http.Request) bool {
	path := r.URL.Path
	if path == "/api/v1/install-records" || path == "/api/v1/platform/settings" {
		return true
	}
	if path == "/api/v1/security-settings" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/tenants/") && strings.HasSuffix(path, "/clusters") {
		return true
	}
	if path == "/api/v1/admission-event-settings" {
		return r.Method != http.MethodGet
	}
	if strings.HasPrefix(path, "/api/v1/clusters/") {
		return r.Method != http.MethodGet || strings.HasSuffix(path, "/latest-install-params")
	}
	return path == "/api/v1/clusters" && r.Method != http.MethodGet
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && u.Host == r.Host
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
