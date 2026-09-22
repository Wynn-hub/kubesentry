package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

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
	Connections     int     `json:"connections" binding:"required"`
	MaxConnections  int     `json:"maxConnections" binding:"required"`
	Transactions    int     `json:"transactions" binding:"required"`
	Rollbacks       int     `json:"rollbacks" binding:"required"`
	CacheHitRatio   float64 `json:"cacheHitRatio" binding:"required" format:"double"`
	DatabaseBytes   int     `json:"databaseBytes" binding:"required"`
	PoolInUse       int     `json:"poolInUse" binding:"required"`
	PoolIdle        int     `json:"poolIdle" binding:"required"`
	PoolWaitCount   int     `json:"poolWaitCount" binding:"required"`
	PoolWaitSeconds float64 `json:"poolWaitSeconds" binding:"required" format:"double"`
}

type PlatformInstanceView struct {
	Instance         string                     `json:"instance" binding:"required"`
	Role             string                     `json:"role" binding:"required" enums:"leader,standby,none"`
	Health           string                     `json:"health" binding:"required" enums:"ok,degraded,no_data"`
	HealthReason     string                     `json:"healthReason" binding:"required"`
	UptimeSeconds    int                        `json:"uptimeSeconds" binding:"required"`
	RestartCount     int                        `json:"restartCount" binding:"required"`
	MemoryBytes      int                        `json:"memoryBytes" binding:"required"`
	MemoryLimitBytes int                        `json:"memoryLimitBytes" binding:"required"`
	CPUMillis        int                        `json:"cpuMillis" binding:"required"`
	Goroutines       int                        `json:"goroutines" binding:"required"`
	HeapAllocBytes   int                        `json:"heapAllocBytes" binding:"required"`
	HeapSysBytes     int                        `json:"heapSysBytes" binding:"required"`
	LastObservedAt   time.Time                  `json:"lastObservedAt" binding:"required" format:"date-time"`
	DBStats          *PlatformDatabaseStatsView `json:"dbStats,omitempty"`
}

type PlatformComponentView struct {
	Component            string                 `json:"component" binding:"required" enums:"gateway,postgres,console-api,console-web,operator,webhook"`
	Health               string                 `json:"health" binding:"required" enums:"ok,degraded,unavailable,pending"`
	HealthReason         string                 `json:"healthReason" binding:"required"`
	HasLeadership        bool                   `json:"hasLeadership" binding:"required"`
	NoLeader             bool                   `json:"noLeader" binding:"required"`
	SingleReplica        bool                   `json:"singleReplica" binding:"required"`
	Instances            []PlatformInstanceView `json:"instances" binding:"required"`
	LastLeadershipChange *time.Time             `json:"lastLeadershipChange,omitempty" format:"date-time"`
}

type (
	PlatformClusterView struct {
		Name       string                  `json:"name" binding:"required"`
		Phase      string                  `json:"phase" binding:"required" enums:"Registering,Online,Offline,Uninstalling"`
		Health     string                  `json:"health" binding:"required" enums:"ok,degraded,unavailable"`
		Components []PlatformComponentView `json:"components" binding:"required"`
	}
	PlatformMemberPlaneView struct {
		Clusters []PlatformClusterView `json:"clusters" binding:"required"`
		Total    int                   `json:"total" binding:"required"`
		Page     int                   `json:"page" binding:"required"`
		PageSize int                   `json:"pageSize" binding:"required"`
	}
	PlatformOverviewView struct {
		Enabled        bool                    `json:"enabled" binding:"required"`
		PlatformHealth string                  `json:"platformHealth" binding:"required" enums:"ok,degraded,unavailable,disabled"`
		GatewayPlane   []PlatformComponentView `json:"gatewayPlane" binding:"required"`
		MemberPlane    PlatformMemberPlaneView `json:"memberPlane" binding:"required"`
	}
)

type (
	PlatformTrendPointView struct {
		BucketStartedAt time.Time `json:"bucketStartedAt" binding:"required" format:"date-time"`
		CPUUsageMicros  int       `json:"cpuUsageMicros" binding:"required"`
		MemoryBytes     int       `json:"memoryBytes" binding:"required"`
		Goroutines      int       `json:"goroutines" binding:"required"`
		GCPauseMicros   int       `json:"gcPauseMicros" binding:"required"`
	}
	PlatformTrendView struct {
		Items       []PlatformTrendPointView `json:"items" binding:"required"`
		StepSeconds int                      `json:"stepSeconds" binding:"required"`
		From        time.Time                `json:"from" binding:"required" format:"date-time"`
		To          time.Time                `json:"to" binding:"required" format:"date-time"`
	}
)

