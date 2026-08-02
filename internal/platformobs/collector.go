package platformobs

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	runtimemetrics "runtime/metrics"
	"strconv"
	"strings"
	"time"
)

const unlimitedV1Threshold = int64(1 << 60)

const (
	defaultCgroupRoot = "/sys/fs/cgroup"
	defaultProcCgroup = "/proc/self/cgroup"
)

type Identity struct {
	Component   Component
	Instance    string
	ClusterID   int64
	ClusterName string
	Role        func() Role
}

type Collector struct {
	identity  Identity
	cgroupDir string
	startedAt time.Time
	now       func() time.Time
}

func NewCollector(identity Identity) *Collector {
	return &Collector{
		identity:  identity,
		cgroupDir: defaultCgroupRoot,
		startedAt: NormalizeProcessStart(time.Now()),
		now:       time.Now,
	}
}

func (c *Collector) Collect() Sample {
	now := c.now().UTC()
	sample := UnavailableSample(c.identity.Component, c.identity.Instance, now)
	sample.ClusterID = c.identity.ClusterID
	sample.ClusterName = c.identity.ClusterName
	sample.ProcessStartedAt = c.startedAt
	if c.identity.Role != nil {
		sample.Role = c.identity.Role()
	}

	resource := readCgroup(resolveCurrentCgroup(c.cgroupDir, defaultProcCgroup))
	sample.MemoryBytes = resource.memoryBytes
	sample.MemoryLimitBytes = resource.memoryLimitBytes
	sample.MemoryPressureEvents = resource.memoryPressureEvents
	sample.CPUUsageMicros = resource.cpuUsageMicros
	sample.CPUQuotaMicros = resource.cpuQuotaMicros

	goMetrics := readGoMetrics()
	sample.Goroutines = goMetrics.goroutines
	sample.HeapAllocBytes = goMetrics.heapAllocBytes
	sample.HeapSysBytes = goMetrics.heapSysBytes
	sample.GCPauseTotalMicros = goMetrics.gcPauseTotalMicros
	sample.GCCount = goMetrics.gcCount
	return sample
}

// CollectSiblingCgroup returns a non-Go component sample for a sibling
// container in the same Pod. On cgroup v2 hosts, the current container path
// identifies the Pod parent; the sibling with the largest memory footprint is
// selected so the tiny pause container is not mistaken for the application
// container. Private cgroup namespaces may hide the parent entirely, in which
// case all resource fields remain -1 as required by the wire contract.
func CollectSiblingCgroup(component Component, instance string, observedAt, processStartedAt time.Time) Sample {
	sample := UnavailableSample(component, instance, observedAt.UTC())
	sample.ProcessStartedAt = NormalizeProcessStart(processStartedAt)
	resource := readSiblingCgroup(defaultCgroupRoot, defaultProcCgroup)
	sample.MemoryBytes = resource.memoryBytes
	sample.MemoryLimitBytes = resource.memoryLimitBytes
	sample.MemoryPressureEvents = resource.memoryPressureEvents
	sample.CPUUsageMicros = resource.cpuUsageMicros
	sample.CPUQuotaMicros = resource.cpuQuotaMicros
	return sample
}

type cgroupMetrics struct {
	memoryBytes          int64
	memoryLimitBytes     int64
	memoryPressureEvents int64
	cpuUsageMicros       int64
	cpuQuotaMicros       int64
}

func unavailableCgroupMetrics() cgroupMetrics {
	return cgroupMetrics{
		memoryBytes:          -1,
		memoryLimitBytes:     -1,
		memoryPressureEvents: -1,
		cpuUsageMicros:       -1,
		cpuQuotaMicros:       -1,
	}
}

func readCgroup(root string) cgroupMetrics {
	result := unavailableCgroupMetrics()
	if fileExists(filepath.Join(root, "cgroup.controllers")) ||
		fileExists(filepath.Join(root, "memory.current")) {
		result.memoryBytes = readInt64(filepath.Join(root, "memory.current"), 1)
		result.memoryLimitBytes = readMemoryLimit(filepath.Join(root, "memory.max"), false)
		result.memoryPressureEvents = readMemoryPressureEvents(filepath.Join(root, "memory.events"))
		result.cpuUsageMicros = readCPUStat(filepath.Join(root, "cpu.stat"))
		result.cpuQuotaMicros = readCPUQuotaV2(filepath.Join(root, "cpu.max"))
		return result
	}

	result.memoryBytes = readInt64(filepath.Join(root, "memory", "memory.usage_in_bytes"), 1)
	result.memoryLimitBytes = readMemoryLimit(filepath.Join(root, "memory", "memory.limit_in_bytes"), true)
	// cgroup v1 exposes the same idea as failcnt: allocations that hit the limit.
	result.memoryPressureEvents = readInt64(filepath.Join(root, "memory", "memory.failcnt"), 1)
	result.cpuUsageMicros = readInt64(filepath.Join(root, "cpuacct", "cpuacct.usage"), 1000)
	result.cpuQuotaMicros = readMemoryLimit(filepath.Join(root, "cpu", "cpu.cfs_quota_us"), false)
	return result
}

