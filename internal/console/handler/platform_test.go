package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/handler"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type platformRepository struct {
	settings dbrepo.PlatformObservabilitySettings
}

func (r *platformRepository) GetSettings(context.Context) (*dbrepo.PlatformObservabilitySettings, error) {
	settings := r.settings
	return &settings, nil
}

func (r *platformRepository) UpdateSettings(_ context.Context, enabled bool, retentionDays int) error {
	r.settings.Enabled = enabled
	r.settings.RetentionDays = retentionDays
	return nil
}

func (*platformRepository) LatestSamples(context.Context, time.Time) ([]dbrepo.PlatformComponentSample, error) {
	return nil, nil
}

func (*platformRepository) RestartedInstances(context.Context, time.Time) ([]dbrepo.PlatformRestartedInstance, error) {
	return nil, nil
}

func (*platformRepository) MemoryPressuredInstances(context.Context, time.Time) ([]dbrepo.PlatformRestartedInstance, error) {
	return nil, nil
}

func (*platformRepository) LatestCPUUsage(context.Context, time.Time) ([]dbrepo.PlatformCPUUsage, error) {
	return nil, nil
}

func (*platformRepository) Trend(context.Context, string, string, int64, time.Time, time.Time, time.Duration) ([]dbrepo.PlatformTrendRow, error) {
	return nil, nil
}

func (*platformRepository) LatestLeadershipEvents(context.Context, time.Time) ([]dbrepo.PlatformLeadershipEvent, error) {
	return nil, nil
}

func (*platformRepository) LeadershipEvents(context.Context, string, *int64, time.Time, time.Time, int, int) (dbrepo.LeadershipEventPage, error) {
	return dbrepo.LeadershipEventPage{}, nil
}

type platformClusterRepository struct{}

func (platformClusterRepository) List(context.Context) ([]dbrepo.Cluster, error) {
	return nil, nil
}

func (platformClusterRepository) GetByName(context.Context, string) (*dbrepo.Cluster, error) {
	return nil, nil
}

func TestPlatformHandlerReturnsTypedSettingsWithoutHTTPDispatch(t *testing.T) {
	repo := &platformRepository{settings: dbrepo.PlatformObservabilitySettings{Enabled: true, RetentionDays: 3}}
	platform := service.NewPlatformService(repo, platformClusterRepository{}, nil)
	h := handler.New(service.Services{Platform: platform})

	result, err := h.GetPlatformSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	response, ok := result.(*dto.PlatformSettingsHTTPResponse)
	if !ok {
		t.Fatalf("response type = %T", result)
	}
	if response.Code != 0 || !response.Data.Enabled || response.Data.RetentionDays != 3 {
		t.Fatalf("response = %+v", response)
	}
}

func TestPlatformHandlerMapsSettingsValidationError(t *testing.T) {
	repo := &platformRepository{settings: dbrepo.PlatformObservabilitySettings{RetentionDays: 1}}
	platform := service.NewPlatformService(repo, platformClusterRepository{}, nil)
	h := handler.New(service.Services{Platform: platform})

	result, err := h.UpdatePlatformSettings(context.Background(), &dto.PlatformSettingsInput{Enabled: true, RetentionDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	response, ok := result.(*dto.PlatformBadRequestHTTPError)
	if !ok {
		t.Fatalf("response type = %T", result)
	}
	if response.Code != 40000 {
		t.Fatalf("response = %+v", response)
	}
}
