package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

const defaultDistributionName = "default"

type DistributionService struct {
	repos    repository.Repositories
	notifier Notifier
}

func NewDistributionService(repos repository.Repositories, notifier Notifier) *DistributionService {
	return &DistributionService{repos: repos, notifier: notifier}
}

func (s *DistributionService) List(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*dto.DistributionPageView, error) {
	options := dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"}
	principal := requestPrincipal(ctx)
	var rows []dbrepo.PolicyDistribution
	var total int64
	var err error
	if principal.Role == dbrepo.RoleSystemAdmin {
		rows, total, err = s.repos.Distributions.ListPage(ctx, options)
	} else {
		rows, total, err = s.repos.Distributions.ListPageVisible(ctx, *principal.TenantID, options)
	}
	if err != nil {
		return nil, fmt.Errorf("list policy distributions: %w", err)
	}
	items := make([]dto.DistributionItem, 0, len(rows))
	for _, d := range rows {
		items = append(items, dto.DistributionItem{Name: d.Name, PolicyGroups: d.Spec.Data().PolicyGroups})
	}
	return &dto.DistributionPageView{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (s *DistributionService) Get(ctx context.Context, name string) (*dto.DistributionView, error) {
	d, err := s.visibleDistribution(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policydistribution: %w", err)
	}
	return &dto.DistributionView{Name: d.Name, ResourceVersion: strconv.FormatInt(d.UpdatedAt.UnixNano(), 10), Spec: distributionSpecView(d.Spec.Data())}, nil
}

func (s *DistributionService) Create(ctx context.Context, req dto.DistributionInput) error {
	if err := requireEditor(ctx); err != nil {
		return err
	}
	if err := validateDistributionRequest(&req, true); err != nil {
		return err
	}
	if err := s.validateTenantTargets(ctx, req.Spec); err != nil {
		return err
	}
	d := &dbrepo.PolicyDistribution{Name: req.Name, ResourceOwnerType: ownerSystem, Spec: datatypes.NewJSONType(distributionSpecModel(req.Spec))}
	principal := requestPrincipal(ctx)
	if principal.Role == dbrepo.RoleTenantAdmin {
		d.ResourceOwnerType, d.TenantID = ownerTenant, principal.TenantID
	}
	if err := s.repos.Distributions.Create(ctx, d); err != nil {
		if dbrepo.IsUniqueViolation(err) {
			return newConflict("policydistribution %s already exists", req.Name)
		}
		return fmt.Errorf("create policydistribution: %w", err)
	}
	s.notify(ctx, "distribution")
	return nil
}

func (s *DistributionService) Update(ctx context.Context, name string, req dto.DistributionInput) error {
	if err := validateDistributionRequest(&req, false); err != nil {
		return err
	}
	d, err := s.visibleDistribution(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := authorizeOwner(ctx, d.ResourceOwnerType, d.TenantID); err != nil {
		return err
	}
	if err := s.validateTenantTargets(ctx, req.Spec); err != nil {
		return err
	}
	if req.ResourceVersion != "" && req.ResourceVersion != strconv.FormatInt(d.UpdatedAt.UnixNano(), 10) {
		return newConflict("policydistribution has been modified, refresh and retry")
	}
	if err := s.repos.Distributions.UpdateCAS(ctx, d.ID, d.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(distributionSpecModel(req.Spec))}); err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			return newConflict("policydistribution has been modified, refresh and retry")
		}
		return fmt.Errorf("update policydistribution: %w", err)
	}
	s.notify(ctx, "distribution")
	return nil
}

func distributionSpecModel(spec dto.DistributionSpecView) gatewaymodel.PolicyDistributionSpec {
	return gatewaymodel.PolicyDistributionSpec{
		Clusters:         gatewaymodel.DistributionClusters{ByName: spec.Clusters.ByName, ClusterSelector: labelSelectorModel(spec.Clusters.ClusterSelector)},
		PolicyGroups:     spec.PolicyGroups,
		PolicyExceptions: spec.PolicyExceptions,
		Overrides:        distributionOverridesModel(spec.Overrides),
	}
}

