package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
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

func (s *ExceptionService) Get(ctx context.Context, name string) (*dto.ExceptionView, error) {
	e, err := s.repos.Exceptions.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	spec := e.Spec.Data()
	st := dto.ExceptionStatusView{Phase: "Active"}
	if e.ExpiresAt != nil {
		st.ExpiresAt = e.ExpiresAt
		st.EffectiveAt = e.CreatedAt
		if e.ExpiresAt.Before(time.Now()) {
			st.Phase = "Expired"
		}
	}
	scope, err := exceptionScopeFor(ctx, s.repos, name)
	if err != nil {
		return nil, err
	}
	return &dto.ExceptionView{Name: e.Name, ResourceVersion: strconv.FormatInt(e.UpdatedAt.UnixNano(), 10), Spec: spec, Status: st, Scope: scope}, nil
}

func (s *ExceptionService) List(ctx context.Context, page, size int, sortBy, order string) (*dto.ExceptionPageView, error) {
	xs, err := s.repos.Exceptions.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]dto.ExceptionItem, 0, len(xs))
	for _, e := range xs {
		sp := e.Spec.Data()
		phase := "Active"
		if e.ExpiresAt != nil && e.ExpiresAt.Before(time.Now()) {
			phase = "Expired"
		}
		items = append(items, dto.ExceptionItem{Name: e.Name, Phase: phase, Reason: sp.Reason, Duration: sp.Duration, TargetSummary: exceptionTarget(sp), ExpiresAt: e.ExpiresAt})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		comparison := compareExceptionListItem(a, b, sortBy)
		if comparison == 0 {
			return a.Name < b.Name
		}
		if order == "desc" {
			return comparison > 0
		}
		return comparison < 0
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
	return &dto.ExceptionPageView{Total: total, Page: page, PageSize: size, Items: items[start:end]}, nil
}

func compareExceptionListItem(a, b dto.ExceptionItem, sortBy string) int {
	switch sortBy {
	case "phase":
		return cmp.Compare(a.Phase, b.Phase)
	case "targetSummary":
		return cmp.Compare(a.TargetSummary, b.TargetSummary)
	case "duration":
		aDuration, aErr := time.ParseDuration(a.Duration)
		bDuration, bErr := time.ParseDuration(b.Duration)
		if aErr == nil && bErr == nil {
			return cmp.Compare(aDuration, bDuration)
		}
		return cmp.Compare(a.Duration, b.Duration)
	case "expiresAt":
		aTime, bTime := a.ExpiresAt, b.ExpiresAt
		switch {
		case aTime == nil && bTime == nil:
			return 0
		case aTime == nil:
			return -1
		case bTime == nil:
			return 1
		default:
			return aTime.Compare(*bTime)
		}
	default:
		return cmp.Compare(a.Name, b.Name)
	}
}

func exceptionTarget(s v1alpha1.PolicyExceptionSpec) string {
	if s.AllPolicies {
		return "all policies"
	}
	if len(s.PolicyRefs) > 0 {
		return "policies: " + summarizeExceptionRefs(s.PolicyRefs)
	}
	if len(s.PolicyGroupRefs) > 0 {
		return "groups: " + summarizeExceptionRefs(s.PolicyGroupRefs)
	}
	return ""
}

func summarizeExceptionRefs(refs []string) string {
	const limit = 3
	if len(refs) <= limit {
		return strings.Join(refs, ", ")
	}
	return strings.Join(refs[:limit], ", ") + fmt.Sprintf(" (+%d more)", len(refs)-limit)
}

