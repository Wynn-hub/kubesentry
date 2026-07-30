// Package dbrepo is the GORM data-access layer shared by cmd/gateway and
// cmd/console. Table shapes match spec section 二; jsonb columns reuse the
// existing v1alpha1 Spec structs verbatim instead of redefining them.
package dbrepo

import (
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

type Cluster struct {
	dbmodel.Base
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
	dbmodel.Base
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
	dbmodel.Base
	PolicyID     int64
	Version      int
	Rego         string
	SpecSnapshot datatypes.JSONType[v1alpha1.PolicySpec]
}

func (PolicyVersion) TableName() string { return "kubesentry.policy_versions" }

type PolicyGroup struct {
	dbmodel.Base
	Name        string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec        datatypes.JSONType[v1alpha1.PolicyGroupSpec]
	Labels      datatypes.JSONType[map[string]string]
	Annotations datatypes.JSONType[map[string]string]
}

func (PolicyGroup) TableName() string { return "kubesentry.policy_groups" }

type PolicyException struct {
	dbmodel.Base
	Name        string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec        datatypes.JSONType[v1alpha1.PolicyExceptionSpec]
	ExpiresAt   *time.Time
	Labels      datatypes.JSONType[map[string]string]
	Annotations datatypes.JSONType[map[string]string]
}

func (PolicyException) TableName() string { return "kubesentry.policy_exceptions" }

type PolicyDistribution struct {
	dbmodel.Base
	Name      string `gorm:"uniqueIndex:,where:deleted_at IS NULL"`
	Spec      datatypes.JSONType[v1alpha1.PolicyDistributionSpec]
	Managed   bool
	OwnerType string
	OwnerName string
}

func (PolicyDistribution) TableName() string { return "kubesentry.policy_distributions" }

type DistributionClusterStatus struct {
	dbmodel.Base
	DistributionID     int64
	ClusterID          int64
	Status             string
	AppliedVersionHash string
	LastError          string
}

func (DistributionClusterStatus) TableName() string { return "kubesentry.distribution_cluster_status" }

type ClusterInstallRecord struct {
	dbmodel.Base
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
	dbmodel.Base
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
	ID                   int64 `gorm:"primaryKey;autoIncrement:false"`
	DefaultRetentionDays int
	RetryEnabled         bool
	MaxRetryAttempts     int
	QueueCapacity        int
	StoragePolicyVersion int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (AdmissionEventSettings) TableName() string { return "kubesentry.admission_event_settings" }

// AdmissionStoragePolicy is a conditions-only storage rule: matching any rule
// stores the event, an empty rule set stores everything, and retention comes
// from the global AdmissionEventSettings. Priority only fixes the display order.
type AdmissionStoragePolicy struct {
	dbmodel.Base
	Priority   int
	MatchMode  string
	Verdicts   pq.StringArray `gorm:"type:text[]"`
	Clusters   pq.StringArray `gorm:"type:text[]"`
	GVKs       pq.StringArray `gorm:"column:gvks;type:text[]"`
	Namespaces pq.StringArray `gorm:"type:text[]"`
}

func (AdmissionStoragePolicy) TableName() string { return "kubesentry.admission_storage_policies" }

type AdmissionEvent struct {
	dbmodel.Base
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
	ID                       int64 `gorm:"primaryKey;autoIncrement:false"`
	AdmissionPipelineEnabled bool
	LastCleanupAt            *time.Time
	// RetentionDays remains a transient compatibility field for the existing
	// admission-pipeline cleanup path. Platform samples use fixed retention.
	RetentionDays int `gorm:"-"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (PlatformObservabilitySettings) TableName() string {
	return "kubesentry.platform_observability_settings"
}

type PlatformComponentSample struct {
	ID                 int64 `gorm:"primaryKey;autoIncrement:false"`
	BucketStartedAt    time.Time
	Component          string
	Instance           string
	ClusterID          int64
	ClusterName        string
	Role               string
	ProcessStartedAt   time.Time
	RestartCount       int64
	MemoryBytes        int64
	MemoryLimitBytes   int64
	CPUUsageMicros     int64 `gorm:"column:cpu_usage_micros"`
	CPUQuotaMicros     int64 `gorm:"column:cpu_quota_micros"`
	Goroutines         int64
	HeapAllocBytes     int64
	HeapSysBytes       int64
	GCPauseTotalMicros int64 `gorm:"column:gc_pause_total_micros"`
	GCCount            int64 `gorm:"column:gc_count"`
	Health             string
	HealthReason       string
	DBStats            datatypes.JSONType[map[string]any]
	LastObservedAt     time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (PlatformComponentSample) TableName() string {
	return "kubesentry.platform_component_samples"
}

type PlatformLeadershipEvent struct {
	ID          int64 `gorm:"primaryKey;autoIncrement:false"`
	Component   string
	ClusterID   int64
	ClusterName string
	Instance    string
	RoleFrom    string
	RoleTo      string
	ChangedAt   time.Time
	DetectedBy  string
	CreatedAt   time.Time
}

func (PlatformLeadershipEvent) TableName() string {
	return "kubesentry.platform_leadership_events"
}

type AdmissionObservationBucket struct {
	ID                     int64 `gorm:"primaryKey;autoIncrement:false"`
	BucketStartedAt        time.Time
	ClusterID              int64
	ClusterName            string
	Component              string
	GeneratedCount         int64
	ReceivedCount          int64
	ForwardedCount         int64
	WrittenCount           int64
	DuplicateCount         int64
	PolicyFilteredCount    int64
	RetentionElapsedCount  int64
	QueueRejectedCount     int64
	RetryCount             int64
	DroppedCount           int64
	RetryExhaustedCount    int64
	BatchFailureCount      int64
	AuthFailureCount       int64
	ACKTimeoutCount        int64 `gorm:"column:ack_timeout_count"`
	ReportDroppedCount     int64
	ObservationWriteFailed int64
	SequenceGapCount       int64
	BaselineResetCount     int64
	SeriesStartedCount     int64
	CleanupRunCount        int64
	CleanupDeletedCount    int64
	CleanupFailureCount    int64
	QueueDepthMax          int64
	QueueCapacity          int64
	FaultReasonCounts      datatypes.JSONType[map[string]int64]
	PressureReasonCounts   datatypes.JSONType[map[string]int64]
	FilterReasonCounts     datatypes.JSONType[map[string]int64]
	LastObservedAt         time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (AdmissionObservationBucket) TableName() string {
	return "kubesentry.admission_observation_buckets"
}

type ObservationReportCursor struct {
	ID              int64 `gorm:"primaryKey;autoIncrement:false"`
	ClusterID       int64
	SeriesID        string
	Component       string
	LastSequence    int64
	CounterSnapshot datatypes.JSON
	LastActiveAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (ObservationReportCursor) TableName() string {
	return "kubesentry.observation_report_cursors"
}
