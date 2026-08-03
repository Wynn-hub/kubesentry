package hublink

import (
	"fmt"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func validWebhookIncident(key string, revision uint64) *gatewaypb.LinkIncident {
	now := timestamppb.Now()
	return &gatewaypb.LinkIncident{
		IncidentKey: key, ReporterInstanceId: "webhook-a", Revision: revision,
		Component: string(admissionobs.ComponentWebhook), Reason: admissionobs.ReasonQueueFull,
		StartedAt: now, LastSeenAt: now, AffectedCount: 1,
	}
}

func enabledLeaderLink() *Link {
	link := New(Config{ClusterName: "member", Instance: "operator-a"})
	link.leader.Store(true)
	link.setEventConfig(&gatewaypb.AdmissionEventConfig{ObservationEnabled: true})
	return link
}

func TestSubmitLinkIncidentsValidatesAndKeepsNewestRevision(t *testing.T) {
	link := enabledLeaderLink()
	first := validWebhookIncident("incident-a", 1)
	second := validWebhookIncident("incident-a", 2)
	second.AffectedCount = 3
	results := link.SubmitLinkIncidents(&gatewaypb.LinkIncidentBatch{Incidents: []*gatewaypb.LinkIncident{first, second, {IncidentKey: "invalid"}}})
	if results[0].Status != gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED || results[1].Status != gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED || results[2].Status != gatewaypb.LinkIncidentBatchAck_Result_DISCARDED {
		t.Fatalf("results = %#v", results)
	}
	if len(link.incidents) != 1 || link.incidents["incident-a"].incident.Revision != 2 || link.incidents["incident-a"].incident.AffectedCount != 3 {
		t.Fatalf("queue = %#v", link.incidents)
	}
}

func TestSubmitLinkIncidentsReturnsRetryableAtCapacity(t *testing.T) {
	link := enabledLeaderLink()
	for index := 0; index < operatorIncidentCapacity; index++ {
		key := fmt.Sprintf("incident-%d", index)
		link.incidents[key] = &queuedLinkIncident{incident: validWebhookIncident(key, 1)}
	}
	result := link.SubmitLinkIncidents(&gatewaypb.LinkIncidentBatch{Incidents: []*gatewaypb.LinkIncident{validWebhookIncident("overflow", 1)}})
	if result[0].Status != gatewaypb.LinkIncidentBatchAck_Result_RETRYABLE || result[0].Reason != admissionobs.ReasonQueueFull {
		t.Fatalf("result = %#v", result)
	}
}

func TestAckLinkIncidentsDoesNotDeleteNewerRevision(t *testing.T) {
	link := enabledLeaderLink()
	link.incidents["incident-a"] = &queuedLinkIncident{incident: validWebhookIncident("incident-a", 2), inFlight: true, inFlightRevision: 1, ackDeadline: time.Now().Add(time.Second)}
	link.ackLinkIncidents(&gatewaypb.LinkIncidentBatchAck{Results: []*gatewaypb.LinkIncidentBatchAck_Result{{IncidentKey: "incident-a", Revision: 1, Status: gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED}}})
	if link.incidents["incident-a"] == nil || link.incidents["incident-a"].inFlight {
		t.Fatalf("newer revision was lost: %#v", link.incidents)
	}
}

func TestOperatorTrackerProducesOwnIncident(t *testing.T) {
	link := enabledLeaderLink()
	link.RecordLinkIncident(admissionobs.ReasonACKTimeout, 2, "gateway did not acknowledge")
	link.enqueueLocalIncidents(time.Now().UTC())
	if len(link.incidents) != 1 {
		t.Fatalf("incidents = %d", len(link.incidents))
	}
	for _, queued := range link.incidents {
		if queued.incident.Component != "operator" || queued.incident.AffectedCount != 2 {
			t.Fatalf("incident = %#v", queued.incident)
		}
	}
}
