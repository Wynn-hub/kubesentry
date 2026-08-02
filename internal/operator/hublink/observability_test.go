package hublink

import (
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func validWebhookObservation(reportID string, sequence uint64) *gatewaypb.ObservationReport {
	return &gatewaypb.ObservationReport{
		ReportId:   reportID,
		SeriesId:   "0123456789abcdef0123456789abcdef",
		Sequence:   sequence,
		ObservedAt: timestamppb.Now(),
		Component:  string(admissionobs.ComponentWebhook),
		Counters:   &gatewaypb.ObservationCounters{Generated: int64(sequence)},
	}
}

func TestSubmitObservationReportsValidatesNormalizesAndDeduplicates(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{ObservationEnabled: true})
	report := validWebhookObservation("report-1", 1)
	report.FaultReasonCounts = map[string]int64{"future_fault": 2}

	results := link.SubmitObservationReports(&gatewaypb.ObservationReportBatch{
		Reports: []*gatewaypb.ObservationReport{report, report, {ReportId: "invalid"}},
	})
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].Status != gatewaypb.ObservationReportBatchAck_Result_ACCEPTED ||
		results[1].Status != gatewaypb.ObservationReportBatchAck_Result_ACCEPTED ||
		results[2].Status != gatewaypb.ObservationReportBatchAck_Result_DISCARDED {
		t.Fatalf("results = %#v", results)
	}
	if len(link.observations) != 1 {
		t.Fatalf("queued reports = %d, want one deduplicated report", len(link.observations))
	}
	if got := link.observations["report-1"].report.FaultReasonCounts[admissionobs.ReasonUnknown]; got != 2 {
		t.Fatalf("normalized unknown fault count = %d, want 2", got)
	}
}

func TestSubmitObservationReportsReturnsRetryableAtCapacity(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{ObservationEnabled: true})
	for sequence := uint64(1); sequence <= operatorObservationQueueCapacity; sequence++ {
		report := validWebhookObservation(admissionobs.ReportID("series", sequence), sequence)
		link.observations[report.ReportId] = &queuedObservationReport{report: report}
	}
	result := link.SubmitObservationReports(&gatewaypb.ObservationReportBatch{
		Reports: []*gatewaypb.ObservationReport{validWebhookObservation("overflow", 200)},
	})
	if len(result) != 1 ||
		result[0].Status != gatewaypb.ObservationReportBatchAck_Result_RETRYABLE ||
		result[0].Reason != admissionobs.ReasonObservationQueueFull {
		t.Fatalf("result = %#v, want queue-full retryable", result)
	}
	if got := link.observationStats.Snapshot().Counters.ReportDropped; got != 1 {
		t.Fatalf("report dropped = %d, want 1", got)
	}
}

func TestSubmitObservationReportsDiscardsWhenDisabled(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{ObservationEnabled: false})
	result := link.SubmitObservationReports(&gatewaypb.ObservationReportBatch{
		Reports: []*gatewaypb.ObservationReport{validWebhookObservation("disabled", 1)},
	})
	if len(result) != 1 ||
		result[0].Status != gatewaypb.ObservationReportBatchAck_Result_DISCARDED ||
		len(link.observations) != 0 {
		t.Fatalf("disabled result = %#v, queued = %d", result, len(link.observations))
	}
}

func TestOperatorObservationReportUsesCumulativeCounters(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{QueueCapacity: 50})
	link.observationStats.Add(admissionobs.Counters{Received: 3})
	link.eventQueueGauge.Observe(8, 50)
	link.enqueueOperatorObservation()

	if len(link.observations) != 1 {
		t.Fatalf("reports = %d, want 1", len(link.observations))
	}
	var report *gatewaypb.ObservationReport
	for _, queued := range link.observations {
		report = queued.report
	}
	if report.Component != string(admissionobs.ComponentOperator) ||
		report.Counters.Received != 3 ||
		report.QueueDepthMax != 8 ||
		report.QueueCapacity != 50 {
		t.Fatalf("operator report = %#v", report)
	}
}

