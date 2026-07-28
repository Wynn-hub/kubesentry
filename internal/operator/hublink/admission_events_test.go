package hublink

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func newAdmissionEventLink(config *gatewaypb.AdmissionEventConfig) *Link {
	link := New(Config{ClusterName: "member-a"})
	link.leader.Store(true)
	link.eventConfig = config
	return link
}

func TestSubmitAdmissionEventsUsesNormalizedGVRAndWholeRuleExclusion(t *testing.T) {
	link := newAdmissionEventLink(&gatewaypb.AdmissionEventConfig{StoragePolicies: []*gatewaypb.AdmissionStoragePolicy{
		{MatchMode: "exclude", Namespaces: []string{"kube-system"}, Gvks: []string{"pods"}, Action: "discard"},
	}})
	batch := &gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{
		{EventId: "excluded-condition", ApiGroup: "", Resource: "pods", Namespace: "kube-system"},
		{EventId: "outside-condition", ApiGroup: "", Resource: "pods", Namespace: "payments"},
	}}

	results, ok := link.SubmitAdmissionEvents(batch)
	if !ok || len(results) != 2 {
		t.Fatalf("submit = (%#v, %v), want two results", results, ok)
	}
	if results[0].Status != gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED {
		t.Fatalf("excluded complete condition status = %s, want ACCEPTED", results[0].Status)
	}
	if results[1].Status != gatewaypb.AdmissionEventBatchAck_Result_DISCARDED {
		t.Fatalf("outside condition status = %s, want DISCARDED", results[1].Status)
	}
	if len(link.events) != 1 {
		t.Fatalf("queued events = %d, want 1", len(link.events))
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
