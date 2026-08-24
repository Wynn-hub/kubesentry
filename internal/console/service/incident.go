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

func (s *IncidentService) List(ctx context.Context, p dto.ListAdmissionLinkIncidentsParams) (*dto.AdmissionLinkIncidentListData, error) {
	if s.incidents == nil {
		return nil, ErrUnavailable
	}
	page, size := p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20))
	components := make([]string, 0, len(p.Component))
	for _, v := range p.Component {
		if !admissionobs.Component(v).Valid() {
			return nil, &ValidationError{Msg: "invalid component"}
		}
		components = append(components, string(v))
	}
	for _, reason := range p.Reason {
		if !admissionobs.ValidReason(reason) {
			return nil, &ValidationError{Msg: "invalid reason"}
		}
	}
	statuses := make([]string, 0, len(p.Status))
	for _, v := range p.Status {
		if v != "active" && v != "interrupted" && v != "closed" {
			return nil, &ValidationError{Msg: "invalid status"}
		}
		statuses = append(statuses, string(v))
	}
	var from, to *time.Time
	if v, ok := p.From.Get(); ok {
		t := v
		from = &t
	}
	if v, ok := p.To.Get(); ok {
		t := v
		to = &t
	}
	items, total, err := s.incidents.ListPage(ctx, dbrepo.IncidentPageOptions{PageOptions: dbrepo.PageOptions{Limit: size, Offset: (page - 1) * size, SortBy: "lastSeenAt", Desc: true}, ClusterNames: p.Cluster, Components: components, Reasons: p.Reason, Statuses: statuses, From: from, To: to, Now: time.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("list admission link incidents: %w", err)
	}
	now := time.Now().UTC()
	out := make([]dto.AdmissionLinkIncidentItem, 0, len(items))
	for _, item := range items {
		out = append(out, incidentItemDTO(item, now))
	}
	return &dto.AdmissionLinkIncidentListData{Items: out, Total: int(total), Page: page, PageSize: dto.AdmissionLinkIncidentListDataPageSize(size)}, nil
}

func (s *IncidentService) Get(ctx context.Context, id dto.ResourceID) (*dto.AdmissionLinkIncidentDetail, error) {
	if s.incidents == nil {
		return nil, ErrUnavailable
	}
	n, err := strconv.ParseInt(string(id), 10, 64)
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
	result := &dto.AdmissionLinkIncidentDetail{ID: dto.ResourceID(strconv.FormatInt(item.ID, 10)), ClusterName: item.ClusterName, ReporterInstanceId: item.ReporterInstanceID, Component: dto.AdmissionLinkIncidentDetailComponent(item.Component), Reason: item.Reason, StartedAt: item.StartedAt, LastSeenAt: item.LastSeenAt, AffectedCount: item.AffectedCount, Status: dto.AdmissionLinkIncidentDetailStatus(incidentStatus(item, time.Now().UTC())), Detail: item.Detail}
	resolved := map[string]int64{}
	if s.admission != nil {
		resolved, err = s.admission.EventDatabaseIDs(ctx, item.SampleEventIDs)
		if err != nil {
			return nil, fmt.Errorf("resolve incident samples: %w", err)
		}
	}
	result.Samples = make([]dto.AdmissionLinkIncidentSample, 0, len(item.SampleEventIDs))
	for _, eventID := range item.SampleEventIDs {
		databaseID, found := resolved[eventID]
		sample := dto.AdmissionLinkIncidentSample{EventId: eventID, Expired: !found}
		if found {
			sample.EventDatabaseId = dto.OptResourceID{Value: dto.ResourceID(strconv.FormatInt(databaseID, 10)), Set: true}
		}
		result.Samples = append(result.Samples, sample)
	}
	return result, nil
}

func incidentItemDTO(v dbrepo.AdmissionLinkIncident, now time.Time) dto.AdmissionLinkIncidentItem {
	return dto.AdmissionLinkIncidentItem{ID: dto.ResourceID(strconv.FormatInt(v.ID, 10)), ClusterName: v.ClusterName, ReporterInstanceId: v.ReporterInstanceID, Component: dto.AdmissionLinkIncidentItemComponent(v.Component), Reason: v.Reason, StartedAt: v.StartedAt, LastSeenAt: v.LastSeenAt, AffectedCount: v.AffectedCount, Status: dto.AdmissionLinkIncidentItemStatus(incidentStatus(&v, now))}
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
