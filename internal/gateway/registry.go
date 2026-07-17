package gateway

import (
	"sync"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// sessionRegistry tracks live Sync sessions by cluster name, so Dispatch can
// look one up by the literal `to` the caller gave — gateway does no
// destination resolution here (spec D11a: Dispatch is a dumb relay).
type sessionRegistry struct {
	mu   sync.RWMutex
	byID map[string]*syncSession
}

func newSessionRegistry() *sessionRegistry { return &sessionRegistry{byID: map[string]*syncSession{}} }

func (r *sessionRegistry) put(name string, s *syncSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[name] = s
}

func (r *sessionRegistry) get(name string) (*syncSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[name]
	return s, ok
}

func (r *sessionRegistry) remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, name)
}

// connectRegistry tracks live Connect streams by component id (e.g.
// "console-api"). No consumer reads it yet in this task — it's the
// bookkeeping side of Connect's registration handshake, laid down so a
// later task (console NotifyChange/Connect wiring) can push Envelopes to a
// connected component the same way sessionRegistry lets Dispatch reach a
// cluster's Sync stream.
type connectRegistry struct {
	mu   sync.RWMutex
	byID map[string]gatewaypb.Hub_ConnectServer
}

func newConnectRegistry() *connectRegistry {
	return &connectRegistry{byID: map[string]gatewaypb.Hub_ConnectServer{}}
}

func (r *connectRegistry) register(id string, stream gatewaypb.Hub_ConnectServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[id] = stream
}

func (r *connectRegistry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}

func (r *connectRegistry) get(id string) (gatewaypb.Hub_ConnectServer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	return s, ok
}
