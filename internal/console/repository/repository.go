package repository

import (
	"context"
	"time"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"gorm.io/gorm"
)

type DistributionRepository interface {
	List(context.Context) ([]dbrepo.PolicyDistribution, error)
	GetByName(context.Context, string) (*dbrepo.PolicyDistribution, error)
	ListPage(context.Context, dbrepo.PageOptions) ([]dbrepo.PolicyDistribution, int64, error)
	ListPageVisible(context.Context, int64, dbrepo.PageOptions) ([]dbrepo.PolicyDistribution, int64, error)
	Create(context.Context, *dbrepo.PolicyDistribution) error
	Update(context.Context, *dbrepo.PolicyDistribution) error
	UpdateCAS(context.Context, int64, time.Time, map[string]any) error
	Delete(context.Context, int64) error
}
type ClusterRepository interface {
	Create(context.Context, *dbrepo.Cluster) error
	GetByName(context.Context, string) (*dbrepo.Cluster, error)
	GetByNameForTenant(context.Context, int64, string) (*dbrepo.Cluster, error)
	List(context.Context) ([]dbrepo.Cluster, error)
	ListForTenant(context.Context, int64) ([]dbrepo.Cluster, error)
	ListPage(context.Context, dbrepo.PageOptions) ([]dbrepo.Cluster, int64, error)
	ListPageForTenant(context.Context, int64, dbrepo.PageOptions) ([]dbrepo.Cluster, int64, error)
	CountByPhase(context.Context) (dbrepo.ClusterPhaseCounts, error)
	UpdateLabels(context.Context, int64, map[string]string) error
	UpdateRegToken(context.Context, int64, string) error
	UpdatePhase(context.Context, int64, string) error
	Delete(context.Context, int64) error
	AssignTenant(context.Context, int64, *int64) error
}

// PlatformClusterRepository is the read-only cluster view needed by platform
// observability. Keeping it narrow lets PlatformService depend only on the two
// cluster lookups it actually performs.
type PlatformClusterRepository interface {
	List(context.Context) ([]dbrepo.Cluster, error)
	ListForTenant(context.Context, int64) ([]dbrepo.Cluster, error)
	GetByName(context.Context, string) (*dbrepo.Cluster, error)
	GetByNameForTenant(context.Context, int64, string) (*dbrepo.Cluster, error)
}

type PlatformRepository interface {
	GetSettings(context.Context) (*dbrepo.PlatformObservabilitySettings, error)
	UpdateSettings(context.Context, bool, int) error
	LatestSamples(context.Context, time.Time) ([]dbrepo.PlatformComponentSample, error)
	RestartedInstances(context.Context, time.Time) ([]dbrepo.PlatformRestartedInstance, error)
	MemoryPressuredInstances(context.Context, time.Time) ([]dbrepo.PlatformRestartedInstance, error)
	LatestCPUUsage(context.Context, time.Time) ([]dbrepo.PlatformCPUUsage, error)
	Trend(context.Context, string, string, int64, time.Time, time.Time, time.Duration) ([]dbrepo.PlatformTrendRow, error)
	LatestLeadershipEvents(context.Context, time.Time) ([]dbrepo.PlatformLeadershipEvent, error)
	LeadershipEvents(context.Context, string, *int64, time.Time, time.Time, int, int) (dbrepo.LeadershipEventPage, error)
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
	GetForTenant(context.Context, int64, int64) (*dbrepo.AdmissionLinkIncident, error)
}

type AdmissionRepository interface {
	EventDatabaseIDs(context.Context, []string) (map[string]int64, error)
	GetSettings(context.Context) (*dbrepo.AdmissionEventSettings, error)
	ListPolicies(context.Context) ([]dbrepo.AdmissionStoragePolicy, error)
	UpdateSettingsAndPolicies(context.Context, *dbrepo.AdmissionEventSettings, []dbrepo.AdmissionStoragePolicy) error
	ListEventsPage(context.Context, dbrepo.AdmissionEventPageOptions) ([]dbrepo.AdmissionEvent, int64, error)
	GetEvent(context.Context, int64) (*dbrepo.AdmissionEvent, error)
	GetEventForTenant(context.Context, int64, int64) (*dbrepo.AdmissionEvent, error)
	Trend(context.Context, time.Time, time.Time, bool) ([]dbrepo.AdmissionEventTrendBucket, error)
	TrendForTenant(context.Context, int64, time.Time, time.Time, bool) ([]dbrepo.AdmissionEventTrendBucket, error)
}

