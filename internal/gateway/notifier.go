package gateway

import "sync"

// Notifier is a broadcast "something changed" signal: Notify closes the
// current channel and installs a fresh one, so every pending Wait fires.
type Notifier struct {
	mu          sync.Mutex
	ch          chan struct{}
	lastTraceID string
}

func NewNotifier() *Notifier {
	return &Notifier{ch: make(chan struct{})}
}

func (n *Notifier) Notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	close(n.ch)
	n.ch = make(chan struct{})
}

// NotifyWithTrace stashes traceID for the next consumeTraceID call, then
// notifies exactly like Notify. The plain 60s resync ticker path keeps
// calling Notify directly, so it never carries a trace_id.
func (n *Notifier) NotifyWithTrace(traceID string) {
	n.mu.Lock()
	n.lastTraceID = traceID
	n.mu.Unlock()
	n.Notify()
}

// consumeTraceID returns and clears the stashed trace_id — read-once, so a
// later resync tick (which never calls NotifyWithTrace) naturally observes
// an empty string instead of a stale trace_id from a prior change.
func (n *Notifier) consumeTraceID() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	id := n.lastTraceID
	n.lastTraceID = ""
	return id
}

// Wait returns a channel closed by the next Notify.
func (n *Notifier) Wait() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ch
}
