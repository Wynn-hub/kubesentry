package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const defaultGroupName = "default"

var ErrDefaultPolicyGroupScope = errors.New("default policy group must target all clusters")

type PolicyGroupService struct {
	repos    repository.Repositories
	notifier Notifier
}

func NewPolicyGroupService(r repository.Repositories, n Notifier) *PolicyGroupService {
	return &PolicyGroupService{repos: r, notifier: n}
}

func policyList(in []dbrepo.Policy) []v1alpha1.Policy {
	out := make([]v1alpha1.Policy, 0, len(in))
	for _, p := range in {
		out = append(out, v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{Name: p.Name, Labels: p.Labels.Data(), Annotations: p.Annotations.Data()},
			Spec: v1alpha1.PolicySpec{
				Description:     p.Description,
				Rego:            p.Rego,
				EnforcementMode: p.EnforcementMode,
				Match:           p.Match.Data(),
			},
		})
	}
	return out
}

func groupSpec(req *dto.PolicyGroupInput) v1alpha1.PolicyGroupSpec {
	byName := make([]v1alpha1.PolicyRef, len(req.Policies.ByName))
	for i, ref := range req.Policies.ByName {
		byName[i] = v1alpha1.PolicyRef{Name: ref.Name, EnforcementMode: stringOr(ref.EnforcementMode, "")}
	}
	return v1alpha1.PolicyGroupSpec{DisplayName: stringOr(req.DisplayName, ""), Description: stringOr(req.Description, ""), Enabled: req.Enabled, NamespaceSelector: labelSelectorModel(req.NamespaceSelector), Policies: v1alpha1.PolicyGroupPolicies{ByName: byName, BySelector: labelSelectorModel(req.Policies.BySelector), Exclude: req.Policies.Exclude}, SelectorEnforcementMode: stringOr(req.SelectorEnforcementMode, "")}
}

func scopeFor(ctx context.Context, r repository.Repositories, owner string) (dto.PolicyGroupScopeView, error) {
	ds, e := r.Distributions.List(ctx)
	if e != nil {
		return dto.PolicyGroupScopeView{}, e
	}
	for _, d := range ds {
		if d.Managed && d.OwnerType == "PolicyGroup" && d.OwnerName == owner {
			s := d.Spec.Data()
			return dto.PolicyGroupScopeView{AllClusters: s.Clusters.ClusterSelector != nil, ClusterNames: append([]string{}, s.Clusters.ByName...)}, nil
		}
	}
	return dto.PolicyGroupScopeView{AllClusters: true, ClusterNames: []string{}}, nil
}

func syncScope(ctx context.Context, r repository.Repositories, owner string, all bool, names []string) error {
	if !all {
		if len(names) == 0 {
			return &ValidationError{Msg: "clusterNames is required when allClusters is false"}
		}
		for _, n := range names {
			c, e := r.Clusters.GetByName(ctx, n)
			if e != nil {
				return e
			}
			if c.Phase == gatewaymodel.ClusterPhaseUninstalled {
				return fmt.Errorf("target cluster %q is uninstalled", n)
			}
		}
	}
	ds, e := r.Distributions.List(ctx)
	if e != nil {
		return e
	}
	spec := gatewaymodel.PolicyDistributionSpec{}
	if all {
		spec.Clusters.ClusterSelector = &metav1.LabelSelector{}
	} else {
		spec.Clusters.ByName = append([]string(nil), names...)
	}
	spec.PolicyGroups = []string{owner}
	for _, d := range ds {
		if d.Managed && d.OwnerType == "PolicyGroup" && d.OwnerName == owner {
			d.Spec = datatypes.NewJSONType(spec)
			return r.Distributions.Update(ctx, &d)
		}
	}
	return r.Distributions.Create(ctx, &dbrepo.PolicyDistribution{Name: "scope-group-" + strings.ToLower(owner), Managed: true, OwnerType: "PolicyGroup", OwnerName: owner, Spec: datatypes.NewJSONType(spec)})
}

