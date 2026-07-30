package gateway

import (
	"sync"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// sessionRegistry tracks live Sync sessions by cluster name, so Dispatch can
// look one up by the literal `to` the caller gave — gateway does no
// destination resolution here (spec D11a: Dispatch is a dumb relay).
type sessionRegistry struct {
	mu   sync.RWMutex
	byID map[string]map[string]sessionEntry
}

type sessionEntry struct {
	session   *syncSession
	role      string
	updatedAt time.Time
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{byID: map[string]map[string]sessionEntry{}}
}

// put indexes sessions by cluster and operator instance. If more than one
// instance claims leadership, the newest claim wins and the previous leader
// sessions are returned so their DesiredState push loops can be stopped.
func (r *sessionRegistry) put(cluster, instance, role string, s *syncSession) []*syncSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	instances := r.byID[cluster]
	if instances == nil {
		instances = map[string]sessionEntry{}
		r.byID[cluster] = instances
	}
	now := time.Now()
	instances[instance] = sessionEntry{session: s, role: role, updatedAt: now}
	if role != "leader" {
		return nil
	}
	var demoted []*syncSession
	for key, entry := range instances {
		if key == instance || entry.role != "leader" {
			continue
		}
		entry.role = "standby"
		instances[key] = entry
		demoted = append(demoted, entry.session)
	}
	return demoted
}

func (r *sessionRegistry) leader(cluster string) (*syncSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var selected sessionEntry
	var found bool
	for _, entry := range r.byID[cluster] {
		if entry.role == "leader" && (!found || entry.updatedAt.After(selected.updatedAt)) {
			selected, found = entry, true
		}
	}
	return selected.session, found
}

// get is retained for Dispatch-facing call sites; only the authoritative
// leader is a routable session.
func (r *sessionRegistry) get(cluster string) (*syncSession, bool) {
	return r.leader(cluster)
}

// remove is compare-and-delete: it only removes the entry if it still holds
// sess. A disconnect/reconnect race can otherwise let a stale Sync's deferred
// remove(cluster) evict the new, live session that already replaced it.
func (r *sessionRegistry) remove(cluster, instance string, sess *syncSession) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	instances := r.byID[cluster]
	entry, ok := instances[instance]
	if !ok || entry.session != sess {
		return false
	}
	wasLeader := entry.role == "leader"
	delete(instances, instance)
	if len(instances) == 0 {
		delete(r.byID, cluster)
	}
	return wasLeader
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

// unregister is compare-and-delete, same rationale as sessionRegistry.remove.
func (r *connectRegistry) unregister(id string, stream gatewaypb.Hub_ConnectServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID[id] == stream {
		delete(r.byID, id)
	}
}

func (r *connectRegistry) get(id string) (gatewaypb.Hub_ConnectServer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	return s, ok
}