func (s *ExceptionService) Create(ctx context.Context, r *dto.ExceptionInput) error {
	if err := validateExceptionRequest(r, true); err != nil {
		return err
	}
	if err := validateExceptionScopeTargets(ctx, s.repos.Clusters, boolOr(r.AllClusters, false), r.ClusterNames); err != nil {
		return err
	}
	name := stringOr(r.Name, "")
	spec := exceptionSpec(r)
	exp := time.Now().Add(mustDuration(r.Duration))
	e := &dbrepo.PolicyException{Name: name, Spec: datatypes.NewJSONType(spec), ExpiresAt: &exp}
	if s.repos.ExceptionUOW == nil {
		return errors.New("exception unit of work is unavailable")
	}
	if err := s.repos.ExceptionUOW.WithinTransaction(ctx, func(exceptions repository.ExceptionRepository, distributions repository.DistributionRepository) error {
		if err := exceptions.Create(ctx, e); err != nil {
			if dbrepo.IsUniqueViolation(err) {
				return newConflict("exception %s already exists", name)
			}
			return err
		}
		return syncExceptionScope(ctx, distributions, name, boolOr(r.AllClusters, false), r.ClusterNames)
	}); err != nil {
		return err
	}
	s.notify(ctx)
	return nil
}
func mustDuration(v string) time.Duration { d, _ := time.ParseDuration(v); return d }
func (s *ExceptionService) Update(ctx context.Context, name string, r *dto.ExceptionInput) error {
	if err := validateExceptionRequest(r, false); err != nil {
		return err
	}
	if err := validateExceptionScopeTargets(ctx, s.repos.Clusters, boolOr(r.AllClusters, false), r.ClusterNames); err != nil {
		return err
	}
	spec := exceptionSpec(r)
	exp := time.Now().Add(mustDuration(r.Duration))
	if s.repos.ExceptionUOW == nil {
		return errors.New("exception unit of work is unavailable")
	}
	if err := s.repos.ExceptionUOW.WithinTransaction(ctx, func(exceptions repository.ExceptionRepository, distributions repository.DistributionRepository) error {
		e, err := exceptions.GetByName(ctx, name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if rv := stringOr(r.ResourceVersion, ""); rv != "" && rv != strconv.FormatInt(e.UpdatedAt.UnixNano(), 10) {
			return newConflict("exception has been modified, refresh and retry")
		}
		if err := exceptions.UpdateCAS(ctx, e.ID, e.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(spec), "expires_at": &exp}); err != nil {
			if errors.Is(err, dbrepo.ErrConflict) {
				return newConflict("exception has been modified, refresh and retry")
			}
			return err
		}
		return syncExceptionScope(ctx, distributions, name, boolOr(r.AllClusters, false), r.ClusterNames)
	}); err != nil {
		return err
	}
	s.notify(ctx)
	return nil
}

func exceptionSpec(r *dto.ExceptionInput) v1alpha1.PolicyExceptionSpec {
	return v1alpha1.PolicyExceptionSpec{
		PolicyRefs:      r.PolicyRefs,
		PolicyGroupRefs: r.PolicyGroupRefs,
		AllPolicies:     boolOr(r.AllPolicies, false),
		Match: v1alpha1.PolicyExceptionMatch{
			Namespaces:        r.Match.Namespaces,
			NamespaceSelector: labelSelectorModel(r.Match.NamespaceSelector),
			ResourceSelector:  labelSelectorModel(r.Match.ResourceSelector),
		},
		Duration:          r.Duration,
		RetainAfterExpiry: stringOr(r.RetainAfterExpiry, ""),
		Reason:            r.Reason,
	}
}

