// internal/console/gatewaylink.go
package console

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// gatewayLinkMaxBackoff caps RunGatewayLink's reconnect backoff, mirroring
// internal/operator/hublink's maxBackoff.
const gatewayLinkMaxBackoff = 60 * time.Second

// RunGatewayLink maintains console-api's Connect registration stream to the
// gateway (spec D11c). This is a soft dependency: it retries forever with
// exponential backoff and only returns when ctx is canceled, so it must run
// in its own goroutine — a temporarily unreachable gateway must never block
// console-api's own HTTP readiness.
func RunGatewayLink(ctx context.Context, client gatewaypb.HubClient, componentID, sharedToken string) {
	backoff := time.Second
	for {
		connected := runGatewayLinkOnce(ctx, client, componentID, sharedToken)
		if ctx.Err() != nil {
			return
		}
		if connected {
			// A session that reached the server is not a persistent
			// failure — start the next retry fast, same as hublink.
			backoff = time.Second
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
// handshake, and blocks on Recv until the stream breaks. The returned bool
// reports whether the handshake frame was sent (used only to decide whether
// the caller resets its backoff) — it is not proof the gateway accepted the
// token, since a rejection surfaces asynchronously on the following Recv.
func runGatewayLinkOnce(ctx context.Context, client gatewaypb.HubClient, componentID, sharedToken string) bool {
	stream, err := client.Connect(ctx)
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
	slog.Info("registered with gateway", "component", componentID)
	for {
		if _, err := stream.Recv(); err != nil {
			slog.Warn("gateway connect stream closed, reconnecting", "error", err)
			return true
		}
	}
}
