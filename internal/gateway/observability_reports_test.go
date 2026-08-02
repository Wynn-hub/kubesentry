package gateway

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestHandleObservationReportBatchStoresAndAcknowledgesEachReport(t *testing.T) {
	repos := newTestRepos(t)
	cluster := &dbrepo.Cluster{Name: "observed", Phase: "Online"}
	mustCreateCluster(t, repos, cluster)
	if err := enableAdmissionObservation(context.Background(), repos); err != nil {
		t.Fatal(err)
	}
	server := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	now := time.Now().UTC()
	batch := &gatewaypb.ObservationReportBatch{Reports: []*gatewaypb.ObservationReport{
		{
			ReportId: "valid-1", SeriesId: "series-1", Sequence: 1,
			ObservedAt: timestamppb.New(now), Component: string(admissionobs.ComponentWebhook),
			Counters:      &gatewaypb.ObservationCounters{Generated: 7},
			QueueDepthMax: 5, QueueCapacity: 256,
			FaultReasonCounts: map[string]int64{"future_fault": 2},
		},
		{
			ReportId: "invalid-1", SeriesId: "series-2", Sequence: 1,
			ObservedAt: timestamppb.New(now), Component: "invalid",
			Counters: &gatewaypb.ObservationCounters{},
		},
	}}
	ack := server.handleObservationReportBatch(context.Background(), cluster.Name, batch)
	if len(ack.Results) != 2 {
		t.Fatalf("ack results = %d, want 2", len(ack.Results))
	}
	if ack.Results[0].Status != gatewaypb.ObservationReportBatchAck_Result_ACCEPTED {
		t.Fatalf("valid ack = %+v", ack.Results[0])
	}
	if ack.Results[1].Status != gatewaypb.ObservationReportBatchAck_Result_DISCARDED {
		t.Fatalf("invalid ack = %+v", ack.Results[1])
	}
	aggregates, err := repos.Observability.Aggregate(context.Background(), &cluster.ID, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].Generated != 7 {
		t.Fatalf("aggregates = %+v", aggregates)
	}
	if aggregates[0].FaultReasonCounts[admissionobs.ReasonUnknown] != 2 {
		t.Fatalf("fault reasons = %+v", aggregates[0].FaultReasonCounts)
	}
}

func TestGatewayObservationCollectorEvictsOldestAndTracksLoss(t *testing.T) {
	collector := NewGatewayObservationCollector(nil, time.Hour)
	now := time.Now().UTC().Truncate(time.Minute)
	for index := 0; index <= gatewayObservationBufferCapacity; index++ {
		collector.Record(dbrepo.ObservationBucketDelta{
			BucketStartedAt: now.Add(time.Duration(index) * time.Minute),
			ClusterID:       int64(index + 1),
			Component:       admissionobs.ComponentGateway,
			LastObservedAt:  now,
		})
	}
	if got := collector.BufferedBuckets(); got != gatewayObservationBufferCapacity {
		t.Fatalf("buffered = %d, want %d", got, gatewayObservationBufferCapacity)
	}
	if got := collector.PendingWriteFailures(); got != 1 {
		t.Fatalf("pending write failures = %d, want 1", got)
	}
}

func TestGatewayObservationCollectorDropsBufferedAndNewDataWhenDisabled(t *testing.T) {
	collector := NewGatewayObservationCollector(nil, time.Hour)
	collector.Record(dbrepo.ObservationBucketDelta{
		BucketStartedAt: time.Now(),
		Component:       admissionobs.ComponentGateway,
	})
	if collector.BufferedBuckets() != 1 {
		t.Fatal("collector did not buffer the enabled observation")
	}
	collector.SetEnabled(false)
	collector.Record(dbrepo.ObservationBucketDelta{
		BucketStartedAt: time.Now(),
		Component:       admissionobs.ComponentGateway,
	})
	if collector.BufferedBuckets() != 0 || collector.PendingWriteFailures() != 0 {
		t.Fatalf("disabled collector retained data: buckets=%d failures=%d",
			collector.BufferedBuckets(), collector.PendingWriteFailures())
	}
}

func TestGatewayObservationCollectorPersistsPendingWriteFailuresFirst(t *testing.T) {
	repos := newTestRepos(t)
	collector := NewGatewayObservationCollector(repos.Observability, time.Hour)
	collector.pendingWriteFailed.Store(3)
	now := time.Now().UTC()
	collector.flush(context.Background())
	if collector.PendingWriteFailures() != 0 {
		t.Fatalf("pending write failures were not cleared")
	}
	platform := int64(0)
	aggregates, err := repos.Observability.Aggregate(context.Background(), &platform, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].ObservationWriteFailed != 3 {
		t.Fatalf("aggregates = %+v", aggregates)
	}
}

func TestGatewayObservationCollectorFlushesBufferedDataOnShutdown(t *testing.T) {
	repos := newTestRepos(t)
	collector := NewGatewayObservationCollector(repos.Observability, time.Hour)
	now := time.Now().UTC().Truncate(time.Minute)
	collector.Record(dbrepo.ObservationBucketDelta{
		BucketStartedAt: now,
		ClusterID:       1,
		Component:       admissionobs.ComponentGateway,
		Snapshot:        admissionobs.Snapshot{Counters: admissionobs.Counters{Received: 2}},
		LastObservedAt:  now,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := collector.Start(ctx); err != nil {
		t.Fatal(err)
	}
	aggregates, err := repos.Observability.Aggregate(
		context.Background(),
		nil,
		now.Add(-time.Minute),
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].Received != 2 {
		t.Fatalf("aggregates = %+v, want shutdown-flushed count", aggregates)
	}
}

func TestNotifyChangeTriggersObservabilityCleanup(t *testing.T) {
	cleaner := NewObservabilityCleaner(&dbrepo.ObservabilityRepo{}, nil, time.Hour)
	server := NewServer(Repos{}, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	server.SetObservabilityCleaner(cleaner)
	if _, err := server.NotifyChange(context.Background(), &gatewaypb.NotifyChangeRequest{Topic: "policy"}); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.trigger) != 0 {
		t.Fatal("unrelated topic triggered observability cleanup")
	}
	if _, err := server.NotifyChange(context.Background(), &gatewaypb.NotifyChangeRequest{Topic: PlatformObservabilitySettingsTopic}); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.trigger) != 1 {
		t.Fatalf("pending cleanups = %d, want 1", len(cleaner.trigger))
	}
}
