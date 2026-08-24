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

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

const defaultDistributionName = "default"

type DistributionListItem struct {
	Name         string   `json:"name"`
	PolicyGroups []string `json:"policyGroups"`
}
type DistributionPage struct {
	Items    []DistributionListItem `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
}
type DistributionDetail struct {
	Name            string                          `json:"name"`
	ResourceVersion string                          `json:"resourceVersion"`
	Spec            v1alpha1.PolicyDistributionSpec `json:"spec"`
}
type DistributionRequest struct {
	Name            string                          `json:"name,omitempty"`
	Spec            v1alpha1.PolicyDistributionSpec `json:"spec"`
	ResourceVersion string                          `json:"resourceVersion,omitempty"`
}

type DistributionService struct {
	repos    repository.Repositories
	notifier Notifier
}

func NewDistributionService(repos repository.Repositories, notifier Notifier) *DistributionService {
	return &DistributionService{repos: repos, notifier: notifier}
}

func (s *DistributionService) List(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*DistributionPage, error) {
	rows, total, err := s.repos.Distributions.ListPage(ctx, dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"})
	if err != nil {
		return nil, fmt.Errorf("list policy distributions: %w", err)
	}
	items := make([]DistributionListItem, 0, len(rows))
	for _, d := range rows {
		items = append(items, DistributionListItem{Name: d.Name, PolicyGroups: d.Spec.Data().PolicyGroups})
	}
	return &DistributionPage{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (s *DistributionService) Get(ctx context.Context, name string) (*DistributionDetail, error) {
	d, err := s.repos.Distributions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policydistribution: %w", err)
	}
	return &DistributionDetail{Name: d.Name, ResourceVersion: strconv.FormatInt(d.UpdatedAt.UnixNano(), 10), Spec: d.Spec.Data()}, nil
}

func (s *DistributionService) Create(ctx context.Context, req DistributionRequest) error {
	if err := validateDistributionRequest(&req, true); err != nil {
		return err
	}
	d := &dbrepo.PolicyDistribution{Name: req.Name, Spec: datatypes.NewJSONType(req.Spec)}
	if err := s.repos.Distributions.Create(ctx, d); err != nil {
		if dbrepo.IsUniqueViolation(err) {
			return newConflict("policydistribution %s already exists", req.Name)
		}
		return fmt.Errorf("create policydistribution: %w", err)
	}
	s.notify("distribution")
	return nil
}

func (s *DistributionService) Update(ctx context.Context, name string, req DistributionRequest) error {
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
	if err := s.repos.Distributions.UpdateCAS(ctx, d.ID, d.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(req.Spec)}); err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			return newConflict("policydistribution has been modified, refresh and retry")
		}
		return fmt.Errorf("update policydistribution: %w", err)
	}
	s.notify("distribution")
	return nil
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
	s.notify("distribution")
	return nil
}

func (s *DistributionService) notify(topic string) {
	if s.notifier != nil {
		s.notifier.NotifyChange(context.Background(), topic)
	}
}

func validateDistributionRequest(req *DistributionRequest, create bool) error {
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
