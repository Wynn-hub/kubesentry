package repository

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

type DistributionRepository interface {
	List(context.Context) ([]dbrepo.PolicyDistribution, error)
	Create(context.Context, *dbrepo.PolicyDistribution) error
	Update(context.Context, *dbrepo.PolicyDistribution) error
	Delete(context.Context, int64) error
}
type ClusterRepository interface {
	GetByName(context.Context, string) (*dbrepo.Cluster, error)
	CountByPhase(context.Context) (dbrepo.ClusterPhaseCounts, error)
}

// Repositories is the console's data-access bundle. It grows one field per
// migrated resource domain; batches that haven't migrated yet keep using
// gateway.Repos directly from the legacy handlers.
type Repositories struct {
	Policies      PolicyRepository
	Groups        GroupRepository
	PolicyGroups  PolicyGroupRepository
	Exceptions    ExceptionRepository
	Distributions DistributionRepository
	Clusters      ClusterRepository
}

// FromGatewayRepos narrows the shared gateway.Repos bundle down to the
// console's interfaces. cmd/console keeps constructing gateway.Repos (it is
// what internal/gateway and cmd/db also use); this is the single adaptation
// point rather than threading concrete repo types through the service layer.
func FromGatewayRepos(r gateway.Repos) Repositories {
	return Repositories{
		Policies:      r.Policies,
		Groups:        r.Groups,
		PolicyGroups:  r.Groups,
		Exceptions:    r.Exceptions,
		Distributions: r.Distributions,
		Clusters:      r.Clusters,
	}
}
