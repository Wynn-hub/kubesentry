package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ExceptionService struct {
	repos    repository.Repositories
	notifier Notifier
}

func NewExceptionService(r repository.Repositories, n Notifier) *ExceptionService {
	return &ExceptionService{repos: r, notifier: n}
}

func (s *ExceptionService) Get(ctx context.Context, name string) (*dto.ExceptionDetail, error) {
	e, err := s.repos.Exceptions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var spec dto.ExceptionSpec
	b, _ := json.Marshal(e.Spec.Data())
	_ = json.Unmarshal(b, &spec)
	var st dto.ExceptionStatus
	st.Phase = dto.ExceptionStatusPhaseActive
	if e.ExpiresAt != nil {
		st.ExpiresAt.SetTo(*e.ExpiresAt)
		st.EffectiveAt = e.CreatedAt
		if e.ExpiresAt.Before(time.Now()) {
			st.Phase = dto.ExceptionStatusPhaseExpired
		}
	}
	return &dto.ExceptionDetail{Name: e.Name, ResourceVersion: strconv.FormatInt(e.UpdatedAt.UnixNano(), 10), Spec: spec, Status: st, Scope: dto.ExceptionScope{AllClusters: true}}, nil
}

func (s *ExceptionService) List(ctx context.Context, page, size int, sortBy, order string) (*dto.ExceptionListData, error) {
	xs, err := s.repos.Exceptions.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]dto.ExceptionListItem, 0, len(xs))
	for _, e := range xs {
		sp := e.Spec.Data()
		phase := dto.ExceptionListItemPhaseActive
		if e.ExpiresAt != nil && e.ExpiresAt.Before(time.Now()) {
			phase = dto.ExceptionListItemPhaseExpired
		}
		var expires dto.OptDateTime
		if e.ExpiresAt != nil {
			expires.SetTo(*e.ExpiresAt)
		}
		items = append(items, dto.ExceptionListItem{Name: e.Name, Phase: phase, Reason: sp.Reason, Duration: sp.Duration, TargetSummary: exceptionTarget(sp), ExpiresAt: expires})
	}
	sort.Slice(items, func(i, j int) bool {
		if order == "desc" {
			return items[i].Name > items[j].Name
		}
		return items[i].Name < items[j].Name
	})
	total := len(items)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return &dto.ExceptionListData{Total: total, Page: page, PageSize: dto.ExceptionListDataPageSize(size), Items: items[start:end]}, nil
}

func exceptionTarget(s v1alpha1.PolicyExceptionSpec) string {
	if s.AllPolicies {
		return "all policies"
	}
	if len(s.PolicyRefs) > 0 {
		return "policies: " + s.PolicyRefs[0]
	}
	if len(s.PolicyGroupRefs) > 0 {
		return "groups: " + s.PolicyGroupRefs[0]
	}
	return ""
}

func (s *ExceptionService) Create(ctx context.Context, r *dto.ExceptionRequest) error {
	name := r.Name.Or("")
	if name == "" {
		return &ValidationError{Msg: "name is required"}
	}
	if _, e := time.ParseDuration(r.Duration); e != nil {
		return &ValidationError{Msg: "invalid duration"}
	}
	var spec v1alpha1.PolicyExceptionSpec
	b, _ := json.Marshal(r)
	if e := json.Unmarshal(b, &spec); e != nil {
		return e
	}
	exp := time.Now().Add(mustDuration(r.Duration))
	e := &dbrepo.PolicyException{Name: name, Spec: datatypes.NewJSONType(spec), ExpiresAt: &exp}
	if err := s.repos.Exceptions.Create(ctx, e); err != nil {
		if dbrepo.IsUniqueViolation(err) {
			return newConflict("exception %s already exists", name)
		}
		return err
	}
	return nil
}
func mustDuration(v string) time.Duration { d, _ := time.ParseDuration(v); return d }
func (s *ExceptionService) Update(ctx context.Context, name string, r *dto.ExceptionRequest) error {
	e, err := s.repos.Exceptions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if rv, ok := r.ResourceVersion.Get(); ok && rv != "" && rv != strconv.FormatInt(e.UpdatedAt.UnixNano(), 10) {
		return newConflict("exception has been modified, refresh and retry")
	}
	var spec v1alpha1.PolicyExceptionSpec
	b, _ := json.Marshal(r)
	if err = json.Unmarshal(b, &spec); err != nil {
		return err
	}
	exp := time.Now().Add(mustDuration(r.Duration))
	return s.repos.Exceptions.UpdateCAS(ctx, e.ID, e.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(spec), "expires_at": &exp})
}

func (s *ExceptionService) Delete(ctx context.Context, name string) error {
	e, err := s.repos.Exceptions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.repos.Exceptions.Delete(ctx, e.ID)
}

var _ = fmt.Sprintf
