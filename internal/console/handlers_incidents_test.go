package console

import (
	"context"
	"net/url"
	"testing"
	"time"

	admissionobs "github.com/Wynn-hub/kubesentry/internal/admission/observability"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestAdmissionLinkIncidentListAndDetailResolveSamples(t *testing.T) {
	server, repos := newDBTestServer(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "member-a", Phase: "Online"}
	if err := repos.Clusters.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	event := &dbrepo.AdmissionEvent{
		EventID: "event-present", ClusterID: cluster.ID, ClusterName: cluster.Name,
		OccurredAt: time.Now().UTC(), Verdict: "deny",
	}
	if err := repos.Admission.CreateEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	incident := admissionobs.Incident{
		IncidentKey: "incident-a", ReporterInstanceID: "webhook-a", Revision: 2,
		Component: admissionobs.ComponentWebhook, Reason: admissionobs.ReasonQueueFull,
		StartedAt: now.Add(-time.Minute), LastSeenAt: now, AffectedCount: 4,
		SampleEventIDs: []string{event.EventID, "event-expired"}, Detail: "queue reached capacity",
	}
	if err := repos.Incidents.Upsert(ctx, cluster.ID, cluster.Name, incident, now); err != nil {
		t.Fatal(err)
	}

	query := url.Values{
		"cluster": {cluster.Name}, "component": {string(incident.Component)},
		"reason": {incident.Reason}, "status": {"active"},
		"from": {now.Add(-2 * time.Minute).Format(time.RFC3339)},
		"to":   {now.Add(time.Minute).Format(time.RFC3339)},
	}.Encode()
	rec, env := doRequest(t, server, "GET", "/api/v1/admission-link-incidents?"+query, nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("list incidents = %d, body=%s", rec.Code, rec.Body.String())
	}
	page := mustUnmarshal[struct {
		Items []dto.IncidentItem `json:"items"`
		Total int64              `json:"total"`
	}](t, env.Data)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Status != "active" || page.Items[0].AffectedCount != 4 {
		t.Fatalf("incident page = %+v", page)
	}

	rec, env = doRequest(t, server, "GET", "/api/v1/admission-link-incidents/"+string(page.Items[0].ID), nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("get incident = %d, body=%s", rec.Code, rec.Body.String())
	}
	detail := mustUnmarshal[dto.IncidentView](t, env.Data)
	if detail.Detail != incident.Detail || len(detail.Samples) != 2 {
		t.Fatalf("incident detail = %+v", detail)
	}
	if detail.Samples[0].Expired || detail.Samples[0].EventDatabaseID == nil {
		t.Fatalf("present sample = %+v", detail.Samples[0])
	}
	if !detail.Samples[1].Expired || detail.Samples[1].EventDatabaseID != nil {
		t.Fatalf("expired sample = %+v", detail.Samples[1])
	}
}

func TestAdmissionLinkIncidentListRejectsInvalidFilters(t *testing.T) {
	server, _ := newDBTestServer(t)
	for _, query := range []string{"component=unknown", "reason=unknown", "status=unknown", "from=not-a-time", "pageSize=101"} {
		rec, _ := doRequest(t, server, "GET", "/api/v1/admission-link-incidents?"+query, nil)
		if rec.Code != 400 {
			t.Fatalf("query %q returned %d, want 400", query, rec.Code)
		}
	}
}
