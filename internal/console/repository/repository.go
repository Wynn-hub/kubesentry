package repository

import (
	"context"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

type DistributionRepository interface {
	List(context.Context) ([]dbrepo.PolicyDistribution, error)
	GetByName(context.Context, string) (*dbrepo.PolicyDistribution, error)
	ListPage(context.Context, dbrepo.PageOptions) ([]dbrepo.PolicyDistribution, int64, error)
	Create(context.Context, *dbrepo.PolicyDistribution) error
	Update(context.Context, *dbrepo.PolicyDistribution) error
	UpdateCAS(context.Context, int64, time.Time, map[string]any) error
	Delete(context.Context, int64) error
}
type ClusterRepository interface {
	Create(context.Context, *dbrepo.Cluster) error
	GetByName(context.Context, string) (*dbrepo.Cluster, error)
	ListPage(context.Context, dbrepo.PageOptions) ([]dbrepo.Cluster, int64, error)
	CountByPhase(context.Context) (dbrepo.ClusterPhaseCounts, error)
	UpdateLabels(context.Context, int64, map[string]string) error
	UpdateRegToken(context.Context, int64, string) error
	UpdatePhase(context.Context, int64, string) error
	Delete(context.Context, int64) error
}

type InstallRecordRepository interface {
	Create(context.Context, *dbrepo.ClusterInstallRecord) error
	LatestForCluster(context.Context, string) (*dbrepo.ClusterInstallRecord, error)
	ListPage(context.Context, dbrepo.PageOptions) ([]dbrepo.ClusterInstallRecord, int64, error)
}

type UninstallTaskRepository interface {
	Create(context.Context, *dbrepo.UninstallTask) error
	HasActive(context.Context, int64) (bool, error)
	GetByID(context.Context, int64) (*dbrepo.UninstallTask, error)
	ListByClusterID(context.Context, int64) ([]dbrepo.UninstallTask, error)
	Retry(context.Context, int64, time.Time) error
}

type IncidentRepository interface {
	ListPage(context.Context, dbrepo.IncidentPageOptions) ([]dbrepo.AdmissionLinkIncident, int64, error)
	Get(context.Context, int64) (*dbrepo.AdmissionLinkIncident, error)
}

type AdmissionRepository interface {
	EventDatabaseIDs(context.Context, []string) (map[string]int64, error)
	GetSettings(context.Context) (*dbrepo.AdmissionEventSettings, error)
	ListPolicies(context.Context) ([]dbrepo.AdmissionStoragePolicy, error)
	UpdateSettingsAndPolicies(context.Context, *dbrepo.AdmissionEventSettings, []dbrepo.AdmissionStoragePolicy) error
	ListEventsPage(context.Context, dbrepo.AdmissionEventPageOptions) ([]dbrepo.AdmissionEvent, int64, error)
	GetEvent(context.Context, int64) (*dbrepo.AdmissionEvent, error)
	Trend(context.Context, time.Time, time.Time, bool) ([]dbrepo.AdmissionEventTrendBucket, error)
}

// Repositories is the console's data-access bundle. It grows one field per
// migrated resource domain; batches that haven't migrated yet keep using
// gateway.Repos directly from the legacy handlers.
type Repositories struct {
	Policies       PolicyRepository
	Groups         GroupRepository
	PolicyGroups   PolicyGroupRepository
	Exceptions     ExceptionRepository
	Distributions  DistributionRepository
	Clusters       ClusterRepository
	InstallRecords InstallRecordRepository
	UninstallTasks UninstallTaskRepository
	Incidents      IncidentRepository
	Admission      AdmissionRepository
}

// FromGatewayRepos narrows the shared gateway.Repos bundle down to the
// console's interfaces. cmd/console keeps constructing gateway.Repos (it is
// what internal/gateway and cmd/db also use); this is the single adaptation
// point rather than threading concrete repo types through the service layer.
func FromGatewayRepos(r gateway.Repos) Repositories {
	return Repositories{
		Policies:       r.Policies,
		Groups:         r.Groups,
		PolicyGroups:   r.Groups,
		Exceptions:     r.Exceptions,
		Distributions:  r.Distributions,
		Clusters:       r.Clusters,
		InstallRecords: r.InstallRecords,
		UninstallTasks: r.UninstallTasks,
		Incidents:      r.Incidents,
		Admission:      r.Admission,
	}
}
