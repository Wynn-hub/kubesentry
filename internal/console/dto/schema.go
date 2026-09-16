package dto

type FieldSchemaOptions struct {
	Group    string
	Version  string
	Resource string
	Refresh  bool
}

type FieldNodeView struct {
	Name         string          `json:"name" binding:"required"`
	Type         string          `json:"type" binding:"required" enums:"string,integer,number,boolean,array,object"`
	IsArray      bool            `json:"isArray" binding:"required"`
	IsMap        bool            `json:"isMap" binding:"required"`
	MapValueType *string         `json:"mapValueType,omitempty"`
	Children     []FieldNodeView `json:"children" binding:"required"`
}
