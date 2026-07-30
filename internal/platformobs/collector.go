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
		cgroupDir: "/sys/fs/cgroup",
		startedAt: time.Now().UTC(),
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

	resource := readCgroup(c.cgroupDir)
	sample.MemoryBytes = resource.memoryBytes
	sample.MemoryLimitBytes = resource.memoryLimitBytes
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

type cgroupMetrics struct {
	memoryBytes      int64
	memoryLimitBytes int64
	cpuUsageMicros   int64
	cpuQuotaMicros   int64
}

func unavailableCgroupMetrics() cgroupMetrics {
	return cgroupMetrics{
		memoryBytes:      -1,
		memoryLimitBytes: -1,
		cpuUsageMicros:   -1,
		cpuQuotaMicros:   -1,
	}
}

func readCgroup(root string) cgroupMetrics {
	result := unavailableCgroupMetrics()
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err == nil {
		result.memoryBytes = readInt64(filepath.Join(root, "memory.current"), 1)
		result.memoryLimitBytes = readMemoryLimit(filepath.Join(root, "memory.max"), false)
		result.cpuUsageMicros = readCPUStat(filepath.Join(root, "cpu.stat"))
		result.cpuQuotaMicros = readCPUQuotaV2(filepath.Join(root, "cpu.max"))
		return result
	}

	result.memoryBytes = readInt64(filepath.Join(root, "memory", "memory.usage_in_bytes"), 1)
	result.memoryLimitBytes = readMemoryLimit(filepath.Join(root, "memory", "memory.limit_in_bytes"), true)
	result.cpuUsageMicros = readInt64(filepath.Join(root, "cpuacct", "cpuacct.usage"), 1000)
	result.cpuQuotaMicros = readMemoryLimit(filepath.Join(root, "cpu", "cpu.cfs_quota_us"), false)
	return result
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
