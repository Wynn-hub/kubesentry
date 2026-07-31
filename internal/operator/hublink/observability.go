package hublink

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	operatorObservationInterval      = 10 * time.Second
	operatorObservationQueueCapacity = 128
	maxObservationRetryAttempts      = 3
)

type queuedObservationReport struct {
	report      *gatewaypb.ObservationReport
	retryCount  int
	inFlight    bool
	ackDeadline time.Time
}

func (l *Link) RecordAdmissionObservationFailure(class admissionobs.ReasonClass, reason string) {
	if reason == admissionobs.ReasonAuthFailed {
		l.observationStats.Add(admissionobs.Counters{AuthFailure: 1})
	}
	l.observationStats.AddReason(class, reason, 1)
}

func (l *Link) SubmitObservationReports(batch *gatewaypb.ObservationReportBatch) []*gatewaypb.ObservationReportBatchAck_Result {
	if batch == nil || len(batch.Reports) == 0 {
		return nil
	}
	results := make([]*gatewaypb.ObservationReportBatchAck_Result, 0, len(batch.Reports))
	if !l.admissionObservationEnabled() {
		for _, report := range batch.Reports {
			reportID := ""
			if report != nil {
				reportID = report.ReportId
			}
			results = append(results, &gatewaypb.ObservationReportBatchAck_Result{
				ReportId: reportID,
				Status:   gatewaypb.ObservationReportBatchAck_Result_DISCARDED,
				Reason:   "admission_pipeline_observation_disabled",
			})
		}
		return results
	}
	if !l.IsLeader() {
		for _, report := range batch.Reports {
			reportID := ""
			if report != nil {
				reportID = report.ReportId
			}
			results = append(results, &gatewaypb.ObservationReportBatchAck_Result{
				ReportId: reportID,
				Status:   gatewaypb.ObservationReportBatchAck_Result_RETRYABLE,
				Reason:   admissionobs.ReasonNotLeader,
			})
		}
		l.RecordAdmissionObservationFailure(admissionobs.ReasonFault, admissionobs.ReasonNotLeader)
		return results
	}
	l.observationsMu.Lock()
	defer l.observationsMu.Unlock()
	for _, incoming := range batch.Reports {
		report, err := normalizeWebhookObservationReport(incoming)
		if err != nil {
			l.RecordAdmissionObservationFailure(admissionobs.ReasonFault, admissionobs.ReasonInvalidReport)
			reportID := ""
			if incoming != nil {
				reportID = incoming.ReportId
			}
			results = append(results, &gatewaypb.ObservationReportBatchAck_Result{
				ReportId: reportID,
				Status:   gatewaypb.ObservationReportBatchAck_Result_DISCARDED,
				Reason:   admissionobs.ReasonInvalidReport,
			})
			continue
		}
		if _, exists := l.observations[report.ReportId]; exists {
			results = append(results, &gatewaypb.ObservationReportBatchAck_Result{
				ReportId: report.ReportId,
				Status:   gatewaypb.ObservationReportBatchAck_Result_ACCEPTED,
			})
			continue
		}
		if len(l.observations) >= operatorObservationQueueCapacity {
			l.observationStats.Add(admissionobs.Counters{ReportDropped: 1})
			l.observationStats.AddReason(admissionobs.ReasonFault, admissionobs.ReasonObservationQueueFull, 1)
			results = append(results, &gatewaypb.ObservationReportBatchAck_Result{
				ReportId: report.ReportId,
				Status:   gatewaypb.ObservationReportBatchAck_Result_RETRYABLE,
				Reason:   admissionobs.ReasonObservationQueueFull,
			})
			continue
		}
		l.observations[report.ReportId] = &queuedObservationReport{report: report}
		results = append(results, &gatewaypb.ObservationReportBatchAck_Result{
			ReportId: report.ReportId,
			Status:   gatewaypb.ObservationReportBatchAck_Result_ACCEPTED,
		})
	}
	select {
	case l.observationsWake <- struct{}{}:
	default:
	}
	return results
}

