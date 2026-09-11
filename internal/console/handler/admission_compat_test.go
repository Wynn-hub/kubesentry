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

func TestAdmissionStableDTOsAdaptToOgen(t *testing.T) {
	raw := json.RawMessage(`{"nested":true}`)
	detail := dto.AdmissionEventDetailView{AdmissionEventItemView: dto.AdmissionEventItemView{ID: "42", PolicyNames: []string{}, Violations: []string{}}, UserGroups: []string{}, PolicyGroupNames: []string{}, MetadataSnapshot: map[string]json.RawMessage{"labels": raw}, SpecSnapshot: map[string]json.RawMessage{}}
	converted, err := dtoData[dto.AdmissionEventDetail](detail)
	if err != nil {
		t.Fatal(err)
	}
	if converted.ID != "42" || len(converted.PolicyNames) != 0 || len(converted.UserGroups) != 0 || string(converted.MetadataSnapshot["labels"]) != string(raw) {
		t.Fatalf("converted detail = %+v", converted)
	}
	if _, err := dtoData[[]dto.AdmissionEventTrendItem]([]dto.AdmissionTrendItemView{}); err != nil {
		t.Fatal(err)
	}
}