func deleteScope(ctx context.Context, r repository.Repositories, owner string) error {
	ds, e := r.Distributions.List(ctx)
	if e != nil {
		return e
	}
	for _, d := range ds {
		if d.Managed && d.OwnerType == "PolicyGroup" && d.OwnerName == owner {
			return r.Distributions.Delete(ctx, d.ID)
		}
	}
	return nil
}

func detail(g *dbrepo.PolicyGroup, ps []v1alpha1.Policy, scope dto.PolicyGroupScopeView) dto.PolicyGroupView {
	s := g.Spec.Data()
	m, c := groupresolve.Resolve(s, ps)
	phase := "Ready"
	if len(c) > 0 {
		phase = "Invalid"
	}
	source := "custom"
	if g.Labels.Data()["kubesentry.io/builtin"] == "true" {
		source = "builtin"
	}
	members := make([]dto.EffectiveMemberView, 0, len(m))
	for _, x := range m {
		members = append(members, dto.EffectiveMemberView{Name: x.Name, EnforcementMode: x.EnforcementMode, Source: x.Source})
	}
	return dto.PolicyGroupView{Name: g.Name, Source: source, ResourceVersion: strconv.FormatInt(g.UpdatedAt.UnixNano(), 10), Spec: normalizeGroupSpec(s), Status: dto.PolicyGroupStatusView{Phase: phase, ResolvedCount: len(m), ResolvedPolicies: members}, Scope: scope}
}

func normalizeGroupSpec(spec v1alpha1.PolicyGroupSpec) v1alpha1.PolicyGroupSpec {
	spec.Policies.ByName = append([]v1alpha1.PolicyRef{}, spec.Policies.ByName...)
	spec.Policies.Exclude = append([]string{}, spec.Policies.Exclude...)
	return spec
}

func (s *PolicyGroupService) Get(ctx context.Context, name string) (*dto.PolicyGroupView, error) {
	g, e := s.repos.PolicyGroups.GetByName(ctx, name)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, e
	}
	ps, e := s.repos.Policies.List(ctx)
	if e != nil {
		return nil, e
	}
	sc, e := scopeFor(ctx, s.repos, name)
	if e != nil {
		return nil, e
	}
	d := detail(g, policyList(ps), sc)
	return &d, nil
}

func (s *PolicyGroupService) List(ctx context.Context, page, size int, sortBy, order string) (*dto.PolicyGroupPageView, error) {
	gs, e := s.repos.PolicyGroups.List(ctx)
	if e != nil {
		return nil, e
	}
	ps, e := s.repos.Policies.List(ctx)
	if e != nil {
		return nil, e
	}
	pol := policyList(ps)
	items := make([]dto.PolicyGroupItem, 0, len(gs))
	for _, g := range gs {
		d := detail(&g, pol, dto.PolicyGroupScopeView{})
		items = append(items, dto.PolicyGroupItem{Name: g.Name, Source: d.Source, DisplayName: d.Spec.DisplayName, Enabled: d.Spec.Enabled, Phase: d.Status.Phase, ResolvedCount: d.Status.ResolvedCount})
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		var less bool
		switch sortBy {
		case "displayName":
			less = a.DisplayName < b.DisplayName
		case "enabled":
			less = !a.Enabled && b.Enabled
		case "phase":
			less = a.Phase < b.Phase
		case "resolvedCount":
			less = a.ResolvedCount < b.ResolvedCount
		default:
			less = a.Name < b.Name
		}
		if order == "desc" {
			return !less && a.Name != b.Name
		}
		return less
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
	return &dto.PolicyGroupPageView{Total: total, Page: page, PageSize: size, Items: items[start:end]}, nil
}

func (s *PolicyGroupService) Create(ctx context.Context, req *dto.PolicyGroupInput) error {
	name := stringOr(req.Name, "")
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return &ValidationError{Msg: "invalid name: " + strings.Join(errs, "; ")}
	}
	if name == defaultGroupName {
		return &ValidationError{Msg: "default policy group cannot be created"}
	}
	for _, name := range req.Policies.Exclude {
		if strings.TrimSpace(name) == "" {
			return &ValidationError{Msg: "excluded policy name must not be empty"}
		}
	}
	spec := groupSpec(req)
	if e := validatePolicyGroupScope(boolOr(req.AllClusters, false), req.ClusterNames); e != nil {
		return e
	}
	g := &dbrepo.PolicyGroup{Name: name, Spec: datatypes.NewJSONType(spec), Labels: datatypes.NewJSONType(map[string]string{}), Annotations: datatypes.NewJSONType(map[string]string{})}
	if e := s.repos.PolicyGroups.Create(ctx, g); e != nil {
		if dbrepo.IsUniqueViolation(e) {
			return newConflict("policygroup %s already exists", name)
		}
		return e
	}
	if e := syncScope(ctx, s.repos, name, boolOr(req.AllClusters, false), req.ClusterNames); e != nil {
		return e
	}
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "group")
	}
	return nil
}

