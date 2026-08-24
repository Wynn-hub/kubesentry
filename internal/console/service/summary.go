package service

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SummaryService struct{ repos repository.Repositories }

func NewSummaryService(r repository.Repositories) *SummaryService { return &SummaryService{repos: r} }
func (s *SummaryService) Get(ctx context.Context) (*dto.SummaryData, error) {
	ps, e := s.repos.Policies.List(ctx)
	if e != nil {
		return nil, e
	}
	gs, e := s.repos.Groups.List(ctx)
	if e != nil {
		return nil, e
	}
	xs, e := s.repos.Exceptions.List(ctx)
	if e != nil {
		return nil, e
	}
	cc, e := s.repos.Clusters.CountByPhase(ctx)
	if e != nil {
		return nil, e
	}
	pol := make([]v1alpha1.Policy, len(ps))
	for i := range ps {
		pol[i] = v1alpha1.Policy{ObjectMeta: metav1.ObjectMeta{Name: ps[i].Name, Labels: ps[i].Labels.Data()}, Spec: v1alpha1.PolicySpec{EnforcementMode: ps[i].EnforcementMode}}
	}
	groups := make([]dto.SummaryGroup, 0, len(gs))
	for _, g := range gs {
		sp := g.Spec.Data()
		m, _ := groupresolve.Resolve(sp, pol)
		x := dto.SummaryGroup{Name: g.Name, DisplayName: sp.DisplayName, Enabled: sp.Enabled, ResolvedCount: len(m)}
		for _, v := range m {
			if v.EnforcementMode == "enforce" {
				x.EnforceCount++
			} else if v.EnforcementMode == "audit" {
				x.AuditCount++
			}
		}
		groups = append(groups, x)
	}
	return &dto.SummaryData{Totals: dto.SummaryDataTotals{"clusters": int(cc.Total), "policies": len(ps), "policygroups": len(gs), "exceptions": len(xs)}, ClusterPhases: dto.SummaryDataClusterPhases{Online: int(cc.Online), Offline: int(cc.Offline), Registering: int(cc.Registering)}, Groups: groups}, nil
}

var _ = dbrepo.ClusterPhaseCounts{}
