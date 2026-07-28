package gateway

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestAdmissionEventFailureAckPreservesEventIDsAndFinalDiscards(t *testing.T) {
	batch := &gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{
		{EventId: "event-1", DeliveryAttempt: 2},
		nil,
		{EventId: ""},
		{EventId: "event-2", DeliveryAttempt: 4},
	}}
	ack := admissionEventFailureAck(&gatewaypb.AdmissionEventBatchAck{Results: []*gatewaypb.AdmissionEventBatchAck_Result{{
		EventId: "event-1", DeliveryAttempt: 2, Status: gatewaypb.AdmissionEventBatchAck_Result_DISCARDED, Reason: "storage_policy_no_retain",
	}}}, batch, "gateway_storage_failed")
	if len(ack.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(ack.Results))
	}
	if got := ack.Results[0]; got.EventId != "event-1" || got.Status != gatewaypb.AdmissionEventBatchAck_Result_DISCARDED || got.Reason != "storage_policy_no_retain" {
		t.Fatalf("discarded result = %#v", got)
	}
	if got := ack.Results[1]; got.EventId != "event-2" || got.DeliveryAttempt != 4 || got.Status != gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE || got.Reason != "gateway_storage_failed" {
		t.Fatalf("retryable result = %#v", got)
	}
}

func TestAdmissionEventFailureAckChangesUncommittedAcceptedResultToRetryable(t *testing.T) {
	event := &gatewaypb.AdmissionEvent{EventId: "event-1", DeliveryAttempt: 3}
	ack := admissionEventFailureAck(&gatewaypb.AdmissionEventBatchAck{Results: []*gatewaypb.AdmissionEventBatchAck_Result{
		acceptedAdmissionEvent(event),
	}}, &gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{event}}, "gateway_storage_failed")
	if got := ack.Results[0]; got.Status != gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE || got.Reason != "gateway_storage_failed" || got.DeliveryAttempt != 3 {
		t.Fatalf("result = %#v", got)
	}
}
