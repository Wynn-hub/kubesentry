package console

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func validInstallParams() map[string]any {
	return map[string]any{
		"namespace": "kubesentry-system",
		"webhookImage": map[string]any{
			"repository": "wynnhub/kubesentry-webhook",
			"tag":        "latest",
			"pullPolicy": "IfNotPresent",
		},
		"operatorImage": map[string]any{
			"repository": "wynnhub/kubesentry-operator",
			"tag":        "latest",
			"pullPolicy": "IfNotPresent",
		},
		"highAvailability": false,
		"gatewayAddress":   "10.0.0.1:30443",
	}
}

func TestCreateClusterReturnsCommandAndRegisteringPhase(t *testing.T) {
	h, repos := newDBTestServer(t)
	rec, env := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{
		"name":          "prod-east",
		"installParams": validInstallParams(),
	})
	if rec.Code != 200 {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	created := mustUnmarshal[dto.ClusterCommand](t, env.Data)
	if created.Name != "prod-east" || created.Command == "" {
		t.Fatalf("created = %+v", created)
	}
	for _, want := range []string{
		"--set webhook.image.repository=\"wynnhub/kubesentry-webhook\"",
		"--set operator.image.repository=\"wynnhub/kubesentry-operator\"",
		"--set gateway.address=\"10.0.0.1:30443\"",
		"--set gateway.clusterName=\"prod-east\"",
	} {
		if !strings.Contains(created.Command, want) {
			t.Errorf("command missing %q\ngot: %s", want, created.Command)
		}
	}
	if strings.Contains(created.Command, "gateway.caBundle") {
		t.Error("command should omit gateway.caBundle when not provided")
	}
	if strings.Contains(created.Command, "highAvailability.enabled") {
		t.Error("default command should not enable high availability")
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

func TestCreateClusterRecordsInstallHistory(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{
		"name":          "prod-east",
		"installParams": validInstallParams(),
	})

	records, err := repos.InstallRecords.List(context.Background())
	if err != nil {
		t.Fatalf("list install records: %v", err)
	}
	if len(records) != 1 || records[0].ClusterName != "prod-east" {
		t.Fatalf("records = %+v, want one record for prod-east", records)
	}
	if records[0].Namespace != "kubesentry-system" {
		t.Errorf("namespace = %q, want kubesentry-system", records[0].Namespace)
	}
}

func TestCreateClusterHighAvailabilityAddsReplicaOverridesAndPersistsChoice(t *testing.T) {
	h, repos := newDBTestServer(t)
	p := validInstallParams()
	p["highAvailability"] = true
	rec, env := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{
		"name":          "ha-member",
		"installParams": p,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	created := mustUnmarshal[dto.ClusterCommand](t, env.Data)
	for _, want := range []string{
		"--set webhook.highAvailability.enabled=true",
		"--set operator.highAvailability.enabled=true",
	} {
		if !strings.Contains(created.Command, want) {
			t.Errorf("command missing %q\ngot: %s", want, created.Command)
		}
	}
	records, err := repos.InstallRecords.List(context.Background())
	if err != nil {
		t.Fatalf("list install records: %v", err)
	}
	if len(records) != 1 || !records[0].HighAvailability {
		t.Fatalf("records = %+v, want highAvailability=true", records)
	}
}

func TestCreateClusterDuplicateNameConflict(t *testing.T) {
	h, _ := newDBTestServer(t)
	body := map[string]any{"name": "dup", "installParams": validInstallParams()}
	doRequest(t, h, "POST", "/api/v1/clusters", body)
	rec, env := doRequest(t, h, "POST", "/api/v1/clusters", body)
	if rec.Code != 409 {
		t.Fatalf("code = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if env.Code == 0 || !strings.Contains(env.Message, "dup") {
		t.Errorf("message = %q (code %d), want it to mention the cluster name", env.Message, env.Code)
	}
}

func TestCreateClusterInvalidNameRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, _ := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{
		"name":          "Not_A_Valid_Name!",
		"installParams": validInstallParams(),
	})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateClusterDangerousCharacterRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	p := validInstallParams()
	p["gatewayAddress"] = "10.0.0.1:30443`whoami`"
	rec, _ := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "danger", "installParams": p})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateClusterInvalidPullPolicyRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	p := validInstallParams()
	p["webhookImage"].(map[string]any)["pullPolicy"] = "Sometimes"
	rec, _ := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "bad-policy", "installParams": p})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateClusterMissingRequiredFieldRejected(t *testing.T) {
	h, _ := newDBTestServer(t)
	p := validInstallParams()
	p["gatewayAddress"] = ""
	rec, _ := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "no-address", "installParams": p})
	if rec.Code != 400 {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListClustersSortedByName(t *testing.T) {
	h, _ := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "zeta", "installParams": validInstallParams()})
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "alpha", "installParams": validInstallParams()})

	rec, env := doRequest(t, h, "GET", "/api/v1/clusters", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	page := mustUnmarshal[testPageResult[dto.ClusterListItem]](t, env.Data)
	items := page.Items
	if page.Total != 2 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page metadata = %+v", page)
	}
	if len(items) != 2 || items[0].Name != "alpha" || items[1].Name != "zeta" {
		t.Fatalf("items = %+v, want [alpha, zeta]", items)
	}
}

