package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultGroupName = "default"

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
		var x v1alpha1.Policy
		b, _ := json.Marshal(p)
		if json.Unmarshal(b, &x) == nil {
			out = append(out, x)
		}
	}
	return out
}

func groupSpec(req *dto.PolicyGroupRequest) (v1alpha1.PolicyGroupSpec, error) {
	b, e := json.Marshal(req)
	if e != nil {
		return v1alpha1.PolicyGroupSpec{}, e
	}
	var s v1alpha1.PolicyGroupSpec
	e = json.Unmarshal(b, &s)
	return s, e
}

func scopeFor(ctx context.Context, r repository.Repositories, owner string) (dto.GroupScope, error) {
	ds, e := r.Distributions.List(ctx)
	if e != nil {
		return dto.GroupScope{}, e
	}
	for _, d := range ds {
		if d.Managed && d.OwnerType == "PolicyGroup" && d.OwnerName == owner {
			s := d.Spec.Data()
			return dto.GroupScope{AllClusters: s.Clusters.ClusterSelector != nil, ClusterNames: s.Clusters.ByName}, nil
		}
	}
	return dto.GroupScope{AllClusters: true}, nil
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
			if c.Phase == v1alpha1.ClusterPhaseUninstalled {
				return fmt.Errorf("target cluster %q is uninstalled", n)
			}
		}
	}
	ds, e := r.Distributions.List(ctx)
	if e != nil {
		return e
	}
	spec := v1alpha1.PolicyDistributionSpec{}
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

func detail(g *dbrepo.PolicyGroup, ps []v1alpha1.Policy, scope dto.GroupScope) dto.PolicyGroupDetail {
	s := g.Spec.Data()
	m, c := groupresolve.Resolve(s, ps)
	phase := dto.PolicyGroupStatusPhaseReady
	if len(c) > 0 {
		phase = dto.PolicyGroupStatusPhaseInvalid
	}
	source := dto.PolicyGroupDetailSourceCustom
	if g.Labels.Data()["kubesentry.io/builtin"] == "true" {
		source = dto.PolicyGroupDetailSourceBuiltin
	}
	var spec dto.PolicyGroupSpec
	b, _ := json.Marshal(s)
	_ = json.Unmarshal(b, &spec)
	members := make([]dto.EffectiveMember, 0, len(m))
	for _, x := range m {
		members = append(members, dto.EffectiveMember{Name: x.Name, EnforcementMode: dto.EffectiveMemberEnforcementMode(x.EnforcementMode), Source: dto.EffectiveMemberSource(x.Source)})
	}
	return dto.PolicyGroupDetail{Name: g.Name, Source: source, ResourceVersion: strconv.FormatInt(g.UpdatedAt.UnixNano(), 10), Spec: spec, Status: dto.PolicyGroupStatus{Phase: phase, ResolvedCount: len(m), ResolvedPolicies: members}, Scope: scope}
}

func (s *PolicyGroupService) Get(ctx context.Context, name string) (*dto.PolicyGroupDetail, error) {
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

func (s *PolicyGroupService) List(ctx context.Context, page, size int, sortBy, order string) (*dto.PolicyGroupListData, error) {
	gs, e := s.repos.PolicyGroups.List(ctx)
	if e != nil {
		return nil, e
	}
	ps, e := s.repos.Policies.List(ctx)
	if e != nil {
		return nil, e
	}
	pol := policyList(ps)
	items := make([]dto.PolicyGroupListItem, 0, len(gs))
	for _, g := range gs {
		d := detail(&g, pol, dto.GroupScope{})
		items = append(items, dto.PolicyGroupListItem{Name: g.Name, Source: dto.PolicyGroupListItemSource(d.Source), DisplayName: d.Spec.DisplayName, Enabled: d.Spec.Enabled, Phase: dto.PolicyGroupListItemPhase(d.Status.Phase), ResolvedCount: d.Status.ResolvedCount})
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
	return &dto.PolicyGroupListData{Total: total, Page: page, PageSize: dto.PolicyGroupListDataPageSize(size), Items: items[start:end]}, nil
}

func (s *PolicyGroupService) Create(ctx context.Context, req *dto.PolicyGroupRequest) error {
	name := req.Name.Or("")
	if name == "" {
		return &ValidationError{Msg: "name is required"}
	}
	if name == defaultGroupName {
		return &ValidationError{Msg: "default policy group cannot be created"}
	}
	spec, e := groupSpec(req)
	if e != nil {
		return e
	}
	if !req.AllClusters && len(req.ClusterNames) == 0 {
		return &ValidationError{Msg: "clusterNames is required when allClusters is false"}
	}
	g := &dbrepo.PolicyGroup{Name: name, Spec: datatypes.NewJSONType(spec), Labels: datatypes.NewJSONType(map[string]string{}), Annotations: datatypes.NewJSONType(map[string]string{})}
	if e = s.repos.PolicyGroups.Create(ctx, g); e != nil {
		if dbrepo.IsUniqueViolation(e) {
			return newConflict("policygroup %s already exists", name)
		}
		return e
	}
	if e = syncScope(ctx, s.repos, name, req.AllClusters, req.ClusterNames); e != nil {
		return e
	}
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "group")
	}
	return nil
}

func (s *PolicyGroupService) Update(ctx context.Context, name string, req *dto.PolicyGroupRequest) error {
	if name == defaultGroupName {
		return fmt.Errorf("default policy group scope is managed; use the enabled endpoint to disable it")
	}
	g, e := s.repos.PolicyGroups.GetByName(ctx, name)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return e
	}
	if rv, ok := req.ResourceVersion.Get(); ok && rv != "" && rv != strconv.FormatInt(g.UpdatedAt.UnixNano(), 10) {
		return newConflict("policygroup has been modified, refresh and retry")
	}
	spec, e := groupSpec(req)
	if e != nil {
		return e
	}
	if e = s.repos.PolicyGroups.UpdateCAS(ctx, g.ID, g.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(spec)}); e != nil {
		return e
	}
	return syncScope(ctx, s.repos, name, req.AllClusters, req.ClusterNames)
}

func (s *PolicyGroupService) SetEnabled(ctx context.Context, name string, enabled bool) error {
	g, e := s.repos.PolicyGroups.GetByName(ctx, name)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return e
	}
	if e = s.repos.PolicyGroups.UpdateCAS(ctx, g.ID, g.UpdatedAt, map[string]any{"spec": datatypes.NewJSONType(func() v1alpha1.PolicyGroupSpec { x := g.Spec.Data(); x.Enabled = enabled; return x }())}); e != nil {
		return e
	}
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
	return deleteScope(ctx, s.repos, name)
}
