package webhook

import (
	"fmt"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestWebhookObservationReportUsesCumulativeCountersAndWindowGauge(t *testing.T) {
	reporter := &webhookObservationReporter{
		counters: admissionobs.NewAtomicCounters(),
		seriesID: "0123456789abcdef0123456789abcdef",
	}
	reporter.counters.Add(admissionobs.Counters{Generated: 2})
	reporter.gauge.Observe(9, 256)
	reporter.gauge.Observe(2, 256)

	first := reporter.newReport()
	if first.Sequence != 1 || first.ReportId != reporter.seriesID+"-1" {
		t.Fatalf("first report identity = (%d, %q)", first.Sequence, first.ReportId)
	}
	if first.Counters.Generated != 2 || first.QueueDepthMax != 9 || first.QueueCapacity != 256 {
		t.Fatalf("first report = %#v", first)
	}

	reporter.counters.Add(admissionobs.Counters{Generated: 1})
	second := reporter.newReport()
	if second.Counters.Generated != 3 {
		t.Fatalf("second generated = %d, want cumulative value 3", second.Counters.Generated)
	}
	if second.QueueDepthMax != 2 {
		t.Fatalf("second peak = %d, want current queue depth 2", second.QueueDepthMax)
	}
}

func TestWebhookObservationQueueDropsOldestReport(t *testing.T) {
	reporter := &webhookObservationReporter{
		counters: admissionobs.NewAtomicCounters(),
		queue:    make([]*queuedObservation, 0, webhookObservationQueueSize),
		limit:    webhookObservationQueueSize,
		wake:     make(chan struct{}, 1),
	}
	for sequence := uint64(1); sequence <= webhookObservationQueueSize; sequence++ {
		reporter.enqueue(&gatewaypb.ObservationReport{ReportId: admissionobs.ReportID("series", sequence)})
	}
	reporter.enqueue(&gatewaypb.ObservationReport{ReportId: "newest"})

	if got := reporter.queue[0].report.ReportId; got != admissionobs.ReportID("series", 2) {
		t.Fatalf("oldest report = %q, want sequence 2", got)
	}
	snapshot := reporter.counters.Snapshot()
	if snapshot.Counters.ReportDropped != 1 ||
		snapshot.FaultReasonCounts[admissionobs.ReasonObservationQueueFull] != 1 {
		t.Fatalf("snapshot = %#v, want one observation queue drop", snapshot)
	}
}

func TestWebhookObservationQueueKeepsInFlightReportAndDropsOldestPending(t *testing.T) {
	reporter := &webhookObservationReporter{
		counters: admissionobs.NewAtomicCounters(),
		queue:    make([]*queuedObservation, 0, 2),
		limit:    2,
		wake:     make(chan struct{}, 1),
	}
	reporter.enqueue(&gatewaypb.ObservationReport{ReportId: "active"})
	reporter.enqueue(&gatewaypb.ObservationReport{ReportId: "pending"})
	if queued := reporter.front(); queued == nil || queued.report.ReportId != "active" {
		t.Fatalf("front = %#v, want active report", queued)
	}

	reporter.enqueue(&gatewaypb.ObservationReport{ReportId: "newest"})

	if len(reporter.queue) != 2 {
		t.Fatalf("queue length = %d, want 2", len(reporter.queue))
	}
	if got := reporter.queue[0].report.ReportId; got != "active" {
		t.Fatalf("in-flight report = %q, want active", got)
	}
	if got := reporter.queue[1].report.ReportId; got != "newest" {
		t.Fatalf("pending report = %q, want newest", got)
	}
}

func TestWebhookObservationQueueLimitSurvivesRepeatedDelivery(t *testing.T) {
	reporter := &webhookObservationReporter{
		counters: admissionobs.NewAtomicCounters(),
		queue:    make([]*queuedObservation, 0, 2),
		limit:    2,
		wake:     make(chan struct{}, 1),
	}
	for sequence := 1; sequence <= 100; sequence++ {
		id := fmt.Sprintf("series/%d", sequence)
		reporter.enqueue(&gatewaypb.ObservationReport{ReportId: id})
		if got := reporter.front(); got == nil || got.report.ReportId != id {
			t.Fatalf("front at sequence %d = %+v", sequence, got)
		}
		reporter.remove(id)
	}
	if len(reporter.queue) != 0 || cap(reporter.queue) != 2 {
		t.Fatalf("queue len/cap = %d/%d, want 0/2", len(reporter.queue), cap(reporter.queue))
	}
	if got := reporter.counters.Snapshot().Counters.ReportDropped; got != 0 {
		t.Fatalf("report dropped = %d, want 0", got)
	}
}
