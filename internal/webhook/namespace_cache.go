package webhook

import (
	"context"
	"log/slog"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

// NamespaceFetcher returns a Namespace by name, bypassing the local cache.
// Used as a fallback when the informer cache has not yet observed a
// namespace — without this, requests against a freshly-created namespace
// would silently bypass all PolicyGroups whose namespaceSelector is set.
type NamespaceFetcher func(ctx context.Context, name string) (*corev1.Namespace, error)

// NamespaceCache is a thread-safe in-memory map of namespace name → labels.
// It is populated by an informer event handler and consumed by the Handler to
// evaluate PolicyException namespaceSelector matches.
type NamespaceCache struct {
	mu      sync.RWMutex
	store   map[string]map[string]string
	ready   bool
	fetcher NamespaceFetcher
}

func NewNamespaceCache() *NamespaceCache {
	return &NamespaceCache{store: map[string]map[string]string{}}
}

// SetFetcher installs a function used to look up a namespace from the API
// server when the local cache misses. Optional: if unset, cache misses
// remain misses.
func (c *NamespaceCache) SetFetcher(f NamespaceFetcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetcher = f
}

func (c *NamespaceCache) Upsert(ns *corev1.Namespace) {
	if ns == nil {
		return
	}
	labels := make(map[string]string, len(ns.Labels))
	for k, v := range ns.Labels {
		labels[k] = v
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[ns.Name] = labels
}

func (c *NamespaceCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, name)
}

// GetLabels returns the labels for the named namespace and whether it was
// resolved. A namespace with no labels returns (empty-map, true). If the
// local cache misses and a fetcher was installed, it falls back to a live
// API lookup — this closes the race window where a freshly-created
// namespace would otherwise fail-open in MatchingForRequest.
func (c *NamespaceCache) GetLabels(ctx context.Context, name string) (map[string]string, bool) {
	c.mu.RLock()
	labels, ok := c.store[name]
	fetcher := c.fetcher
	c.mu.RUnlock()
	if ok {
		out := make(map[string]string, len(labels))
		for k, v := range labels {
			out[k] = v
		}
		return out, true
	}
	if fetcher == nil {
		return nil, false
	}
	ns, err := fetcher(ctx, name)
	if err != nil {
		slog.Warn("namespace fetch fallback failed", "namespace", name, "error", err)
		return nil, false
	}
	out := make(map[string]string, len(ns.Labels))
	for k, v := range ns.Labels {
		out[k] = v
	}
	// Cache the result so subsequent requests hit the local store, even if
	// the informer hasn't observed the namespace yet.
	c.mu.Lock()
	c.store[name] = out
	c.mu.Unlock()
	cached := make(map[string]string, len(out))
	for k, v := range out {
		cached[k] = v
	}
	return cached, true
}

func (c *NamespaceCache) SetReady() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = true
}

func (c *NamespaceCache) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}
