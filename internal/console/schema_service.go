package console

import (
	"context"
	"fmt"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handlers) fieldSchema(ctx context.Context, p dto.FieldSchemaOptions) ([]dto.FieldNodeView, error) {
	group := p.Group
	version, resource := p.Version, p.Resource
	if version == "" || resource == "" {
		return nil, fmt.Errorf("version and resource query params are required")
	}
	key := group + "/" + version + "/" + resource
	if !p.Refresh {
		if v, ok := h.schemaCache().Load(key); ok {
			e := v.(schemaCacheEntry)
			if ctx.Err() == nil && time.Now().Before(e.expiresAt) {
				return toDTOFields(e.fields), nil
			}
		}
	}
	fetcher := h.SchemaFetcher
	if fetcher == nil {
		fetcher = &discoverySchemaFetcher{Discovery: h.Discovery}
	}
	fields, e := fetcher.FieldsFor(group, version, resource)
	if e != nil {
		return nil, e
	}
	h.schemaCache().Store(key, schemaCacheEntry{fields: fields, expiresAt: time.Now().Add(schemaCacheTTL)})
	return toDTOFields(fields), nil
}

func toDTOFields(in []FieldNode) []dto.FieldNodeView {
	out := make([]dto.FieldNodeView, len(in))
	for i := range in {
		var mapValue *string
		if in[i].MapValueType != "" {
			value := in[i].MapValueType
			mapValue = &value
		}
		out[i] = dto.FieldNodeView{Name: in[i].Name, Type: in[i].Type, IsArray: in[i].IsArray, IsMap: in[i].IsMap, MapValueType: mapValue, Children: toDTOFields(in[i].Children)}
	}
	return out
}
