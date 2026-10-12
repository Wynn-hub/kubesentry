package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolerepo "github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type tenantClusterRepoFake struct {
	consolerepo.ClusterRepository
	tenantID      int64
	names         []string
	allowTransfer bool
	err           error
}

func (f *tenantClusterRepoFake) SyncTenantClusters(_ context.Context, tenantID int64, names []string, allowTransfer bool) error {
	f.tenantID, f.names, f.allowTransfer = tenantID, names, allowTransfer
	return f.err
}

func TestTenantClusterSyncNormalizesAndAuthorizes(t *testing.T) {
	repo := &tenantClusterRepoFake{}
	notifier := &recordingNotifier{}
	svc := NewTenantClusterService(repo, notifier)
	input := dto.TenantClustersInput{ClusterNames: []string{" zeta ", "alpha", "alpha", ""}, AllowTransfer: true}
	if err := svc.Sync(t.Context(), dbrepo.Principal{Role: dbrepo.RoleTenantAdmin}, 7, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("tenant admin error = %v", err)
	}
	if err := svc.Sync(t.Context(), dbrepo.Principal{Role: dbrepo.RoleSystemAdmin}, 7, input); err != nil {
		t.Fatal(err)
	}
	if repo.tenantID != 7 || !repo.allowTransfer || !reflect.DeepEqual(repo.names, []string{"alpha", "zeta"}) {
		t.Fatalf("sync args = %d %#v %v", repo.tenantID, repo.names, repo.allowTransfer)
	}
	if !reflect.DeepEqual(notifier.topics, []string{"cluster-tenant"}) {
		t.Fatalf("notification topics = %#v", notifier.topics)
	}
}

func TestTenantClusterSyncMapsRepositoryErrors(t *testing.T) {
	repo := &tenantClusterRepoFake{err: dbrepo.ErrClusterTenantTransferRequired}
	svc := NewTenantClusterService(repo, nil)
	err := svc.Sync(t.Context(), dbrepo.Principal{Role: dbrepo.RoleSystemAdmin}, 7, dto.TenantClustersInput{})
	if !IsConflict(err) {
		t.Fatalf("transfer error = %v", err)
	}
	repo.err = dbrepo.ErrClusterNotFound
	if err := svc.Sync(t.Context(), dbrepo.Principal{Role: dbrepo.RoleSystemAdmin}, 7, dto.TenantClustersInput{}); !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("not found error = %v", err)
	}
}
