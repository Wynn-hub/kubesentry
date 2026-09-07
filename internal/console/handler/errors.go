package handler

import (
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// referencedByData marshals the group list into the error envelope's data
// field. jx.Raw is raw JSON, so strings need quotes — json.Marshal handles it.
func referencedByData(groups []string) dto.OptReferencedByData {
	return dto.OptReferencedByData{
		Value: dto.ReferencedByData{ReferencedBy: groups},
		Set:   true,
	}
}

// mapError maps service errors to ogen response types. The caller specifies
// the concrete type constructors for each status.
func mapCreatePolicyError(err error) dto.CreatePolicyRes {
	var valErr *service.ValidationError
	switch {
	case errors.As(err, &valErr):
		return &dto.CreatePolicyBadRequest{Code: 40000, Message: err.Error()}
	case service.IsConflict(err):
		return &dto.CreatePolicyConflict{Code: 40900, Message: err.Error()}
	default:
		return &dto.CreatePolicyInternalServerError{Code: 50000, Message: "request failed"}
	}
}

func mapGetPolicyError(err error) dto.GetPolicyRes {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return &dto.GetPolicyNotFound{Code: 40400, Message: err.Error()}
	default:
		return &dto.GetPolicyInternalServerError{Code: 50000, Message: "request failed"}
	}
}

func mapUpdatePolicyError(err error) dto.UpdatePolicyRes {
	var valErr *service.ValidationError
	switch {
	case errors.As(err, &valErr):
		return &dto.UpdatePolicyBadRequest{Code: 40000, Message: err.Error()}
	case errors.Is(err, service.ErrNotFound):
		return &dto.UpdatePolicyNotFound{Code: 40400, Message: err.Error()}
	case service.IsConflict(err):
		return &dto.UpdatePolicyConflict{Code: 40900, Message: err.Error()}
	default:
		return &dto.UpdatePolicyInternalServerError{Code: 50000, Message: "request failed"}
	}
}

func mapDeletePolicyError(err error) dto.DeletePolicyRes {
	var refErr *service.ReferencedByError
	switch {
	case errors.As(err, &refErr):
		return &dto.DeletePolicyConflict{
			Code:    40900,
			Message: err.Error(),
			Data:    referencedByData(refErr.Groups),
		}
	case errors.Is(err, service.ErrNotFound):
		return &dto.DeletePolicyNotFound{Code: 40400, Message: err.Error()}
	default:
		return &dto.DeletePolicyInternalServerError{Code: 50000, Message: "request failed"}
	}
}

func mapListPolicyVersionsError(err error) dto.ListPolicyVersionsRes {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return &dto.ListPolicyVersionsNotFound{Code: 40400, Message: err.Error()}
	default:
		return &dto.ListPolicyVersionsInternalServerError{Code: 50000, Message: "request failed"}
	}
}

func mapRollbackPolicyError(err error) dto.RollbackPolicyRes {
	var valErr *service.ValidationError
	switch {
	case errors.As(err, &valErr):
		return &dto.RollbackPolicyBadRequest{Code: 40000, Message: err.Error()}
	case errors.Is(err, service.ErrNotFound):
		return &dto.RollbackPolicyNotFound{Code: 40400, Message: err.Error()}
	case service.IsConflict(err):
		return &dto.RollbackPolicyConflict{Code: 40900, Message: err.Error()}
	case errors.Is(err, service.ErrGone):
		return &dto.RollbackPolicyGone{Code: 41000, Message: err.Error()}
	default:
		return &dto.RollbackPolicyInternalServerError{Code: 50000, Message: "request failed"}
	}
}
