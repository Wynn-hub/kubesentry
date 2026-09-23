package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestUninstallTaskRepoLifecycle(t *testing.T) {
	db := newTestDB(t)
	clusters := dbrepo.NewClusterRepo(db)
	tasks := dbrepo.NewUninstallTaskRepo(db)
	ctx := context.Background()

	cluster := &dbrepo.Cluster{Name: "uninstall-target", Phase: "Online"}
	if err := clusters.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	task := &dbrepo.UninstallTask{ClusterID: cluster.ID}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.ID == 0 || task.Status != dbrepo.UninstallTaskPending || task.Attempts != 1 {
		t.Fatalf("created task = %+v", task)
	}
	active, err := tasks.HasActive(ctx, cluster.ID)
	if err != nil || !active {
		t.Fatalf("HasActive = %v, %v; want true, nil", active, err)
	}

	now := time.Now().UTC()
	if err := tasks.Transition(ctx, task.ID, dbrepo.UninstallTaskPending, dbrepo.UninstallTaskReceived, "", now); err != nil {
		t.Fatalf("mark received: %v", err)
	}
	if err := tasks.Transition(ctx, task.ID, dbrepo.UninstallTaskReceived, dbrepo.UninstallTaskSucceeded, "", now); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	active, err = tasks.HasActive(ctx, cluster.ID)
	if err != nil || active {
		t.Fatalf("HasActive after completion = %v, %v; want false, nil", active, err)
	}
	got, err := tasks.GetByID(ctx, task.ID)
	if err != nil || got.ReceivedAt == nil || got.CompletedAt == nil || got.Status != dbrepo.UninstallTaskSucceeded {
		t.Fatalf("completed task = %+v, err=%v", got, err)
	}
}

func TestUninstallTaskRepoTransitionRejectsStaleState(t *testing.T) {
	db := newTestDB(t)
	clusters := dbrepo.NewClusterRepo(db)
	tasks := dbrepo.NewUninstallTaskRepo(db)
	ctx := context.Background()

	cluster := &dbrepo.Cluster{Name: "stale-task", Phase: "Online"}
	if err := clusters.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	task := &dbrepo.UninstallTask{ClusterID: cluster.ID}
	if err := tasks.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := tasks.Transition(ctx, task.ID, dbrepo.UninstallTaskReceived, dbrepo.UninstallTaskFailed, "late", time.Now().UTC()); !errors.Is(err, dbrepo.ErrConflict) {
		t.Fatalf("stale transition error = %v, want ErrConflict", err)
	}
}
