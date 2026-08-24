// Package service holds the console's business logic. It is the only layer
// that holds a repository: handlers must go through it, so no HTTP handler can
// reach the database directly.
package service

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
)

// Services is the bundle handlers receive. It grows one field per migrated
// resource domain.
type Services struct {
	Policy      *PolicyService
	PolicyGroup *PolicyGroupService
	Exception   *ExceptionService
	Summary     *SummaryService
	Admission   *AdmissionService
	Incident    *IncidentService
	Schema      func(context.Context, dto.GetFieldSchemaParams) ([]dto.FieldNode, error)
}

// New wires every service from the repository bundle and an optional Notifier.
// notifier may be nil (no gateway configured), in which case all NotifyChange
// calls are silently skipped.
func New(repos repository.Repositories, notifier Notifier) Services {
	return Services{
		Policy:      NewPolicyService(repos.Policies, repos.Groups, notifier),
		PolicyGroup: NewPolicyGroupService(repos, notifier),
		Exception:   NewExceptionService(repos, notifier),
		Summary:     NewSummaryService(repos),
		Incident:    NewIncidentService(repos),
		Admission:   NewAdmissionService(repos.Admission),
	}
}
