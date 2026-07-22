package console

import (
	"context"
	"net/http"
	"testing"

	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// summaryTestPolicy inserts a Policy row directly via the repo (bypassing the
// HTTP handler's synchronous-validate path) so tests can set up fixtures fast.
// Names are expected to carry the zzTestKeyword prefix (see
// handlers_policy_test.go) so they can't collide with a seeded builtin.
func summaryTestPolicy(t *testing.T, repo *dbrepo.PolicyRepo, name, mode string, labels map[string]string) {
	t.Helper()
	p := &dbrepo.Policy{
		Name:            name,
		EnforcementMode: mode,
		Rego:            validRego,
		CompileOK:       true,
		Labels:          datatypes.NewJSONType(labels),
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("create policy %q: %v", name, err)
	}
}

func findSummaryGroup(groups []groupSummary, name string) (groupSummary, bool) {
	for _, g := range groups {
		if g.Name == name {
			return g, true
		}
	}
	return groupSummary{}, false
}

func getSummary(t *testing.T, handler http.Handler) summaryData {
	t.Helper()
	rec, env := doRequest(t, handler, "GET", "/api/v1/summary", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	return mustUnmarshal[summaryData](t, env.Data)
}

// TestSummaryTotals seeds fixtures on top of whatever the migration already
// seeded (37 builtin policies + the "default" group — see
// internal/dbmigrate/seed_test.go) and asserts totals grew by exactly the
// fixture count, since a fresh DB is never actually empty here.
func TestSummaryTotals(t *testing.T) {
	handler, repos := newDBTestServer(t)
	ctx := context.Background()

	before := getSummary(t, handler)

	summaryTestPolicy(t, repos.Policies, zzTestKeyword+"p1", "enforce", nil)
	summaryTestPolicy(t, repos.Policies, zzTestKeyword+"p2", "audit", nil)

	group := &dbrepo.PolicyGroup{
		Name: zzTestKeyword + "g1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			DisplayName: "Group One",
			Enabled:     true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{
					{Name: zzTestKeyword + "p1"},
					{Name: zzTestKeyword + "p2", EnforcementMode: "audit"},
				},
			},
		}),
	}
	if err := repos.Groups.Create(ctx, group); err != nil {
		t.Fatalf("create group: %v", err)
	}

	exception := &dbrepo.PolicyException{
		Name: zzTestKeyword + "e1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{PolicyGroupRefs: []string{group.Name}}),
	}
	if err := repos.Exceptions.Create(ctx, exception); err != nil {
		t.Fatalf("create exception: %v", err)
	}

	after := getSummary(t, handler)

	if after.Totals["policies"] != before.Totals["policies"]+2 {
		t.Fatalf("policies total = %d, want %d", after.Totals["policies"], before.Totals["policies"]+2)
	}
	if after.Totals["policygroups"] != before.Totals["policygroups"]+1 {
		t.Fatalf("policygroups total = %d, want %d", after.Totals["policygroups"], before.Totals["policygroups"]+1)
	}
	if after.Totals["exceptions"] != before.Totals["exceptions"]+1 {
		t.Fatalf("exceptions total = %d, want %d", after.Totals["exceptions"], before.Totals["exceptions"]+1)
	}
	if after.Totals["clusters"] != before.Totals["clusters"] {
		t.Fatalf("clusters total = %d, want %d", after.Totals["clusters"], before.Totals["clusters"])
	}

	g, ok := findSummaryGroup(after.Groups, group.Name)
	if !ok {
		t.Fatalf("group %q missing from summary: %+v", group.Name, after.Groups)
	}
	if g.DisplayName != "Group One" || !g.Enabled {
		t.Fatalf("group summary = %+v", g)
	}
	if g.ResolvedCount != 2 || g.EnforceCount != 1 || g.AuditCount != 1 {
		t.Fatalf("group resolved stats = %+v", g)
	}
}

func TestSummaryClusterPhaseCounts(t *testing.T) {
	handler, repos := newDBTestServer(t)
	before := getSummary(t, handler)

	for _, cluster := range []*dbrepo.Cluster{
		{Name: "summary-online", Phase: "Online"},
		{Name: "summary-offline", Phase: "Offline"},
		{Name: "summary-registering", Phase: "Registering"},
	} {
		if err := repos.Clusters.Create(t.Context(), cluster); err != nil {
			t.Fatalf("create cluster %s: %v", cluster.Name, err)
		}
	}

	after := getSummary(t, handler)
	if after.Totals["clusters"] != before.Totals["clusters"]+3 {
		t.Fatalf("clusters total = %d, want %d", after.Totals["clusters"], before.Totals["clusters"]+3)
	}
	if after.ClusterPhases.Online != before.ClusterPhases.Online+1 ||
		after.ClusterPhases.Offline != before.ClusterPhases.Offline+1 ||
		after.ClusterPhases.Registering != before.ClusterPhases.Registering+1 {
		t.Fatalf("cluster phases = %+v, before = %+v", after.ClusterPhases, before.ClusterPhases)
	}
}

// TestSummaryGroupStatsComputedNotStored proves ResolvedCount/EnforceCount
// change immediately after a group's spec is edited via the group repo
// directly (no separate reconciler run in between) — the stats must come
// from groupresolve.Resolve at request time, never a stored status field
// (spec D7), since a DB-backed console has no such field to read.
func TestSummaryGroupStatsComputedNotStored(t *testing.T) {
	handler, repos := newDBTestServer(t)
	ctx := context.Background()

	summaryTestPolicy(t, repos.Policies, zzTestKeyword+"p1", "enforce", map[string]string{"tier": "core"})
	summaryTestPolicy(t, repos.Policies, zzTestKeyword+"p2", "enforce", map[string]string{"tier": "core"})

	group := &dbrepo.PolicyGroup{
		Name: zzTestKeyword + "g1",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
			Enabled: true,
			Policies: v1alpha1.PolicyGroupPolicies{
				ByName: []v1alpha1.PolicyRef{{Name: zzTestKeyword + "p1"}},
			},
		}),
	}
	if err := repos.Groups.Create(ctx, group); err != nil {
		t.Fatalf("create group: %v", err)
	}

	s := getSummary(t, handler)
	g, ok := findSummaryGroup(s.Groups, group.Name)
	if !ok || g.ResolvedCount != 1 {
		t.Fatalf("expected resolvedCount=1 before widening selector, found=%v got %+v", ok, g)
	}

	// Widen the group to pick up p2 via a label selector, entirely on the DB
	// side — no reconciler, no status write. The next summary call must
	// reflect it immediately.
	group.Spec = datatypes.NewJSONType(v1alpha1.PolicyGroupSpec{
		Enabled: true,
		Policies: v1alpha1.PolicyGroupPolicies{
			ByName:     []v1alpha1.PolicyRef{{Name: zzTestKeyword + "p1"}},
			BySelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "core"}},
		},
	})
	if err := repos.Groups.Update(ctx, group); err != nil {
		t.Fatalf("update group: %v", err)
	}

	s = getSummary(t, handler)
	g, ok = findSummaryGroup(s.Groups, group.Name)
	if !ok || g.ResolvedCount != 2 || g.EnforceCount != 2 {
		t.Fatalf("expected resolvedCount=2 enforceCount=2 after widening selector, found=%v got %+v", ok, g)
	}
}
