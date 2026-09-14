package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStableDTOJSONCompatibility(t *testing.T) {
	timestamp := time.Date(2026, time.September, 11, 3, 4, 5, 0, time.UTC)
	data, err := json.Marshal(ClusterItem{Name: "member-1", CreatedAt: timestamp})
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"createdAt":"2026-09-11T03:04:05Z"`) {
		t.Fatalf("createdAt is not RFC3339: %s", got)
	}
	if strings.Contains(got, "lastHeartbeatAt") {
		t.Fatalf("nil optional time was not omitted: %s", got)
	}

	data, err = json.Marshal(DistributionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, `"name"`) || strings.Contains(got, `"resourceVersion"`) {
		t.Fatalf("empty optional strings were not omitted: %s", got)
	}

	data, err = json.Marshal(SummaryView{Groups: []SummaryGroupView{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"groups":[]`) {
		t.Fatalf("empty groups must remain an array: %s", data)
	}
}

func TestIncidentSampleDatabaseIDOmission(t *testing.T) {
	data, err := json.Marshal(IncidentSampleView{EventID: "expired", Expired: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "eventDatabaseId") {
		t.Fatalf("expired sample database ID must be omitted: %s", data)
	}

	id := "42"
	data, err = json.Marshal(IncidentSampleView{EventID: "stored", EventDatabaseID: &id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"eventDatabaseId":"42"`) {
		t.Fatalf("stored sample database ID must be a string: %s", data)
	}
}

func TestPlatformDTOJSONCompatibility(t *testing.T) {
	data, err := json.Marshal(PlatformOverviewView{
		GatewayPlane: []PlatformComponentView{},
		MemberPlane: PlatformMemberPlaneView{
			Clusters: []PlatformClusterView{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"gatewayPlane":[]`) || !strings.Contains(got, `"clusters":[]`) {
		t.Fatalf("empty platform collections must remain arrays: %s", got)
	}
	if strings.Contains(got, "lastLeadershipChange") || strings.Contains(got, "dbStats") {
		t.Fatalf("nil platform optionals must be omitted: %s", got)
	}
}
