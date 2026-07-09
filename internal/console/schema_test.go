package console

import (
	"testing"

	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// buildPodFixture 手工搭一棵最小的 Pod schema：
// Pod{apiVersion, kind, metadata: ObjectMeta{name, namespace, labels: map[string]string},
//
//	spec: PodSpec{hostNetwork bool, containers: []Container{name, image, securityContext: {privileged bool}}}}
func buildPodFixture() *spec3.OpenAPI {
	securityContext := spec.Schema{SchemaProps: spec.SchemaProps{
		Type:       spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{"privileged": {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"boolean"}}}},
	}}
	container := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{
			"name":            {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}},
			"image":           {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}},
			"securityContext": {SchemaProps: spec.SchemaProps{Ref: spec.MustCreateRef("#/components/schemas/Container.securityContext")}},
		},
	}}
	objectMeta := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{
			"name":      {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}},
			"namespace": {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}},
			"labels": {SchemaProps: spec.SchemaProps{
				Type:                 spec.StringOrArray{"object"},
				AdditionalProperties: &spec.SchemaOrBool{Schema: &spec.Schema{SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}}},
			}},
		},
	}}
	podSpec := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{
			"hostNetwork": {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"boolean"}}},
			"containers": {SchemaProps: spec.SchemaProps{
				Type:  spec.StringOrArray{"array"},
				Items: &spec.SchemaOrArray{Schema: spec.RefSchema("#/components/schemas/Container")},
			}},
		},
	}}
	pod := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{
			"apiVersion": {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}},
			"kind":       {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"string"}}},
			"metadata":   {SchemaProps: spec.SchemaProps{Ref: spec.MustCreateRef("#/components/schemas/ObjectMeta")}},
			"spec":       {SchemaProps: spec.SchemaProps{Ref: spec.MustCreateRef("#/components/schemas/PodSpec")}},
		},
	}}
	pod.AddExtension("x-kubernetes-group-version-kind", []map[string]string{{"group": "", "version": "v1", "kind": "Pod"}})

	return &spec3.OpenAPI{
		Components: &spec3.Components{
			Schemas: map[string]*spec.Schema{
				"io.k8s.api.core.v1.Pod":    &pod,
				"Container":                 &container,
				"Container.securityContext": &securityContext,
				"ObjectMeta":                &objectMeta,
				"PodSpec":                   &podSpec,
			},
		},
	}
}

func findChild(t *testing.T, nodes []FieldNode, name string) FieldNode {
	t.Helper()
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
	}
	t.Fatalf("child %q not found among %+v", name, nodes)
	return FieldNode{}
}

func TestBuildFieldTree_TopLevel(t *testing.T) {
	doc := buildPodFixture()
	fields, err := buildFieldTree(doc, "", "v1", "Pod")
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 {
		t.Fatalf("top-level fields = %+v, want 4", fields)
	}
	spec := findChild(t, fields, "spec")
	if spec.Type != "object" || spec.IsArray || spec.IsMap {
		t.Fatalf("spec node = %+v", spec)
	}
}

func TestBuildFieldTree_ArrayOfObjectCascades(t *testing.T) {
	doc := buildPodFixture()
	fields, _ := buildFieldTree(doc, "", "v1", "Pod")
	spec := findChild(t, fields, "spec")
	containers := findChild(t, spec.Children, "containers")
	if !containers.IsArray {
		t.Fatalf("containers.IsArray = false, want true")
	}
	securityContext := findChild(t, containers.Children, "securityContext")
	privileged := findChild(t, securityContext.Children, "privileged")
	if privileged.Type != "boolean" {
		t.Fatalf("privileged.Type = %q, want boolean", privileged.Type)
	}
}

func TestBuildFieldTree_MapField(t *testing.T) {
	doc := buildPodFixture()
	fields, _ := buildFieldTree(doc, "", "v1", "Pod")
	metadata := findChild(t, fields, "metadata")
	labels := findChild(t, metadata.Children, "labels")
	if !labels.IsMap {
		t.Fatalf("labels.IsMap = false, want true")
	}
	if len(labels.Children) != 0 {
		t.Fatalf("labels.Children = %+v, want empty (map fields don't cascade)", labels.Children)
	}
}

func TestBuildFieldTree_NotFound(t *testing.T) {
	doc := buildPodFixture()
	if _, err := buildFieldTree(doc, "apps", "v1", "Deployment"); err == nil {
		t.Fatal("want error for unknown GVK, got nil")
	}
}

func TestBuildFieldTree_AllOfWrappedRef(t *testing.T) {
	// Real k8s /openapi/v3 output wraps referenced properties in
	// allOf: [{$ref: ...}] rather than a bare $ref (to attach a sibling
	// description, which OpenAPI 3.0 disallows next to a bare $ref).
	podSpec := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{
			"hostNetwork": {SchemaProps: spec.SchemaProps{Type: spec.StringOrArray{"boolean"}}},
		},
	}}
	pod := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{
			"spec": {SchemaProps: spec.SchemaProps{
				Description: "pod spec",
				AllOf:       []spec.Schema{{SchemaProps: spec.SchemaProps{Ref: spec.MustCreateRef("#/components/schemas/PodSpec")}}},
			}},
		},
	}}
	pod.AddExtension("x-kubernetes-group-version-kind", []map[string]string{{"group": "", "version": "v1", "kind": "Pod"}})
	doc := &spec3.OpenAPI{Components: &spec3.Components{Schemas: map[string]*spec.Schema{
		"io.k8s.api.core.v1.Pod": &pod,
		"PodSpec":                &podSpec,
	}}}

	fields, err := buildFieldTree(doc, "", "v1", "Pod")
	if err != nil {
		t.Fatal(err)
	}
	specNode := findChild(t, fields, "spec")
	hostNetwork := findChild(t, specNode.Children, "hostNetwork")
	if hostNetwork.Type != "boolean" {
		t.Fatalf("hostNetwork.Type = %q, want boolean (allOf-wrapped ref should still cascade)", hostNetwork.Type)
	}
}

func TestBuildFieldTree_DepthLimit(t *testing.T) {
	// 自引用 schema：A.self -> A，验证深度限制会截断而不是死循环/栈溢出。
	self := &spec.Schema{}
	self.SchemaProps = spec.SchemaProps{
		Type:       spec.StringOrArray{"object"},
		Properties: map[string]spec.Schema{"self": {SchemaProps: spec.SchemaProps{Ref: spec.MustCreateRef("#/components/schemas/Self")}}},
	}
	self.AddExtension("x-kubernetes-group-version-kind", []map[string]string{{"group": "x", "version": "v1", "kind": "Self"}})
	doc := &spec3.OpenAPI{Components: &spec3.Components{Schemas: map[string]*spec.Schema{"Self": self}}}

	fields, err := buildFieldTree(doc, "x", "v1", "Self")
	if err != nil {
		t.Fatal(err)
	}
	depth := 0
	cur := fields
	for len(cur) > 0 {
		depth++
		if depth > maxSchemaDepth+1 {
			t.Fatalf("recursion did not stop at maxSchemaDepth=%d", maxSchemaDepth)
		}
		cur = cur[0].Children
	}
}
