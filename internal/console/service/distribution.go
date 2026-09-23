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
	rows, total, err := s.repos.Distributions.ListPage(ctx, dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"})
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
	d, err := s.repos.Distributions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policydistribution: %w", err)
	}
	return &dto.DistributionView{Name: d.Name, ResourceVersion: strconv.FormatInt(d.UpdatedAt.UnixNano(), 10), Spec: distributionSpecView(d.Spec.Data())}, nil
}

func (s *DistributionService) Create(ctx context.Context, req dto.DistributionInput) error {
	if err := validateDistributionRequest(&req, true); err != nil {
		return err
	}
	d := &dbrepo.PolicyDistribution{Name: req.Name, Spec: datatypes.NewJSONType(distributionSpecModel(req.Spec))}
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
	d, err := s.repos.Distributions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
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
	d, err := s.repos.Distributions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
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
