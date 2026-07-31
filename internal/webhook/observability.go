package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	webhookObservationInterval     = 10 * time.Second
	webhookObservationQueueSize    = 32
	maxObservationDeliveryAttempts = 3
)

type queuedObservation struct {
	report   *gatewaypb.ObservationReport
	attempts int
	inFlight bool
}

type webhookObservationReporter struct {
	endpoint string
	token    string
	client   *http.Client
	counters *admissionobs.AtomicCounters
	gauge    admissionobs.QueueGauge

	seriesID string
	sequence atomic.Uint64

	mu    sync.Mutex
	queue []*queuedObservation
	limit int
	wake  chan struct{}

	workersMu     sync.Mutex
	workersCancel context.CancelFunc
	configURL     string
}

func newWebhookObservationReporter(endpoint, token string, client *http.Client) *webhookObservationReporter {
	seriesID, err := admissionobs.NewSeriesID()
	if err != nil {
		panic("generate webhook observation series id: " + err.Error())
	}
	reporter := &webhookObservationReporter{
		endpoint:  strings.TrimSuffix(endpoint, "/") + "/observations",
		configURL: strings.TrimSuffix(endpoint, "/") + "/observations/config",
		token:     token,
		client:    client,
		counters:  admissionobs.NewAtomicCounters(),
		seriesID:  seriesID,
		queue:     make([]*queuedObservation, 0, webhookObservationQueueSize),
		limit:     webhookObservationQueueSize,
		wake:      make(chan struct{}, 1),
	}
	go reporter.pollConfig()
	return reporter
}

