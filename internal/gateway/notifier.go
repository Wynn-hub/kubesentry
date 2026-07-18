package gateway

import "sync"

// Notifier is a broadcast "something changed" signal: Notify closes the
// current channel and installs a fresh one, so every pending Wait fires.
type Notifier struct {
	mu      sync.Mutex
	ch      chan struct{}
	traceID string
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

// NotifyWithTrace stashes traceID for the next lastTraceID peek, then
// notifies exactly like Notify. The plain 60s resync ticker path keeps
// calling Notify directly, so it never stashes a trace_id.
func (n *Notifier) NotifyWithTrace(traceID string) {
	n.mu.Lock()
	n.traceID = traceID
	n.mu.Unlock()
	n.Notify()
}

// lastTraceID is a non-destructive peek at the currently stashed trace_id.
// Notify() broadcasts to every connected cluster's pushLoop goroutine at
// once, and each independently decides (based on whether ITS OWN render hash
// actually changed) whether to attach this value to its outgoing
// DesiredState — see pushLoop in server.go. A read-then-clear accessor would
// make that decision a race: whichever goroutine wins the mutex consumes the
// trace_id, even if that goroutine has nothing to send, silently starving
// the goroutine that actually has the real content change. Peeking instead
// lets every goroutine see the same value and gate on its own state.
//
// ponytail: a rapid second NotifyWithTrace can overwrite this stash before a
// slower pushLoop iteration reads it — "most recent trigger wins" for
// back-to-back changes. Accepted tradeoff, not a bug: a queue/history of
// trace_ids is more machinery than this needs.
func (n *Notifier) lastTraceID() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.traceID
}

// Wait returns a channel closed by the next Notify.
func (n *Notifier) Wait() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ch
}
