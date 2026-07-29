package dbrepo_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestAdmissionRepoStoresEmptyEventFieldsAsNonNullValues(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	clusters := dbrepo.NewClusterRepo(db)
	cluster := &dbrepo.Cluster{Name: "event-cluster", Phase: "Online"}
	if err := clusters.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	repo := dbrepo.NewAdmissionRepo(db)
	if err := repo.CreateEventsIdempotent(ctx, []dbrepo.AdmissionEvent{{
		EventID: "event-without-violations", ClusterID: cluster.ID, ClusterName: cluster.Name,
		OccurredAt: time.Now().UTC(), Verdict: "audit",
	}}); err != nil {
		t.Fatalf("create event with empty fields: %v", err)
	}

	var event dbrepo.AdmissionEvent
	if err := db.Where("event_id = ?", "event-without-violations").First(&event).Error; err != nil {
		t.Fatalf("load event: %v", err)
	}
	if event.UserGroups == nil || event.PolicyNames == nil || event.PolicyGroupNames == nil {
		t.Fatalf("array fields must be non-nil: %+v", event)
	}
	if got := string(event.Violations); got != "[]" {
		t.Errorf("violations = %s, want []", got)
	}
	if got := string(event.MetadataSnapshot); got != "{}" {
		t.Errorf("metadata snapshot = %s, want {}", got)
	}
	if got := string(event.SpecSnapshot); got != "{}" {
		t.Errorf("spec snapshot = %s, want {}", got)
	}
}

func TestAdmissionRepoListEventsPageFilters(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "filter-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	repo := dbrepo.NewAdmissionRepo(db)
	now := time.Now().UTC()
	events := []dbrepo.AdmissionEvent{
		{EventID: "core-pod", ClusterID: cluster.ID, ClusterName: cluster.Name, OccurredAt: now, Verdict: "deny", APIGroup: "", Resource: "pods", Namespace: "prod"},
		{EventID: "apps-deploy", ClusterID: cluster.ID, ClusterName: cluster.Name, OccurredAt: now, Verdict: "audit", APIGroup: "apps", Resource: "deployments", Namespace: "staging"},
		{EventID: "core-svc", ClusterID: cluster.ID, ClusterName: cluster.Name, OccurredAt: now, Verdict: "audit", APIGroup: "", Resource: "services", Namespace: "prod"},
	}
	if err := repo.CreateEventsIdempotent(ctx, events); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	page := func(opts dbrepo.AdmissionEventPageOptions) []string {
		opts.PageOptions = dbrepo.PageOptions{Limit: 50, Offset: 0, SortBy: "occurredAt", Desc: true}
		items, _, err := repo.ListEventsPage(ctx, opts)
		if err != nil {
			t.Fatalf("list events: %v", err)
		}
		ids := make([]string, len(items))
		for i, e := range items {
			ids[i] = e.EventID
		}
		return ids
	}

	// GVK normalized: core group matches bare "pods" (no leading slash).
	if got := page(dbrepo.AdmissionEventPageOptions{GVKs: []string{"pods"}}); len(got) != 1 || got[0] != "core-pod" {
		t.Errorf("gvk=pods -> %v, want [core-pod]", got)
	}
	// Non-core group matches "<group>/<resource>".
	if got := page(dbrepo.AdmissionEventPageOptions{GVKs: []string{"apps/deployments"}}); len(got) != 1 || got[0] != "apps-deploy" {
		t.Errorf("gvk=apps/deployments -> %v, want [apps-deploy]", got)
	}
	// Multi-value verdict uses IN.
	if got := page(dbrepo.AdmissionEventPageOptions{Verdicts: []string{"audit", "deny"}}); len(got) != 3 {
		t.Errorf("verdict in (audit,deny) -> %v, want 3 events", got)
	}
	// Multi-value namespace uses IN.
	if got := page(dbrepo.AdmissionEventPageOptions{Namespaces: []string{"prod"}}); len(got) != 2 {
		t.Errorf("namespace=prod -> %v, want 2 events", got)
	}
	// Multi-value namespace is OR (prod OR staging => all three).
	if got := page(dbrepo.AdmissionEventPageOptions{Namespaces: []string{"prod", "staging"}}); len(got) != 3 {
		t.Errorf("namespace in (prod,staging) -> %v, want 3 events", got)
	}
	// Multi-value cluster is OR; the known cluster returns all, an unknown one returns none.
	if got := page(dbrepo.AdmissionEventPageOptions{ClusterNames: []string{cluster.Name, "absent"}}); len(got) != 3 {
		t.Errorf("cluster in (%s,absent) -> %v, want 3 events", cluster.Name, got)
	}
}

