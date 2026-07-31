package platformobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadCgroupV2(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "cgroup.controllers", "cpu memory\n")
	writeTestFile(t, root, "memory.current", "4096\n")
	writeTestFile(t, root, "memory.max", "8192\n")
	writeTestFile(t, root, "cpu.stat", "usage_usec 12345\nuser_usec 10000\n")
	writeTestFile(t, root, "cpu.max", "50000 100000\n")

	got := readCgroup(root)
	if got.memoryBytes != 4096 || got.memoryLimitBytes != 8192 {
		t.Fatalf("memory = %d/%d, want 4096/8192", got.memoryBytes, got.memoryLimitBytes)
	}
	if got.cpuUsageMicros != 12345 || got.cpuQuotaMicros != 50000 {
		t.Fatalf("cpu = %d/%d, want 12345/50000", got.cpuUsageMicros, got.cpuQuotaMicros)
	}
}

func TestReadCgroupV2UnlimitedMemory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "cgroup.controllers", "cpu memory\n")
	writeTestFile(t, root, "memory.max", "max\n")

	if got := readCgroup(root).memoryLimitBytes; got != -1 {
		t.Fatalf("memory limit = %d, want -1", got)
	}
}

func TestReadCgroupV1AndTreatsHugeLimitAsUnlimited(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "memory/memory.usage_in_bytes", "2048\n")
	writeTestFile(t, root, "memory/memory.limit_in_bytes", "9223372036854771712\n")
	writeTestFile(t, root, "cpuacct/cpuacct.usage", "9000000\n")
	writeTestFile(t, root, "cpu/cpu.cfs_quota_us", "25000\n")

	got := readCgroup(root)
	if got.memoryBytes != 2048 || got.memoryLimitBytes != -1 {
		t.Fatalf("memory = %d/%d, want 2048/-1", got.memoryBytes, got.memoryLimitBytes)
	}
	if got.cpuUsageMicros != 9000 || got.cpuQuotaMicros != 25000 {
		t.Fatalf("cpu = %d/%d, want 9000/25000", got.cpuUsageMicros, got.cpuQuotaMicros)
	}
}

func TestReadCgroupMissingFieldsAreUnavailable(t *testing.T) {
	got := readCgroup(t.TempDir())
	if got.memoryBytes != -1 || got.memoryLimitBytes != -1 ||
		got.cpuUsageMicros != -1 || got.cpuQuotaMicros != -1 {
		t.Fatalf("missing metrics = %+v, want all -1", got)
	}
}

func TestCollectorProducesValidGoSample(t *testing.T) {
	start := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	collector := NewCollector(Identity{
		Component: ComponentGateway,
		Instance:  "gateway-0",
		Role:      func() Role { return RoleLeader },
	})
	collector.cgroupDir = t.TempDir()
	collector.startedAt = start
	collector.now = func() time.Time { return start.Add(time.Minute) }

	sample := collector.Collect()
	if err := Validate(sample); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if sample.Role != RoleLeader || sample.Goroutines < 1 || sample.HeapAllocBytes < 0 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
}

func TestUnavailableSampleUsesMinusOne(t *testing.T) {
	sample := UnavailableSample(ComponentConsoleWeb, "console-0", time.Now())
	if sample.Goroutines != -1 || sample.HeapAllocBytes != -1 ||
		sample.GCPauseTotalMicros != -1 || sample.MemoryBytes != -1 {
		t.Fatalf("unavailable sample = %+v", sample)
	}
}

func TestReadSiblingCgroupSelectsApplicationOverPauseContainer(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "cgroup.controllers", "cpu memory\n")
	writeTestFile(t, root, "pod/api/memory.current", "100\n")
	writeTestFile(t, root, "pod/api/memory.max", "1000\n")
	writeTestFile(t, root, "pod/api/cpu.stat", "usage_usec 10\n")
	writeTestFile(t, root, "pod/api/cpu.max", "100 100000\n")
	writeTestFile(t, root, "pod/web/memory.current", "300\n")
	writeTestFile(t, root, "pod/web/memory.max", "2000\n")
	writeTestFile(t, root, "pod/web/cpu.stat", "usage_usec 20\n")
	writeTestFile(t, root, "pod/web/cpu.max", "200 100000\n")
	writeTestFile(t, root, "pod/pause/memory.current", "5\n")
	writeTestFile(t, root, "pod/pause/memory.max", "max\n")
	writeTestFile(t, root, "pod/pause/cpu.stat", "usage_usec 1\n")
	writeTestFile(t, root, "pod/pause/cpu.max", "max 100000\n")
	proc := filepath.Join(t.TempDir(), "cgroup")
	if err := os.WriteFile(proc, []byte("0::/pod/api\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := readSiblingCgroup(root, proc)
	if got.memoryBytes != 300 || got.memoryLimitBytes != 2000 ||
		got.cpuUsageMicros != 20 || got.cpuQuotaMicros != 200 {
		t.Fatalf("sibling metrics = %+v", got)
	}
}

func TestComponentSampleWireRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sample := platformSampleForWire(now)
	wire := ToProto(sample)
	got, err := FromProto(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Component != sample.Component || got.Instance != sample.Instance ||
		got.MemoryBytes != sample.MemoryBytes || got.DBStats["connections"] != float64(3) {
		t.Fatalf("round trip = %+v", got)
	}
}

func platformSampleForWire(now time.Time) Sample {
	sample := UnavailableSample(ComponentPostgres, "postgres", now)
	sample.ProcessStartedAt = now.Add(-time.Hour)
	sample.DBStats = map[string]any{"connections": 3}
	return sample
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