func (s *PolicyGroupService) Update(ctx context.Context, name string, req *dto.PolicyGroupInput) error {
	for _, excluded := range req.Policies.Exclude {
		if strings.TrimSpace(excluded) == "" {
			return &ValidationError{Msg: "excluded policy name must not be empty"}
		}
	}
	allClusters := boolOr(req.AllClusters, false)
	if name == defaultGroupName {
		if (req.AllClusters != nil && !*req.AllClusters) || len(req.ClusterNames) > 0 {
			return ErrDefaultPolicyGroupScope
		}
		allClusters = true
	} else if e := validatePolicyGroupScope(allClusters, req.ClusterNames); e != nil {
		return e
	}
	g, e := s.repos.PolicyGroups.GetByName(ctx, name)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return e
	}
	if rv := stringOr(req.ResourceVersion, ""); rv != "" && rv != strconv.FormatInt(g.UpdatedAt.UnixNano(), 10) {
		return newConflict("policygroup has been modified, refresh and retry")
	}
	spec := groupSpec(req)
	if name == defaultGroupName {
		spec.DisplayName = g.Spec.Data().DisplayName
	}
	if e = s.repos.PolicyGroups.UpdateCAS(ctx, g.ID, g.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(spec)}); e != nil {
		return e
	}
	if e = syncScope(ctx, s.repos, name, allClusters, req.ClusterNames); e != nil {
		return e
	}
	s.notify(ctx)
	return nil
}

func (s *PolicyGroupService) SetEnabled(ctx context.Context, name string, enabled bool) error {
	g, e := s.repos.PolicyGroups.GetByName(ctx, name)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return e
	}
	spec := g.Spec.Data()
	if spec.Enabled == enabled {
		return nil
	}
	spec.Enabled = enabled
	if e = s.repos.PolicyGroups.UpdateCAS(ctx, g.ID, g.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(spec)}); e != nil {
		return e
	}
	s.notify(ctx)
	return nil
}

func (s *PolicyGroupService) Delete(ctx context.Context, name string) error {
	if name == defaultGroupName {
		return fmt.Errorf("default policy group cannot be deleted")
	}
	g, e := s.repos.PolicyGroups.GetByName(ctx, name)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return e
	}
	if e = s.repos.PolicyGroups.Delete(ctx, g.ID); e != nil {
		return e
	}
	if e = deleteScope(ctx, s.repos, name); e != nil {
		return e
	}
	s.notify(ctx)
	return nil
}

func validatePolicyGroupScope(all bool, names []string) error {
	if all && len(names) > 0 {
		return &ValidationError{Msg: "clusterNames must be empty when allClusters is true"}
	}
	if !all && len(names) == 0 {
		return &ValidationError{Msg: "clusterNames is required when allClusters is false"}
	}
	return nil
}

func (s *PolicyGroupService) notify(ctx context.Context) {
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "group")
	}
}
