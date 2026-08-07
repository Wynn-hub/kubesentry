package service

import "context"

// Notifier is the service layer's only dependency on the gateway link.
// The implementation lives in the console root package (it needs the trace
// id helpers there); service code depends on the interface so it stays free
// of gatewaypb and HTTP types. A nil Notifier means "no gateway configured"
// and every notification is silently skipped — mirroring notifyChange's
// soft-dependency semantics.
type Notifier interface {
	NotifyChange(ctx context.Context, topic string)
}
