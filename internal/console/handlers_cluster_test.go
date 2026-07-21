package console

import (
	"context"
	"strings"
	"testing"
)

func TestCreateClusterReturnsTokenAndRegisteringPhase(t *testing.T) {
	h, repos := newDBTestServer(t)
	rec, env := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "prod-east"})
	if rec.Code != 200 {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	created := mustUnmarshal[createClusterResponse](t, env.Data)
	if created.Name != "prod-east" || created.Token == "" {
		t.Fatalf("created = %+v", created)
	}

	c, err := repos.Clusters.GetByName(context.Background(), "prod-east")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if c.Phase != "Registering" {
		t.Errorf("phase = %q, want Registering", c.Phase)
	}
	if c.RegTokenHash == "" {
		t.Error("expected RegTokenHash to be set")
	}
}

func TestCreateClusterDuplicateNameConflict(t *testing.T) {
	h, _ := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "dup"})
	rec, env := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "dup"})
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if env.Error == nil || !strings.Contains(*env.Error, "dup") {
		t.Errorf("error = %v, want it to mention the cluster name", env.Error)
	}
}

func TestCreateClusterInvalidNameRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "Not_A_Valid_Name!"})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListClustersSortedByName(t *testing.T) {
	h, _ := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "zeta"})
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "alpha"})

	rec, env := doRequest(t, h, "GET", "/api/v1/clusters", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	items := mustUnmarshal[[]clusterListItem](t, env.Data)
	if len(items) != 2 || items[0].Name != "alpha" || items[1].Name != "zeta" {
		t.Fatalf("items = %+v, want [alpha, zeta]", items)
	}
}

func TestGetClusterNotFound(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "GET", "/api/v1/clusters/nope", nil)
	if rec.Code != 404 {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteClusterOnlineRejected(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "live"})
	c, err := repos.Clusters.GetByName(context.Background(), "live")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if err := repos.Clusters.UpdateHeartbeat(context.Background(), c.ID, "Online", nil, "v1.0.0"); err != nil {
		t.Fatalf("simulate online: %v", err)
	}

	rec, _ := doRequest(t, h, "DELETE", "/api/v1/clusters/live", nil)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestDeleteClusterOfflineSucceeds(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "gone"})
	c, err := repos.Clusters.GetByName(context.Background(), "gone")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if err := repos.Clusters.UpdateHeartbeat(context.Background(), c.ID, "Offline", nil, "v1.0.0"); err != nil {
		t.Fatalf("simulate offline: %v", err)
	}

	rec, _ := doRequest(t, h, "DELETE", "/api/v1/clusters/gone", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := repos.Clusters.GetByName(context.Background(), "gone"); err == nil {
		t.Fatal("expected cluster to be gone after delete")
	}
}

func TestReissueTokenOnlyWhenRegistering(t *testing.T) {
	h, repos := newDBTestServer(t)
	rec1, env1 := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "reissue-me"})
	if rec1.Code != 200 {
		t.Fatalf("create code = %d", rec1.Code)
	}
	firstToken := mustUnmarshal[createClusterResponse](t, env1.Data).Token

	rec2, env2 := doRequest(t, h, "POST", "/api/v1/clusters/reissue-me/reissue-token", nil)
	if rec2.Code != 200 {
		t.Fatalf("reissue code = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	second := mustUnmarshal[createClusterResponse](t, env2.Data)
	if second.Token == "" || second.Token == firstToken {
		t.Fatalf("expected a fresh, different token; first=%q second=%q", firstToken, second.Token)
	}

	c, err := repos.Clusters.GetByName(context.Background(), "reissue-me")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if err := repos.Clusters.UpdateHeartbeat(context.Background(), c.ID, "Online", nil, "v1.0.0"); err != nil {
		t.Fatalf("simulate online: %v", err)
	}
	rec3, _ := doRequest(t, h, "POST", "/api/v1/clusters/reissue-me/reissue-token", nil)
	if rec3.Code != 409 {
		t.Fatalf("code = %d, want 409 once cluster is Online", rec3.Code)
	}
}

func TestUpdateClusterLabels(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "labeled"})

	rec, _ := doRequest(t, h, "PUT", "/api/v1/clusters/labeled", map[string]any{"labels": map[string]string{"env": "prod"}})
	if rec.Code != 200 {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}

	c, err := repos.Clusters.GetByName(context.Background(), "labeled")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if c.Labels.Data()["env"] != "prod" {
		t.Errorf("labels[env] = %q, want prod", c.Labels.Data()["env"])
	}
}
