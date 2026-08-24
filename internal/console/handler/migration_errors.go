package handler

import "github.com/Wynn-hub/kubesentry/internal/console/dto"

func createClusterError(status int) dto.CreateClusterRes {
	switch status {
	case 400:
		return &dto.CreateClusterBadRequest{Code: 40000, Message: "request failed"}
	case 409:
		return &dto.CreateClusterConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.CreateClusterInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func getClusterError(status int) dto.GetClusterRes {
	if status == 404 {
		return &dto.GetClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.GetClusterInternalServerError{Code: status * 100, Message: "request failed"}
}
func updateClusterError(status int) dto.UpdateClusterRes {
	if status == 404 {
		return &dto.UpdateClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.UpdateClusterInternalServerError{Code: status * 100, Message: "request failed"}
}
func deleteClusterError(status int) dto.DeleteClusterRes {
	switch status {
	case 404:
		return &dto.DeleteClusterNotFound{Code: 40400, Message: "cluster not found"}
	case 409:
		return &dto.DeleteClusterConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.DeleteClusterInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func reissueClusterError(status int) dto.ReissueClusterTokenRes {
	switch status {
	case 404:
		return &dto.ReissueClusterTokenNotFound{Code: 40400, Message: "cluster not found"}
	case 409:
		return &dto.ReissueClusterTokenConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.ReissueClusterTokenInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func latestInstallParamsError(status int) dto.GetLatestInstallParamsRes {
	if status == 404 {
		return &dto.GetLatestInstallParamsNotFound{Code: 40400, Message: "install parameters not found"}
	}
	return &dto.GetLatestInstallParamsInternalServerError{Code: status * 100, Message: "request failed"}
}
func createUninstallError(status int) dto.CreateUninstallTaskRes {
	switch status {
	case 404:
		return &dto.CreateUninstallTaskNotFound{Code: 40400, Message: "cluster not found"}
	case 409:
		return &dto.CreateUninstallTaskConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.CreateUninstallTaskInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func listUninstallError(status int) dto.ListUninstallTasksRes {
	if status == 404 {
		return &dto.ListUninstallTasksNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.ListUninstallTasksInternalServerError{Code: status * 100, Message: "request failed"}
}
func retryUninstallError(status int) dto.RetryUninstallTaskRes {
	switch status {
	case 404:
		return &dto.RetryUninstallTaskNotFound{Code: 40400, Message: "task not found"}
	case 409:
		return &dto.RetryUninstallTaskConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.RetryUninstallTaskInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func createDistributionError(status int) dto.CreateDistributionRes {
	switch status {
	case 400:
		return &dto.CreateDistributionBadRequest{Code: 40000, Message: "request failed"}
	case 409:
		return &dto.CreateDistributionConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.CreateDistributionInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func getDistributionError(status int) dto.GetDistributionRes {
	if status == 404 {
		return &dto.GetDistributionNotFound{Code: 40400, Message: "distribution not found"}
	}
	return &dto.GetDistributionInternalServerError{Code: status * 100, Message: "request failed"}
}
func updateDistributionError(status int) dto.UpdateDistributionRes {
	switch status {
	case 400:
		return &dto.UpdateDistributionBadRequest{Code: 40000, Message: "request failed"}
	case 404:
		return &dto.UpdateDistributionNotFound{Code: 40400, Message: "distribution not found"}
	case 409:
		return &dto.UpdateDistributionConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.UpdateDistributionInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
func deleteDistributionError(status int) dto.DeleteDistributionRes {
	switch status {
	case 404:
		return &dto.DeleteDistributionNotFound{Code: 40400, Message: "distribution not found"}
	case 409:
		return &dto.DeleteDistributionConflict{Code: 40900, Message: "request failed"}
	default:
		return &dto.DeleteDistributionInternalServerError{Code: status * 100, Message: "request failed"}
	}
}