func (r *webhookObservationReporter) generate(ctx context.Context) {
	ticker := time.NewTicker(webhookObservationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.enqueue(r.newReport())
		}
	}
}

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
		r.setEnabled(false)
		return
	}
	defer response.Body.Close() //nolint:errcheck
	var state struct {
		Enabled bool `json:"enabled"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&state) != nil {
		r.setEnabled(false)
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
		go r.generate(ctx)
		go r.deliver(ctx)
		return
	}
	if r.workersCancel != nil {
		r.workersCancel()
		r.workersCancel = nil
	}
	r.mu.Lock()
	for index := range r.queue {
		r.queue[index] = nil
	}
	r.queue = r.queue[:0]
	r.mu.Unlock()
}

func (r *webhookObservationReporter) newReport() *gatewaypb.ObservationReport {
	sequence := r.sequence.Add(1)
	snapshot := r.counters.Snapshot()
	gauge := r.gauge.SnapshotAndReset()
	return &gatewaypb.ObservationReport{
		ReportId:             admissionobs.ReportID(r.seriesID, sequence),
		SeriesId:             r.seriesID,
		Sequence:             sequence,
		ObservedAt:           timestamppb.Now(),
		Component:            string(admissionobs.ComponentWebhook),
		Counters:             observationCountersProto(snapshot.Counters),
		QueueDepthMax:        gauge.DepthPeak,
		QueueCapacity:        gauge.Capacity,
		FaultReasonCounts:    snapshot.FaultReasonCounts,
		PressureReasonCounts: snapshot.PressureReasonCounts,
		FilterReasonCounts:   snapshot.FilterReasonCounts,
	}
}

func (r *webhookObservationReporter) enqueue(report *gatewaypb.ObservationReport) {
	r.mu.Lock()
	if len(r.queue) == r.limit {
		dropIndex := 0
		if r.queue[0].inFlight && len(r.queue) > 1 {
			dropIndex = 1
		}
		copy(r.queue[dropIndex:], r.queue[dropIndex+1:])
		r.queue[len(r.queue)-1] = &queuedObservation{report: report}
		r.counters.Add(admissionobs.Counters{ReportDropped: 1})
		r.counters.AddReason(admissionobs.ReasonFault, admissionobs.ReasonObservationQueueFull, 1)
	} else {
		r.queue = append(r.queue, &queuedObservation{report: report})
	}
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *webhookObservationReporter) deliver(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		}
		for {
			queued := r.front()
			if queued == nil {
				break
			}
			queued.attempts++
			status, reason := r.post(queued.report)
			switch status {
			case gatewaypb.ObservationReportBatchAck_Result_ACCEPTED:
				r.remove(queued.report.ReportId)
			case gatewaypb.ObservationReportBatchAck_Result_DISCARDED:
				r.counters.Add(admissionobs.Counters{ReportDropped: 1})
				r.counters.AddReason(admissionobs.ReasonFault, reason, 1)
				r.remove(queued.report.ReportId)
			default:
				if queued.attempts >= maxObservationDeliveryAttempts {
					r.counters.Add(admissionobs.Counters{ReportDropped: 1})
					r.counters.AddReason(admissionobs.ReasonFault, admissionobs.ReasonRetryExhausted, 1)
					slog.Warn("webhook observation report dropped",
						"report", queued.report.ReportId, "attempts", queued.attempts, "reason", reason)
					r.remove(queued.report.ReportId)
					continue
				}
				r.counters.Add(admissionobs.Counters{Retry: 1})
				r.counters.AddReason(admissionobs.ReasonFault, reason, 1)
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryDelay):
				}
			}
		}
	}
}

func (r *webhookObservationReporter) front() *queuedObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return nil
	}
	r.queue[0].inFlight = true
	return r.queue[0]
}

func (r *webhookObservationReporter) remove(reportID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 || r.queue[0].report.ReportId != reportID {
		return
	}
	copy(r.queue, r.queue[1:])
	r.queue[len(r.queue)-1] = nil
	r.queue = r.queue[:len(r.queue)-1]
}

func (r *webhookObservationReporter) post(report *gatewaypb.ObservationReport) (gatewaypb.ObservationReportBatchAck_Result_Status, string) {
	body, err := protojson.Marshal(&gatewaypb.ObservationReportBatch{Reports: []*gatewaypb.ObservationReport{report}})
	if err != nil {
		return gatewaypb.ObservationReportBatchAck_Result_DISCARDED, admissionobs.ReasonMarshalFailed
	}
	req, err := http.NewRequest(http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return gatewaypb.ObservationReportBatchAck_Result_DISCARDED, admissionobs.ReasonRequestBuildFailed
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)
	response, err := r.client.Do(req)
	if err != nil {
		return gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.ReasonPostFailed
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.HTTPFailureReason(response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.ReasonACKReadFailed
	}
	var ack gatewaypb.ObservationReportBatchAck
	if protojson.Unmarshal(data, &ack) != nil ||
		len(ack.Results) != 1 ||
		ack.Results[0].ReportId != report.ReportId {
		return gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.ReasonACKInvalid
	}
	return ack.Results[0].Status, ack.Results[0].Reason
}

func observationCountersProto(counters admissionobs.Counters) *gatewaypb.ObservationCounters {
	return &gatewaypb.ObservationCounters{
		Generated:        counters.Generated,
		Received:         counters.Received,
		Forwarded:        counters.Forwarded,
		Written:          counters.Written,
		Duplicate:        counters.Duplicate,
		PolicyFiltered:   counters.PolicyFiltered,
		RetentionElapsed: counters.RetentionElapsed,
		QueueRejected:    counters.QueueRejected,
		Retry:            counters.Retry,
		Dropped:          counters.Dropped,
		RetryExhausted:   counters.RetryExhausted,
		BatchFailure:     counters.BatchFailure,
		AuthFailure:      counters.AuthFailure,
		AckTimeout:       counters.ACKTimeout,
		ReportDropped:    counters.ReportDropped,
		CleanupRun:       counters.CleanupRun,
		CleanupDeleted:   counters.CleanupDeleted,
		CleanupFailure:   counters.CleanupFailure,
	}
}
