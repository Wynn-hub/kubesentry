package hublink

import (
	"context"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	operatorIncidentInterval = 10 * time.Second
	operatorIncidentCapacity = 256
)

type queuedLinkIncident struct {
	incident         *gatewaypb.LinkIncident
	inFlight         bool
	inFlightRevision uint64
	ackDeadline      time.Time
}

func (l *Link) RecordLinkIncident(reason string, affected int64, detail string) {
	if !l.admissionObservationEnabled() {
		return
	}
	l.incidentTracker.Record(reason, affected, "", detail, time.Now().UTC())
}

func (l *Link) SubmitLinkIncidents(batch *gatewaypb.LinkIncidentBatch) []*gatewaypb.LinkIncidentBatchAck_Result {
	if batch == nil || len(batch.Incidents) == 0 {
		return nil
	}
	results := make([]*gatewaypb.LinkIncidentBatchAck_Result, 0, len(batch.Incidents))
	if len(batch.Incidents) > admissionobs.MaxBatchSize {
		for _, incident := range batch.Incidents {
			results = append(results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_DISCARDED, "batch_too_large"))
		}
		return results
	}
	if !l.admissionObservationEnabled() {
		for _, incident := range batch.Incidents {
			results = append(results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_DISCARDED, "admission_pipeline_observation_disabled"))
		}
		return results
	}
	if !l.IsLeader() {
		for _, incident := range batch.Incidents {
			results = append(results, linkIncidentACK(incidentIdentity(incident), incidentRevision(incident), gatewaypb.LinkIncidentBatchAck_Result_RETRYABLE, admissionobs.ReasonNotLeader))
		}
		l.RecordLinkIncident(admissionobs.ReasonBatchFailure, int64(len(batch.Incidents)), admissionobs.ReasonNotLeader)
		return results
	}

	l.incidentsMu.Lock()
	for _, incoming := range batch.Incidents {
		incident, err := normalizeWebhookIncident(incoming)
		if err != nil {
			results = append(results, linkIncidentACK(incidentIdentity(incoming), incidentRevision(incoming), gatewaypb.LinkIncidentBatchAck_Result_DISCARDED, admissionobs.ReasonInvalidReport))
			continue
		}
		if queued := l.incidents[incident.IncidentKey]; queued != nil {
			if incident.Revision > queued.incident.Revision {
				queued.incident = incident
			}
			results = append(results, linkIncidentACK(incident.IncidentKey, incident.Revision, gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED, ""))
			continue
		}
		if len(l.incidents) >= operatorIncidentCapacity {
			results = append(results, linkIncidentACK(incident.IncidentKey, incident.Revision, gatewaypb.LinkIncidentBatchAck_Result_RETRYABLE, admissionobs.ReasonQueueFull))
			continue
		}
		l.incidents[incident.IncidentKey] = &queuedLinkIncident{incident: incident}
		results = append(results, linkIncidentACK(incident.IncidentKey, incident.Revision, gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED, ""))
	}
	l.incidentsMu.Unlock()
	select {
	case l.incidentsWake <- struct{}{}:
	default:
	}
	return results
}

func normalizeWebhookIncident(incoming *gatewaypb.LinkIncident) (*gatewaypb.LinkIncident, error) {
	model, err := incidentModel(incoming)
	if err != nil {
		return nil, err
	}
	if model.Component != admissionobs.ComponentWebhook {
		return nil, errors.New("HTTP incidents must come from webhook")
	}
	return proto.Clone(incoming).(*gatewaypb.LinkIncident), nil
}

func incidentModel(wire *gatewaypb.LinkIncident) (admissionobs.Incident, error) {
	if wire == nil || wire.StartedAt == nil || wire.LastSeenAt == nil {
		return admissionobs.Incident{}, errors.New("missing incident fields")
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
		AffectedCount: wire.AffectedCount, SampleEventIDs: wire.SampleEventIds,
		Detail: wire.Detail, Closed: wire.Closed,
	}
	return model, model.Validate()
}

func (l *Link) generateOperatorIncidents(ctx context.Context) {
	ticker := time.NewTicker(operatorIncidentInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if l.IsLeader() && l.admissionObservationEnabled() {
				l.enqueueLocalIncidents(now.UTC())
			}
		}
	}
}

func (l *Link) admissionObservationEnabled() bool {
	l.configMu.RLock()
	defer l.configMu.RUnlock()
	return l.eventConfig != nil && l.eventConfig.ObservationEnabled
}

func (l *Link) AdmissionObservationEnabled() bool { return l.admissionObservationEnabled() }

