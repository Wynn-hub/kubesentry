package console

import (
	"context"
	"net"
	"sync"
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
	orig := gatewayLinkMinHealthyDuration
	gatewayLinkMinHealthyDuration = 50 * time.Millisecond
	defer func() { gatewayLinkMinHealthyDuration = orig }()

	srv := gateway.NewServer(gateway.Repos{}, "test-ns", gateway.NewHeartbeatTracker(), gateway.NewNotifier(), "right-token")
	client := startBufconnGateway(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if connected := runGatewayLinkOnce(ctx, client, "console-api", "right-token"); !connected {
		t.Fatal("expected connected=true once the stream outlives gatewayLinkMinHealthyDuration")
	}
}

func TestRunGatewayLinkOnceReturnsNotConnectedOnBadToken(t *testing.T) {
	srv := gateway.NewServer(gateway.Repos{}, "test-ns", gateway.NewHeartbeatTracker(), gateway.NewNotifier(), "right-token")
	client := startBufconnGateway(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Send succeeds locally regardless of token validity; the gateway's
	// Unauthenticated rejection only surfaces on the following Recv. The
	// fixed function must report connected=false here so RunGatewayLink
	// keeps backing off instead of hot-retrying an auth failure that can
	// never resolve itself (see runGatewayLinkOnce's doc comment) — it must
	// not panic or hang either way.
	if connected := runGatewayLinkOnce(ctx, client, "console-api", "wrong-token"); connected {
		t.Fatal("expected connected=false for a rejected (Unauthenticated) token")
	}
}

// countingHubClient wraps a real gatewaypb.HubClient and counts Connect
// calls, letting a test observe RunGatewayLink's retry cadence from the
// outside instead of reaching into its private backoff variable.
type countingHubClient struct {
	gatewaypb.HubClient
	mu       sync.Mutex
	connects int
}

func (c *countingHubClient) Connect(ctx context.Context, opts ...grpc.CallOption) (gatewaypb.Hub_ConnectClient, error) {
	c.mu.Lock()
	c.connects++
	c.mu.Unlock()
	return c.HubClient.Connect(ctx, opts...)
}

func (c *countingHubClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connects
}

func TestRunGatewayLinkKeepsBackingOffOnRepeatedAuthRejection(t *testing.T) {
	origInitial := gatewayLinkInitialBackoff
	gatewayLinkInitialBackoff = 20 * time.Millisecond
	defer func() { gatewayLinkInitialBackoff = origInitial }()

	srv := gateway.NewServer(gateway.Repos{}, "test-ns", gateway.NewHeartbeatTracker(), gateway.NewNotifier(), "right-token")
	counting := &countingHubClient{HubClient: startBufconnGateway(t, srv)}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	RunGatewayLink(ctx, counting, "console-api", "wrong-token")

	got := counting.count()
	// Every attempt is rejected (Unauthenticated) instantly, so a buggy
	// implementation that resets backoff to gatewayLinkInitialBackoff (20ms)
	// on every attempt would retry roughly every 20ms — about 300/20 = 15
	// attempts in this window. A correctly growing backoff (20, 40, 80,
	// 160, ...ms) yields only a handful.
	if got > 8 {
		t.Fatalf("expected backoff to keep growing across rejected attempts, got %d connect attempts in 300ms (want <= 8)", got)
	}
	if got < 2 {
		t.Fatalf("expected at least a couple of attempts, got %d", got)
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
