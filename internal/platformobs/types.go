package platformobs

import "time"

type Component string

const (
	ComponentWebhook    Component = "webhook"
	ComponentOperator   Component = "operator"
	ComponentGateway    Component = "gateway"
	ComponentConsoleAPI Component = "console-api"
	ComponentConsoleWeb Component = "console-web"
	ComponentPostgres   Component = "postgres"
)

func (c Component) Valid() bool {
	switch c {
	case ComponentWebhook, ComponentOperator, ComponentGateway,
		ComponentConsoleAPI, ComponentConsoleWeb, ComponentPostgres:
		return true
	default:
		return false
	}
}

type Role string

const (
	RoleLeader  Role = "leader"
	RoleStandby Role = "standby"
	RoleNone    Role = "none"
)

func (r Role) Valid() bool {
	switch r {
	case RoleLeader, RoleStandby, RoleNone:
		return true
	default:
		return false
	}
}

type Health string

const (
	HealthOK       Health = "ok"
	HealthDegraded Health = "degraded"
)

func (h Health) Valid() bool {
	switch h {
	case HealthOK, HealthDegraded:
		return true
	default:
		return false
	}
}

// Sample is a cumulative process observation. Counter fields are intentionally
// cumulative so a missed sample reduces resolution without losing totals.
type Sample struct {
	Component          Component      `json:"component"`
	Instance           string         `json:"instance"`
	ClusterID          int64          `json:"clusterId"`
	ClusterName        string         `json:"clusterName"`
	ObservedAt         time.Time      `json:"observedAt"`
	Role               Role           `json:"role"`
	ProcessStartedAt   time.Time      `json:"processStartedAt"`
	MemoryBytes        int64          `json:"memoryBytes"`
	MemoryLimitBytes   int64          `json:"memoryLimitBytes"`
	CPUUsageMicros     int64          `json:"cpuUsageMicros"`
	CPUQuotaMicros     int64          `json:"cpuQuotaMicros"`
	Goroutines         int64          `json:"goroutines"`
	HeapAllocBytes     int64          `json:"heapAllocBytes"`
	HeapSysBytes       int64          `json:"heapSysBytes"`
	GCPauseTotalMicros int64          `json:"gcPauseTotalMicros"`
	GCCount            int64          `json:"gcCount"`
	Health             Health         `json:"health"`
	HealthReason       string         `json:"healthReason"`
	DBStats            map[string]any `json:"dbStats,omitempty"`
}

func UnavailableSample(component Component, instance string, observedAt time.Time) Sample {
	return Sample{
		Component:          component,
		Instance:           instance,
		ObservedAt:         observedAt,
		Role:               RoleNone,
		MemoryBytes:        -1,
		MemoryLimitBytes:   -1,
		CPUUsageMicros:     -1,
		CPUQuotaMicros:     -1,
		Goroutines:         -1,
		HeapAllocBytes:     -1,
		HeapSysBytes:       -1,
		GCPauseTotalMicros: -1,
		GCCount:            -1,
		Health:             HealthOK,
	}
}
