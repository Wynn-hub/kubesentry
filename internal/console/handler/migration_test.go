package handler

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func TestClusterAdapterPreservesLegacyResponse(t *testing.T) {
	h := New(service.Services{LegacyInvoke: func(_ context.Context, method, path string, _ []byte) (int, []byte) {
		if method != "GET" || path != "/api/v1/clusters" {
			t.Fatalf("unexpected request %s %s", method, path)
		}
		return 200, []byte(`{"code":0,"message":"success","data":{"items":[]}}`)
	}})
	res, err := h.ListClusters(context.Background(), dto.ListClustersParams{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.(*dto.ListClustersOK); !ok {
		t.Fatalf("response type %T, want *dto.ListClustersOK", res)
	}
}

func TestClusterAdapterUsesGeneratedErrorBranch(t *testing.T) {
	h := New(service.Services{LegacyInvoke: func(context.Context, string, string, []byte) (int, []byte) {
		return 404, []byte(`{"code":40400,"message":"cluster not found"}`)
	}})
	res, err := h.GetCluster(context.Background(), dto.GetClusterParams{Name: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.(*dto.GetClusterNotFound); !ok {
		t.Fatalf("response type %T, want *dto.GetClusterNotFound", res)
	}
}