// Member-reported inventory is written by the gateway's StatusFlusher; the
// console must surface it on both the list and the detail response.
func TestClusterResponsesExposeMemberReportedInventory(t *testing.T) {
	h, repos := newDBTestServer(t)
	ctx := context.Background()
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "member", "installParams": validInstallParams()})

	cluster, err := repos.Clusters.GetByName(ctx, "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.Clusters.UpdateInventory(ctx, cluster.ID, dbrepo.ClusterInventory{
		KubernetesVersion: "v1.31.2",
		APIServerAddress:  "https://10.96.0.1:443",
		NodeCount:         5,
		ReadyNodeCount:    4,
	}); err != nil {
		t.Fatal(err)
	}

	rec, env := doRequest(t, h, "GET", "/api/v1/clusters/member", nil)
	if rec.Code != 200 {
		t.Fatalf("detail code = %d, body = %s", rec.Code, rec.Body.String())
	}
	detail := mustUnmarshal[dto.ClusterDetail](t, env.Data)
	if detail.KubernetesVersion != "v1.31.2" || detail.ApiServerAddress != "https://10.96.0.1:443" ||
		detail.NodeCount != 5 || detail.ReadyNodeCount != 4 {
		t.Fatalf("detail inventory = %+v", detail)
	}

	rec, env = doRequest(t, h, "GET", "/api/v1/clusters", nil)
	if rec.Code != 200 {
		t.Fatalf("list code = %d", rec.Code)
	}
	items := mustUnmarshal[testPageResult[dto.ClusterListItem]](t, env.Data).Items
	if len(items) != 1 || items[0].KubernetesVersion != "v1.31.2" || items[0].NodeCount != 5 || items[0].ReadyNodeCount != 4 {
		t.Fatalf("list inventory = %+v", items)
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
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "live", "installParams": validInstallParams()})
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

func TestUninstallClusterCreatesTaskAndRetryFailedTask(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "live", "installParams": validInstallParams()})
	c, err := repos.Clusters.GetByName(context.Background(), "live")
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.Clusters.UpdateHeartbeat(context.Background(), c.ID, "Online", nil, "v1.0.0"); err != nil {
		t.Fatal(err)
	}

	rec, env := doRequest(t, h, "POST", "/api/v1/clusters/live/uninstall", nil)
	if rec.Code != 200 {
		t.Fatalf("create uninstall = %d: %s", rec.Code, rec.Body.String())
	}
	task := mustUnmarshal[dto.UninstallTask](t, env.Data)
	if task.Status != dto.UninstallTaskStatusPending || task.ID == "" {
		t.Fatalf("task = %+v", task)
	}
	taskID, err := strconv.ParseInt(string(task.ID), 10, 64)
	if err != nil {
		t.Fatalf("parse task ID %q: %v", task.ID, err)
	}
	c, err = repos.Clusters.GetByName(context.Background(), "live")
	if err != nil || c.Phase != "Uninstalling" {
		t.Fatalf("cluster after request = %+v, err=%v", c, err)
	}
	if err := repos.UninstallTasks.Transition(context.Background(), taskID, "Pending", "Received", "", task.RequestedAt); err != nil {
		t.Fatal(err)
	}
	if err := repos.UninstallTasks.Transition(context.Background(), taskID, "Received", "Failed", "connection lost", task.RequestedAt); err != nil {
		t.Fatal(err)
	}

	rec, _ = doRequest(t, h, "POST", "/api/v1/clusters/live/uninstall-tasks/"+fmt.Sprint(task.ID)+"/retry", nil)
	if rec.Code != 200 {
		t.Fatalf("retry = %d: %s", rec.Code, rec.Body.String())
	}
	retried, err := repos.UninstallTasks.GetByID(context.Background(), taskID)
	if err != nil || retried.Status != "Pending" || retried.Attempts != 2 || retried.Error != "" {
		t.Fatalf("retried task = %+v, err=%v", retried, err)
	}
}

