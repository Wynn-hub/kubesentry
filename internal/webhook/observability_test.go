package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/encoding/protojson"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestWebhookIncidentReporterKeepsLastConfigOnPollingFailure(t *testing.T) {
	tracker := admissionobs.NewTracker(admissionobs.ComponentWebhook, "webhook-a")
	tracker.Record(admissionobs.ReasonQueueFull, 1, "event-a", "full", time.Now().UTC())
	canceled := false
	reporter := &webhookObservationReporter{
		configURL: "http://operator/config", token: "token", tracker: tracker,
		client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("temporarily unavailable"))}, nil
		})},
		workersCancel: func() { canceled = true },
	}
	reporter.refreshEnabled()
	if canceled || reporter.workersCancel == nil {
		t.Fatal("transient config failure disabled an already-enabled reporter")
	}
	if len(tracker.Snapshot(time.Now().UTC())) != 1 {
		t.Fatal("transient config failure discarded pending incidents")
	}
}

func TestWebhookIncidentReporterPostsAndAcknowledgesClosedIncident(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	var received *gatewaypb.LinkIncidentBatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch gatewaypb.LinkIncidentBatch
		data, _ := io.ReadAll(r.Body)
		if err := protojson.Unmarshal(data, &batch); err != nil {
			t.Errorf("decode batch: %v", err)
		}
		received = &batch
		ack := &gatewaypb.LinkIncidentBatchAck{}
		for _, incident := range batch.Incidents {
			ack.Results = append(ack.Results, &gatewaypb.LinkIncidentBatchAck_Result{IncidentKey: incident.IncidentKey, Revision: incident.Revision, Status: gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED})
		}
		encoded, _ := protojson.Marshal(ack)
		_, _ = w.Write(encoded)
	}))
	defer server.Close()
	tracker := admissionobs.NewTracker(admissionobs.ComponentWebhook, "webhook-a")
	reporter := &webhookObservationReporter{endpoint: server.URL, client: server.Client(), tracker: tracker}
	tracker.Record(admissionobs.ReasonQueueFull, 2, "event-a", "full", now)
	tracker.Snapshot(now.Add(61 * time.Second))
	reporter.flush(now.Add(61 * time.Second))
	if received == nil || len(received.Incidents) != 1 || !received.Incidents[0].Closed {
		t.Fatalf("received = %#v", received)
	}
	if tracker.PendingCount() != 0 {
		t.Fatal("closed incident was not acknowledged")
	}
}

func TestWebhookIncidentReporterRecordsDeliveryFailureWithoutRecursion(t *testing.T) {
	now := time.Now().UTC()
	tracker := admissionobs.NewTracker(admissionobs.ComponentWebhook, "webhook-a")
	reporter := &webhookObservationReporter{endpoint: "http://127.0.0.1:1", client: &http.Client{Timeout: time.Millisecond}, tracker: tracker}
	tracker.Record(admissionobs.ReasonQueueFull, 1, "event-a", "full", now)
	reporter.flush(now)
	items := tracker.Snapshot(now)
	found := false
	for _, item := range items {
		found = found || item.Reason == admissionobs.ReasonReportDropped
	}
	if !found {
		t.Fatalf("incidents = %#v", items)
	}
}
