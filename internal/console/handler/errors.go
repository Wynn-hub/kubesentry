package handler

import (
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func mapCreatePolicyError(err error) dto.CreatePolicyRes {
	var valErr *service.ValidationError
	switch {
	case errors.As(err, &valErr):
		return &dto.PolicyBadRequestHTTPError{Code: 40000, Message: err.Error()}
	case service.IsConflict(err):
		return &dto.PolicyConflictHTTPError{Code: 40900, Message: err.Error()}
	default:
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}
	}
}

func mapGetPolicyError(err error) dto.GetPolicyRes {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return &dto.PolicyNotFoundHTTPError{Code: 40400, Message: err.Error()}
	default:
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}
	}
}

func mapUpdatePolicyError(err error) dto.UpdatePolicyRes {
	var valErr *service.ValidationError
	switch {
	case errors.As(err, &valErr):
		return &dto.PolicyBadRequestHTTPError{Code: 40000, Message: err.Error()}
	case errors.Is(err, service.ErrNotFound):
		return &dto.PolicyNotFoundHTTPError{Code: 40400, Message: err.Error()}
	case service.IsConflict(err):
		return &dto.PolicyConflictHTTPError{Code: 40900, Message: err.Error()}
	default:
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}
	}
}

func mapDeletePolicyError(err error) dto.DeletePolicyRes {
	var refErr *service.ReferencedByError
	switch {
	case errors.As(err, &refErr):
		return &dto.DeletePolicyConflictHTTPError{Code: 40900, Message: err.Error(), Data: dto.PolicyReferencedByView{ReferencedBy: refErr.Groups}}
	case errors.Is(err, service.ErrNotFound):
		return &dto.PolicyNotFoundHTTPError{Code: 40400, Message: err.Error()}
	default:
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}
	}
}

func mapListPolicyVersionsError(err error) dto.ListPolicyVersionsRes {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return &dto.PolicyNotFoundHTTPError{Code: 40400, Message: err.Error()}
	default:
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}
	}
}

func mapRollbackPolicyError(err error) dto.RollbackPolicyRes {
	var valErr *service.ValidationError
	switch {
	case errors.As(err, &valErr):
		return &dto.PolicyBadRequestHTTPError{Code: 40000, Message: err.Error()}
	case errors.Is(err, service.ErrNotFound):
		return &dto.PolicyNotFoundHTTPError{Code: 40400, Message: err.Error()}
	case service.IsConflict(err):
		return &dto.PolicyConflictHTTPError{Code: 40900, Message: err.Error()}
	case errors.Is(err, service.ErrGone):
		return &dto.PolicyGoneHTTPError{Code: 41000, Message: err.Error()}
	default:
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}
	}
}