func TestClusterUninstallNotificationPreservesTraceAndIsNonFatal(t *testing.T) {
	h, repos, handlers := newDBTestServerWithHandlers(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "trace-live", "installParams": validInstallParams()})
	c, err := repos.Clusters.GetByName(t.Context(), "trace-live")
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.Clusters.UpdateHeartbeat(t.Context(), c.ID, "Online", nil, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeHubClient{notifyChangeErr: context.DeadlineExceeded}
	handlers.Gateway = fake

	rec, env := doRequestWithHeaders(t, h, "POST", "/api/v1/clusters/trace-live/uninstall", nil, map[string]string{"X-Request-Id": "uninstall-trace"})
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("code=%d body=%s, want non-fatal notification failure", rec.Code, rec.Body.String())
	}
	topics, traces := fake.calls()
	if len(topics) != 1 || topics[0] != "cluster-uninstall" || len(traces) != 1 || traces[0] != "uninstall-trace" {
		t.Fatalf("topics=%v traces=%v, want uninstall topic with request trace", topics, traces)
	}
}

func TestDeleteClusterOfflineSucceeds(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "gone", "installParams": validInstallParams()})
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
	rec1, env1 := doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "reissue-me", "installParams": validInstallParams()})
	if rec1.Code != 200 {
		t.Fatalf("create code = %d", rec1.Code)
	}
	firstCommand := mustUnmarshal[dto.ClusterCommand](t, env1.Data).Command

	rec2, env2 := doRequest(t, h, "POST", "/api/v1/clusters/reissue-me/reissue-token", map[string]any{"installParams": validInstallParams()})
	if rec2.Code != 200 {
		t.Fatalf("reissue code = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	second := mustUnmarshal[dto.ClusterCommand](t, env2.Data)
	if second.Command == "" || second.Command == firstCommand {
		t.Fatalf("expected a fresh, different command (new token embedded); first=%q second=%q", firstCommand, second.Command)
	}

	c, err := repos.Clusters.GetByName(context.Background(), "reissue-me")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if err := repos.Clusters.UpdateHeartbeat(context.Background(), c.ID, "Online", nil, "v1.0.0"); err != nil {
		t.Fatalf("simulate online: %v", err)
	}
	rec3, _ := doRequest(t, h, "POST", "/api/v1/clusters/reissue-me/reissue-token", map[string]any{"installParams": validInstallParams()})
	if rec3.Code != 409 {
		t.Fatalf("code = %d, want 409 once cluster is Online", rec3.Code)
	}
}

func TestReissueTokenRecordsInstallHistory(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "reissue-me", "installParams": validInstallParams()})
	doRequest(t, h, "POST", "/api/v1/clusters/reissue-me/reissue-token", map[string]any{"installParams": validInstallParams()})

	records, err := repos.InstallRecords.List(context.Background())
	if err != nil {
		t.Fatalf("list install records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want 2 (one from create, one from reissue)", records)
	}
}

func TestUpdateClusterLabels(t *testing.T) {
	h, repos := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "labeled", "installParams": validInstallParams()})

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

func TestGetLatestInstallParamsNoneExists(t *testing.T) {
	h, _ := newDBTestServer(t)
	rec, env := doRequest(t, h, "GET", "/api/v1/clusters/nope/latest-install-params", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if string(env.Data) != "" {
		t.Errorf("data = %s, want omitted for a cluster with no history", env.Data)
	}
}

func TestGetLatestInstallParamsReturnsNewest(t *testing.T) {
	h, _ := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "prod-east", "installParams": validInstallParams()})
	p2 := validInstallParams()
	p2["namespace"] = "custom-ns"
	p2["highAvailability"] = true
	doRequest(t, h, "POST", "/api/v1/clusters/prod-east/reissue-token", map[string]any{"installParams": p2})

	rec, env := doRequest(t, h, "GET", "/api/v1/clusters/prod-east/latest-install-params", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	got := mustUnmarshal[dto.InstallParams](t, env.Data)
	if got.Namespace != "custom-ns" {
		t.Errorf("namespace = %q, want custom-ns (the most recent)", got.Namespace)
	}
	if !got.HighAvailability {
		t.Error("highAvailability = false, want the most recent value true")
	}
}

func TestListInstallRecordsAcrossClusters(t *testing.T) {
	h, _ := newDBTestServer(t)
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "cluster-a", "installParams": validInstallParams()})
	doRequest(t, h, "POST", "/api/v1/clusters", map[string]any{"name": "cluster-b", "installParams": validInstallParams()})

	rec, env := doRequest(t, h, "GET", "/api/v1/install-records", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	page := mustUnmarshal[testPageResult[dto.InstallRecord]](t, env.Data)
	items := page.Items
	if page.Total != 2 || page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page metadata = %+v", page)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want 2 records", items)
	}
}
