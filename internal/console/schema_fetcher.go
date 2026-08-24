package console

import (
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/openapi"
	"k8s.io/client-go/openapi3"
)

const schemaCacheTTL = 10 * time.Minute

// schemaFetcher resolves the field tree for a given apiGroup/version/resource
// plural name. Defined narrowly (vs. the full discovery.DiscoveryInterface)
// so tests can fake it without touching real k8s discovery types.
type schemaFetcher interface {
	FieldsFor(group, version, resource string) ([]FieldNode, error)
}

// discoveryInterface is the slice of discovery.DiscoveryInterface this
// package actually needs.
type discoveryInterface interface {
	OpenAPIV3() openapi.Client
	ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error)
}

// discoverySchemaFetcher implements schemaFetcher against a live cluster.
type discoverySchemaFetcher struct {
	Discovery discoveryInterface
}

func (f *discoverySchemaFetcher) FieldsFor(group, version, resource string) ([]FieldNode, error) {
	if f.Discovery == nil {
		return nil, fmt.Errorf("discovery client not configured")
	}
	gv := schema.GroupVersion{Group: group, Version: version}
	list, err := f.Discovery.ServerResourcesForGroupVersion(gv.String())
	if err != nil {
		return nil, fmt.Errorf("list server resources for %s: %w", gv.String(), err)
	}
	kind := ""
	for _, r := range list.APIResources {
		if r.Name == resource {
			kind = r.Kind
			break
		}
	}
	if kind == "" {
		return nil, fmt.Errorf("resource %q not found in %s", resource, gv.String())
	}

	root := openapi3.NewRoot(f.Discovery.OpenAPIV3())
	doc, err := root.GVSpec(gv)
	if err != nil {
		return nil, fmt.Errorf("fetch openapi v3 schema for %s: %w", gv.String(), err)
	}
	return buildFieldTree(doc, group, version, kind)
}

type schemaCacheEntry struct {
	fields    []FieldNode
	expiresAt time.Time
}

func (h *Handlers) schemaCache() *sync.Map {
	h.schemaCacheOnce.Do(func() { h.schemaCacheMap = &sync.Map{} })
	return h.schemaCacheMap
}
