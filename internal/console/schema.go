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
	Name         string      `json:"name"`
	Type         string      `json:"type"` // string|integer|number|boolean|array|object
	IsArray      bool        `json:"isArray"`
	IsMap        bool        `json:"isMap"`
	MapValueType string      `json:"mapValueType,omitempty"` // value type when IsMap; e.g. "string" for map[string]string
	Children     []FieldNode `json:"children,omitempty"`
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
// aren't refs are returned unchanged. Real k8s OpenAPI v3 output wraps a
// referenced property in `allOf: [{$ref: ...}]` rather than a bare `$ref`
// (OpenAPI 3.0 disallows sibling keywords like `description` next to a bare
// $ref, and k8s always attaches one) — this unwraps that pattern too.
func resolveSchema(s *spec.Schema, schemas map[string]*spec.Schema) *spec.Schema {
	if s == nil {
		return nil
	}
	if ref := s.Ref.String(); ref != "" {
		if resolved, ok := schemas[refName(ref)]; ok {
			return resolved
		}
		return s
	}
	for _, sub := range s.AllOf {
		if ref := sub.Ref.String(); ref != "" {
			if resolved, ok := schemas[refName(ref)]; ok {
				return resolved
			}
		}
	}
	return s
}

func refName(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
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
		node.MapValueType = mapValueType(s.AdditionalProperties, schemas)
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

// scalarMapValueTypes are the primitive types a map value can resolve to for
// operator-selection purposes on the frontend.
var scalarMapValueTypes = map[string]bool{
	"string": true, "integer": true, "number": true, "boolean": true,
}

// mapValueType determines the value type of a map (additionalProperties)
// field for the frontend's operator-selection UI. Only scalar value types
// are distinguished; complex (object/array) map values, and maps with no
// declared value schema (arbitrary/unstructured `Allows: true`), default to
// "string" — most k8s maps (labels, annotations) are map[string]string, and
// deep map-value drilling is out of scope for v1.
func mapValueType(ap *spec.SchemaOrBool, schemas map[string]*spec.Schema) string {
	if ap.Schema != nil {
		valueSchema := resolveSchema(ap.Schema, schemas)
		if valueSchema != nil && len(valueSchema.Type) > 0 {
			if t := valueSchema.Type[0]; scalarMapValueTypes[t] {
				return t
			}
		}
	}
	return "string"
}
