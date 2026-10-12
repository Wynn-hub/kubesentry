package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"gorm.io/gorm"
)

type TenantClusterService struct {
	clusters repository.ClusterRepository
	notifier Notifier
}

func NewTenantClusterService(clusters repository.ClusterRepository, notifier Notifier) *TenantClusterService {
	return &TenantClusterService{clusters: clusters, notifier: notifier}
}

func (s *TenantClusterService) Sync(ctx context.Context, actor dbrepo.Principal, tenantID int64, input dto.TenantClustersInput) error {
	if actor.Role != dbrepo.RoleSystemAdmin {
		return ErrForbidden
	}
	if tenantID <= 0 {
		return ErrIdentityNotFound
	}
	names := normalizeClusterNames(input.ClusterNames)
	if err := s.clusters.SyncTenantClusters(ctx, tenantID, names, input.AllowTransfer); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, dbrepo.ErrClusterNotFound):
			return ErrIdentityNotFound
		case errors.Is(err, dbrepo.ErrClusterTenantTransferRequired):
			return newConflict("%s", err.Error())
		case errors.Is(err, dbrepo.ErrInactiveTenant):
			return ErrInvalidIdentity
		default:
			return err
		}
	}
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "cluster-tenant")
	}
	return nil
}

func normalizeClusterNames(names []string) []string {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := strings.TrimSpace(name); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
