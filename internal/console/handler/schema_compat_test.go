package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestFieldSchemaOptionsPreserveParameters(t *testing.T) {
	options := fieldSchemaOptions(dto.GetFieldSchemaParams{Group: dto.NewOptString("apps"), Version: "v1", Resource: "deployments", Refresh: dto.NewOptBool(true)})
	if options != (dto.FieldSchemaOptions{Group: "apps", Version: "v1", Resource: "deployments", Refresh: true}) {
		t.Fatalf("schema options = %+v", options)
	}
}

func TestFieldSchemaResponsePreservesLeafJSONShape(t *testing.T) {
	mapValue := "string"
	encoded, err := json.Marshal(dto.FieldSchemaHTTPResponse{Data: []dto.FieldNodeView{{
		Name: "labels", Type: "object", IsMap: true, MapValueType: &mapValue, Children: []dto.FieldNodeView{},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	for _, want := range []string{`"type":"object"`, `"isMap":true`, `"mapValueType":"string"`, `"children":[]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("response %s does not contain %s", got, want)
		}
	}
}
