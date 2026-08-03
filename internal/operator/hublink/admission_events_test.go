package hublink

import (
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func newAdmissionEventLink(config *gatewaypb.AdmissionEventConfig) *Link {
	link := New(Config{ClusterName: "member-a"})
	link.leader.Store(true)
	link.eventConfig = config
	return link
}

// Storage rules are an allow-list: an exclude rule stores everything outside its
// conditions, so the event its conditions describe is the one dropped locally.
func TestSubmitAdmissionEventsUsesNormalizedGVRAndWholeRuleExclusion(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{StoragePolicies: []*gatewaypb.AdmissionStoragePolicy{
		{MatchMode: "exclude", Namespaces: []string{"kube-system"}, Gvks: []string{"pods"}},
	}})
	batch := &gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{
		{EventId: "inside-excluded-condition", ApiGroup: "", Resource: "pods", Namespace: "kube-system"},
		{EventId: "outside-condition", ApiGroup: "", Resource: "pods", Namespace: "payments"},
	}}

	results, ok := link.SubmitAdmissionEvents(batch)
	if !ok || len(results) != 2 {
		t.Fatalf("submit = (%#v, %v), want two results", results, ok)
	}
	if results[0].Status != gatewaypb.AdmissionEventBatchAck_Result_DISCARDED {
		t.Fatalf("event inside the excluded condition status = %s, want DISCARDED", results[0].Status)
	}
	if results[1].Status != gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED {
		t.Fatalf("event outside the condition status = %s, want ACCEPTED", results[1].Status)
	}
	if len(link.events) != 1 {
		t.Fatalf("queued events = %d, want 1", len(link.events))
	}
}

// An include rule for one namespace/resource pair must drop everything else,
// which is the "only store default/deployments" case.
func TestSubmitAdmissionEventsIncludeRuleDropsNonMatchingEvents(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{StoragePolicies: []*gatewaypb.AdmissionStoragePolicy{
		{MatchMode: "include", Namespaces: []string{"default"}, Gvks: []string{"apps/deployments"}},
	}})
	results, ok := link.SubmitAdmissionEvents(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{
		{EventId: "wanted", ApiGroup: "apps", Resource: "deployments", Namespace: "default"},
		{EventId: "other-namespace", ApiGroup: "apps", Resource: "deployments", Namespace: "test-builtin-rules"},
		{EventId: "other-resource", ApiGroup: "", Resource: "pods", Namespace: "test-builtin-rules"},
	}})
	if !ok || len(results) != 3 {
		t.Fatalf("submit = (%#v, %v), want three results", results, ok)
	}
	if results[0].Status != gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED {
		t.Errorf("wanted event status = %s, want ACCEPTED", results[0].Status)
	}
	for _, i := range []int{1, 2} {
		if results[i].Status != gatewaypb.AdmissionEventBatchAck_Result_DISCARDED {
			t.Errorf("result[%d] status = %s, want DISCARDED", i, results[i].Status)
		}
	}
	if len(link.events) != 1 {
		t.Fatalf("queued events = %d, want only the matching one", len(link.events))
	}
}

func TestSubmitAdmissionEventsReturnsRetryableWhenQueueIsFull(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{QueueCapacity: 1})
	results, ok := link.SubmitAdmissionEvents(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{
		{EventId: "first"},
		{EventId: "second"},
	}})
	if !ok || len(results) != 2 {
		t.Fatalf("submit = (%#v, %v), want two results", results, ok)
	}
	if results[0].Status != gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED {
		t.Fatalf("first status = %s, want ACCEPTED", results[0].Status)
	}
	if results[1].Status != gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE || results[1].Reason != "operator_event_queue_full" {
		t.Fatalf("second result = %#v, want queue-full RETRYABLE", results[1])
	}
	incidents := link.incidentTracker.Snapshot(time.Now().UTC())
	if len(incidents) != 1 || incidents[0].Reason != admissionobs.ReasonQueueFull || incidents[0].AffectedCount != 1 {
		t.Fatalf("link incidents = %#v", incidents)
	}
}

func TestRetryableAckDropsImmediatelyWhenRetryDisabled(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{RetryEnabled: false})
	_, _ = link.SubmitAdmissionEvents(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{{EventId: "event-1"}}})
	link.ackAdmissionEvents(&gatewaypb.AdmissionEventBatchAck{Results: []*gatewaypb.AdmissionEventBatchAck_Result{{
		EventId: "event-1",
		Status:  gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE,
		Reason:  "gateway_storage_failed",
	}}})
	if len(link.events) != 0 {
		t.Fatalf("queued events = %d, want 0", len(link.events))
	}
}

func TestRetryableAckAllowsConfiguredNumberOfRetries(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{RetryEnabled: true, MaxRetryAttempts: 3})
	_, _ = link.SubmitAdmissionEvents(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{{EventId: "event-1"}}})
	ack := &gatewaypb.AdmissionEventBatchAck{Results: []*gatewaypb.AdmissionEventBatchAck_Result{{
		EventId: "event-1",
		Status:  gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE,
		Reason:  "gateway_storage_failed",
	}}}
	for retry := 1; retry <= 3; retry++ {
		link.ackAdmissionEvents(ack)
		queued := link.events["event-1"]
		if queued == nil || queued.retryCount != retry {
			t.Fatalf("after failure %d queued = %#v, want retryCount %d", retry, queued, retry)
		}
	}
	link.ackAdmissionEvents(ack)
	if len(link.events) != 0 {
		t.Fatalf("queued events = %d after retry exhaustion, want 0", len(link.events))
	}
}

func TestAdmissionEventAckIgnoresStaleDeliveryAttempt(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{RetryEnabled: true, MaxRetryAttempts: 3})
	_, _ = link.SubmitAdmissionEvents(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{{EventId: "event-1"}}})
	link.events["event-1"].attempt = 2
	link.events["event-1"].inFlight = true

	link.ackAdmissionEvents(&gatewaypb.AdmissionEventBatchAck{Results: []*gatewaypb.AdmissionEventBatchAck_Result{{
		EventId:         "event-1",
		Status:          gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE,
		Reason:          "gateway_storage_failed",
		DeliveryAttempt: 1,
	}}})

	queued := link.events["event-1"]
	if queued == nil || queued.retryCount != 0 || !queued.inFlight {
		t.Fatalf("queued event changed after stale ack: %#v", queued)
	}
}

func TestAdmissionEventAckTimeoutCreatesLinkIncident(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{RetryEnabled: true, MaxRetryAttempts: 3, ObservationEnabled: true})
	_, _ = link.SubmitAdmissionEvents(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{{EventId: "event-timeout"}}})
	link.failAdmissionEventLocked("event-timeout", admissionobs.ReasonGatewayACKTimeout)

	incidents := link.incidentTracker.Snapshot(time.Now().UTC())
	if len(incidents) != 1 || incidents[0].Reason != admissionobs.ReasonACKTimeout || len(incidents[0].SampleEventIDs) != 1 || incidents[0].SampleEventIDs[0] != "event-timeout" {
		t.Fatalf("incidents = %#v", incidents)
	}
}
