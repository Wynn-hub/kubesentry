package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
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
	scope, err := exceptionScopeFor(ctx, s.repos, name)
	if err != nil {
		return nil, err
	}
	return &dto.ExceptionDetail{Name: e.Name, ResourceVersion: strconv.FormatInt(e.UpdatedAt.UnixNano(), 10), Spec: spec, Status: st, Scope: scope}, nil
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
	if err := validateExceptionRequest(r, true); err != nil {
		return err
	}
	name := r.Name.Or("")
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
	if err := syncExceptionScope(ctx, s.repos, name, r.AllClusters.Or(false), r.ClusterNames); err != nil {
		return err
	}
	s.notify(ctx)
	return nil
}
func mustDuration(v string) time.Duration { d, _ := time.ParseDuration(v); return d }
func (s *ExceptionService) Update(ctx context.Context, name string, r *dto.ExceptionRequest) error {
	if err := validateExceptionRequest(r, false); err != nil {
		return err
	}
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
	if err := s.repos.Exceptions.UpdateCAS(ctx, e.ID, e.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(spec), "expires_at": &exp}); err != nil {
		return err
	}
	if err := syncExceptionScope(ctx, s.repos, name, r.AllClusters.Or(false), r.ClusterNames); err != nil {
		return err
	}
	s.notify(ctx)
	return nil
}

func (s *ExceptionService) Delete(ctx context.Context, name string) error {
	e, err := s.repos.Exceptions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.repos.Exceptions.Delete(ctx, e.ID); err != nil {
		return err
	}
	if err := deleteExceptionScope(ctx, s.repos, name); err != nil {
		return err
	}
	s.notify(ctx)
	return nil
}

func (s *ExceptionService) notify(ctx context.Context) {
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "exception")
	}
}

func validateExceptionRequest(req *dto.ExceptionRequest, create bool) error {
	if create {
		name := req.Name.Or("")
		if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
			return &ValidationError{Msg: "invalid name: " + strings.Join(errs, "; ")}
		}
	}
	targets := 0
	if len(req.PolicyRefs) > 0 {
		targets++
	}
	if len(req.PolicyGroupRefs) > 0 {
		targets++
	}
	if req.AllPolicies.Or(false) {
		targets++
	}
	if targets != 1 {
		return &ValidationError{Msg: "exactly one of policyRefs, policyGroupRefs, allPolicies must be set"}
	}
	duration, err := time.ParseDuration(req.Duration)
	if err != nil || duration <= 0 {
		return &ValidationError{Msg: fmt.Sprintf("duration must be a positive Go duration (e.g. 24h): %q", req.Duration)}
	}
	if retain := req.RetainAfterExpiry.Or(""); retain != "" {
		retention, err := time.ParseDuration(retain)
		if err != nil || retention < 0 {
			return &ValidationError{Msg: fmt.Sprintf("retainAfterExpiry must be a non-negative Go duration: %q", retain)}
		}
	}
	if strings.TrimSpace(req.Reason) == "" {
		return &ValidationError{Msg: "reason is required"}
	}
	all := req.AllClusters.Or(false)
	if all == (len(req.ClusterNames) > 0) {
		return &ValidationError{Msg: "exactly one of allClusters or clusterNames must be set"}
	}
	for _, name := range req.ClusterNames {
		if strings.TrimSpace(name) == "" {
			return &ValidationError{Msg: "clusterNames entries must not be empty"}
		}
	}
	return nil
}

func exceptionScopeFor(ctx context.Context, repos repository.Repositories, owner string) (dto.ExceptionScope, error) {
	distributions, err := repos.Distributions.List(ctx)
	if err != nil {
		return dto.ExceptionScope{}, err
	}
	for _, distribution := range distributions {
		if distribution.Managed && distribution.OwnerType == "PolicyException" && distribution.OwnerName == owner {
			spec := distribution.Spec.Data()
			return dto.ExceptionScope{AllClusters: spec.Clusters.ClusterSelector != nil, ClusterNames: spec.Clusters.ByName}, nil
		}
	}
	return dto.ExceptionScope{AllClusters: true}, nil
}

func syncExceptionScope(ctx context.Context, repos repository.Repositories, owner string, all bool, names []string) error {
	if !all {
		for _, name := range names {
			cluster, err := repos.Clusters.GetByName(ctx, name)
			if err != nil {
				return fmt.Errorf("target cluster %q: %w", name, err)
			}
			if cluster.Phase == v1alpha1.ClusterPhaseUninstalled {
				return fmt.Errorf("target cluster %q is uninstalled", name)
			}
		}
	}
	spec := v1alpha1.PolicyDistributionSpec{PolicyExceptions: []string{owner}}
	if all {
		spec.Clusters.ClusterSelector = &metav1.LabelSelector{}
	} else {
		spec.Clusters.ByName = append([]string(nil), names...)
	}
	distributions, err := repos.Distributions.List(ctx)
	if err != nil {
		return err
	}
	for i := range distributions {
		distribution := &distributions[i]
		if distribution.Managed && distribution.OwnerType == "PolicyException" && distribution.OwnerName == owner {
			distribution.Spec = datatypes.NewJSONType(spec)
			return repos.Distributions.Update(ctx, distribution)
		}
	}
	return repos.Distributions.Create(ctx, &dbrepo.PolicyDistribution{
		Name: "scope-exception-" + strings.ToLower(owner), Managed: true,
		OwnerType: "PolicyException", OwnerName: owner, Spec: datatypes.NewJSONType(spec),
	})
}

func deleteExceptionScope(ctx context.Context, repos repository.Repositories, owner string) error {
	distributions, err := repos.Distributions.List(ctx)
	if err != nil {
		return err
	}
	for _, distribution := range distributions {
		if distribution.Managed && distribution.OwnerType == "PolicyException" && distribution.OwnerName == owner {
			return repos.Distributions.Delete(ctx, distribution.ID)
		}
	}
	return nil
}
