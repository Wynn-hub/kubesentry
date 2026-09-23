package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/database/model"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type admissionRepositoryStub struct {
	events        []dbrepo.AdmissionEvent
	total         int64
	listOptions   dbrepo.AdmissionEventPageOptions
	getEvent      *dbrepo.AdmissionEvent
	getEventErr   error
	settings      dbrepo.AdmissionEventSettings
	policies      []dbrepo.AdmissionStoragePolicy
	updateCalled  bool
	updatedPolicy []dbrepo.AdmissionStoragePolicy
}

func (*admissionRepositoryStub) EventDatabaseIDs(context.Context, []string) (map[string]int64, error) {
	return nil, nil
}

func (r *admissionRepositoryStub) GetSettings(context.Context) (*dbrepo.AdmissionEventSettings, error) {
	copy := r.settings
	return &copy, nil
}

func (r *admissionRepositoryStub) ListPolicies(context.Context) ([]dbrepo.AdmissionStoragePolicy, error) {
	return append([]dbrepo.AdmissionStoragePolicy(nil), r.policies...), nil
}

func (r *admissionRepositoryStub) UpdateSettingsAndPolicies(_ context.Context, settings *dbrepo.AdmissionEventSettings, policies []dbrepo.AdmissionStoragePolicy) error {
	r.updateCalled = true
	r.settings = *settings
	r.settings.StoragePolicyVersion++
	r.policies = append([]dbrepo.AdmissionStoragePolicy(nil), policies...)
	r.updatedPolicy = append([]dbrepo.AdmissionStoragePolicy(nil), policies...)
	return nil
}

func (r *admissionRepositoryStub) ListEventsPage(_ context.Context, options dbrepo.AdmissionEventPageOptions) ([]dbrepo.AdmissionEvent, int64, error) {
	r.listOptions = options
	return append([]dbrepo.AdmissionEvent(nil), r.events...), r.total, nil
}

func (r *admissionRepositoryStub) GetEvent(context.Context, int64) (*dbrepo.AdmissionEvent, error) {
	return r.getEvent, r.getEventErr
}

func (*admissionRepositoryStub) Trend(context.Context, time.Time, time.Time, bool) ([]dbrepo.AdmissionEventTrendBucket, error) {
	return nil, nil
}

func TestAdmissionListOwnsPaginationMappingAndPolicyFallback(t *testing.T) {
	repo := &admissionRepositoryStub{
		total: 1,
		events: []dbrepo.AdmissionEvent{{
			Base:       model.Base{ID: 2081582208838783001},
			Violations: datatypes.JSON(`["[host-ipc-set via default] host IPC is not allowed"]`),
		}},
	}
	svc := NewAdmissionService(repo, nil)
	params := dto.AdmissionEventListOptions{
		Page:     2,
		PageSize: 10,
		Verdicts: []string{"deny"},
	}
	data, err := svc.List(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	if repo.listOptions.Limit != 10 || repo.listOptions.Offset != 10 || !reflect.DeepEqual(repo.listOptions.Verdicts, []string{"deny"}) {
		t.Fatalf("repository options = %+v", repo.listOptions)
	}
	if len(data.Items) != 1 || data.Items[0].ID != "2081582208838783001" || !reflect.DeepEqual(data.Items[0].PolicyNames, []string{"host-ipc-set"}) {
		t.Fatalf("mapped items = %+v", data.Items)
	}
}

func TestAdmissionGetNormalizesRepositoryNotFound(t *testing.T) {
	svc := NewAdmissionService(&admissionRepositoryStub{getEventErr: gorm.ErrRecordNotFound}, nil)
	_, err := svc.Get(t.Context(), "42")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestAdmissionUpdateSettingsRejectsVersionedGVRBeforeWrite(t *testing.T) {
	repo := &admissionRepositoryStub{}
	svc := NewAdmissionService(repo, nil)
	retryEnabled, maxRetryAttempts, queueCapacity := true, 3, 1024
	incidentRetentionDays, observationEnabled := 7, false
	_, err := svc.UpdateSettings(t.Context(), &dto.AdmissionSettingsInput{
		DefaultRetentionDays:  14,
		RetryEnabled:          &retryEnabled,
		MaxRetryAttempts:      &maxRetryAttempts,
		QueueCapacity:         &queueCapacity,
		IncidentRetentionDays: &incidentRetentionDays,
		ObservationEnabled:    &observationEnabled,
		StoragePolicies: []dto.AdmissionStoragePolicyView{{
			MatchMode:  "include",
			Verdicts:   []string{"deny"},
			Clusters:   []string{},
			GVRs:       []string{"apps/v1/deployments"},
			Namespaces: []string{},
		}},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || repo.updateCalled {
		t.Fatalf("error=%v updateCalled=%v, want validation before write", err, repo.updateCalled)
	}
}