// readMemoryPressureEvents sums the cgroup v2 counters that mean "the limit
// actually bit": `max` counts allocations forced to reclaim at the ceiling, and
// `oom_kill` counts processes killed for it. `low`/`high` are excluded — they
// fire under soft targets that are not configured here.
func readMemoryPressureEvents(path string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	total := int64(-1)
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[0] != "max" && fields[0] != "oom_kill") {
			continue
		}
		value, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil || value < 0 {
			continue
		}
		if total < 0 {
			total = 0
		}
		total += value
	}
	return total
}

func resolveCurrentCgroup(root, procCgroup string) string {
	if !fileExists(filepath.Join(root, "cgroup.controllers")) {
		return root
	}
	raw, err := os.ReadFile(procCgroup)
	if err != nil {
		return root
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 || fields[0] != "0" {
			continue
		}
		relative := strings.TrimPrefix(filepath.Clean(fields[2]), string(filepath.Separator))
		if relative == "." || relative == "" {
			return root
		}
		candidate := filepath.Join(root, relative)
		if fileExists(filepath.Join(candidate, "memory.current")) {
			return candidate
		}
	}
	return root
}

func readSiblingCgroup(root, procCgroup string) cgroupMetrics {
	current := resolveCurrentCgroup(root, procCgroup)
	if current == root {
		return unavailableCgroupMetrics()
	}
	parent := filepath.Dir(current)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return unavailableCgroupMetrics()
	}
	var selected cgroupMetrics
	found := false
	for _, entry := range entries {
		if !entry.IsDir() || filepath.Join(parent, entry.Name()) == current {
			continue
		}
		candidate := readCgroup(filepath.Join(parent, entry.Name()))
		if candidate.memoryBytes < 0 {
			continue
		}
		if !found || candidate.memoryBytes > selected.memoryBytes {
			selected, found = candidate, true
		}
	}
	if !found {
		return unavailableCgroupMetrics()
	}
	return selected
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readMemoryLimit(path string, v1 bool) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	value := strings.TrimSpace(string(raw))
	if value == "max" {
		return -1
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || (v1 && parsed >= unlimitedV1Threshold) {
		return -1
	}
	return parsed
}

func readInt64(path string, divisor int64) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || value < 0 {
		return -1
	}
	return value / divisor
}

func readCPUStat(path string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			value, parseErr := strconv.ParseInt(fields[1], 10, 64)
			if parseErr == nil && value >= 0 {
				return value
			}
		}
	}
	return -1
}

func readCPUQuotaV2(path string) int64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 || fields[0] == "max" {
		return -1
	}
	value, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || value < 0 {
		return -1
	}
	return value
}

type goRuntimeMetrics struct {
	goroutines         int64
	heapAllocBytes     int64
	heapSysBytes       int64
	gcPauseTotalMicros int64
	gcCount            int64
}

func readGoMetrics() goRuntimeMetrics {
	samples := []runtimemetrics.Sample{
		{Name: "/sched/goroutines:goroutines"},
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/heap/free:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
		{Name: "/memory/classes/heap/unused:bytes"},
	}
	runtimemetrics.Read(samples)

	result := goRuntimeMetrics{
		goroutines:     metricUint(samples[0]),
		heapAllocBytes: metricUint(samples[1]),
	}
	for _, sample := range samples[1:] {
		value := metricUint(sample)
		if value >= 0 && result.heapSysBytes <= math.MaxInt64-value {
			result.heapSysBytes += value
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	if mem.PauseTotalNs <= math.MaxInt64 {
		result.gcPauseTotalMicros = int64(mem.PauseTotalNs / 1000)
	} else {
		result.gcPauseTotalMicros = math.MaxInt64
	}
	result.gcCount = int64(mem.NumGC)
	return result
}

func metricUint(sample runtimemetrics.Sample) int64 {
	if sample.Value.Kind() != runtimemetrics.KindUint64 {
		return -1
	}
	value := sample.Value.Uint64()
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

func Validate(sample Sample) error {
	if !sample.Component.Valid() {
		return errors.New("invalid component")
	}
	if strings.TrimSpace(sample.Instance) == "" {
		return errors.New("instance is required")
	}
	if !sample.Role.Valid() {
		return errors.New("invalid role")
	}
	if !sample.Health.Valid() {
		return errors.New("invalid health")
	}
	if sample.ObservedAt.IsZero() || sample.ProcessStartedAt.IsZero() {
		return errors.New("observation timestamps are required")
	}
	return nil
}
