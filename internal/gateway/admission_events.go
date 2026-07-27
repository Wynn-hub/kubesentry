package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"

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
		return nil, err
	}
	policies, err := s.repos.Admission.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	accepted := make([]dbrepo.AdmissionEvent, 0, len(batch.Events))
	for _, in := range batch.Events {
		if in == nil || in.EventId == "" || (in.Verdict != "audit" && in.Verdict != "deny") {
			continue // omitted from Ack: sender retries malformed/transient frames only after reconnect
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
		if p := dbrepo.MatchAdmissionStoragePolicy(policies, event); p != nil && p.Action == dbrepo.AdmissionStorageActionDiscard {
			ack.Results = append(ack.Results, discardedAdmissionEvent(in.EventId, "storage_policy_no_retain"))
			continue
		}
		_ = settings // default retention is consumed by the cleanup worker, not persisted per event.
		accepted = append(accepted, event)
		ack.Results = append(ack.Results, acceptedAdmissionEvent(in.EventId))
	}
	if err := s.repos.Admission.CreateEventsIdempotent(ctx, accepted); err != nil {
		return nil, err
	}
	return ack, nil
}

func acceptedAdmissionEvent(id string) *gatewaypb.AdmissionEventBatchAck_Result {
	return &gatewaypb.AdmissionEventBatchAck_Result{EventId: id, Status: gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED}
}

func discardedAdmissionEvent(id, reason string) *gatewaypb.AdmissionEventBatchAck_Result {
	return &gatewaypb.AdmissionEventBatchAck_Result{EventId: id, Status: gatewaypb.AdmissionEventBatchAck_Result_DISCARDED, Reason: reason}
}
