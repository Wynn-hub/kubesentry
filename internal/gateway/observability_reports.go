package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func (s *Server) handleObservationReportBatch(ctx context.Context, clusterName string, batch *gatewaypb.ObservationReportBatch) *gatewaypb.ObservationReportBatchAck {
	ack := &gatewaypb.ObservationReportBatchAck{}
	if batch == nil || len(batch.Reports) == 0 {
		return ack
	}
	if s.repos.Observability == nil {
		for _, report := range batch.Reports {
			if report != nil && report.ReportId != "" {
				ack.Results = append(ack.Results, observationReportACK(report.ReportId, gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.ReasonDBWriteFailed))
			}
		}
		return ack
	}
	cluster, err := s.repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		for _, report := range batch.Reports {
			if report != nil && report.ReportId != "" {
				ack.Results = append(ack.Results, observationReportACK(report.ReportId, gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.ReasonDBWriteFailed))
			}
		}
		return ack
	}
	now := time.Now().UTC()
	for _, wire := range batch.Reports {
		if wire == nil || wire.ReportId == "" {
			continue
		}
		report, normalized := observationReportFromProto(wire)
		if normalized {
			slog.Warn("normalized unknown observation reason", "cluster", clusterName, "report_id", wire.ReportId)
		}
		if err := report.Validate(); err != nil {
			slog.Warn("discard invalid observation report", "cluster", clusterName, "report_id", wire.ReportId, "error", err)
			ack.Results = append(ack.Results, observationReportACK(wire.ReportId, gatewaypb.ObservationReportBatchAck_Result_DISCARDED, admissionobs.ReasonInvalidReport))
			continue
		}
		if _, err := s.repos.Observability.ApplyMemberReport(ctx, cluster.ID, cluster.Name, report, now); err != nil {
			slog.Error("store observation report", "cluster", clusterName, "report_id", wire.ReportId, "error", err)
			ack.Results = append(ack.Results, observationReportACK(wire.ReportId, gatewaypb.ObservationReportBatchAck_Result_RETRYABLE, admissionobs.ReasonDBWriteFailed))
			continue
		}
		ack.Results = append(ack.Results, observationReportACK(wire.ReportId, gatewaypb.ObservationReportBatchAck_Result_ACCEPTED, ""))
	}
	return ack
}

func observationReportACK(reportID string, state gatewaypb.ObservationReportBatchAck_Result_Status, reason string) *gatewaypb.ObservationReportBatchAck_Result {
	return &gatewaypb.ObservationReportBatchAck_Result{ReportId: reportID, Status: state, Reason: reason}
}

func observationReportFromProto(report *gatewaypb.ObservationReport) (admissionobs.Report, bool) {
	counters := report.GetCounters()
	snapshot := admissionobs.Snapshot{
		FaultReasonCounts:    cloneReasonCounts(report.FaultReasonCounts),
		PressureReasonCounts: cloneReasonCounts(report.PressureReasonCounts),
		FilterReasonCounts:   cloneReasonCounts(report.FilterReasonCounts),
	}
	if counters != nil {
		snapshot.Counters = admissionobs.Counters{
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
		}
	}
	normalized := admissionobs.NormalizeSnapshot(snapshot)
	changed := !equalReasonCounts(snapshot.FaultReasonCounts, normalized.FaultReasonCounts) ||
		!equalReasonCounts(snapshot.PressureReasonCounts, normalized.PressureReasonCounts) ||
		!equalReasonCounts(snapshot.FilterReasonCounts, normalized.FilterReasonCounts)
	observedAt := time.Time{}
	if report.ObservedAt != nil && report.ObservedAt.IsValid() {
		observedAt = report.ObservedAt.AsTime()
	}
	return admissionobs.Report{
		ReportID:      report.ReportId,
		SeriesID:      report.SeriesId,
		Sequence:      report.Sequence,
		ObservedAt:    observedAt,
		Component:     admissionobs.Component(report.Component),
		Snapshot:      normalized,
		QueueDepthMax: report.QueueDepthMax,
		QueueCapacity: report.QueueCapacity,
	}, changed
}

func cloneReasonCounts(values map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func equalReasonCounts(left, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