func (l *Link) setEventConfig(config *gatewaypb.AdmissionEventConfig) {
	l.configMu.Lock()
	l.eventConfig = config
	l.configMu.Unlock()
	if config == nil || !config.ObservationEnabled {
		l.incidentsMu.Lock()
		l.incidents = map[string]*queuedLinkIncident{}
		l.incidentsMu.Unlock()
		l.incidentTracker.Reset()
	}
}

func (l *Link) enqueueLocalIncidents(now time.Time) {
	items := l.incidentTracker.Snapshot(now)
	l.incidentsMu.Lock()
	for _, item := range items {
		wire := incidentProto(item)
		if queued := l.incidents[item.IncidentKey]; queued != nil {
			if item.Revision > queued.incident.Revision {
				queued.incident = wire
			}
			continue
		}
		if len(l.incidents) >= operatorIncidentCapacity {
			l.incidentTracker.Record(admissionobs.ReasonReportDropped, 1, "", "operator incident queue reached capacity", now)
			break
		}
		l.incidents[item.IncidentKey] = &queuedLinkIncident{incident: wire}
	}
	l.incidentsMu.Unlock()
	select {
	case l.incidentsWake <- struct{}{}:
	default:
	}
}

func (l *Link) sendLinkIncidents(now time.Time, send func(*gatewaypb.MemberMessage) error) bool {
	l.incidentsMu.Lock()
	incidents := make([]*gatewaypb.LinkIncident, 0, min(len(l.incidents), admissionobs.MaxBatchSize))
	keys := make([]string, 0, cap(incidents))
	for key, queued := range l.incidents {
		if queued.inFlight && !queued.ackDeadline.After(now) {
			queued.inFlight = false
			queued.ackDeadline = time.Time{}
			l.incidentTracker.Record(admissionobs.ReasonACKTimeout, 1, "", key, now)
		}
		if queued.inFlight || len(incidents) >= admissionobs.MaxBatchSize {
			continue
		}
		queued.inFlight = true
		queued.inFlightRevision = queued.incident.Revision
		queued.ackDeadline = now.Add(eventAckTimeout)
		incidents = append(incidents, proto.Clone(queued.incident).(*gatewaypb.LinkIncident))
		keys = append(keys, key)
	}
	l.incidentsMu.Unlock()
	if len(incidents) == 0 {
		return true
	}
	if err := send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_LinkIncidentBatch{LinkIncidentBatch: &gatewaypb.LinkIncidentBatch{Incidents: incidents}}}); err != nil {
		l.incidentsMu.Lock()
		for _, key := range keys {
			if queued := l.incidents[key]; queued != nil {
				queued.inFlight = false
			}
		}
		l.incidentsMu.Unlock()
		return false
	}
	return true
}

func (l *Link) ackLinkIncidents(ack *gatewaypb.LinkIncidentBatchAck) {
	if ack == nil {
		return
	}
	l.incidentsMu.Lock()
	defer l.incidentsMu.Unlock()
	for _, result := range ack.Results {
		if result == nil {
			continue
		}
		queued := l.incidents[result.IncidentKey]
		if queued == nil || result.Revision < queued.inFlightRevision {
			continue
		}
		terminal := result.Status == gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED || result.Status == gatewaypb.LinkIncidentBatchAck_Result_DISCARDED
		if terminal {
			l.incidentTracker.Acknowledge(result.IncidentKey, result.Revision, true)
			if queued.incident.Revision <= result.Revision {
				delete(l.incidents, result.IncidentKey)
			} else {
				queued.inFlight = false
			}
		} else {
			queued.inFlight = false
			queued.ackDeadline = time.Time{}
		}
	}
}

func (l *Link) failInFlightLinkIncidents() {
	l.incidentsMu.Lock()
	defer l.incidentsMu.Unlock()
	for _, queued := range l.incidents {
		queued.inFlight = false
		queued.ackDeadline = time.Time{}
	}
}

func incidentProto(incident admissionobs.Incident) *gatewaypb.LinkIncident {
	return &gatewaypb.LinkIncident{
		IncidentKey: incident.IncidentKey, ReporterInstanceId: incident.ReporterInstanceID,
		Revision: incident.Revision, Component: string(incident.Component), Reason: incident.Reason,
		StartedAt: timestamppb.New(incident.StartedAt), LastSeenAt: timestamppb.New(incident.LastSeenAt),
		AffectedCount: incident.AffectedCount, SampleEventIds: incident.SampleEventIDs,
		Detail: incident.Detail, Closed: incident.Closed,
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

func (l *Link) SetPlatformObservationEnabled(enabled bool) {
	if l.platformObservation.Swap(enabled) == enabled {
		return
	}
	if !enabled {
		for {
			select {
			case <-l.platformSamples:
			default:
				return
			}
		}
	}
}

func (l *Link) PlatformObservationEnabled() bool { return l.platformObservation.Load() }
