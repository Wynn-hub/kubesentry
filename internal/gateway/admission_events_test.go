package gateway

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
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

// Events whose retention window already elapsed (a member replaying a multi-day
// backlog) must not be written just for the cleanup worker to delete them.
func TestHandleAdmissionEventBatchDiscardsAlreadyExpiredEvents(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "prod", Phase: "Online"}
	if err := repos.Clusters.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	server := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "")

	settings, err := repos.Admission.GetSettings(ctx)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	stale := time.Now().UTC().Add(-time.Duration(settings.DefaultRetentionDays+1) * 24 * time.Hour)

	ack, err := server.handleAdmissionEventBatch(ctx, cluster.Name, &gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{
		{EventId: "fresh", Verdict: "deny", Resource: "pods", OccurredAt: timestamppb.New(time.Now().UTC())},
		{EventId: "expired", Verdict: "deny", Resource: "pods", OccurredAt: timestamppb.New(stale)},
	}})
	if err != nil {
		t.Fatalf("handle batch: %v", err)
	}

	statuses := map[string]gatewaypb.AdmissionEventBatchAck_Result_Status{}
	reasons := map[string]string{}
	for _, result := range ack.Results {
		statuses[result.EventId] = result.Status
		reasons[result.EventId] = result.Reason
	}
	if statuses["fresh"] != gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED {
		t.Errorf("fresh event status = %v, want ACCEPTED", statuses["fresh"])
	}
	if statuses["expired"] != gatewaypb.AdmissionEventBatchAck_Result_DISCARDED || reasons["expired"] != "retention_already_elapsed" {
		t.Errorf("expired event = (%v, %q), want DISCARDED/retention_already_elapsed", statuses["expired"], reasons["expired"])
	}

	var stored int64
	if err := newTestCountEvents(t, repos, &stored); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if stored != 1 {
		t.Errorf("stored events = %d, want only the fresh one", stored)
	}
}

func newTestCountEvents(t *testing.T, repos Repos, out *int64) error {
	t.Helper()
	_, total, err := repos.Admission.ListEventsPage(context.Background(), dbrepo.AdmissionEventPageOptions{
		PageOptions: dbrepo.PageOptions{Limit: 100, Offset: 0, SortBy: "occurredAt", Desc: true},
	})
	*out = total
	return err
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
