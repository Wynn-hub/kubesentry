package dbrepo_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/lib/pq"
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