func (s *ExceptionService) Delete(ctx context.Context, name string) error {
	if s.repos.ExceptionUOW == nil {
		return errors.New("exception unit of work is unavailable")
	}
	if err := s.repos.ExceptionUOW.WithinTransaction(ctx, func(exceptions repository.ExceptionRepository, distributions repository.DistributionRepository) error {
		e, err := exceptions.GetByName(ctx, name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := exceptions.Delete(ctx, e.ID); err != nil {
			return err
		}
		return deleteExceptionScope(ctx, distributions, name)
	}); err != nil {
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

func validateExceptionRequest(req *dto.ExceptionInput, create bool) error {
	if create {
		name := stringOr(req.Name, "")
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
	if boolOr(req.AllPolicies, false) {
		targets++
	}
	if targets != 1 {
		return &ValidationError{Msg: "exactly one of policyRefs, policyGroupRefs, allPolicies must be set"}
	}
	duration, err := time.ParseDuration(req.Duration)
	if err != nil || duration <= 0 {
		return &ValidationError{Msg: fmt.Sprintf("duration must be a positive Go duration (e.g. 24h): %q", req.Duration)}
	}
	if retain := stringOr(req.RetainAfterExpiry, ""); retain != "" {
		retention, err := time.ParseDuration(retain)
		if err != nil || retention < 0 {
			return &ValidationError{Msg: fmt.Sprintf("retainAfterExpiry must be a non-negative Go duration: %q", retain)}
		}
	}
	if strings.TrimSpace(req.Reason) == "" {
		return &ValidationError{Msg: "reason is required"}
	}
	all := boolOr(req.AllClusters, false)
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

func exceptionScopeFor(ctx context.Context, repos repository.Repositories, owner string) (dto.ExceptionScopeView, error) {
	distributions, err := repos.Distributions.List(ctx)
	if err != nil {
		return dto.ExceptionScopeView{}, err
	}
	for _, distribution := range distributions {
		if distribution.Managed && distribution.OwnerType == "PolicyException" && distribution.OwnerName == owner {
			spec := distribution.Spec.Data()
			names := append([]string{}, spec.Clusters.ByName...)
			return dto.ExceptionScopeView{AllClusters: spec.Clusters.ClusterSelector != nil, ClusterNames: names}, nil
		}
	}
	return dto.ExceptionScopeView{AllClusters: true, ClusterNames: []string{}}, nil
}

func validateExceptionScopeTargets(ctx context.Context, clusters repository.ClusterRepository, all bool, names []string) error {
	if !all {
		for _, name := range names {
			cluster, err := clusters.GetByName(ctx, name)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return &ValidationError{Msg: fmt.Sprintf("target cluster %q does not exist", name)}
				}
				return fmt.Errorf("target cluster %q: %w", name, err)
			}
			if cluster.Phase == gatewaymodel.ClusterPhaseUninstalled {
				return &ValidationError{Msg: fmt.Sprintf("target cluster %q is uninstalled", name)}
			}
		}
	}
	return nil
}

func syncExceptionScope(ctx context.Context, distributions repository.DistributionRepository, owner string, all bool, names []string) error {
	spec := gatewaymodel.PolicyDistributionSpec{PolicyExceptions: []string{owner}}
	if all {
		spec.Clusters.ClusterSelector = &metav1.LabelSelector{}
	} else {
		spec.Clusters.ByName = append([]string(nil), names...)
	}
	rows, err := distributions.List(ctx)
	if err != nil {
		return err
	}
	for i := range rows {
		distribution := &rows[i]
		if distribution.Managed && distribution.OwnerType == "PolicyException" && distribution.OwnerName == owner {
			distribution.Spec = datatypes.NewJSONType(spec)
			return distributions.Update(ctx, distribution)
		}
	}
	return distributions.Create(ctx, &dbrepo.PolicyDistribution{
		Name: "scope-exception-" + strings.ToLower(owner), Managed: true,
		OwnerType: "PolicyException", OwnerName: owner, Spec: datatypes.NewJSONType(spec),
	})
}

func deleteExceptionScope(ctx context.Context, distributions repository.DistributionRepository, owner string) error {
	rows, err := distributions.List(ctx)
	if err != nil {
		return err
	}
	for _, distribution := range rows {
		if distribution.Managed && distribution.OwnerType == "PolicyException" && distribution.OwnerName == owner {
			return distributions.Delete(ctx, distribution.ID)
		}
	}
	return nil
}
