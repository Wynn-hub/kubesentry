package dto

// SwagPolicyListResponse is the stage-one schema probe for the existing JSON response.
type SwagPolicyListResponse struct {
	Code    int                 `json:"code" binding:"required" example:"0"`
	Message string              `json:"message" binding:"required" example:"success"`
	Data    *SwagPolicyListData `json:"data,omitempty"`
}

type SwagPolicyListData struct {
	Total    int                  `json:"total" binding:"required" example:"137"`
	Page     int                  `json:"page" binding:"required" example:"1"`
	PageSize int                  `json:"pageSize" binding:"required" enums:"10,20,50,100" example:"20"`
	Items    []SwagPolicyListItem `json:"items" binding:"required"`
}

type SwagPolicyListItem struct {
	Name            string `json:"name" binding:"required" example:"deny-privileged-pod"`
	Source          string `json:"source" binding:"required" enums:"builtin,custom" example:"builtin"`
	EnforcementMode string `json:"enforcementMode" binding:"required" enums:"enforce,audit" example:"enforce"`
	Phase           string `json:"phase" binding:"required" enums:"Ready,Invalid" example:"Ready"`
	Description     string `json:"description" binding:"required" example:"Deny privileged containers"`
	CreatedAt       string `json:"createdAt" binding:"required" format:"date-time" example:"2026-08-03T10:15:30Z"`
	UpdatedAt       string `json:"updatedAt" binding:"required" format:"date-time" example:"2026-08-03T11:20:45Z"`
	OwnerType       string `json:"ownerType" binding:"required" enums:"system,tenant" example:"system"`
	TenantID        *int64 `json:"tenantId,omitempty,string" swaggertype:"string"`
}

type SwagErrorResponse struct {
	Code      int                   `json:"code" binding:"required" example:"40000"`
	Message   string                `json:"message" binding:"required" example:"invalid request"`
	ErrorCode string                `json:"errorCode,omitempty" example:"INVALID_REQUEST"`
	Details   []string              `json:"details,omitempty"`
	Data      *SwagReferencedByData `json:"data,omitempty"`
}

type SwagReferencedByData struct {
	ReferencedBy []string `json:"referencedBy" binding:"required"`
}