type IdentityRepository interface {
	GetUserByUsername(context.Context, string) (*dbrepo.User, error)
	GetUser(context.Context, int64) (*dbrepo.User, error)
	ListUsers(context.Context) ([]dbrepo.User, error)
	ListUsersByTenant(context.Context, int64) ([]dbrepo.User, error)
	UpdateUser(context.Context, *dbrepo.User) error
	SetUserStatusAndRevoke(context.Context, int64, dbrepo.UserStatus, time.Time) error
	SetUserPasswordAndRevoke(context.Context, int64, string, time.Time) error
	DeleteUserAndRevoke(context.Context, int64, time.Time) error
	GetTenant(context.Context, int64) (*dbrepo.Tenant, error)
	ListTenants(context.Context) ([]dbrepo.Tenant, error)
	CreateTenant(context.Context, *dbrepo.Tenant) error
	UpdateTenantStatus(context.Context, int64, dbrepo.UserStatus) error
	CreateUser(context.Context, *dbrepo.User) error
	InitializeAdmin(context.Context, string, time.Time) (bool, error)
	CreateSession(context.Context, *dbrepo.Session) error
	GetActiveSession(context.Context, string, time.Time) (*dbrepo.Session, *dbrepo.User, *dbrepo.Tenant, error)
	TouchSession(context.Context, int64, time.Time) error
	RevokeSession(context.Context, int64, time.Time) error
	RevokeUserSessions(context.Context, int64) error
}

// ExceptionUnitOfWork commits an exception mutation and its managed scope
// distribution as one database transaction. The callback receives repositories
// bound to the same transaction.
type ExceptionUnitOfWork interface {
	WithinTransaction(context.Context, func(ExceptionRepository, DistributionRepository) error) error
}

// ClusterUnitOfWork commits coupled cluster lifecycle writes atomically.
type ClusterUnitOfWork interface {
	WithinTransaction(context.Context, func(ClusterRepository, InstallRecordRepository, UninstallTaskRepository) error) error
}

type clusterUnitOfWork struct{ repo *dbrepo.ClusterRepo }

func (u clusterUnitOfWork) WithinTransaction(ctx context.Context, fn func(ClusterRepository, InstallRecordRepository, UninstallTaskRepository) error) error {
	return u.repo.WithTx(ctx, func(clusters *dbrepo.ClusterRepo, records *dbrepo.InstallRecordRepo, tasks *dbrepo.UninstallTaskRepo) error {
		return fn(clusters, records, tasks)
	})
}

type exceptionUnitOfWork struct{ repo *dbrepo.ExceptionRepo }

func (u exceptionUnitOfWork) WithinTransaction(ctx context.Context, fn func(ExceptionRepository, DistributionRepository) error) error {
	return u.repo.WithTx(ctx, func(exceptions *dbrepo.ExceptionRepo, distributions *dbrepo.DistributionRepo) error {
		return fn(exceptions, distributions)
	})
}

// Repositories is the console's data-access bundle.
type Repositories struct {
	Policies       *dbrepo.PolicyRepo
	Groups         *dbrepo.GroupRepo
	PolicyGroups   *dbrepo.GroupRepo
	Exceptions     *dbrepo.ExceptionRepo
	Distributions  *dbrepo.DistributionRepo
	Clusters       *dbrepo.ClusterRepo
	InstallRecords *dbrepo.InstallRecordRepo
	UninstallTasks *dbrepo.UninstallTaskRepo
	Incidents      *dbrepo.IncidentRepo
	Admission      *dbrepo.AdmissionRepo
	Platform       *dbrepo.PlatformObservabilityRepo
	Identity       IdentityRepository
	ExceptionUOW   ExceptionUnitOfWork
	ClusterUOW     ClusterUnitOfWork
}

// New constructs the console's repository adapters from its database.
func New(db *gorm.DB) Repositories {
	clusters := dbrepo.NewClusterRepo(db)
	groups := dbrepo.NewGroupRepo(db)
	exceptions := dbrepo.NewExceptionRepo(db)
	return Repositories{
		Policies:       dbrepo.NewPolicyRepo(db),
		Groups:         groups,
		PolicyGroups:   groups,
		Exceptions:     exceptions,
		Distributions:  dbrepo.NewDistributionRepo(db),
		Clusters:       clusters,
		InstallRecords: dbrepo.NewInstallRecordRepo(db),
		UninstallTasks: dbrepo.NewUninstallTaskRepo(db),
		Incidents:      dbrepo.NewIncidentRepo(db),
		Admission:      dbrepo.NewAdmissionRepo(db),
		Platform:       dbrepo.NewPlatformObservabilityRepo(db),
		Identity:       dbrepo.NewIdentityRepo(db),
		ExceptionUOW:   exceptionUnitOfWork{repo: exceptions},
		ClusterUOW:     clusterUnitOfWork{repo: clusters},
	}
}