func TestObservationReportsRetryThreeTimesThenDrop(t *testing.T) {
	link := newAdmissionEventLink(nil)
	report := validWebhookObservation("report-1", 1)
	link.observations[report.ReportId] = &queuedObservationReport{report: report, inFlight: true}
	ack := &gatewaypb.ObservationReportBatchAck{Results: []*gatewaypb.ObservationReportBatchAck_Result{{
		ReportId: report.ReportId,
		Status:   gatewaypb.ObservationReportBatchAck_Result_RETRYABLE,
		Reason:   admissionobs.ReasonObservationWriteFailed,
	}}}

	for retry := 1; retry <= maxObservationRetryAttempts; retry++ {
		link.ackObservationReports(ack)
		queued := link.observations[report.ReportId]
		if queued == nil || queued.retryCount != retry || queued.inFlight {
			t.Fatalf("retry %d queued = %#v", retry, queued)
		}
		queued.inFlight = true
	}
	link.ackObservationReports(ack)
	if len(link.observations) != 0 {
		t.Fatalf("reports = %d after exhaustion, want 0", len(link.observations))
	}
	snapshot := link.observationStats.Snapshot()
	if snapshot.Counters.Retry != 3 || snapshot.Counters.RetryExhausted != 1 || snapshot.Counters.ReportDropped != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestSendObservationReportsMarksBatchInFlight(t *testing.T) {
	link := newAdmissionEventLink(nil)
	report := validWebhookObservation("report-1", 1)
	link.observations[report.ReportId] = &queuedObservationReport{report: report}
	var sent *gatewaypb.MemberMessage

	if ok := link.sendObservationReports(time.Now(), func(message *gatewaypb.MemberMessage) error {
		sent = message
		return nil
	}); !ok {
		t.Fatal("send returned false")
	}
	if sent.GetObservationReportBatch() == nil || len(sent.GetObservationReportBatch().Reports) != 1 {
		t.Fatalf("sent message = %#v", sent)
	}
	if !link.observations[report.ReportId].inFlight {
		t.Fatal("report was not marked in flight")
	}
}

// The platform observation switch must not come from DesiredState. Only the
// leader receives DesiredState, so a standby fed from there would read the
// switch as permanently off and never sample itself — which is exactly what
// happened: an HA cluster showed a primary and no secondary.
func TestPlatformObservationSwitchReachesStandbyThroughAck(t *testing.T) {
	standby := New(Config{ClusterName: "member-a", Instance: "operator-1"})
	if standby.IsLeader() {
		t.Fatal("a fresh link must not claim leadership")
	}
	if standby.PlatformObservationEnabled() {
		t.Fatal("sampling must be off until the gateway says otherwise")
	}

	// DesiredState alone must not enable it — that path never reaches a standby,
	// so relying on it is the bug this test guards.
	standby.setEventConfig(&gatewaypb.AdmissionEventConfig{PlatformObservationEnabled: true})
	if standby.PlatformObservationEnabled() {
		t.Fatal("the switch must be driven by Ack, not by DesiredState")
	}

	standby.SetPlatformObservationEnabled(true)
	if !standby.PlatformObservationEnabled() {
		t.Fatal("standby did not enable sampling after the Ack carried the switch")
	}
}

// Switching off drains whatever is queued: sending a burst of minutes-old
// samples when it is switched back on would misreport the gap as live data.
func TestPlatformObservationDisableDrainsQueuedSamples(t *testing.T) {
	link := New(Config{ClusterName: "member-a", Instance: "operator-0"})
	link.SetPlatformObservationEnabled(true)
	link.enqueuePlatformSample(&gatewaypb.ComponentSample{Component: "operator"})
	if len(link.platformSamples) != 1 {
		t.Fatalf("queued samples = %d, want 1", len(link.platformSamples))
	}
	link.SetPlatformObservationEnabled(false)
	if len(link.platformSamples) != 0 {
		t.Fatalf("queued samples after disabling = %d, want 0", len(link.platformSamples))
	}
}
