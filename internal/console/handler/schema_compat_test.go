package handler

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func TestFieldSchemaStableDTOsAdaptToOgen(t *testing.T) {
	options := fieldSchemaOptions(dto.GetFieldSchemaParams{Group: dto.NewOptString("apps"), Version: "v1", Resource: "deployments", Refresh: dto.NewOptBool(true)})
	if options != (dto.FieldSchemaOptions{Group: "apps", Version: "v1", Resource: "deployments", Refresh: true}) {
		t.Fatalf("schema options = %+v", options)
	}

	mapValue := "string"
	fields, err := dtoData[[]dto.FieldNode]([]dto.FieldNodeView{{Name: "labels", Type: "object", IsMap: true, MapValueType: &mapValue, Children: []dto.FieldNodeView{}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields[0].Type != dto.FieldNodeTypeObject || fields[0].MapValueType.Or("") != "string" || fields[0].Children == nil {
		t.Fatalf("converted fields = %+v", fields)
	}
}
