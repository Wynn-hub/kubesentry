package handler

import "github.com/Wynn-hub/kubesentry/internal/console/dto"

func objectError(status int) *dto.ErrorResponse {
	if status < 400 {
		status = 500
	}
	return &dto.ErrorResponse{Code: status * 100, Message: "request failed"}
}
