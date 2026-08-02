package platformobs

import (
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func ToProto(sample Sample) *gatewaypb.ComponentSample {
	stats, _ := json.Marshal(sample.DBStats)
	return &gatewaypb.ComponentSample{
		Component:            string(sample.Component),
		Instance:             sample.Instance,
		ClusterId:            sample.ClusterID,
		ClusterName:          sample.ClusterName,
		ObservedAt:           timestamppb.New(sample.ObservedAt),
		Role:                 string(sample.Role),
		ProcessStartedAt:     timestamppb.New(sample.ProcessStartedAt),
		MemoryBytes:          sample.MemoryBytes,
		MemoryLimitBytes:     sample.MemoryLimitBytes,
		MemoryPressureEvents: sample.MemoryPressureEvents,
		CpuUsageMicros:       sample.CPUUsageMicros,
		CpuQuotaMicros:       sample.CPUQuotaMicros,
		Goroutines:           sample.Goroutines,
		HeapAllocBytes:       sample.HeapAllocBytes,
		HeapSysBytes:         sample.HeapSysBytes,
		GcPauseTotalMicros:   sample.GCPauseTotalMicros,
		GcCount:              sample.GCCount,
		Health:               string(sample.Health),
		HealthReason:         sample.HealthReason,
		DbStatsJson:          stats,
	}
}

func FromProto(wire *gatewaypb.ComponentSample) (Sample, error) {
	if wire == nil || wire.ObservedAt == nil || wire.ProcessStartedAt == nil {
		return Sample{}, errors.New("component sample timestamps are required")
	}
	sample := Sample{
		Component:            Component(wire.Component),
		Instance:             wire.Instance,
		ClusterID:            wire.ClusterId,
		ClusterName:          wire.ClusterName,
		ObservedAt:           wire.ObservedAt.AsTime(),
		Role:                 Role(wire.Role),
		ProcessStartedAt:     NormalizeProcessStart(wire.ProcessStartedAt.AsTime()),
		MemoryBytes:          wire.MemoryBytes,
		MemoryLimitBytes:     wire.MemoryLimitBytes,
		MemoryPressureEvents: wire.MemoryPressureEvents,
		CPUUsageMicros:       wire.CpuUsageMicros,
		CPUQuotaMicros:       wire.CpuQuotaMicros,
		Goroutines:           wire.Goroutines,
		HeapAllocBytes:       wire.HeapAllocBytes,
		HeapSysBytes:         wire.HeapSysBytes,
		GCPauseTotalMicros:   wire.GcPauseTotalMicros,
		GCCount:              wire.GcCount,
		Health:               Health(wire.Health),
		HealthReason:         wire.HealthReason,
		DBStats:              map[string]any{},
	}
	if len(wire.DbStatsJson) > 0 {
		if err := json.Unmarshal(wire.DbStatsJson, &sample.DBStats); err != nil {
			return Sample{}, errors.New("invalid component sample db stats")
		}
	}
	return sample, Validate(sample)
}
