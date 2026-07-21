package console

import (
	"context"
	"testing"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

func seedDistribution(t *testing.T, repos gateway.Repos, name string, groups []string) *dbrepo.PolicyDistribution {
	t.Helper()
	d := &dbrepo.PolicyDistribution{
		Name: name,
		Spec: datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ByName: []string{"local"}},
			PolicyGroups: groups,
		}),
	}
	if err := repos.Distributions.Create(context.Background(), d); err != nil {
		t.Fatalf("seed distribution %s: %v", name, err)
	}
	return d
}

func validDistributionReq() *distributionRequest {
	return &distributionRequest{
		Name: "my-dist",
		Spec: v1alpha1.PolicyDistributionSpec{
			Clusters:     v1alpha1.DistributionClusters{ByName: []string{"local"}},
			PolicyGroups: []string{"g1"},
		},
	}
}

func TestListDistributions(t *testing.T) {
	// The migration seeds a "default" distribution alongside the 37 builtin
	// policies, so the list is never empty even in a fresh DB — assert our
	// seeded distribution is present rather than an exact total count.
	h, repos := newDBTestServer(t)
	seedDistribution(t, repos, "d1", []string{"g1", "g2"})
	rec, env := doRequest(t, h, "GET", "/api/v1/distributions", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	items := mustUnmarshal[[]distributionListItem](t, env.Data)
	var found bool
	for _, it := range items {
		if it.Name == "d1" {
			found = true
			if len(it.PolicyGroups) != 2 {
				t.Fatalf("d1.PolicyGroups = %+v, want 2 entries", it.PolicyGroups)
			}
		}
	}
	if !found {
		t.Fatalf("d1 not found in items = %+v", items)
	}
}

func TestGetDistributionDetail(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedDistribution(t, repos, "d1", []string{"g1"})
	rec, env := doRequest(t, h, "GET", "/api/v1/distributions/d1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d := mustUnmarshal[distributionDetail](t, env.Data)
	if len(d.Spec.PolicyGroups) != 1 || d.Spec.PolicyGroups[0] != "g1" {
		t.Fatalf("detail = %+v", d)
	}
	if len(d.Spec.Clusters.ByName) != 1 || d.Spec.Clusters.ByName[0] != "local" {
		t.Fatalf("detail.Spec.Clusters = %+v", d.Spec.Clusters)
	}
}

func TestGetDistributionNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "GET", "/api/v1/distributions/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestCreateDistribution(t *testing.T) {
	h, repos := newDBTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/distributions", validDistributionReq())
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	d, err := repos.Distributions.GetByName(context.Background(), "my-dist")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Spec.Data().PolicyGroups) != 1 || d.Spec.Data().PolicyGroups[0] != "g1" {
		t.Fatalf("distribution = %+v", d.Spec.Data())
	}
}

func TestCreateDistributionDuplicate(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedDistribution(t, repos, "my-dist", []string{"g1"})
	rec, _ := doRequest(t, h, "POST", "/api/v1/distributions", validDistributionReq())
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreateDistributionInvalidNameRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validDistributionReq()
	req.Name = "Not_Valid!"
	rec, _ := doRequest(t, h, "POST", "/api/v1/distributions", req)
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// TestCreateDistributionToleratesDanglingGroupReference confirms the
// deliberate non-validation documented in the plan's Global Constraints —
// referencing a PolicyGroup name that doesn't exist must still succeed.
func TestCreateDistributionToleratesDanglingGroupReference(t *testing.T) {
	h, repos := newDBTestServer(t)
	req := validDistributionReq()
	req.Spec.PolicyGroups = []string{"does-not-exist"}
	rec, _ := doRequest(t, h, "POST", "/api/v1/distributions", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200 (dangling group refs are tolerated), body should not matter here", rec.Code)
	}
	d, err := repos.Distributions.GetByName(context.Background(), "my-dist")
	if err != nil {
		t.Fatal(err)
	}
	if d.Spec.Data().PolicyGroups[0] != "does-not-exist" {
		t.Fatalf("distribution = %+v", d.Spec.Data())
	}
}

func TestUpdateDistribution(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedDistribution(t, repos, "d1", []string{"g1"})
	req := validDistributionReq()
	req.Name = ""
	req.Spec.PolicyGroups = []string{"g1", "g2"}
	rec, _ := doRequest(t, h, "PUT", "/api/v1/distributions/d1", req)
	if rec.Code != 200 {
		t.Fatalf("code = %d, body should not matter here", rec.Code)
	}
	d, _ := repos.Distributions.GetByName(context.Background(), "d1")
	if len(d.Spec.Data().PolicyGroups) != 2 {
		t.Fatalf("policyGroups = %+v, want 2 entries", d.Spec.Data().PolicyGroups)
	}
}

func TestUpdateDistributionStaleResourceVersion(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedDistribution(t, repos, "d1", []string{"g1"})
	req := validDistributionReq()
	req.Name = ""
	req.ResourceVersion = "stale-does-not-match"
	rec, _ := doRequest(t, h, "PUT", "/api/v1/distributions/d1", req)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestUpdateDistributionNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	req := validDistributionReq()
	req.Name = ""
	rec, _ := doRequest(t, h, "PUT", "/api/v1/distributions/nope", req)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteDistribution(t *testing.T) {
	h, repos := newDBTestServer(t)
	seedDistribution(t, repos, "d1", []string{"g1"})
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/distributions/d1", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if _, err := repos.Distributions.GetByName(context.Background(), "d1"); err == nil {
		t.Fatal("expected distribution to be gone after delete")
	}
}

func TestDeleteDistributionNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/distributions/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteDistributionRejectsDefaultName(t *testing.T) {
	// The migration already seeds a "default" distribution — no need to
	// seed our own, and seeding one would collide on the unique name
	// constraint.
	h, repos := newDBTestServer(t)
	rec, _ := doRequest(t, h, "DELETE", "/api/v1/distributions/default", nil)
	if rec.Code != 403 {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
	if _, err := repos.Distributions.GetByName(context.Background(), "default"); err != nil {
		t.Fatalf("default distribution must survive the rejected delete: %v", err)
	}
}
