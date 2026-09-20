package handler

import (
	"reflect"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestPlatformOptionsPreserveDefaultsAndFilters(t *testing.T) {
	from := time.Date(2026, 9, 14, 1, 2, 3, 0, time.UTC)
	to := from.Add(time.Hour)
	overview := platformOverviewOptions(dto.GetPlatformOverviewParams{Health: dto.NewOptGetPlatformOverviewHealth(dto.GetPlatformOverviewHealthDegraded)})
	if want := (dto.PlatformOverviewOptions{Page: 1, PageSize: 20, Health: "degraded"}); !reflect.DeepEqual(overview, want) {
		t.Fatalf("overview options = %+v, want %+v", overview, want)
	}

	trend := platformTrendOptions(dto.GetPlatformInstanceTrendParams{Component: dto.GetPlatformInstanceTrendComponentGateway, Instance: "gateway-0", Cluster: dto.NewOptString("member-1"), From: dto.NewOptDateTime(from), To: dto.NewOptDateTime(to)})
	if want := (dto.PlatformTrendOptions{Component: "gateway", Instance: "gateway-0", Cluster: "member-1", From: &from, To: &to}); !reflect.DeepEqual(trend, want) {
		t.Fatalf("trend options = %+v, want %+v", trend, want)
	}

	leadership := platformLeadershipOptions(dto.ListPlatformLeadershipEventsParams{Page: dto.NewOptInt(2), PageSize: dto.NewOptPageSize(dto.PageSize50), Component: dto.NewOptListPlatformLeadershipEventsComponent(dto.ListPlatformLeadershipEventsComponentOperator), Cluster: dto.NewOptString("member-1"), From: dto.NewOptDateTime(from), To: dto.NewOptDateTime(to)})
	if want := (dto.PlatformLeadershipOptions{Page: 2, PageSize: 50, Component: "operator", Cluster: "member-1", From: &from, To: &to}); !reflect.DeepEqual(leadership, want) {
		t.Fatalf("leadership options = %+v, want %+v", leadership, want)
	}
}
