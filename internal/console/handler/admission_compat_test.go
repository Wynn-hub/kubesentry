package handler

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestAdmissionListOptionsPreserveDefaultsAndFilters(t *testing.T) {
	from := time.Date(2026, 9, 10, 1, 2, 3, 0, time.UTC)
	to := from.Add(time.Hour)
	params := dto.ListAdmissionEventsParams{Cluster: []string{"member-1"}, Verdict: []dto.ListAdmissionEventsVerdictItem{dto.ListAdmissionEventsVerdictItemDeny}, Policy: dto.NewOptString("host-ipc"), Namespace: []string{"default"}, Gvk: []string{"pods"}, ResourceName: dto.NewOptString("pod-1"), Username: dto.NewOptString("alice"), From: dto.NewOptDateTime(from), To: dto.NewOptDateTime(to)}
	got := admissionListOptions(params)
	want := dto.AdmissionEventListOptions{Page: 1, PageSize: 20, SortBy: "occurredAt", SortOrder: "desc", Clusters: []string{"member-1"}, Verdicts: []string{"deny"}, PolicyName: "host-ipc", Namespaces: []string{"default"}, GVRs: []string{"pods"}, ResourceName: "pod-1", Username: "alice", From: &from, To: &to}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mapped options = %+v", got)
	}
}

func TestAdmissionDetailResponsePreservesJSONShape(t *testing.T) {
	occurredAt := time.Date(2026, 9, 10, 1, 2, 3, 0, time.UTC)
	response := dto.AdmissionEventDetailHTTPResponse{Data: &dto.AdmissionEventDetailView{
		AdmissionEventItemView: dto.AdmissionEventItemView{OccurredAt: occurredAt, PolicyNames: []string{}, Violations: []string{}},
		UserGroups:             []string{}, PolicyGroupNames: []string{},
		MetadataSnapshot: map[string]json.RawMessage{"labels": json.RawMessage(`{"nested":true}`)},
		SpecSnapshot:     map[string]json.RawMessage{},
	}}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"policyNames", "violations", "userGroups", "policyGroupNames"} {
		if string(envelope.Data[field]) != "[]" {
			t.Fatalf("%s = %s, want []", field, envelope.Data[field])
		}
	}
	if got := string(envelope.Data["occurredAt"]); got != `"2026-09-10T01:02:03Z"` {
		t.Fatalf("occurredAt = %s", got)
	}
	if got := string(envelope.Data["metadataSnapshot"]); got != `{"labels":{"nested":true}}` {
		t.Fatalf("metadataSnapshot = %s", got)
	}
}