type (
	PlatformLeadershipEventView struct {
		ID          string    `json:"id" binding:"required" pattern:"^[0-9]+$"`
		Component   string    `json:"component" binding:"required"`
		ClusterID   string    `json:"clusterId" binding:"required" pattern:"^[0-9]+$"`
		Instance    string    `json:"instance" binding:"required"`
		ClusterName string    `json:"clusterName" binding:"required"`
		RoleFrom    string    `json:"roleFrom" binding:"required" enums:"leader,standby,none"`
		RoleTo      string    `json:"roleTo" binding:"required" enums:"leader,standby,none"`
		ChangedAt   time.Time `json:"changedAt" binding:"required" format:"date-time"`
		DetectedBy  string    `json:"detectedBy" binding:"required"`
		CreatedAt   time.Time `json:"createdAt" binding:"required" format:"date-time"`
	}
	PlatformLeadershipPageView struct {
		Total    int                           `json:"total" binding:"required"`
		Page     int                           `json:"page" binding:"required"`
		PageSize int                           `json:"pageSize" binding:"required" enums:"10,20,50,100"`
		Items    []PlatformLeadershipEventView `json:"items" binding:"required"`
	}
)

type (
	PlatformSettingsView struct {
		Enabled          bool `json:"enabled" binding:"required"`
		RetentionDays    int  `json:"retentionDays" binding:"required" minimum:"1"`
		MaxRetentionDays int  `json:"maxRetentionDays" binding:"required"`
	}
	PlatformSettingsInput struct {
		Enabled       bool `json:"enabled" binding:"required"`
		RetentionDays int  `json:"retentionDays" binding:"required" minimum:"1"`
	}
)

func (r *PlatformSettingsInput) UnmarshalJSON(data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	enabled, ok := object["enabled"]
	if !ok {
		return fmt.Errorf("enabled: field is required")
	}
	if bytes.Equal(bytes.TrimSpace(enabled), []byte("null")) {
		return fmt.Errorf("enabled: must be a boolean")
	}
	if err := json.Unmarshal(enabled, &r.Enabled); err != nil {
		return fmt.Errorf("enabled: %w", err)
	}
	retentionDays, ok := object["retentionDays"]
	if !ok {
		return fmt.Errorf("retentionDays: field is required")
	}
	if bytes.Equal(bytes.TrimSpace(retentionDays), []byte("null")) {
		return fmt.Errorf("retentionDays: must be an integer")
	}
	if err := json.Unmarshal(retentionDays, &r.RetentionDays); err != nil {
		return fmt.Errorf("retentionDays: %w", err)
	}
	return nil
}

func (r *PlatformSettingsInput) Validate() error {
	if r.RetentionDays < 1 {
		return fmt.Errorf("retentionDays: must be at least 1")
	}
	return nil
}

type PlatformOverviewHTTPResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    PlatformOverviewView `json:"data"`
}

func (*PlatformOverviewHTTPResponse) getPlatformOverviewRes() {}

type PlatformTrendHTTPResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    PlatformTrendView `json:"data"`
}

func (*PlatformTrendHTTPResponse) getPlatformInstanceTrendRes() {}

type PlatformLeadershipHTTPResponse struct {
	Code    int                        `json:"code"`
	Message string                     `json:"message"`
	Data    PlatformLeadershipPageView `json:"data"`
}

func (*PlatformLeadershipHTTPResponse) listPlatformLeadershipEventsRes() {}

type PlatformSettingsHTTPResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    PlatformSettingsView `json:"data"`
}

func (*PlatformSettingsHTTPResponse) getPlatformSettingsRes()    {}
func (*PlatformSettingsHTTPResponse) updatePlatformSettingsRes() {}

type PlatformBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PlatformBadRequestHTTPError) HTTPStatus() int                  { return 400 }
func (*PlatformBadRequestHTTPError) getPlatformOverviewRes()          {}
func (*PlatformBadRequestHTTPError) getPlatformInstanceTrendRes()     {}
func (*PlatformBadRequestHTTPError) listPlatformLeadershipEventsRes() {}
func (*PlatformBadRequestHTTPError) updatePlatformSettingsRes()       {}

type PlatformNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PlatformNotFoundHTTPError) HTTPStatus() int                  { return 404 }
func (*PlatformNotFoundHTTPError) getPlatformInstanceTrendRes()     {}
func (*PlatformNotFoundHTTPError) listPlatformLeadershipEventsRes() {}

type PlatformInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*PlatformInternalServerHTTPError) HTTPStatus() int                  { return 500 }
func (*PlatformInternalServerHTTPError) getPlatformOverviewRes()          {}
func (*PlatformInternalServerHTTPError) getPlatformInstanceTrendRes()     {}
func (*PlatformInternalServerHTTPError) listPlatformLeadershipEventsRes() {}
func (*PlatformInternalServerHTTPError) getPlatformSettingsRes()          {}
func (*PlatformInternalServerHTTPError) updatePlatformSettingsRes()       {}
