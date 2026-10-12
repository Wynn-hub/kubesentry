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
	Policy           *PolicyService
	PolicyGroup      *PolicyGroupService
	Exception        *ExceptionService
	Summary          *SummaryService
	Admission        *AdmissionService
	Incident         *IncidentService
	Cluster          *ClusterService
	Distribution     *DistributionService
	Schema           func(context.Context, dto.FieldSchemaOptions) ([]dto.FieldNodeView, error)
	Platform         *PlatformService
	Auth             *AuthService
	Identity         *IdentityService
	SecuritySettings *SecuritySettingsService
	TenantCluster    *TenantClusterService
}

// New wires every service from the repository bundle and an optional Notifier.
// notifier may be nil (no gateway configured), in which case all NotifyChange
// calls are silently skipped.
func New(repos repository.Repositories, notifier Notifier) Services {
	return Services{
		Policy:           NewPolicyService(repos.Policies, repos.Groups, notifier),
		PolicyGroup:      NewPolicyGroupService(repos, notifier),
		Exception:        NewExceptionService(repos, notifier),
		Summary:          NewSummaryService(repos),
		Incident:         NewIncidentService(repos),
		Cluster:          NewClusterService(repos, notifier),
		Distribution:     NewDistributionService(repos, notifier),
		Platform:         NewPlatformService(repos.Platform, repos.Clusters, notifier),
		Admission:        NewAdmissionService(repos.Admission, notifier),
		Auth:             NewAuthService(repos.Identity),
		Identity:         NewIdentityService(repos.Identity, repos.Clusters),
		SecuritySettings: NewSecuritySettingsService(repos.Identity),
		TenantCluster:    NewTenantClusterService(repos.Clusters, notifier),
	}
}
