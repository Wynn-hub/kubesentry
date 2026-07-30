package webhook

import (
	"bytes"
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
	wake  chan struct{}
}

func newWebhookObservationReporter(endpoint, token string, client *http.Client) *webhookObservationReporter {
	seriesID, err := admissionobs.NewSeriesID()
	if err != nil {
		panic("generate webhook observation series id: " + err.Error())
	}
	reporter := &webhookObservationReporter{
		endpoint: strings.TrimSuffix(endpoint, "/") + "/observations",
		token:    token,
		client:   client,
		counters: admissionobs.NewAtomicCounters(),
		seriesID: seriesID,
		queue:    make([]*queuedObservation, 0, webhookObservationQueueSize),
		wake:     make(chan struct{}, 1),
	}
	go reporter.generate()
	go reporter.deliver()
	return reporter
}

func (r *webhookObservationReporter) generate() {
	ticker := time.NewTicker(webhookObservationInterval)
	defer ticker.Stop()
	for range ticker.C {
		r.enqueue(r.newReport())
	}
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
	if len(r.queue) == cap(r.queue) {
		copy(r.queue, r.queue[1:])
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

func (r *webhookObservationReporter) deliver() {
	for range r.wake {
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
				time.Sleep(retryDelay)
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
	return r.queue[0]
}

func (r *webhookObservationReporter) remove(reportID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 || r.queue[0].report.ReportId != reportID {
		return
	}
	r.queue = r.queue[1:]
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