func TestAdmissionRepoUpdatesDeliverySettingsAndMatchModeAtomically(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewAdmissionRepo(db)
	ctx := context.Background()
	err := repo.UpdateSettingsAndPolicies(ctx, &dbrepo.AdmissionEventSettings{
		ID:                   1,
		DefaultRetentionDays: 21,
		RetryEnabled:         true,
		MaxRetryAttempts:     5,
		QueueCapacity:        2048,
	}, []dbrepo.AdmissionStoragePolicy{{
		Priority:   1,
		MatchMode:  dbrepo.AdmissionMatchModeExclude,
		Namespaces: pq.StringArray{"kube-system"},
	}})
	if err != nil {
		t.Fatalf("update settings and policies: %v", err)
	}

	settings, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.DefaultRetentionDays != 21 || !settings.RetryEnabled || settings.MaxRetryAttempts != 5 || settings.QueueCapacity != 2048 || settings.StoragePolicyVersion != 2 {
		t.Fatalf("settings = %+v", settings)
	}
	policies, err := repo.ListPolicies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 || policies[0].MatchMode != dbrepo.AdmissionMatchModeExclude || !reflect.DeepEqual([]string(policies[0].Namespaces), []string{"kube-system"}) {
		t.Fatalf("policies = %+v", policies)
	}
}

// seedCleanupEvent inserts one event with an explicit入库 time so cleanup's
// retention math is deterministic. GORM only auto-fills CreatedAt when it is
// the zero value, so an explicit timestamp survives the insert.
func seedCleanupEvent(t *testing.T, repo *dbrepo.AdmissionRepo, cluster *dbrepo.Cluster, eventID, namespace, resource string, createdAt time.Time) {
	t.Helper()
	err := repo.CreateEventsIdempotent(context.Background(), []dbrepo.AdmissionEvent{{
		Base:        dbmodel.Base{CreatedAt: createdAt},
		EventID:     eventID,
		ClusterID:   cluster.ID,
		ClusterName: cluster.Name,
		OccurredAt:  createdAt,
		Verdict:     "deny",
		Resource:    resource,
		Namespace:   namespace,
	}})
	if err != nil {
		t.Fatalf("seed event %s: %v", eventID, err)
	}
}

func remainingEventIDs(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var events []dbrepo.AdmissionEvent
	if err := db.Order("event_id ASC").Find(&events).Error; err != nil {
		t.Fatalf("list remaining events: %v", err)
	}
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.EventID
	}
	return ids
}

// The routine daily pass must leave the most recent day alone (nothing can
// have expired yet) while still deleting older events that either aged past the
// global retention or no longer match the rule set.
func TestAdmissionRepoCleanupSkipsTheShortestRetentionWindow(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "cleanup-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	repo := dbrepo.NewAdmissionRepo(db)
	now := time.Now().UTC()

	// Rule set stores only the "default" namespace, so anything else is
	// unmatched and therefore ineligible for storage at any age.
	if err := repo.UpdateSettingsAndPolicies(ctx, &dbrepo.AdmissionEventSettings{
		ID: 1, DefaultRetentionDays: 7, MaxRetryAttempts: 3, QueueCapacity: 1024,
	}, []dbrepo.AdmissionStoragePolicy{{
		Priority:   1,
		MatchMode:  dbrepo.AdmissionMatchModeInclude,
		Namespaces: pq.StringArray{"default"},
	}}); err != nil {
		t.Fatalf("configure storage rules: %v", err)
	}

	seedCleanupEvent(t, repo, cluster, "fresh-unmatched", "other", "pods", now.Add(-time.Hour))
	seedCleanupEvent(t, repo, cluster, "old-unmatched", "other", "pods", now.Add(-48*time.Hour))
	seedCleanupEvent(t, repo, cluster, "matched-within-retention", "default", "pods", now.Add(-48*time.Hour))
	seedCleanupEvent(t, repo, cluster, "matched-past-retention", "default", "pods", now.Add(-8*24*time.Hour))

	deleted, err := repo.Cleanup(ctx, now)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	want := []string{"fresh-unmatched", "matched-within-retention"}
	if got := remainingEventIDs(t, db); !reflect.DeepEqual(got, want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}
}