func normalizeWebhookObservationReport(incoming *gatewaypb.ObservationReport) (*gatewaypb.ObservationReport, error) {
	report, err := observationReportModel(incoming)
	if err != nil {
		return nil, err
	}
	if report.Component != admissionobs.ComponentWebhook {
		return nil, &invalidObservationError{message: "HTTP observation report must come from webhook"}
	}
	report.Snapshot = admissionobs.NormalizeSnapshot(report.Snapshot)
	normalized := proto.Clone(incoming).(*gatewaypb.ObservationReport)
	normalized.FaultReasonCounts = report.Snapshot.FaultReasonCounts
	normalized.PressureReasonCounts = report.Snapshot.PressureReasonCounts
	normalized.FilterReasonCounts = report.Snapshot.FilterReasonCounts
	return normalized, nil
}

type invalidObservationError struct {
	message string
}

func (e *invalidObservationError) Error() string { return e.message }

func observationReportModel(report *gatewaypb.ObservationReport) (admissionobs.Report, error) {
	if report == nil || report.Counters == nil || report.ObservedAt == nil {
		return admissionobs.Report{}, &invalidObservationError{message: "missing observation fields"}
	}
	if err := report.ObservedAt.CheckValid(); err != nil {
		return admissionobs.Report{}, err
	}
	counters := report.Counters
	model := admissionobs.Report{
		ReportID:   report.ReportId,
		SeriesID:   report.SeriesId,
		Sequence:   report.Sequence,
		ObservedAt: report.ObservedAt.AsTime(),
		Component:  admissionobs.Component(report.Component),
		Snapshot: admissionobs.Snapshot{
			Counters: admissionobs.Counters{
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
				ACKTimeout:       counters.AckTimeout,
				ReportDropped:    counters.ReportDropped,
				CleanupRun:       counters.CleanupRun,
				CleanupDeleted:   counters.CleanupDeleted,
				CleanupFailure:   counters.CleanupFailure,
			},
			FaultReasonCounts:    report.FaultReasonCounts,
			PressureReasonCounts: report.PressureReasonCounts,
			FilterReasonCounts:   report.FilterReasonCounts,
		},
		QueueDepthMax: report.QueueDepthMax,
		QueueCapacity: report.QueueCapacity,
	}
	model.Snapshot = admissionobs.NormalizeSnapshot(model.Snapshot)
	if err := model.Validate(); err != nil {
		return admissionobs.Report{}, err
	}
	return model, nil
}

func (l *Link) generateOperatorObservations(ctx context.Context) {
	ticker := time.NewTicker(operatorObservationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if l.IsLeader() && l.admissionObservationEnabled() {
				l.enqueueOperatorObservation()
			}
		}
	}
}

func (l *Link) admissionObservationEnabled() bool {
	l.configMu.RLock()
	defer l.configMu.RUnlock()
	return l.eventConfig != nil && l.eventConfig.ObservationEnabled
}

func (l *Link) AdmissionObservationEnabled() bool {
	return l.admissionObservationEnabled()
}

func (l *Link) setEventConfig(config *gatewaypb.AdmissionEventConfig) {
	l.configMu.Lock()
	l.eventConfig = config
	l.configMu.Unlock()
	if config == nil || !config.ObservationEnabled {
		l.observationsMu.Lock()
		l.observations = map[string]*queuedObservationReport{}
		l.observationsMu.Unlock()
	}
}

