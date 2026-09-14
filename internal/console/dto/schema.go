package dto

type FieldSchemaOptions struct {
	Group    string
	Version  string
	Resource string
	Refresh  bool
}

type FieldNodeView struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	IsArray      bool            `json:"isArray"`
	IsMap        bool            `json:"isMap"`
	MapValueType *string         `json:"mapValueType,omitempty"`
	Children     []FieldNodeView `json:"children"`
}
