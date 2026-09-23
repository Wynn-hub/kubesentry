package repository_test

import (
	"context"
	"errors"
	"testing"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestClusterRepoWithTxRollsBackCoupledLifecycleWrites(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	wantErr := errors.New("second write failed")
	err := repo.WithTx(context.Background(), func(clusters *dbrepo.ClusterRepo, records *dbrepo.InstallRecordRepo, _ *dbrepo.UninstallTaskRepo) error {
		cluster := &dbrepo.Cluster{Name: "rollback-cluster", Phase: "Registering"}
		if err := clusters.Create(context.Background(), cluster); err != nil {
			return err
		}
		if err := records.Create(context.Background(), &dbrepo.ClusterInstallRecord{ClusterName: cluster.Name}); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx error = %v, want %v", err, wantErr)
	}
	if _, err := repo.GetByName(context.Background(), "rollback-cluster"); err == nil {
		t.Fatal("cluster write survived rollback")
	}
	var count int64
	if err := db.Table("cluster_install_records").Where("cluster_name = ?", "rollback-cluster").Count(&count).Error; err != nil {
		t.Fatalf("count install records: %v", err)
	}
	if count != 0 {
		t.Fatalf("install record count = %d, want 0", count)
	}
}
