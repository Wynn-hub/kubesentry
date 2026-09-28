package console

import (
	"errors"
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type identityHTTP struct{ service *service.IdentityService }

func (h identityHTTP) listTenants(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListTenants(r.Context(), requestPrincipal(r))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success", Data: items})
}

func (h identityHTTP) createTenant(w http.ResponseWriter, r *http.Request) {
	var input dto.CreateTenantRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateTenant(r.Context(), requestPrincipal(r), input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success", Data: item})
}

func (h identityHTTP) setTenantStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := identityPathID(w, r)
	if !ok {
		return
	}
	var input dto.TenantStatusRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := h.service.SetTenantStatus(r.Context(), requestPrincipal(r), id, input.Status); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
}

func (h identityHTTP) listUsers(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListUsers(r.Context(), requestPrincipal(r))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success", Data: items})
}

func (h identityHTTP) createUser(w http.ResponseWriter, r *http.Request) {
	var input dto.CreateUserRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	item, err := h.service.CreateUser(r.Context(), requestPrincipal(r), input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success", Data: item})
}

func (h identityHTTP) setUserStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := identityPathID(w, r)
	if !ok {
		return
	}
	var input dto.UserStatusRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := h.service.SetUserStatus(r.Context(), requestPrincipal(r), id, input.Status); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
}

func (h identityHTTP) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := identityPathID(w, r)
	if !ok {
		return
	}
	var input dto.ResetPasswordRequest
	if !decodeAuthJSON(w, r, &input) {
		return
	}
	if err := h.service.ResetPassword(r.Context(), requestPrincipal(r), id, input.Password); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
}

func (h identityHTTP) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := identityPathID(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteUser(r.Context(), requestPrincipal(r), id); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
}

func requestPrincipal(r *http.Request) dbrepo.Principal {
	principal, _ := PrincipalFromContext(r.Context())
	return principal
}

func identityPathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func writeIdentityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrDefaultAdmin):
		writeErr(w, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrIdentityNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, service.ErrInvalidIdentity), errors.Is(err, service.ErrInvalidPassword):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, dbrepo.ErrLastSystemAdmin), dbrepo.IsUniqueViolation(err):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "request failed")
	}
}
