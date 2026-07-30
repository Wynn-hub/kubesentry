package console

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

func TestAdmissionObservabilityEndpointsAndSettings(t *testing.T) {
	server, repos, handlers := newDBTestServerWithHandlers(t)
	ctx := context.Background()
	now := time.Now().UTC()
	cluster := &dbrepo.Cluster{Name: "observed-member", Phase: "Online"}
	if err := repos.Clusters.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	webhookReport := admissionobs.Report{
		ReportID: "webhook-1", SeriesID: "webhook-series", Sequence: 1,
		ObservedAt: now, Component: admissionobs.ComponentWebhook,
		Snapshot: admissionobs.Snapshot{
			Counters:             admissionobs.Counters{Generated: 5},
			FaultReasonCounts:    map[string]int64{},
			PressureReasonCounts: map[string]int64{},
			FilterReasonCounts:   map[string]int64{},
		},
		QueueDepthMax: 5, QueueCapacity: 256,
	}
	operatorReport := admissionobs.Report{
		ReportID: "operator-1", SeriesID: "operator-series", Sequence: 1,
		ObservedAt: now, Component: admissionobs.ComponentOperator,
		Snapshot: admissionobs.Snapshot{
			Counters:             admissionobs.Counters{Received: 4},
			FaultReasonCounts:    map[string]int64{},
			PressureReasonCounts: map[string]int64{},
			FilterReasonCounts:   map[string]int64{},
		},
		QueueDepthMax: 2, QueueCapacity: 1024,
	}
	for _, report := range []admissionobs.Report{webhookReport, operatorReport} {
		if _, err := repos.Observability.ApplyMemberReport(ctx, cluster.ID, cluster.Name, report, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.Observability.UpsertBucket(ctx, dbrepo.ObservationBucketDelta{
		BucketStartedAt: now,
		ClusterID:       cluster.ID,
		ClusterName:     cluster.Name,
		Component:       admissionobs.ComponentGateway,
		Snapshot: admissionobs.Snapshot{
			Counters:          admissionobs.Counters{Received: 4, Written: 3, Dropped: 1},
			FaultReasonCounts: map[string]int64{admissionobs.ReasonDBWriteFailed: 1},
		},
		LastObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	from := url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339))
	to := url.QueryEscape(now.Add(time.Minute).Format(time.RFC3339))
	overviewPath := "/api/v1/observability/admission/overview?cluster=observed-member&from=" + from + "&to=" + to
	rec, env := doRequest(t, server, "GET", overviewPath, nil)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("overview = %d: %s", rec.Code, rec.Body.String())
	}
	overview := mustUnmarshal[observationOverviewResponse](t, env.Data)
	if overview.Health != "warning" || !overview.ObservabilityOnboarded ||
		overview.Generated != 5 || overview.OperatorReceived != 4 ||
		overview.GatewayReceived != 4 || overview.Written != 3 ||
		overview.WindowFaultTotal != 1 {
		t.Fatalf("overview = %+v", overview)
	}
	if !overview.Freshness[string(admissionobs.ComponentWebhook)].Fresh ||
		!overview.Freshness[string(admissionobs.ComponentOperator)].Fresh {
		t.Fatalf("freshness = %+v", overview.Freshness)
	}

	rec, env = doRequest(t, server, "GET",
		"/api/v1/observability/admission/trend?cluster=observed-member&from="+from+"&to="+to, nil)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("trend = %d: %s", rec.Code, rec.Body.String())
	}
	trend := mustUnmarshal[struct {
		Items []observationTrendPoint `json:"items"`
	}](t, env.Data)
	found := false
	for _, point := range trend.Items {
		if point.Present && point.Generated == 5 && point.Written == 3 && point.Faults == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("trend points = %+v", trend.Items)
	}

	rec, env = doRequest(t, server, "GET",
		"/api/v1/observability/admission/clusters?page=1&pageSize=20&health=warning&from="+from+"&to="+to, nil)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("clusters = %d: %s", rec.Code, rec.Body.String())
	}
	page := mustUnmarshal[struct {
		Items []observationClusterItem `json:"items"`
		Total int                      `json:"total"`
	}](t, env.Data)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Name != cluster.Name ||
		page.Items[0].Health != "warning" || len(page.Items[0].MajorFaultReasons) != 1 {
		t.Fatalf("cluster page = %+v", page)
	}

	fake := &fakeHubClient{}
	handlers.Gateway = fake
	rec, env = doRequest(t, server, "PUT", "/api/v1/observability/settings", map[string]any{"retentionDays": 45})
	if rec.Code != 200 || !env.Success {
		t.Fatalf("settings = %d: %s", rec.Code, rec.Body.String())
	}
	settings := mustUnmarshal[observationSettingsResponse](t, env.Data)
	if settings.RetentionDays != 45 {
		t.Fatalf("settings = %+v", settings)
	}
	topics, _ := fake.calls()
	if len(topics) != 1 || topics[0] != gateway.PlatformObservabilitySettingsTopic {
		t.Fatalf("topics = %v", topics)
	}

	recovered := &dbrepo.Cluster{Name: "recovered-member", Phase: "Online"}
	if err := repos.Clusters.Create(ctx, recovered); err != nil {
		t.Fatal(err)
	}
	for _, report := range []admissionobs.Report{
		{
			ReportID: "recovered-webhook-1", SeriesID: "recovered-webhook", Sequence: 1,
			ObservedAt: now, Component: admissionobs.ComponentWebhook,
			Snapshot: admissionobs.Snapshot{
				Counters:          admissionobs.Counters{Generated: 2},
				FaultReasonCounts: map[string]int64{}, PressureReasonCounts: map[string]int64{}, FilterReasonCounts: map[string]int64{},
			},
		},
		{
			ReportID: "recovered-operator-1", SeriesID: "recovered-operator", Sequence: 1,
			ObservedAt: now, Component: admissionobs.ComponentOperator,
			Snapshot: admissionobs.Snapshot{
				Counters:          admissionobs.Counters{Received: 2, PolicyFiltered: 1, Retry: 1},
				FaultReasonCounts: map[string]int64{}, PressureReasonCounts: map[string]int64{},
				FilterReasonCounts: map[string]int64{admissionobs.ReasonStoragePolicyNoRetain: 1},
			},
		},
	} {
		if _, err := repos.Observability.ApplyMemberReport(ctx, recovered.ID, recovered.Name, report, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.Observability.UpsertBucket(ctx, dbrepo.ObservationBucketDelta{
		BucketStartedAt: now.Add(-6 * time.Minute), ClusterID: recovered.ID, ClusterName: recovered.Name,
		Component: admissionobs.ComponentGateway,
		Snapshot: admissionobs.Snapshot{Counters: admissionobs.Counters{
			Written: 1, Duplicate: 1, Dropped: 1,
		}},
		LastObservedAt: now.Add(-6 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Observability.UpsertBucket(ctx, dbrepo.ObservationBucketDelta{
		BucketStartedAt: now, ClusterID: 0, Component: admissionobs.ComponentCleanup,
		ObservationWriteFailed: 1, LastObservedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	recoveredPath := "/api/v1/observability/admission/overview?cluster=recovered-member&from=" + from + "&to=" + to
	rec, env = doRequest(t, server, "GET", recoveredPath, nil)
	if rec.Code != 200 {
		t.Fatalf("recovered overview = %d: %s", rec.Code, rec.Body.String())
	}
	recoveredOverview := mustUnmarshal[observationOverviewResponse](t, env.Data)
	if recoveredOverview.Health != "normal" ||
		recoveredOverview.WindowFaults[string(admissionobs.ComponentGateway)].Dropped != 1 ||
		recoveredOverview.PlatformHealth.Health != "warning" {
		t.Fatalf("recovered overview = %+v", recoveredOverview)
	}
}

func TestObservationHealthSeparatesPartialNoDataAndNotOnboarded(t *testing.T) {
	now := time.Now().UTC()
	onboardedAt := now.Add(-time.Hour)
	clusters := []dbrepo.Cluster{
		{Phase: "Online", ObservabilityOnboardedAt: &onboardedAt},
		{Phase: "Online"},
		{Phase: "Offline", ObservabilityOnboardedAt: &onboardedAt},
	}
	clusters[0].ID, clusters[1].ID, clusters[2].ID = 1, 2, 3
	aggregates := map[int64][]dbrepo.ObservationAggregate{
		clusters[0].ID: {{
			Component:      string(admissionobs.ComponentWebhook),
			LastObservedAt: &now,
		}},
	}
	states := observationStates(clusters, aggregates, nil, now)
	if states[clusters[0].ID].health != "partial_no_data" {
		t.Fatalf("first health = %q", states[clusters[0].ID].health)
	}
	if states[clusters[1].ID].health != "not_onboarded" {
		t.Fatalf("second health = %q", states[clusters[1].ID].health)
	}
	if states[clusters[2].ID].health != "offline" {
		t.Fatalf("third health = %q", states[clusters[2].ID].health)
	}
}

func TestObservationTrendStepCoversRetentionRange(t *testing.T) {
	cases := []struct {
		span time.Duration
		step time.Duration
	}{
		{time.Hour, time.Minute},
		{6 * time.Hour, 2 * time.Minute},
		{24 * time.Hour, 5 * time.Minute},
		{7 * 24 * time.Hour, time.Hour},
		{90 * 24 * time.Hour, 24 * time.Hour},
		{365 * 24 * time.Hour, 3 * 24 * time.Hour},
	}
	for _, test := range cases {
		if got := observationTrendStep(test.span); got != test.step {
			t.Errorf("span %s: step = %s, want %s", test.span, got, test.step)
		}
	}
}

func TestObservationHealthIgnoresExpectedFilteringRetryAndIntegritySignals(t *testing.T) {
	now := time.Now().UTC()
	onboardedAt := now.Add(-time.Hour)
	cluster := dbrepo.Cluster{Phase: "Online", ObservabilityOnboardedAt: &onboardedAt}
	cluster.ID = 1
	states := observationStates([]dbrepo.Cluster{cluster}, map[int64][]dbrepo.ObservationAggregate{
		cluster.ID: {
			{
				Component: string(admissionobs.ComponentWebhook), LastObservedAt: &now,
				Counters:         admissionobs.Counters{Retry: 3},
				SequenceGapCount: 2, BaselineResetCount: 1, SeriesStartedCount: 1,
			},
			{
				Component: string(admissionobs.ComponentOperator), LastObservedAt: &now,
				Counters: admissionobs.Counters{PolicyFiltered: 4, Duplicate: 2},
			},
		},
	}, nil, now)
	if states[cluster.ID].health != "normal" {
		t.Fatalf("health = %q, want normal", states[cluster.ID].health)
	}
	if !states[cluster.ID].incomplete {
		t.Fatal("sequence gap did not set the informational incomplete flag")
	}
}

func TestObservationAggregateCacheSeparatesWindows(t *testing.T) {
	handlers := &Handlers{}
	now := time.Now().UTC()
	firstKey := "clusters/" + now.Add(-time.Hour).Format(time.RFC3339Nano) + "/" + now.Format(time.RFC3339Nano)
	secondKey := "clusters/" + now.Add(-24*time.Hour).Format(time.RFC3339Nano) + "/" + now.Format(time.RFC3339Nano)
	handlers.storeObservationAggregates(firstKey, []dbrepo.ObservationAggregate{{Component: "webhook"}})
	if cached, ok := handlers.cachedObservationAggregates(firstKey); !ok || len(cached) != 1 {
		t.Fatalf("first window cache = (%+v, %v)", cached, ok)
	}
	if _, ok := handlers.cachedObservationAggregates(secondKey); ok {
		t.Fatal("different observation window reused the cached aggregate")
	}
	handlers.clearObservationCache()
	if _, ok := handlers.cachedObservationAggregates(firstKey); ok {
		t.Fatal("cache entry survived invalidation")
	}
}
