package console

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

const (
	gvkExtensionKey = "x-kubernetes-group-version-kind"
	maxSchemaDepth  = 6
)

// FieldNode is one node of the field tree returned by GET /api/v1/schema/fields,
// used by the console frontend to drive the cascading field selector.
type FieldNode struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"` // string|integer|number|boolean|array|object
	IsArray  bool        `json:"isArray"`
	IsMap    bool        `json:"isMap"`
	Children []FieldNode `json:"children,omitempty"`
}

// buildFieldTree resolves the root schema for the given GVK among schemas
// and returns its top-level fields as a depth-limited tree.
func buildFieldTree(doc *spec3.OpenAPI, group, version, kind string) ([]FieldNode, error) {
	if doc == nil || doc.Components == nil {
		return nil, fmt.Errorf("openapi document has no components")
	}
	root, err := findSchemaByGVK(doc.Components.Schemas, group, version, kind)
	if err != nil {
		return nil, err
	}
	node := schemaToFieldNode("", root, doc.Components.Schemas, 0)
	return node.Children, nil
}

// findSchemaByGVK scans schemas for the one carrying a matching
// x-kubernetes-group-version-kind extension entry.
func findSchemaByGVK(schemas map[string]*spec.Schema, group, version, kind string) (*spec.Schema, error) {
	for _, s := range schemas {
		if s == nil {
			continue
		}
		var gvks []map[string]string
		if err := s.Extensions.GetObject(gvkExtensionKey, &gvks); err != nil {
			continue
		}
		for _, gvk := range gvks {
			if gvk["group"] == group && gvk["version"] == version && gvk["kind"] == kind {
				return s, nil
			}
		}
	}
	return nil, fmt.Errorf("schema not found for group=%q version=%q kind=%q", group, version, kind)
}

// resolveSchema follows a $ref into the shared schemas map; schemas that
// aren't refs are returned unchanged.
func resolveSchema(s *spec.Schema, schemas map[string]*spec.Schema) *spec.Schema {
	if s == nil {
		return nil
	}
	ref := s.Ref.String()
	if ref == "" {
		return s
	}
	name := ref
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		name = ref[idx+1:]
	}
	if resolved, ok := schemas[name]; ok {
		return resolved
	}
	return s
}

// schemaToFieldNode converts one (possibly $ref'd) schema into a FieldNode,
// recursing into object properties and array-of-object items up to
// maxSchemaDepth. Map fields (additionalProperties) are exposed as leaves —
// the caller collects the key from the user since it isn't enumerable from
// the schema.
func schemaToFieldNode(name string, s *spec.Schema, schemas map[string]*spec.Schema, depth int) FieldNode {
	s = resolveSchema(s, schemas)
	node := FieldNode{Name: name, Type: "object"}
	if s == nil {
		return node
	}

	typ := ""
	if len(s.Type) > 0 {
		typ = s.Type[0]
	}

	switch {
	case typ == "array":
		node.Type = "array"
		node.IsArray = true
		if s.Items != nil && s.Items.Schema != nil && depth < maxSchemaDepth {
			item := schemaToFieldNode("", s.Items.Schema, schemas, depth+1)
			if item.Type == "object" {
				node.Children = item.Children
			}
		}
	case s.AdditionalProperties != nil && (s.AdditionalProperties.Schema != nil || s.AdditionalProperties.Allows):
		node.Type = "object"
		node.IsMap = true
	case typ == "object" || len(s.Properties) > 0 || typ == "":
		node.Type = "object"
		if depth < maxSchemaDepth {
			for fieldName, fieldSchema := range s.Properties {
				fs := fieldSchema
				node.Children = append(node.Children, schemaToFieldNode(fieldName, &fs, schemas, depth+1))
			}
			sort.Slice(node.Children, func(i, j int) bool { return node.Children[i].Name < node.Children[j].Name })
		}
	default:
		node.Type = typ // string|integer|number|boolean
	}
	return node
}
