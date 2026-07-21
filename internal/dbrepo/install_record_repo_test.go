package dbrepo_test

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func newTestRecord(clusterName string) *dbrepo.ClusterInstallRecord {
	return &dbrepo.ClusterInstallRecord{
		ClusterName:          clusterName,
		Namespace:            "kubesentry-system",
		WebhookRepository:    "wynnhub/kubesentry-webhook",
		WebhookTag:           "latest",
		WebhookPullPolicy:    "IfNotPresent",
		OperatorRepository:   "wynnhub/kubesentry-operator",
		OperatorTag:          "latest",
		OperatorPullPolicy:   "IfNotPresent",
		GatewayAddress:       "10.0.0.1:30443",
		GatewayTLSServerName: "",
		GatewayCABundle:      "",
	}
}

func TestInstallRecordRepoCreateAndList(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewInstallRecordRepo(db)
	ctx := context.Background()

	rec := newTestRecord("prod-east")
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.ID == 0 {
		t.Fatal("expected snowflake ID to be assigned after Create")
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ClusterName != "prod-east" {
		t.Fatalf("list = %+v, want one record for prod-east", list)
	}
}

func TestInstallRecordRepoListNewestFirst(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewInstallRecordRepo(db)
	ctx := context.Background()

	first := newTestRecord("cluster-a")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	second := newTestRecord("cluster-b")
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ClusterName != "cluster-b" || list[1].ClusterName != "cluster-a" {
		t.Fatalf("list = %+v, want [cluster-b, cluster-a] (newest first)", list)
	}
}

func TestInstallRecordRepoLatestForClusterNoneExists(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewInstallRecordRepo(db)

	got, err := repo.LatestForCluster(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("latest for cluster: %v", err)
	}
	if got != nil {
		t.Fatalf("got = %+v, want nil for a cluster with no history", got)
	}
}

func TestInstallRecordRepoLatestForClusterReturnsNewest(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewInstallRecordRepo(db)
	ctx := context.Background()

	older := newTestRecord("prod-east")
	older.Namespace = "old-namespace"
	if err := repo.Create(ctx, older); err != nil {
		t.Fatalf("create older: %v", err)
	}
	newer := newTestRecord("prod-east")
	newer.Namespace = "new-namespace"
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("create newer: %v", err)
	}
	// A different cluster's record must never be returned for "prod-east".
	if err := repo.Create(ctx, newTestRecord("prod-west")); err != nil {
		t.Fatalf("create unrelated: %v", err)
	}

	got, err := repo.LatestForCluster(ctx, "prod-east")
	if err != nil {
		t.Fatalf("latest for cluster: %v", err)
	}
	if got == nil || got.Namespace != "new-namespace" {
		t.Fatalf("got = %+v, want the newer record (namespace=new-namespace)", got)
	}
}
