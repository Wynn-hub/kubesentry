package webhook

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
)

// NamespaceCache is a thread-safe in-memory map of namespace name → labels.
// It is populated by an informer event handler and consumed by the Handler to
// evaluate PolicyException namespaceSelector matches.
type NamespaceCache struct {
	mu    sync.RWMutex
	store map[string]map[string]string
	ready bool
}

func NewNamespaceCache() *NamespaceCache {
	return &NamespaceCache{store: map[string]map[string]string{}}
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
// found in the cache. A namespace with no labels returns (empty-map, true).
func (c *NamespaceCache) GetLabels(name string) (map[string]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	labels, ok := c.store[name]
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		out[k] = v
	}
	return out, true
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
