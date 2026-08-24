package service

import (
	"context"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type AdmissionService struct {
	repo repository.AdmissionRepository
}

func NewAdmissionService(repo repository.AdmissionRepository) *AdmissionService {
	return &AdmissionService{repo: repo}
}

func (s *AdmissionService) Settings(ctx context.Context) (*dbrepo.AdmissionEventSettings, []dbrepo.AdmissionStoragePolicy, error) {
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	policies, err := s.repo.ListPolicies(ctx)
	if err != nil {
		return nil, nil, err
	}
	return settings, policies, nil
}

func (s *AdmissionService) UpdateSettings(ctx context.Context, settings *dbrepo.AdmissionEventSettings, policies []dbrepo.AdmissionStoragePolicy) error {
	return s.repo.UpdateSettingsAndPolicies(ctx, settings, policies)
}

func (s *AdmissionService) List(ctx context.Context, options dbrepo.AdmissionEventPageOptions) ([]dbrepo.AdmissionEvent, int64, error) {
	return s.repo.ListEventsPage(ctx, options)
}

func (s *AdmissionService) Get(ctx context.Context, id int64) (*dbrepo.AdmissionEvent, error) {
	return s.repo.GetEvent(ctx, id)
}

func (s *AdmissionService) Trend(ctx context.Context, from, to time.Time) ([]dbrepo.AdmissionEventTrendBucket, error) {
	return s.repo.Trend(ctx, from, to, to.Sub(from) <= 48*time.Hour)
}
