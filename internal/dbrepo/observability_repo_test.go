package dbrepo_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestIncidentRepoConcurrentFirstReportsConverge(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewIncidentRepo(db)
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	for revision := 1; revision <= 12; revision++ {
		revision := revision
		wg.Add(1)
		go func() {
			defer wg.Done()
			incident := testIncident("concurrent-key", "webhook-a", now)
			incident.Revision = uint64(revision)
			incident.AffectedCount = int64(revision)
			incident.LastSeenAt = now.Add(time.Duration(revision) * time.Second)
			errors <- repo.Upsert(context.Background(), 42, "member", incident, incident.LastSeenAt)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := repo.ListPage(context.Background(), dbrepo.IncidentPageOptions{
		PageOptions: dbrepo.PageOptions{Limit: 20, SortBy: "lastSeenAt", Desc: true}, Now: now.Add(time.Minute),
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].AffectedCount != 12 {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
}

func testIncident(key, instance string, now time.Time) admissionobs.Incident {
	return admissionobs.Incident{
		IncidentKey: key, ReporterInstanceID: instance, Revision: 1,
		Component: admissionobs.ComponentWebhook, Reason: admissionobs.ReasonQueueFull,
		StartedAt: now, LastSeenAt: now, AffectedCount: 2,
		SampleEventIDs: []string{"event-a"}, Detail: "first",
	}
}

func TestIncidentRepoUpsertIsIdempotentAndMergesSamples(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewIncidentRepo(db)
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	incident := testIncident("random-key", "webhook-a", now)
	if err := repo.Upsert(context.Background(), 42, "member", incident, now); err != nil {
		t.Fatal(err)
	}
	incident.Revision = 2
	incident.LastSeenAt = now.Add(time.Minute)
	incident.AffectedCount = 5
	incident.SampleEventIDs = []string{"event-b", "event-c"}
	incident.Detail = "latest"
	incident.Closed = true
	if err := repo.Upsert(context.Background(), 42, "member", incident, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	// A stale open heartbeat cannot reopen the row or replace the latest detail.
	stale := testIncident("random-key", "webhook-a", now)
	if err := repo.Upsert(context.Background(), 42, "member", stale, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, total, err := repo.ListPage(context.Background(), dbrepo.IncidentPageOptions{PageOptions: dbrepo.PageOptions{Limit: 20, SortBy: "lastSeenAt", Desc: true}, Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].AffectedCount != 5 || !items[0].Closed || items[0].Detail != "latest" {
		t.Fatalf("stored = %#v", items)
	}
	if !reflect.DeepEqual([]string(items[0].SampleEventIDs), []string{"event-a", "event-b", "event-c"}) {
		t.Fatalf("samples = %v", items[0].SampleEventIDs)
	}
}

func TestIncidentRepoSeparatesSameSecondReplicaKeysAndFiltersStates(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewIncidentRepo(db)
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	active := testIncident("key-a", "webhook-a", now.Add(-time.Minute))
	interrupted := testIncident("key-b", "webhook-b", now.Add(-time.Hour))
	closed := testIncident("key-c", "webhook-c", now.Add(-30*time.Minute))
	closed.Closed = true
	for _, value := range []struct {
		incident admissionobs.Incident
		received time.Time
	}{{active, now}, {interrupted, now.Add(-time.Hour)}, {closed, now.Add(-30 * time.Minute)}} {
		if err := repo.Upsert(context.Background(), 42, "member", value.incident, value.received); err != nil {
			t.Fatal(err)
		}
	}
	for status, want := range map[string]string{"active": "key-a", "interrupted": "key-b", "closed": "key-c"} {
		items, _, err := repo.ListPage(context.Background(), dbrepo.IncidentPageOptions{PageOptions: dbrepo.PageOptions{Limit: 20, SortBy: "lastSeenAt", Desc: true}, Statuses: []string{status}, Now: now})
		if err != nil || len(items) != 1 || items[0].IncidentKey != want {
			t.Fatalf("status %s: items=%v err=%v", status, items, err)
		}
	}
}

func TestIncidentRepoCleanupUsesGatewayReceiveTime(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewIncidentRepo(db)
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	oldOpen := testIncident("old-open", "webhook-a", now.Add(-30*24*time.Hour))
	recentLong := testIncident("recent-long", "webhook-a", now.Add(-30*24*time.Hour))
	recentLong.Closed = true
	if err := repo.Upsert(context.Background(), 42, "member", oldOpen, now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(context.Background(), 42, "member", recentLong, now); err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.Cleanup(context.Background(), now, 7)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	items, total, err := repo.ListPage(context.Background(), dbrepo.IncidentPageOptions{PageOptions: dbrepo.PageOptions{Limit: 20, SortBy: "lastSeenAt", Desc: true}, Now: now})
	if err != nil || total != 1 || items[0].IncidentKey != "recent-long" {
		t.Fatalf("items=%v total=%d err=%v", items, total, err)
	}
}
