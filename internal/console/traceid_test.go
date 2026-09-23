package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"google.golang.org/grpc"

	"github.com/Wynn-hub/kubesentry/internal/database/model"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestTraceIDFromRequestReusesHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Request-Id", "req-123")
	if got := TraceIDFromRequest(r); got != "req-123" {
		t.Errorf("got %q, want req-123", got)
	}
}

func TestTraceIDFromRequestGeneratesWhenMissing(t *testing.T) {
	if err := model.Init(1); err != nil {
		t.Fatalf("model.Init: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	got := TraceIDFromRequest(r)
	if got == "" {
		t.Fatal("expected a generated trace id")
	}
}

func TestTraceIDFromRequestGeneratesUniqueIDs(t *testing.T) {
	if err := model.Init(1); err != nil {
		t.Fatalf("model.Init: %v", err)
	}
	id1 := TraceIDFromRequest(httptest.NewRequest(http.MethodPost, "/", nil))
	id2 := TraceIDFromRequest(httptest.NewRequest(http.MethodPost, "/", nil))
	if id1 == id2 {
		t.Fatalf("expected unique trace ids, got %q twice", id1)
	}
}

// fakeHubClient is a minimal gatewaypb.HubClient stub used by API notification
// tests. Everything except NotifyChange panics if called.
type fakeHubClient struct {
	notifyChangeErr error

	mu     sync.Mutex
	topics []string
	traces []string
}

func (f *fakeHubClient) Register(context.Context, *gatewaypb.RegisterRequest, ...grpc.CallOption) (*gatewaypb.RegisterResponse, error) {
	panic("not stubbed")
}

func (f *fakeHubClient) Sync(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[gatewaypb.MemberMessage, gatewaypb.HubMessage], error) {
	panic("not stubbed")
}

func (f *fakeHubClient) Connect(context.Context, ...grpc.CallOption) (grpc.BidiStreamingClient[gatewaypb.Envelope, gatewaypb.Envelope], error) {
	panic("not stubbed")
}

func (f *fakeHubClient) NotifyChange(ctx context.Context, in *gatewaypb.NotifyChangeRequest, _ ...grpc.CallOption) (*gatewaypb.NotifyChangeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.topics = append(f.topics, in.Topic)
	f.traces = append(f.traces, in.TraceId)
	if f.notifyChangeErr != nil {
		return nil, f.notifyChangeErr
	}
	return &gatewaypb.NotifyChangeResponse{}, nil
}

func (f *fakeHubClient) Dispatch(context.Context, *gatewaypb.DispatchRequest, ...grpc.CallOption) (*gatewaypb.DispatchResponse, error) {
	panic("not stubbed")
}

func (f *fakeHubClient) calls() ([]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.topics...), append([]string(nil), f.traces...)
}
