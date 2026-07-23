package console

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/datatypes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

const (
	scopeOwnerGroup     = "PolicyGroup"
	scopeOwnerException = "PolicyException"
)

type clusterScope struct {
	AllClusters  bool     `json:"allClusters"`
	ClusterNames []string `json:"clusterNames,omitempty"`
}

func (h *Handlers) getScopeBinding(ctx context.Context, ownerType, ownerName string) (clusterScope, error) {
	dists, err := h.Repos.Distributions.List(ctx)
	if err != nil {
		return clusterScope{}, err
	}
	for i := range dists {
		d := &dists[i]
		if d.Managed && d.OwnerType == ownerType && d.OwnerName == ownerName {
			spec := d.Spec.Data()
			return clusterScope{AllClusters: spec.Clusters.ClusterSelector != nil, ClusterNames: spec.Clusters.ByName}, nil
		}
	}
	// Scope bindings were introduced after policy groups and exceptions were
	// already persisted in some installations. Treat a missing legacy binding
	// as all clusters so reads remain compatible; the next edit creates the
	// explicit managed binding and all new writes must pass validateScope.
	return clusterScope{AllClusters: true}, nil
}

func scopeSpec(all bool, names []string, ownerType, ownerName string) v1alpha1.PolicyDistributionSpec {
	s := v1alpha1.PolicyDistributionSpec{}
	if all {
		s.Clusters.ClusterSelector = &metav1.LabelSelector{}
	} else {
		s.Clusters.ByName = append([]string(nil), names...)
	}
	if ownerType == scopeOwnerGroup {
		s.PolicyGroups = []string{ownerName}
	} else {
		s.PolicyExceptions = []string{ownerName}
	}
	return s
}

// syncScopeBinding maintains exactly one system-managed distribution per
// group/exception. Public Distribution CRUD never calls this function.
func (h *Handlers) syncScopeBinding(ctx context.Context, ownerType, ownerName string, all bool, names []string) error {
	if err := h.validateScopeTargets(ctx, all, names); err != nil {
		return err
	}
	return syncScopeBinding(ctx, h.Repos.Distributions, ownerType, ownerName, all, names)
}

func (h *Handlers) validateScopeTargets(ctx context.Context, all bool, names []string) error {
	if err := validateScope(all, names); err != nil {
		return err
	}
	if !all {
		for _, name := range names {
			cluster, err := h.Repos.Clusters.GetByName(ctx, name)
			if err != nil {
				return fmt.Errorf("target cluster %q: %w", name, err)
			}
			if cluster.Phase == v1alpha1.ClusterPhaseUninstalled {
				return fmt.Errorf("target cluster %q is uninstalled", name)
			}
		}
	}
	return nil
}

func syncScopeBinding(ctx context.Context, distributions *dbrepo.DistributionRepo, ownerType, ownerName string, all bool, names []string) error {
	dists, err := distributions.List(ctx)
	if err != nil {
		return err
	}
	for i := range dists {
		d := &dists[i]
		if d.Managed && d.OwnerType == ownerType && d.OwnerName == ownerName {
			d.Spec = datatypes.NewJSONType(scopeSpec(all, names, ownerType, ownerName))
			return distributions.Update(ctx, d)
		}
	}
	prefix := "scope-group-"
	if ownerType == scopeOwnerException {
		prefix = "scope-exception-"
	}
	name := prefix + strings.ToLower(ownerName)
	return distributions.Create(ctx, &dbrepo.PolicyDistribution{Name: name, Managed: true, OwnerType: ownerType, OwnerName: ownerName, Spec: datatypes.NewJSONType(scopeSpec(all, names, ownerType, ownerName))})
}

func (h *Handlers) deleteScopeBinding(ctx context.Context, ownerType, ownerName string) error {
	return deleteScopeBinding(ctx, h.Repos.Distributions, ownerType, ownerName)
}

func deleteScopeBinding(ctx context.Context, distributions *dbrepo.DistributionRepo, ownerType, ownerName string) error {
	dists, err := distributions.List(ctx)
	if err != nil {
		return err
	}
	for i := range dists {
		d := &dists[i]
		if d.Managed && d.OwnerType == ownerType && d.OwnerName == ownerName {
			return distributions.Delete(ctx, d.ID)
		}
	}
	return nil
}
