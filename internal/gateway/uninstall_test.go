package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestFailedUninstallKeepsConnectedClusterOnline(t *testing.T) {
	repos := newTestRepos(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "member", Phase: gatewaymodel.ClusterPhaseUninstalling}
	mustCreateCluster(t, repos, cluster)
	task := &dbrepo.UninstallTask{ClusterID: cluster.ID}
	if err := repos.UninstallTasks.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := repos.UninstallTasks.Transition(ctx, task.ID, dbrepo.UninstallTaskPending, dbrepo.UninstallTaskReceived, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	NewServer(repos, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token").handleUninstallTaskReport(ctx, "member", &gatewaypb.UninstallTaskReport{
		TaskId: task.ID,
		State:  gatewaypb.UninstallTaskReport_FAILED,
		Error:  "delete CustomResourceDefinition: forbidden",
	})

	gotCluster, err := repos.Clusters.GetByName(ctx, cluster.Name)
	if err != nil {
		t.Fatal(err)
	}
	if gotCluster.Phase != gatewaymodel.ClusterPhaseOnline {
		t.Fatalf("cluster phase = %q, want Online", gotCluster.Phase)
	}
	gotTask, err := repos.UninstallTasks.GetByID(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask.Status != dbrepo.UninstallTaskFailed || gotTask.Error != "delete CustomResourceDefinition: forbidden" {
		t.Fatalf("task = %+v", gotTask)
	}
}
