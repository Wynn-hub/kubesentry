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

func TestHandleLinkIncidentBatchStoresAndAcknowledgesEachIncident(t *testing.T) {
	repos := newTestRepos(t)
	cluster := &dbrepo.Cluster{Name: "observed", Phase: "Online"}
	mustCreateCluster(t, repos, cluster)
	if err := enableAdmissionObservation(context.Background(), repos); err != nil {
		t.Fatal(err)
	}
	server := NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	now := time.Now().UTC()
	batch := &gatewaypb.LinkIncidentBatch{Incidents: []*gatewaypb.LinkIncident{
		{
			IncidentKey: "valid-1", ReporterInstanceId: "webhook-1", Revision: 3,
			Component: string(admissionobs.ComponentWebhook), Reason: admissionobs.ReasonQueueFull,
			StartedAt: timestamppb.New(now.Add(-time.Minute)), LastSeenAt: timestamppb.New(now),
			AffectedCount: 7, SampleEventIds: []string{"event-1"}, Detail: "queue capacity reached",
		},
		{
			IncidentKey: "invalid-1", ReporterInstanceId: "webhook-1", Revision: 1,
			Component: "invalid", Reason: admissionobs.ReasonQueueFull,
			StartedAt: timestamppb.New(now), LastSeenAt: timestamppb.New(now), AffectedCount: 1,
		},
	}}
	ack := server.handleLinkIncidentBatch(context.Background(), cluster.Name, batch)
	if len(ack.Results) != 2 {
		t.Fatalf("ack results = %d, want 2", len(ack.Results))
	}
	if ack.Results[0].Status != gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED || ack.Results[0].Revision != 3 {
		t.Fatalf("valid ack = %+v", ack.Results[0])
	}
	if ack.Results[1].Status != gatewaypb.LinkIncidentBatchAck_Result_DISCARDED {
		t.Fatalf("invalid ack = %+v", ack.Results[1])
	}
	items, total, err := repos.Incidents.ListPage(context.Background(), dbrepo.IncidentPageOptions{
		PageOptions:  dbrepo.PageOptions{Limit: 20, SortBy: "lastSeenAt", Desc: true},
		ClusterNames: []string{cluster.Name}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].AffectedCount != 7 {
		t.Fatalf("stored incidents = %+v, total = %d", items, total)
	}
	if items[0].ClusterID != cluster.ID || items[0].ClusterName != cluster.Name {
		t.Fatalf("stored cluster identity = %d/%q", items[0].ClusterID, items[0].ClusterName)
	}
}

func TestHandleLinkIncidentBatchClampsFutureMemberClock(t *testing.T) {
	now := time.Now().UTC()
	incident, err := incidentFromProto(&gatewaypb.LinkIncident{
		IncidentKey: "future", ReporterInstanceId: "operator-1", Revision: 1,
		Component: string(admissionobs.ComponentOperator), Reason: admissionobs.ReasonACKTimeout,
		StartedAt: timestamppb.New(now.Add(time.Hour)), LastSeenAt: timestamppb.New(now.Add(time.Hour)),
		AffectedCount: 1,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !incident.StartedAt.Equal(now) || !incident.LastSeenAt.Equal(now) {
		t.Fatalf("future timestamps were not clamped: %+v", incident)
	}
}

func TestNotifyChangeTriggersIncidentCleanupFromAdmissionSettings(t *testing.T) {
	cleaner := NewIncidentCleaner(&dbrepo.IncidentRepo{}, nil, time.Hour, nil)
	server := NewServer(Repos{}, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	server.SetIncidentCleaner(cleaner)
	if _, err := server.NotifyChange(context.Background(), &gatewaypb.NotifyChangeRequest{Topic: "policy"}); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.trigger) != 0 {
		t.Fatal("unrelated topic triggered incident cleanup")
	}
	if _, err := server.NotifyChange(context.Background(), &gatewaypb.NotifyChangeRequest{Topic: AdmissionEventSettingsTopic}); err != nil {
		t.Fatal(err)
	}
	if len(cleaner.trigger) != 1 {
		t.Fatalf("pending cleanups = %d, want 1", len(cleaner.trigger))
	}
}
