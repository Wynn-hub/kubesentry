package gateway

import (
	"testing"
	"time"
)

func TestNotifierBroadcastsToAllWaiters(t *testing.T) {
	n := NewNotifier()
	w1, w2 := n.Wait(), n.Wait()
	n.Notify()
	for _, w := range []<-chan struct{}{w1, w2} {
		select {
		case <-w:
		case <-time.After(time.Second):
			t.Fatal("waiter not released")
		}
	}
	select {
	case <-n.Wait():
		t.Fatal("fresh channel must block until next Notify")
	default:
	}
}
