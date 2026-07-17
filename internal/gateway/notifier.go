package gateway

import "sync"

// Notifier is a broadcast "something changed" signal: Notify closes the
// current channel and installs a fresh one, so every pending Wait fires.
type Notifier struct {
	mu sync.Mutex
	ch chan struct{}
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

// Wait returns a channel closed by the next Notify.
func (n *Notifier) Wait() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ch
}
