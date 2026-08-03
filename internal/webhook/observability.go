package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const webhookIncidentInterval = 10 * time.Second

type webhookObservationReporter struct {
	endpoint  string
	configURL string
	token     string
	client    *http.Client
	tracker   *admissionobs.Tracker

	workersMu     sync.Mutex
	workersCancel context.CancelFunc
}

func newWebhookObservationReporter(endpoint, token string, client *http.Client) *webhookObservationReporter {
	instance := os.Getenv("POD_NAME")
	reporter := &webhookObservationReporter{
		endpoint:  strings.TrimSuffix(endpoint, "/") + "/incidents",
		configURL: strings.TrimSuffix(endpoint, "/") + "/observations/config",
		token:     token, client: client,
		tracker: admissionobs.NewTracker(admissionobs.ComponentWebhook, instance),
	}
	reporter.start()
	return reporter
}

func (r *webhookObservationReporter) start() { go r.pollConfig() }

func (r *webhookObservationReporter) pollConfig() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		r.refreshEnabled()
		<-ticker.C
	}
}

func (r *webhookObservationReporter) refreshEnabled() {
	req, err := http.NewRequest(http.MethodGet, r.configURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	response, err := r.client.Do(req)
	if err != nil {
		return
	}
	defer response.Body.Close() //nolint:errcheck
	var state struct {
		Enabled bool `json:"enabled"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&state) != nil {
		return
	}
	r.setEnabled(state.Enabled)
}

func (r *webhookObservationReporter) setEnabled(enabled bool) {
	r.workersMu.Lock()
	defer r.workersMu.Unlock()
	if enabled {
		if r.workersCancel != nil {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		r.workersCancel = cancel
		go r.run(ctx)
		return
	}
	if r.workersCancel != nil {
		r.workersCancel()
		r.workersCancel = nil
	}
	r.tracker.Reset()
}

func (r *webhookObservationReporter) run(ctx context.Context) {
	ticker := time.NewTicker(webhookIncidentInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.flush(now.UTC())
		}
	}
}

func (r *webhookObservationReporter) flush(now time.Time) {
	incidents := r.tracker.Snapshot(now)
	for start := 0; start < len(incidents); start += admissionobs.MaxBatchSize {
		end := min(start+admissionobs.MaxBatchSize, len(incidents))
		batch := &gatewaypb.LinkIncidentBatch{Incidents: make([]*gatewaypb.LinkIncident, 0, end-start)}
		for index := start; index < end; index++ {
			batch.Incidents = append(batch.Incidents, incidentProto(incidents[index]))
		}
		ack, err := r.post(batch)
		if err != nil {
			r.tracker.Record(admissionobs.ReasonReportDropped, 1, "", err.Error(), now)
			slog.Warn("webhook incident report failed", "error", err)
			return
		}
		for _, result := range ack.Results {
			if result == nil {
				continue
			}
			terminal := result.Status == gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED || result.Status == gatewaypb.LinkIncidentBatchAck_Result_DISCARDED
			r.tracker.Acknowledge(result.IncidentKey, result.Revision, terminal)
		}
	}
}

func (r *webhookObservationReporter) post(batch *gatewaypb.LinkIncidentBatch) (*gatewaypb.LinkIncidentBatchAck, error) {
	body, err := protojson.Marshal(batch)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)
	response, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("incident endpoint returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	var ack gatewaypb.LinkIncidentBatchAck
	if err := protojson.Unmarshal(data, &ack); err != nil {
		return nil, err
	}
	return &ack, nil
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
