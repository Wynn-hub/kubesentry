package console

import (
	"context"
	"log/slog"

	"github.com/Wynn-hub/kubesentry/internal/console/service"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

type gatewayNotifier struct{ h *Handlers }

func (h *Handlers) notifier() service.Notifier {
	return &gatewayNotifier{h: h}
}

func (n *gatewayNotifier) NotifyChange(ctx context.Context, topic string) {
	if n.h.Gateway == nil {
		return
	}
	traceID := TraceIDFromContext(ctx)
	_, err := n.h.Gateway.NotifyChange(ctx, &gatewaypb.NotifyChangeRequest{
		Topic:   topic,
		TraceId: traceID,
	})
	if err != nil {
		slog.Warn("notify gateway of change", "topic", topic, "trace_id", traceID, "error", err)
	}
}
