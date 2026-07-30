package webhook

import (
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
