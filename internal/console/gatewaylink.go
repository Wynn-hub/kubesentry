// internal/console/gatewaylink.go
package console

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// gatewayLinkMaxBackoff caps RunGatewayLink's reconnect backoff, mirroring
// internal/operator/hublink's maxBackoff.
const gatewayLinkMaxBackoff = 60 * time.Second

// gatewayLinkInitialBackoff and gatewayLinkMinHealthyDuration are vars (not
// consts) purely so tests can shrink them and avoid waiting on real timers;
// production behavior is unaffected by their being mutable.
var (
	gatewayLinkInitialBackoff     = time.Second
	gatewayLinkMinHealthyDuration = 3 * time.Second
)

// RunGatewayLink maintains console-api's Connect registration stream to the
// gateway (spec D11c). This is a soft dependency: it retries forever with
// exponential backoff and only returns when ctx is canceled, so it must run
// in its own goroutine — a temporarily unreachable gateway must never block
// console-api's own HTTP readiness.
func RunGatewayLink(ctx context.Context, client gatewaypb.HubClient, componentID, sharedToken string) {
	RunGatewayLinkWithSamples(ctx, client, componentID, sharedToken, nil)
}

func RunGatewayLinkWithSamples(
	ctx context.Context,
	client gatewaypb.HubClient,
	componentID, sharedToken string,
	samples func() *gatewaypb.ComponentSampleBatch,
) {
	backoff := gatewayLinkInitialBackoff
	for {
		connected := runGatewayLinkOnceWithSamples(ctx, client, componentID, sharedToken, samples)
		if ctx.Err() != nil {
			return
		}
		if connected {
			// The stream proved itself alive/accepted (see
			// runGatewayLinkOnce) — not a persistent failure, so start the
			// next retry fast, same as hublink.
			backoff = gatewayLinkInitialBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > gatewayLinkMaxBackoff {
			backoff = gatewayLinkMaxBackoff
		}
	}
}

// runGatewayLinkOnce opens one Connect stream, sends the ConnectRequest
// handshake, and blocks on Recv until the stream breaks. The gateway's
// Connect handler (internal/gateway/server.go) never sends any payload back
// on a successful registration — it just registers the caller and blocks on
// its own Recv — so there is no "ack" frame to wait for. The returned bool
// therefore reports:
//   - false immediately on Unauthenticated/FailedPrecondition (the gateway's
//     specific rejection of a bad/missing/unconfigured shared token): this
//     can never resolve itself by retrying sooner, so the caller must keep
//     backing off instead of hot-looping;
//   - false if the stream broke before surviving gatewayLinkMinHealthyDuration
//     (handshake-sent-but-unproven, e.g. a network blip right after Send);
//   - true only once the stream has stayed open that long, which is the best
//     available proxy for "genuinely connected" absent any server ack.
func runGatewayLinkOnce(ctx context.Context, client gatewaypb.HubClient, componentID, sharedToken string) bool {
	return runGatewayLinkOnceWithSamples(ctx, client, componentID, sharedToken, nil)
}

func runGatewayLinkOnceWithSamples(
	ctx context.Context,
	client gatewaypb.HubClient,
	componentID, sharedToken string,
	samples func() *gatewaypb.ComponentSampleBatch,
) bool {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.Connect(sessionCtx)
	if err != nil {
		slog.Warn("open gateway connect stream, retrying", "error", err)
		return false
	}
	payload, err := proto.Marshal(&gatewaypb.ConnectRequest{ComponentId: componentID, SharedToken: sharedToken})
	if err != nil {
		slog.Error("marshal gateway connect request", "error", err)
		return false
	}
	if err := stream.Send(&gatewaypb.Envelope{Payload: payload}); err != nil {
		slog.Warn("register with gateway, retrying", "error", err)
		return false
	}
	slog.Info("sent gateway registration handshake", "component", componentID)
	start := time.Now()
	recvErr := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				recvErr <- err
				return
			}
		}
	}()
	ticker := time.NewTicker(platformSampleInterval)
	defer ticker.Stop()
	sendSamples := func() error {
		if samples == nil {
			return nil
		}
		batch := samples()
		if batch == nil || len(batch.Samples) == 0 {
			return nil
		}
		payload, err := proto.Marshal(batch)
		if err != nil {
			return err
		}
		return stream.Send(&gatewaypb.Envelope{
			From: componentID, To: "platform-observability", Payload: payload,
		})
	}
	if err := sendSamples(); err != nil {
		slog.Warn("send initial console platform samples", "error", err)
		return false
	}
	for {
		select {
		case <-ctx.Done():
			return true
		case err := <-recvErr:
			if code := status.Code(err); code == codes.Unauthenticated || code == codes.FailedPrecondition {
				slog.Error("gateway rejected registration, will keep backing off", "component", componentID, "error", err)
				return false
			}
			healthy := time.Since(start) >= gatewayLinkMinHealthyDuration
			slog.Warn("gateway connect stream closed, reconnecting", "error", err, "healthy", healthy)
			return healthy
		case <-ticker.C:
			if err := sendSamples(); err != nil {
				slog.Warn("send console platform samples", "error", err)
				return time.Since(start) >= gatewayLinkMinHealthyDuration
			}
		}
	}
}