func (l *Link) enqueueOperatorObservation() {
	_, _, eventCapacity := l.admissionDeliveryConfig()
	l.eventsMu.Lock()
	l.eventQueueGauge.Observe(int64(len(l.events)), int64(eventCapacity))
	l.eventsMu.Unlock()
	gauge := l.eventQueueGauge.SnapshotAndReset()
	snapshot := l.observationStats.Snapshot()
	sequence := l.observationSequence.Add(1)
	report := &gatewaypb.ObservationReport{
		ReportId:             admissionobs.ReportID(l.observationSeries, sequence),
		SeriesId:             l.observationSeries,
		Sequence:             sequence,
		ObservedAt:           timestamppb.Now(),
		Component:            string(admissionobs.ComponentOperator),
		Counters:             observationCountersProto(snapshot.Counters),
		QueueDepthMax:        gauge.DepthPeak,
		QueueCapacity:        gauge.Capacity,
		FaultReasonCounts:    snapshot.FaultReasonCounts,
		PressureReasonCounts: snapshot.PressureReasonCounts,
		FilterReasonCounts:   snapshot.FilterReasonCounts,
	}

	l.observationsMu.Lock()
	if len(l.observations) >= operatorObservationQueueCapacity {
		l.observationsMu.Unlock()
		l.observationStats.Add(admissionobs.Counters{ReportDropped: 1})
		l.observationStats.AddReason(admissionobs.ReasonFault, admissionobs.ReasonObservationQueueFull, 1)
		return
	}
	l.observations[report.ReportId] = &queuedObservationReport{report: report}
	l.observationsMu.Unlock()
	select {
	case l.observationsWake <- struct{}{}:
	default:
	}
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

func (l *Link) sendObservationReports(
	now time.Time,
	send func(*gatewaypb.MemberMessage) error,
) bool {
	l.observationsMu.Lock()
	for id, queued := range l.observations {
		if queued.inFlight && !queued.ackDeadline.After(now) {
			l.failObservationReportLocked(id, admissionobs.ReasonGatewayACKTimeout)
		}
	}
	reports := make([]*gatewaypb.ObservationReport, 0, len(l.observations))
	reportIDs := make([]string, 0, len(l.observations))
	for id, queued := range l.observations {
		if queued.inFlight {
			continue
		}
		queued.inFlight = true
		queued.ackDeadline = now.Add(eventAckTimeout)
		reports = append(reports, proto.Clone(queued.report).(*gatewaypb.ObservationReport))
		reportIDs = append(reportIDs, id)
	}
	l.observationsMu.Unlock()
	if len(reports) == 0 {
		return true
	}
	if err := send(&gatewaypb.MemberMessage{Msg: &gatewaypb.MemberMessage_ObservationReportBatch{
		ObservationReportBatch: &gatewaypb.ObservationReportBatch{Reports: reports},
	}}); err != nil {
		l.observationsMu.Lock()
		for _, id := range reportIDs {
			if queued := l.observations[id]; queued != nil && queued.inFlight {
				l.failObservationReportLocked(id, admissionobs.ReasonSendFailed)
			}
		}
		l.observationsMu.Unlock()
		return false
	}
	return true
}

func (l *Link) ackObservationReports(ack *gatewaypb.ObservationReportBatchAck) {
	if ack == nil {
		return
	}
	l.observationsMu.Lock()
	defer l.observationsMu.Unlock()
	for _, result := range ack.Results {
		if result == nil {
			continue
		}
		if _, exists := l.observations[result.ReportId]; !exists {
			continue
		}
		switch result.Status {
		case gatewaypb.ObservationReportBatchAck_Result_ACCEPTED,
			gatewaypb.ObservationReportBatchAck_Result_DISCARDED:
			delete(l.observations, result.ReportId)
		case gatewaypb.ObservationReportBatchAck_Result_RETRYABLE:
			l.failObservationReportLocked(result.ReportId, result.Reason)
		}
	}
}

func (l *Link) failObservationReportLocked(reportID, reason string) {
	queued := l.observations[reportID]
	if queued == nil {
		return
	}
	l.observationStats.AddReason(admissionobs.ReasonFault, reason, 1)
	if reason == admissionobs.ReasonGatewayACKTimeout {
		l.observationStats.Add(admissionobs.Counters{ACKTimeout: 1})
	}
	if queued.retryCount >= maxObservationRetryAttempts {
		delete(l.observations, reportID)
		l.observationStats.Add(admissionobs.Counters{ReportDropped: 1, RetryExhausted: 1})
		l.observationStats.AddReason(admissionobs.ReasonFault, admissionobs.ReasonRetryExhausted, 1)
		slog.Warn("observation report delivery dropped", "report", reportID, "reason", reason)
		return
	}
	queued.retryCount++
	queued.inFlight = false
	queued.ackDeadline = time.Time{}
	l.observationStats.Add(admissionobs.Counters{Retry: 1})
}

func (l *Link) failInFlightObservationReports(reason string) {
	l.observationsMu.Lock()
	defer l.observationsMu.Unlock()
	for id, queued := range l.observations {
		if queued.inFlight {
			l.failObservationReportLocked(id, reason)
		}
	}
}
