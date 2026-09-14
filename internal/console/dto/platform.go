package dto

import "time"

type (
	PlatformOverviewOptions struct {
		Page     int
		PageSize int
		Health   string
	}
	PlatformTrendOptions struct {
		Component string
		Instance  string
		Cluster   string
		From      *time.Time
		To        *time.Time
	}
	PlatformLeadershipOptions struct {
		Page      int
		PageSize  int
		Component string
		Cluster   string
		From      *time.Time
		To        *time.Time
	}
)

type PlatformDatabaseStatsView struct {
	Connections     int     `json:"connections"`
	MaxConnections  int     `json:"maxConnections"`
	Transactions    int     `json:"transactions"`
	Rollbacks       int     `json:"rollbacks"`
	CacheHitRatio   float64 `json:"cacheHitRatio"`
	DatabaseBytes   int     `json:"databaseBytes"`
	PoolInUse       int     `json:"poolInUse"`
	PoolIdle        int     `json:"poolIdle"`
	PoolWaitCount   int     `json:"poolWaitCount"`
	PoolWaitSeconds float64 `json:"poolWaitSeconds"`
}

type PlatformInstanceView struct {
	Instance         string                     `json:"instance"`
	Role             string                     `json:"role"`
	Health           string                     `json:"health"`
	HealthReason     string                     `json:"healthReason"`
	UptimeSeconds    int                        `json:"uptimeSeconds"`
	RestartCount     int                        `json:"restartCount"`
	MemoryBytes      int                        `json:"memoryBytes"`
	MemoryLimitBytes int                        `json:"memoryLimitBytes"`
	CPUMillis        int                        `json:"cpuMillis"`
	Goroutines       int                        `json:"goroutines"`
	HeapAllocBytes   int                        `json:"heapAllocBytes"`
	HeapSysBytes     int                        `json:"heapSysBytes"`
	LastObservedAt   time.Time                  `json:"lastObservedAt"`
	DBStats          *PlatformDatabaseStatsView `json:"dbStats,omitempty"`
}

type PlatformComponentView struct {
	Component            string                 `json:"component"`
	Health               string                 `json:"health"`
	HealthReason         string                 `json:"healthReason"`
	HasLeadership        bool                   `json:"hasLeadership"`
	NoLeader             bool                   `json:"noLeader"`
	SingleReplica        bool                   `json:"singleReplica"`
	Instances            []PlatformInstanceView `json:"instances"`
	LastLeadershipChange *time.Time             `json:"lastLeadershipChange,omitempty"`
}

type (
	PlatformClusterView struct {
		Name       string                  `json:"name"`
		Phase      string                  `json:"phase"`
		Health     string                  `json:"health"`
		Components []PlatformComponentView `json:"components"`
	}
	PlatformMemberPlaneView struct {
		Clusters []PlatformClusterView `json:"clusters"`
		Total    int                   `json:"total"`
		Page     int                   `json:"page"`
		PageSize int                   `json:"pageSize"`
	}
	PlatformOverviewView struct {
		Enabled        bool                    `json:"enabled"`
		PlatformHealth string                  `json:"platformHealth"`
		GatewayPlane   []PlatformComponentView `json:"gatewayPlane"`
		MemberPlane    PlatformMemberPlaneView `json:"memberPlane"`
	}
)

type (
	PlatformTrendPointView struct {
		BucketStartedAt time.Time `json:"bucketStartedAt"`
		CPUUsageMicros  int       `json:"cpuUsageMicros"`
		MemoryBytes     int       `json:"memoryBytes"`
		Goroutines      int       `json:"goroutines"`
		GCPauseMicros   int       `json:"gcPauseMicros"`
	}
	PlatformTrendView struct {
		Items       []PlatformTrendPointView `json:"items"`
		StepSeconds int                      `json:"stepSeconds"`
		From        time.Time                `json:"from"`
		To          time.Time                `json:"to"`
	}
)

type (
	PlatformLeadershipEventView struct {
		ID          string    `json:"id"`
		Component   string    `json:"component"`
		ClusterID   string    `json:"clusterId"`
		Instance    string    `json:"instance"`
		ClusterName string    `json:"clusterName"`
		RoleFrom    string    `json:"roleFrom"`
		RoleTo      string    `json:"roleTo"`
		ChangedAt   time.Time `json:"changedAt"`
		DetectedBy  string    `json:"detectedBy"`
		CreatedAt   time.Time `json:"createdAt"`
	}
	PlatformLeadershipPageView struct {
		Total    int                           `json:"total"`
		Page     int                           `json:"page"`
		PageSize int                           `json:"pageSize"`
		Items    []PlatformLeadershipEventView `json:"items"`
	}
)

type (
	PlatformSettingsView struct {
		Enabled          bool `json:"enabled"`
		RetentionDays    int  `json:"retentionDays"`
		MaxRetentionDays int  `json:"maxRetentionDays"`
	}
	PlatformSettingsInput struct {
		Enabled       bool `json:"enabled"`
		RetentionDays int  `json:"retentionDays"`
	}
)
