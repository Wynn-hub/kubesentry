package observability

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

// ProcessClock is the precision every process-start timestamp is rounded to.
//
// PostgreSQL's TIMESTAMPTZ stores microseconds, but time.Now() carries
// nanoseconds. Comparing an in-memory nanosecond value against one that has
// been through the database therefore never matches, and restart detection —
// which is exactly "did the start time change?" — counted a restart on every
// single sample. Truncating at the producer keeps the reported value equal to
// the stored value, so the comparison means what it says.
const ProcessClock = time.Microsecond

// NormalizeProcessStart rounds a start timestamp to the storage precision. Call
// it wherever a start time enters a Sample, not only where one is compared: any
// future comparison site would otherwise reintroduce the same bug.
func NormalizeProcessStart(value time.Time) time.Time {
	return value.UTC().Truncate(ProcessClock)
}

// Sample is a cumulative process observation. Counter fields are intentionally
// cumulative so a missed sample reduces resolution without losing totals.
type Sample struct {
	Component        Component `json:"component"`
	Instance         string    `json:"instance"`
	ClusterID        int64     `json:"clusterId"`
	ClusterName      string    `json:"clusterName"`
	ObservedAt       time.Time `json:"observedAt"`
	Role             Role      `json:"role"`
	ProcessStartedAt time.Time `json:"processStartedAt"`
	MemoryBytes      int64     `json:"memoryBytes"`
	MemoryLimitBytes int64     `json:"memoryLimitBytes"`
	// MemoryPressureEvents is the cgroup's own count of times allocation was
	// throttled or the container was OOM-killed (v2 memory.events max+oom_kill).
	// This is the only trustworthy "memory is a problem" signal: a Go process
	// legitimately grows its RSS toward the limit before GC reclaims, so a high
	// usage ratio is normal operation, not a fault.
	MemoryPressureEvents int64          `json:"memoryPressureEvents"`
	CPUUsageMicros       int64          `json:"cpuUsageMicros"`
	CPUQuotaMicros       int64          `json:"cpuQuotaMicros"`
	Goroutines           int64          `json:"goroutines"`
	HeapAllocBytes       int64          `json:"heapAllocBytes"`
	HeapSysBytes         int64          `json:"heapSysBytes"`
	GCPauseTotalMicros   int64          `json:"gcPauseTotalMicros"`
	GCCount              int64          `json:"gcCount"`
	Health               Health         `json:"health"`
	HealthReason         string         `json:"healthReason"`
	DBStats              map[string]any `json:"dbStats,omitempty"`
}

func UnavailableSample(component Component, instance string, observedAt time.Time) Sample {
	return Sample{
		Component:            component,
		Instance:             instance,
		ObservedAt:           observedAt,
		Role:                 RoleNone,
		MemoryBytes:          -1,
		MemoryLimitBytes:     -1,
		MemoryPressureEvents: -1,
		CPUUsageMicros:       -1,
		CPUQuotaMicros:       -1,
		Goroutines:           -1,
		HeapAllocBytes:       -1,
		HeapSysBytes:         -1,
		GCPauseTotalMicros:   -1,
		GCCount:              -1,
		Health:               HealthOK,
	}
}
