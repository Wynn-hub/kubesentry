package console

import (
	"context"
	"fmt"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handlers) fieldSchema(ctx context.Context, p dto.GetFieldSchemaParams) ([]dto.FieldNode, error) {
	group := p.Group.Or("")
	version, resource := p.Version, p.Resource
	if version == "" || resource == "" {
		return nil, fmt.Errorf("version and resource query params are required")
	}
	key := group + "/" + version + "/" + resource
	if !p.Refresh.Or(false) {
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

func toDTOFields(in []FieldNode) []dto.FieldNode {
	out := make([]dto.FieldNode, len(in))
	for i := range in {
		var mapValue dto.OptString
		if in[i].MapValueType != "" {
			mapValue.SetTo(in[i].MapValueType)
		}
		out[i] = dto.FieldNode{Name: in[i].Name, Type: dto.FieldNodeType(in[i].Type), IsArray: in[i].IsArray, IsMap: in[i].IsMap, MapValueType: mapValue, Children: toDTOFields(in[i].Children)}
	}
	return out
}