// CleanupAll is what a configuration change triggers: narrowing the rule set
// must drop already-stored non-matching events immediately, including the ones
// inside the routine pass's skipped window. Without this the console shows the
// new rule as saved while the events it excludes linger for up to a day,
// looking exactly like the rule never took effect.
func TestAdmissionRepoCleanupAllDropsRecentlyUnmatchedEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "cleanup-all-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	repo := dbrepo.NewAdmissionRepo(db)
	now := time.Now().UTC()

	seedCleanupEvent(t, repo, cluster, "keep-default", "default", "pods", now.Add(-time.Minute))
	seedCleanupEvent(t, repo, cluster, "drop-other", "other", "pods", now.Add(-time.Minute))

	if err := repo.UpdateSettingsAndPolicies(ctx, &dbrepo.AdmissionEventSettings{
		ID: 1, DefaultRetentionDays: 14, MaxRetryAttempts: 3, QueueCapacity: 1024,
	}, []dbrepo.AdmissionStoragePolicy{{
		Priority:   1,
		MatchMode:  dbrepo.AdmissionMatchModeInclude,
		Namespaces: pq.StringArray{"default"},
	}}); err != nil {
		t.Fatalf("narrow storage rules: %v", err)
	}

	// The routine pass cannot see either event yet.
	if deleted, err := repo.Cleanup(ctx, now); err != nil || deleted != 0 {
		t.Fatalf("routine cleanup deleted = %d, err = %v; want 0, nil", deleted, err)
	}
	deleted, err := repo.CleanupAll(ctx, now)
	if err != nil {
		t.Fatalf("cleanup all: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if got := remainingEventIDs(t, db); !reflect.DeepEqual(got, []string{"keep-default"}) {
		t.Errorf("remaining = %v, want [keep-default]", got)
	}
}

// Cleanup pages by primary key, so a candidate set larger than one batch must
// still be fully processed rather than truncated at the batch boundary.
func TestAdmissionRepoCleanupPagesBeyondOneBatch(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "cleanup-batch-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	repo := dbrepo.NewAdmissionRepo(db)
	now := time.Now().UTC()
	if err := repo.UpdateSettingsAndPolicies(ctx, &dbrepo.AdmissionEventSettings{
		ID: 1, DefaultRetentionDays: 1, MaxRetryAttempts: 3, QueueCapacity: 1024,
	}, nil); err != nil {
		t.Fatalf("configure retention: %v", err)
	}

	const total = 1200 // > admissionCleanupBatchSize
	events := make([]dbrepo.AdmissionEvent, 0, total)
	for i := 0; i < total; i++ {
		events = append(events, dbrepo.AdmissionEvent{
			Base:        dbmodel.Base{CreatedAt: now.Add(-72 * time.Hour)},
			EventID:     fmt.Sprintf("batch-%04d", i),
			ClusterID:   cluster.ID,
			ClusterName: cluster.Name,
			OccurredAt:  now.Add(-72 * time.Hour),
			Verdict:     "audit",
			Resource:    "pods",
			Namespace:   "default",
		})
	}
	if err := repo.CreateEventsIdempotent(ctx, events); err != nil {
		t.Fatalf("seed batch events: %v", err)
	}

	deleted, err := repo.Cleanup(ctx, now)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != total {
		t.Errorf("deleted = %d, want %d", deleted, total)
	}
	if got := remainingEventIDs(t, db); len(got) != 0 {
		t.Errorf("remaining = %d events, want 0", len(got))
	}
}
