package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	admissionobs "github.com/Wynn-hub/kubesentry/internal/admission/observability"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

const maxMemberClockLead = 5 * time.Minute

func (s *Server) handleLinkIncidentBatch(ctx context.Context, clusterName string, batch *gatewaypb.LinkIncidentBatch) *gatewaypb.LinkIncidentBatchAck {
	ack := &gatewaypb.LinkIncidentBatchAck{}
	if batch == nil || len(batch.Incidents) == 0 {
		return ack
	}
	if len(batch.Incidents) > admissionobs.MaxBatchSize {
		for _, incident := range batch.Incidents {
			ack.Results = append(ack.Results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_DISCARDED, "batch_too_large"))
		}
		return ack
	}
	if s.repos.Admission != nil {
		settings, err := s.repos.Admission.GetSettings(ctx)
		if err != nil || !settings.ObservationEnabled {
			for _, incident := range batch.Incidents {
				ack.Results = append(ack.Results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_DISCARDED, "admission_pipeline_observation_disabled"))
			}
			return ack
		}
	}
	if s.repos.Incidents == nil || s.repos.Clusters == nil {
		for _, incident := range batch.Incidents {
			ack.Results = append(ack.Results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_RETRYABLE, admissionobs.ReasonWriteFailed))
		}
		return ack
	}
	cluster, err := s.repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		for _, incident := range batch.Incidents {
			ack.Results = append(ack.Results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_RETRYABLE, admissionobs.ReasonWriteFailed))
		}
		return ack
	}
	now := time.Now().UTC()
	for _, wire := range batch.Incidents {
		model, err := incidentFromProto(wire, now)
		if err != nil || !model.Component.ValidMember() {
			slog.Warn("discard invalid link incident", "cluster", clusterName, "incident", incidentIdentity(wire), "error", err)
			ack.Results = append(ack.Results, linkIncidentACK(incidentIdentity(wire), incidentRevision(wire), gatewaypb.LinkIncidentBatchAck_Result_DISCARDED, admissionobs.ReasonInvalidReport))
			continue
		}
		if err := s.repos.Incidents.Upsert(ctx, cluster.ID, cluster.Name, model, now); err != nil {
			s.recordGatewayIncident(admissionobs.ReasonWriteFailed, 1, err.Error(), now)
			slog.Error("store admission link incident", "cluster", clusterName, "incident", model.IncidentKey, "error", err)
			ack.Results = append(ack.Results, linkIncidentACK(model.IncidentKey, model.Revision, gatewaypb.LinkIncidentBatchAck_Result_RETRYABLE, admissionobs.ReasonWriteFailed))
			continue
		}
		ack.Results = append(ack.Results, linkIncidentACK(model.IncidentKey, model.Revision, gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED, ""))
	}
	return ack
}

func incidentFromProto(wire *gatewaypb.LinkIncident, receivedAt time.Time) (admissionobs.Incident, error) {
	model, err := incidentModel(wire)
	if err != nil {
		return admissionobs.Incident{}, err
	}
	latest := receivedAt.Add(maxMemberClockLead)
	if model.StartedAt.After(latest) {
		model.StartedAt = receivedAt
	}
	if model.LastSeenAt.After(latest) {
		model.LastSeenAt = receivedAt
	}
	if model.LastSeenAt.Before(model.StartedAt) {
		model.StartedAt = model.LastSeenAt
	}
	return model, model.Validate()
}

func incidentModel(wire *gatewaypb.LinkIncident) (admissionobs.Incident, error) {
	if wire == nil || wire.StartedAt == nil || wire.LastSeenAt == nil {
		return admissionobs.Incident{}, fmt.Errorf("missing incident fields")
	}
	if err := wire.StartedAt.CheckValid(); err != nil {
		return admissionobs.Incident{}, err
	}
	if err := wire.LastSeenAt.CheckValid(); err != nil {
		return admissionobs.Incident{}, err
	}
	model := admissionobs.Incident{
		IncidentKey: wire.IncidentKey, ReporterInstanceID: wire.ReporterInstanceId,
		Revision: wire.Revision, Component: admissionobs.Component(wire.Component), Reason: wire.Reason,
		StartedAt: wire.StartedAt.AsTime(), LastSeenAt: wire.LastSeenAt.AsTime(),
		AffectedCount: wire.AffectedCount, SampleEventIDs: append([]string(nil), wire.SampleEventIds...),
		Detail: wire.Detail, Closed: wire.Closed,
	}
	return model, model.Validate()
}

func (s *Server) recordGatewayIncident(reason string, affected int64, detail string, now time.Time) {
	if s.gatewayIncidentTracker == nil || !s.admissionObservationEnabled.Load() {
		return
	}
	s.gatewayIncidentTracker.Record(reason, affected, "", detail, now)
}

func (s *Server) RunGatewayIncidentReporter(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.flushGatewayIncidents(ctx, now.UTC())
		}
	}
}

func (s *Server) flushGatewayIncidents(ctx context.Context, now time.Time) {
	if s.repos.Incidents == nil || s.gatewayIncidentTracker == nil || !s.admissionObservationEnabled.Load() {
		return
	}
	for _, incident := range s.gatewayIncidentTracker.Snapshot(now) {
		if err := s.repos.Incidents.Upsert(ctx, 0, "platform", incident, now); err != nil {
			// Do not recursively record a write_failed incident while persisting
			// the write_failed incident itself.
			slog.Warn("flush gateway link incident", "incident", incident.IncidentKey, "error", err)
			return
		}
		if incident.Closed {
			s.gatewayIncidentTracker.Acknowledge(incident.IncidentKey, incident.Revision, true)
		}
	}
}

func linkIncidentACK(key string, revision uint64, status gatewaypb.LinkIncidentBatchAck_Result_Status, reason string) *gatewaypb.LinkIncidentBatchAck_Result {
	return &gatewaypb.LinkIncidentBatchAck_Result{IncidentKey: key, Revision: revision, Status: status, Reason: reason}
}

func incidentIdentity(incident *gatewaypb.LinkIncident) string {
	if incident == nil {
		return ""
	}
	return incident.IncidentKey
}

func incidentRevision(incident *gatewaypb.LinkIncident) uint64 {
	if incident == nil {
		return 0
	}
	return incident.Revision
}
