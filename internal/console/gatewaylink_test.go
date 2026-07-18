package console

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// startBufconnGateway starts a real gateway.Server in-process over bufconn
// and returns a client dialed to it — mirrors the pattern already used by
// internal/gateway's own tests and internal/operator/hublink's tests.
// Connect/NotifyChange never touch repos, so a zero-value gateway.Repos{} is
// enough here (no Postgres testcontainer needed).
func startBufconnGateway(t *testing.T, srv *gateway.Server) gatewaypb.HubClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	gatewaypb.RegisterHubServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return gatewaypb.NewHubClient(conn)
}

func TestRunGatewayLinkOnceRegistersWithValidToken(t *testing.T) {
	srv := gateway.NewServer(gateway.Repos{}, "test-ns", gateway.NewHeartbeatTracker(), gateway.NewNotifier(), "right-token")
	client := startBufconnGateway(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if connected := runGatewayLinkOnce(ctx, client, "console-api", "right-token"); !connected {
		t.Fatal("expected connected=true for a valid token")
	}
}

func TestRunGatewayLinkOnceHandshakesEvenWithBadToken(t *testing.T) {
	srv := gateway.NewServer(gateway.Repos{}, "test-ns", gateway.NewHeartbeatTracker(), gateway.NewNotifier(), "right-token")
	client := startBufconnGateway(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Send succeeds locally regardless of token validity; the gateway's
	// Unauthenticated rejection only surfaces on the following Recv, so the
	// function still returns connected=true (see runGatewayLinkOnce's
	// doc comment) — it must not panic or hang.
	if connected := runGatewayLinkOnce(ctx, client, "console-api", "wrong-token"); !connected {
		t.Fatal("expected connected=true (heuristic: stream reached the server)")
	}
}

func TestRunGatewayLinkStopsOnContextCancel(t *testing.T) {
	srv := gateway.NewServer(gateway.Repos{}, "test-ns", gateway.NewHeartbeatTracker(), gateway.NewNotifier(), "right-token")
	client := startBufconnGateway(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunGatewayLink(ctx, client, "console-api", "right-token")
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunGatewayLink did not stop after context cancel")
	}
}
