package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// handleAdmissionEventBatch only trusts the cluster identified by the already
// authenticated Sync stream. The member-provided cluster UID is diagnostic and
// can never select another cluster's storage policy or database ownership.
func (s *Server) handleAdmissionEventBatch(ctx context.Context, clusterName string, batch *gatewaypb.AdmissionEventBatch) (*gatewaypb.AdmissionEventBatchAck, error) {
	ack := &gatewaypb.AdmissionEventBatchAck{}
	if batch == nil || len(batch.Events) == 0 {
		return ack, nil
	}
	if s.repos.Admission == nil {
		return nil, fmt.Errorf("admission event repository is not configured")
	}
	cluster, err := s.repos.Clusters.GetByName(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	settings, err := s.repos.Admission.GetSettings(ctx)
	if err != nil {
		s.recordGatewayIncident(admissionobs.ReasonWriteFailed, 1, err.Error(), time.Now().UTC())
		return nil, err
	}
	policies, err := s.repos.Admission.ListPolicies(ctx)
	if err != nil {
		s.recordGatewayIncident(admissionobs.ReasonWriteFailed, 1, err.Error(), time.Now().UTC())
		return nil, err
	}
	accepted := make([]dbrepo.AdmissionEvent, 0, len(batch.Events))
	for _, in := range batch.Events {
		if in == nil || in.EventId == "" {
			continue
		}
		if in.Verdict != "audit" && in.Verdict != "deny" {
			if s.gatewayIncidentTracker != nil && s.admissionObservationEnabled.Load() {
				s.gatewayIncidentTracker.Record(admissionobs.ReasonBatchFailure, 1, in.EventId, admissionobs.ReasonInvalidEvent, time.Now().UTC())
			}
			ack.Results = append(ack.Results, discardedAdmissionEvent(in, "invalid_event"))
			continue
		}
		occurred := time.Now().UTC()
		if in.OccurredAt != nil {
			occurred = in.OccurredAt.AsTime()
		}
		violations, err := json.Marshal(in.Violations)
		if err != nil {
			return nil, err
		}
		event := dbrepo.AdmissionEvent{
			EventID: in.EventId, RequestUID: in.RequestUid, ClusterID: cluster.ID, ClusterName: cluster.Name,
			OccurredAt: occurred, Verdict: in.Verdict, APIGroup: in.ApiGroup, APIVersion: in.ApiVersion, Resource: in.Resource,
			Operation: in.Operation, Namespace: in.Namespace, ResourceName: in.ResourceName, Username: in.Username,
			UserGroups: pq.StringArray(in.UserGroups), PolicyNames: pq.StringArray(in.PolicyNames), PolicyGroupNames: pq.StringArray(in.PolicyGroupNames),
			Violations: datatypes.JSON(violations), MetadataSnapshot: datatypes.JSON(in.MetadataSnapshot), SpecSnapshot: datatypes.JSON(in.SpecSnapshot), StoragePolicyVersion: in.StoragePolicyVersion,
		}
		if !dbrepo.ShouldStoreAdmissionEvent(policies, event) {
			// Storage rules are an allow-list: matching any rule stores the
			// event, matching none does not. An empty rule set stores all.
			ack.Results = append(ack.Results, discardedAdmissionEvent(in, "storage_policy_no_match"))
			continue
		}
		// A backlogged member can replay events older than the retention window
		// (e.g. after a multi-day outage). Writing them just to have the cleanup
		// worker delete them on its next pass makes the ingest counters disagree
		// with what is actually retained, so drop them up front.
		if !occurred.Add(time.Duration(settings.DefaultRetentionDays) * 24 * time.Hour).After(time.Now().UTC()) {
			ack.Results = append(ack.Results, discardedAdmissionEvent(in, "retention_already_elapsed"))
			continue
		}
		accepted = append(accepted, event)
		ack.Results = append(ack.Results, acceptedAdmissionEvent(in))
	}
	_, _, err = s.repos.Admission.CreateEventsIdempotentCount(ctx, accepted)
	if err != nil {
		s.recordGatewayIncident(admissionobs.ReasonWriteFailed, int64(len(accepted)), err.Error(), time.Now().UTC())
		return ack, err
	}
	return ack, nil
}

func acceptedAdmissionEvent(event *gatewaypb.AdmissionEvent) *gatewaypb.AdmissionEventBatchAck_Result {
	return &gatewaypb.AdmissionEventBatchAck_Result{EventId: event.EventId, Status: gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED, DeliveryAttempt: event.DeliveryAttempt}
}

func discardedAdmissionEvent(event *gatewaypb.AdmissionEvent, reason string) *gatewaypb.AdmissionEventBatchAck_Result {
	return &gatewaypb.AdmissionEventBatchAck_Result{EventId: event.EventId, Status: gatewaypb.AdmissionEventBatchAck_Result_DISCARDED, Reason: reason, DeliveryAttempt: event.DeliveryAttempt}
}

func admissionEventFailureAck(ack *gatewaypb.AdmissionEventBatchAck, batch *gatewaypb.AdmissionEventBatch, reason string) *gatewaypb.AdmissionEventBatchAck {
	if ack == nil {
		ack = &gatewaypb.AdmissionEventBatchAck{}
	}
	existing := make(map[string]*gatewaypb.AdmissionEventBatchAck_Result, len(ack.Results))
	for _, result := range ack.Results {
		if result == nil {
			continue
		}
		existing[result.EventId] = result
		if result.Status == gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED {
			result.Status = gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE
			result.Reason = reason
		}
	}
	if batch == nil {
		return ack
	}
	for _, event := range batch.Events {
		if event == nil || event.EventId == "" {
			continue
		}
		if _, ok := existing[event.EventId]; ok {
			continue
		}
		ack.Results = append(ack.Results, &gatewaypb.AdmissionEventBatchAck_Result{
			EventId:         event.EventId,
			Status:          gatewaypb.AdmissionEventBatchAck_Result_RETRYABLE,
			Reason:          reason,
			DeliveryAttempt: event.DeliveryAttempt,
		})
	}
	return ack
}
