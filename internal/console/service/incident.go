package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type IncidentService struct {
	incidents repository.IncidentRepository
	admission repository.AdmissionRepository
}

func NewIncidentService(repos repository.Repositories) *IncidentService {
	return &IncidentService{incidents: repos.Incidents, admission: repos.Admission}
}

func (s *IncidentService) List(ctx context.Context, p dto.IncidentListOptions) (*dto.IncidentPageView, error) {
	if s.incidents == nil {
		return nil, ErrUnavailable
	}
	page, size := p.Page, p.PageSize
	components := make([]string, 0, len(p.Components))
	for _, v := range p.Components {
		if !admissionobs.Component(v).Valid() {
			return nil, &ValidationError{Msg: "invalid component"}
		}
		components = append(components, string(v))
	}
	for _, reason := range p.Reasons {
		if !admissionobs.ValidReason(reason) {
			return nil, &ValidationError{Msg: "invalid reason"}
		}
	}
	statuses := make([]string, 0, len(p.Statuses))
	for _, v := range p.Statuses {
		if v != "active" && v != "interrupted" && v != "closed" {
			return nil, &ValidationError{Msg: "invalid status"}
		}
		statuses = append(statuses, string(v))
	}
	items, total, err := s.incidents.ListPage(ctx, dbrepo.IncidentPageOptions{PageOptions: dbrepo.PageOptions{Limit: size, Offset: (page - 1) * size, SortBy: "lastSeenAt", Desc: true}, ClusterNames: p.Clusters, Components: components, Reasons: p.Reasons, Statuses: statuses, From: p.From, To: p.To, Now: time.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("list admission link incidents: %w", err)
	}
	now := time.Now().UTC()
	out := make([]dto.IncidentItem, 0, len(items))
	for _, item := range items {
		out = append(out, incidentItemDTO(item, now))
	}
	return &dto.IncidentPageView{Items: out, Total: int(total), Page: page, PageSize: size}, nil
}

func (s *IncidentService) Get(ctx context.Context, id string) (*dto.IncidentView, error) {
	if s.incidents == nil {
		return nil, ErrUnavailable
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return nil, &ValidationError{Msg: "invalid incident id"}
	}
	item, err := s.incidents.Get(ctx, n)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get admission link incident: %w", err)
	}
	result := &dto.IncidentView{IncidentItem: dto.IncidentItem{ID: strconv.FormatInt(item.ID, 10), ClusterName: item.ClusterName, ReporterInstanceID: item.ReporterInstanceID, Component: item.Component, Reason: item.Reason, StartedAt: item.StartedAt, LastSeenAt: item.LastSeenAt, AffectedCount: item.AffectedCount, Status: incidentStatus(item, time.Now().UTC())}, Detail: item.Detail}
	resolved := map[string]int64{}
	if s.admission != nil {
		resolved, err = s.admission.EventDatabaseIDs(ctx, item.SampleEventIDs)
		if err != nil {
			return nil, fmt.Errorf("resolve incident samples: %w", err)
		}
	}
	result.Samples = make([]dto.IncidentSampleView, 0, len(item.SampleEventIDs))
	for _, eventID := range item.SampleEventIDs {
		databaseID, found := resolved[eventID]
		sample := dto.IncidentSampleView{EventID: eventID, Expired: !found}
		if found {
			value := strconv.FormatInt(databaseID, 10)
			sample.EventDatabaseID = &value
		}
		result.Samples = append(result.Samples, sample)
	}
	return result, nil
}

func incidentItemDTO(v dbrepo.AdmissionLinkIncident, now time.Time) dto.IncidentItem {
	return dto.IncidentItem{ID: strconv.FormatInt(v.ID, 10), ClusterName: v.ClusterName, ReporterInstanceID: v.ReporterInstanceID, Component: v.Component, Reason: v.Reason, StartedAt: v.StartedAt, LastSeenAt: v.LastSeenAt, AffectedCount: v.AffectedCount, Status: incidentStatus(&v, now)}
}

func incidentStatus(v *dbrepo.AdmissionLinkIncident, now time.Time) string {
	if v.Closed {
		return "closed"
	}
	if v.LastReceivedAt.Before(now.Add(-admissionobs.ActiveFreshness)) {
		return "interrupted"
	}
	return "active"
}
