package dto

type SwagSuccessResponse struct {
	Code    int    `json:"code" binding:"required" example:"0"`
	Message string `json:"message" binding:"required" example:"success"`
	Data    any    `json:"data" binding:"required"`
}

type SwagOptionalResponse struct {
	Code    int    `json:"code" binding:"required" example:"0"`
	Message string `json:"message" binding:"required" example:"success"`
	Data    any    `json:"data,omitempty"`
}

type SwagEmptyResponse struct {
	Code    int    `json:"code" binding:"required" example:"0"`
	Message string `json:"message" binding:"required" example:"success"`
}
