// Package dbrepo is the GORM data-access layer shared by cmd/gateway and
// cmd/console. Table shapes match spec section 二; jsonb columns reuse the
// existing v1alpha1 Spec structs verbatim instead of redefining them.
package repository

import (
	"context"
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/database/model"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

type Cluster struct {
	model.Base
	TenantID        *int64
	Name            string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	ClusterUID      string
	Labels          datatypes.JSONType[map[string]string]
	Phase           string
	OperatorVersion string
	// Member-reported inventory, refreshed by StatusReport. Empty/zero means the
	// member has not reported it (yet) or could not read it. Explicit column
	// names guard against GORM initialism edge cases.
	KubernetesVersion        string `gorm:"column:kubernetes_version"`
	APIServerAddress         string `gorm:"column:api_server_address"`
	NodeCount                int    `gorm:"column:node_count"`
	ReadyNodeCount           int    `gorm:"column:ready_node_count"`
	LastHeartbeatAt          *time.Time
	ObservabilityOnboardedAt *time.Time
	RegTokenHash             string
	AuthTokenHash            string
}

func (Cluster) TableName() string { return "kubesentry.clusters" }

type Policy struct {
	model.Base
	OwnerType       string `gorm:"default:system"`
	TenantID        *int64
	Name            string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Description     string
	Rego            string
	EnforcementMode string
	Match           datatypes.JSONType[v1alpha1.PolicyMatch]
	CompileOK       bool
	CompileError    string
	Labels          datatypes.JSONType[map[string]string]
	Annotations     datatypes.JSONType[map[string]string]
}

func (Policy) TableName() string { return "kubesentry.policies" }

type PolicyVersion struct {
	model.Base
	PolicyID     int64
	Version      int
	Rego         string
	SpecSnapshot datatypes.JSONType[v1alpha1.PolicySpec]
}

func (PolicyVersion) TableName() string { return "kubesentry.policy_versions" }

type PolicyGroup struct {
	model.Base
	OwnerType   string `gorm:"default:system"`
	TenantID    *int64
	Name        string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec        datatypes.JSONType[v1alpha1.PolicyGroupSpec]
	Labels      datatypes.JSONType[map[string]string]
	Annotations datatypes.JSONType[map[string]string]
}

func (PolicyGroup) TableName() string { return "kubesentry.policy_groups" }

type PolicyException struct {
	model.Base
	OwnerType   string `gorm:"default:system"`
	TenantID    *int64
	Name        string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec        datatypes.JSONType[v1alpha1.PolicyExceptionSpec]
	ExpiresAt   *time.Time
	Labels      datatypes.JSONType[map[string]string]
	Annotations datatypes.JSONType[map[string]string]
}

func (PolicyException) TableName() string { return "kubesentry.policy_exceptions" }

type PolicyDistribution struct {
	model.Base
	TenantID          *int64
	ResourceOwnerType string `gorm:"default:system"`
	Name              string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec              datatypes.JSONType[gatewaymodel.PolicyDistributionSpec]
	Managed           bool
	OwnerType         string
	OwnerName         string
}

func (PolicyDistribution) TableName() string { return "kubesentry.policy_distributions" }

type Role string

const (
	RoleSystemAdmin Role = "system_admin"
	RoleTenantAdmin Role = "tenant_admin"
	RoleViewer      Role = "viewer"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

type Tenant struct {
	model.Base
	Name   string
	Status UserStatus
}

func (Tenant) TableName() string { return "kubesentry.tenants" }

type User struct {
	model.Base
	Username           string
	PasswordHash       string
	Role               Role
	Status             UserStatus
	TenantID           *int64
	LastLoginAt        *time.Time
	LastLoginIP        string
	PasswordChangedAt  *time.Time
	MustChangePassword bool
	FailedLoginCount   int
	LockedUntil        *time.Time
}

func (User) TableName() string { return "kubesentry.users" }

type UserActivationToken struct {
	model.Base
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (UserActivationToken) TableName() string { return "kubesentry.user_activation_tokens" }

type PasswordHistory struct {
	model.Base
	UserID       int64
	PasswordHash string
}

func (PasswordHistory) TableName() string { return "kubesentry.password_history" }

type SecuritySettings struct {
	ID                      int64 `gorm:"primaryKey;autoIncrement:false"`
	ActivationLifetimeHours int
	PasswordLifetimeDays    int
	PasswordMinLength       int
	PasswordHistoryCount    int
	MaxFailedLoginAttempts  int
	LockoutDurationMinutes  int
	SessionIdleMinutes      int
	SessionAbsoluteMinutes  int
	AllowConcurrentSessions bool
	UpdatedBy               *int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (SecuritySettings) TableName() string { return "kubesentry.security_settings" }

type Session struct {
	model.Base
	UserID     int64
	TokenHash  string
	ExpiresAt  time.Time
	LastSeenAt time.Time
	RevokedAt  *time.Time
}

func (Session) TableName() string { return "kubesentry.sessions" }

type Principal struct {
	UserID             int64
	Username           string
	Role               Role
	TenantID           *int64
	LastLoginAt        *time.Time
	PasswordExpiresAt  *time.Time
	MustChangePassword bool
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

type DistributionClusterStatus struct {
	model.Base
	DistributionID     int64
	ClusterID          int64
	Status             string
	AppliedVersionHash string
	LastError          string
}

func (DistributionClusterStatus) TableName() string { return "kubesentry.distribution_cluster_status" }

type ClusterInstallRecord struct {
	model.Base
	ClusterName          string
	Namespace            string
	WebhookRepository    string
	WebhookTag           string
	WebhookPullPolicy    string
	OperatorRepository   string
	OperatorTag          string
	OperatorPullPolicy   string
	HighAvailability     bool
	GatewayAddress       string
	GatewayTLSServerName string
	GatewayCABundle      string
}

func (ClusterInstallRecord) TableName() string { return "kubesentry.cluster_install_records" }

// UninstallTask is a durable request for one connected member cluster to
// remove the KubeSentry release resources it owns. The member's acknowledgement
// and result are stored before the platform changes the cluster lifecycle.
type UninstallTask struct {
	model.Base
	ClusterID   int64
	Status      string
	RequestedAt time.Time
	ReceivedAt  *time.Time
	CompletedAt *time.Time
	Error       string
	Attempts    int
}

func (UninstallTask) TableName() string { return "kubesentry.cluster_uninstall_tasks" }

// AdmissionEventSettings is the single global delivery and retention
// configuration. Its version changes atomically with every mutation so member
// clusters can report which configuration they used for a local decision.
type AdmissionEventSettings struct {
	ID                    int64 `gorm:"primaryKey;autoIncrement:false"`
	DefaultRetentionDays  int
	RetryEnabled          bool
	MaxRetryAttempts      int
	QueueCapacity         int
	IncidentRetentionDays int
	StoragePolicyVersion  int64
	// ObservationEnabled gates the pipeline funnel observability. It lives here
	// rather than in PlatformObservabilitySettings because it instruments the
	// admission event path, and the operator configures it from the same screen.
	ObservationEnabled bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (AdmissionEventSettings) TableName() string { return "kubesentry.admission_event_settings" }

// AdmissionStoragePolicy is a conditions-only storage rule: matching any rule
// stores the event, an empty rule set stores everything, and retention comes
// from the global AdmissionEventSettings. Priority only fixes the display order.
type AdmissionStoragePolicy struct {
	model.Base
	Priority   int
	MatchMode  string
	Verdicts   pq.StringArray `gorm:"type:text[]"`
	Clusters   pq.StringArray `gorm:"type:text[]"`
	GVKs       pq.StringArray `gorm:"column:gvks;type:text[]"`
	Namespaces pq.StringArray `gorm:"type:text[]"`
}

func (AdmissionStoragePolicy) TableName() string { return "kubesentry.admission_storage_policies" }

type AdmissionEvent struct {
	model.Base
	EventID              string `gorm:"uniqueIndex"`
	RequestUID           string
	ClusterID            int64
	ClusterName          string
	OccurredAt           time.Time
	Verdict              string
	APIGroup             string
	APIVersion           string
	Resource             string
	Operation            string
	Namespace            string
	ResourceName         string
	Username             string
	UserGroups           pq.StringArray `gorm:"type:text[]"`
	PolicyNames          pq.StringArray `gorm:"type:text[]"`
	PolicyGroupNames     pq.StringArray `gorm:"type:text[]"`
	Violations           datatypes.JSON `gorm:"type:jsonb"`
	MetadataSnapshot     datatypes.JSON `gorm:"type:jsonb"`
	SpecSnapshot         datatypes.JSON `gorm:"type:jsonb"`
	StoragePolicyVersion string
}

func (AdmissionEvent) TableName() string { return "kubesentry.admission_events" }

type PlatformObservabilitySettings struct {
	ID            int64 `gorm:"primaryKey;autoIncrement:false"`
	Enabled       bool
	RetentionDays int
	LastCleanupAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (PlatformObservabilitySettings) TableName() string {
	return "kubesentry.platform_observability_settings"
}

type PlatformComponentSample struct {
	ID                   int64 `gorm:"primaryKey;autoIncrement:false"`
	BucketStartedAt      time.Time
	Component            string
	Instance             string
	ClusterID            int64
	ClusterName          string
	Role                 string
	ProcessStartedAt     time.Time
	RestartCount         int64
	MemoryBytes          int64
	MemoryLimitBytes     int64
	MemoryPressureEvents int64
	CPUUsageMicros       int64 `gorm:"column:cpu_usage_micros"`
	CPUQuotaMicros       int64 `gorm:"column:cpu_quota_micros"`
	Goroutines           int64
	HeapAllocBytes       int64
	HeapSysBytes         int64
	GCPauseTotalMicros   int64 `gorm:"column:gc_pause_total_micros"`
	GCCount              int64 `gorm:"column:gc_count"`
	Health               string
	HealthReason         string
	DBStats              datatypes.JSONType[map[string]any]
	LastObservedAt       time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (PlatformComponentSample) TableName() string {
	return "kubesentry.platform_component_samples"
}

type PlatformLeadershipEvent struct {
	ID          int64     `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Component   string    `json:"component"`
	ClusterID   int64     `json:"clusterId"`
	ClusterName string    `json:"clusterName"`
	Instance    string    `json:"instance"`
	RoleFrom    string    `json:"roleFrom"`
	RoleTo      string    `json:"roleTo"`
	ChangedAt   time.Time `json:"changedAt"`
	DetectedBy  string    `json:"detectedBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (PlatformLeadershipEvent) TableName() string {
	return "kubesentry.platform_leadership_events"
}

type AdmissionLinkIncident struct {
	ID                 int64 `gorm:"primaryKey;autoIncrement:false"`
	ClusterID          int64
	ClusterName        string
	IncidentKey        string
	ReporterInstanceID string
	Component          string
	Reason             string
	StartedAt          time.Time
	LastSeenAt         time.Time
	AffectedCount      int64
	SampleEventIDs     pq.StringArray `gorm:"type:text[]"`
	Detail             string
	Closed             bool
	FirstReceivedAt    time.Time
	LastReceivedAt     time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (AdmissionLinkIncident) TableName() string {
	return "kubesentry.admission_link_incidents"
}