func distributionSpecView(spec gatewaymodel.PolicyDistributionSpec) dto.DistributionSpecView {
	return dto.DistributionSpecView{
		Clusters:         dto.DistributionClusters{ByName: spec.Clusters.ByName, ClusterSelector: labelSelectorView(spec.Clusters.ClusterSelector)},
		PolicyGroups:     spec.PolicyGroups,
		PolicyExceptions: spec.PolicyExceptions,
		Overrides:        distributionOverridesView(spec.Overrides),
	}
}

func distributionOverridesModel(values []dto.DistributionOverride) []gatewaymodel.DistributionOverride {
	result := make([]gatewaymodel.DistributionOverride, len(values))
	for i := range values {
		result[i] = gatewaymodel.DistributionOverride{Cluster: values[i].Cluster, EnforcementMode: values[i].EnforcementMode}
	}
	return result
}

func distributionOverridesView(values []gatewaymodel.DistributionOverride) []dto.DistributionOverride {
	result := make([]dto.DistributionOverride, len(values))
	for i := range values {
		result[i] = dto.DistributionOverride{Cluster: values[i].Cluster, EnforcementMode: values[i].EnforcementMode}
	}
	return result
}

func (s *DistributionService) Delete(ctx context.Context, name string) error {
	d, err := s.visibleDistribution(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := authorizeOwner(ctx, d.ResourceOwnerType, d.TenantID); err != nil {
		return err
	}
	if d.Name == defaultDistributionName {
		return newConflict("default policy distribution cannot be deleted")
	}
	if err := s.repos.Distributions.Delete(ctx, d.ID); err != nil {
		return fmt.Errorf("delete policydistribution: %w", err)
	}
	s.notify(ctx, "distribution")
	return nil
}

func (s *DistributionService) notify(ctx context.Context, topic string) {
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, topic)
	}
}

func validateDistributionRequest(req *dto.DistributionInput, create bool) error {
	if create {
		if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
			return &ValidationError{Msg: "invalid name: " + strings.Join(errs, "; ")}
		}
	}
	for _, group := range req.Spec.PolicyGroups {
		if group == "" {
			return &ValidationError{Msg: "policyGroups entries must not be empty"}
		}
	}
	return nil
}

func (s *DistributionService) visibleDistribution(ctx context.Context, name string) (*dbrepo.PolicyDistribution, error) {
	rows, err := s.repos.Distributions.List(ctx)
	if err != nil {
		return nil, err
	}
	principal := requestPrincipal(ctx)
	var system *dbrepo.PolicyDistribution
	for i := range rows {
		distribution := &rows[i]
		if distribution.Name != name || !visibleTo(ctx, distribution.ResourceOwnerType, distribution.TenantID) {
			continue
		}
		if principal.TenantID != nil && distribution.TenantID != nil && *principal.TenantID == *distribution.TenantID {
			return distribution, nil
		}
		if distribution.ResourceOwnerType == ownerSystem {
			system = distribution
		}
	}
	if system != nil {
		return system, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *DistributionService) validateTenantTargets(ctx context.Context, spec dto.DistributionSpecView) error {
	principal := requestPrincipal(ctx)
	if principal.Role != dbrepo.RoleTenantAdmin {
		return nil
	}
	if spec.Clusters.ClusterSelector != nil {
		return &ValidationError{Msg: "tenant distributions must select clusters by name"}
	}
	names := append([]string{}, spec.Clusters.ByName...)
	for _, override := range spec.Overrides {
		names = append(names, override.Cluster)
	}
	for _, name := range names {
		if _, err := s.repos.Clusters.GetByNameForTenant(ctx, *principal.TenantID, name); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return &ValidationError{Msg: fmt.Sprintf("target cluster %q is not assigned to the tenant", name)}
			}
			return err
		}
	}
	groups := NewPolicyGroupService(s.repos, nil)
	for _, name := range spec.PolicyGroups {
		if _, err := groups.visibleGroup(ctx, name); err != nil {
			return &ValidationError{Msg: fmt.Sprintf("policy group %q is not visible to the tenant", name)}
		}
	}
	exceptions := NewExceptionService(s.repos, nil)
	for _, name := range spec.PolicyExceptions {
		if _, err := exceptions.visibleException(ctx, name); err != nil {
			return &ValidationError{Msg: fmt.Sprintf("policy exception %q is not visible to the tenant", name)}
		}
	}
	return nil
}
