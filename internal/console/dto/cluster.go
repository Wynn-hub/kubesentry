package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type ClusterImageInput struct {
	Repository string `json:"repository" binding:"required"`
	Tag        string `json:"tag" binding:"required"`
	PullPolicy string `json:"pullPolicy" binding:"required" enums:"Always,IfNotPresent,Never"`
}

func (i *ClusterImageInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode cluster image: expected object")
	}
	required := map[string]bool{"repository": false, "tag": false, "pullPolicy": false}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "repository":
			required["repository"] = true
			err = decodeNonNullJSON(raw, "repository", &i.Repository)
		case "tag":
			required["tag"] = true
			err = decodeNonNullJSON(raw, "tag", &i.Tag)
		case "pullPolicy":
			required["pullPolicy"] = true
			err = decodeNonNullJSON(raw, "pullPolicy", &i.PullPolicy)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for _, field := range []string{"repository", "tag", "pullPolicy"} {
		if !required[field] {
			return fmt.Errorf("%s: field is required", field)
		}
	}
	return nil
}

type ClusterInstallInput struct {
	Namespace            string            `json:"namespace" binding:"required"`
	WebhookImage         ClusterImageInput `json:"webhookImage" binding:"required"`
	OperatorImage        ClusterImageInput `json:"operatorImage" binding:"required"`
	HighAvailability     bool              `json:"highAvailability" binding:"required"`
	GatewayAddress       string            `json:"gatewayAddress" binding:"required"`
	GatewayTLSServerName string            `json:"gatewayTlsServerName,omitempty"`
	GatewayCABundle      string            `json:"gatewayCaBundle,omitempty"`
}

func (i *ClusterInstallInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode cluster install params: expected object")
	}
	required := map[string]bool{"namespace": false, "webhookImage": false, "operatorImage": false, "highAvailability": false, "gatewayAddress": false}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "namespace":
			required["namespace"] = true
			err = decodeNonNullJSON(raw, "namespace", &i.Namespace)
		case "webhookImage":
			required["webhookImage"] = true
			err = decodeNonNullJSON(raw, "webhookImage", &i.WebhookImage)
		case "operatorImage":
			required["operatorImage"] = true
			err = decodeNonNullJSON(raw, "operatorImage", &i.OperatorImage)
		case "highAvailability":
			required["highAvailability"] = true
			err = decodeNonNullJSON(raw, "highAvailability", &i.HighAvailability)
		case "gatewayAddress":
			required["gatewayAddress"] = true
			err = decodeNonNullJSON(raw, "gatewayAddress", &i.GatewayAddress)
		case "gatewayTlsServerName":
			err = decodeNonNullJSON(raw, "gatewayTlsServerName", &i.GatewayTLSServerName)
		case "gatewayCaBundle":
			err = decodeNonNullJSON(raw, "gatewayCaBundle", &i.GatewayCABundle)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	for _, field := range []string{"namespace", "webhookImage", "operatorImage", "highAvailability", "gatewayAddress"} {
		if !required[field] {
			return fmt.Errorf("%s: field is required", field)
		}
	}
	return nil
}

type ClusterCreateInput struct {
	Name          string              `json:"name" binding:"required"`
	Labels        map[string]string   `json:"labels,omitempty"`
	InstallParams ClusterInstallInput `json:"installParams" binding:"required"`
}

func (i *ClusterCreateInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode cluster create request: expected object")
	}
	var nameSet, installParamsSet bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		switch key {
		case "name":
			nameSet = true
			err = decodeNonNullJSON(raw, "name", &i.Name)
		case "labels":
			i.Labels = nil
			err = decodeNonNullJSON(raw, "labels", &i.Labels)
		case "installParams":
			installParamsSet = true
			err = decodeNonNullJSON(raw, "installParams", &i.InstallParams)
		}
		if err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !nameSet {
		return fmt.Errorf("name: field is required")
	}
	if !installParamsSet {
		return fmt.Errorf("installParams: field is required")
	}
	return nil
}

type ClusterTokenInput struct {
	InstallParams ClusterInstallInput `json:"installParams" binding:"required"`
}

func (i *ClusterTokenInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("decode cluster token request: expected object")
	}
	var installParamsSet bool
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		if key == "installParams" {
			installParamsSet = true
			if err := decodeNonNullJSON(raw, "installParams", &i.InstallParams); err != nil {
				return err
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !installParamsSet {
		return fmt.Errorf("installParams: field is required")
	}
	return nil
}

func (i *ClusterCreateInput) Validate() error {
	return validateClusterInstallInput(i.InstallParams)
}

func (i *ClusterTokenInput) Validate() error {
	return validateClusterInstallInput(i.InstallParams)
}

func validateClusterInstallInput(input ClusterInstallInput) error {
	if !validPullPolicy(input.WebhookImage.PullPolicy) {
		return fmt.Errorf("webhookImage.pullPolicy: invalid value %q", input.WebhookImage.PullPolicy)
	}
	if !validPullPolicy(input.OperatorImage.PullPolicy) {
		return fmt.Errorf("operatorImage.pullPolicy: invalid value %q", input.OperatorImage.PullPolicy)
	}
	return nil
}

func validPullPolicy(value string) bool {
	return value == "Always" || value == "IfNotPresent" || value == "Never"
}

type ClusterLabelsInput struct {
	Labels map[string]string `json:"labels" binding:"required"`
}

func (r *ClusterLabelsInput) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("expected object")
	}
	found := false
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		if key != "labels" {
			continue
		}
		found = true
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("labels: must be an object")
		}
		var labels map[string]string
		if err := json.Unmarshal(raw, &labels); err != nil {
			return fmt.Errorf("labels: %w", err)
		}
		if r.Labels == nil {
			r.Labels = make(map[string]string, len(labels))
		}
		for key, value := range labels {
			r.Labels[key] = value
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("labels: field is required")
	}
	return nil
}

type ClusterCommandView struct {
	Name    string `json:"name" binding:"required"`
	Command string `json:"command" binding:"required"`
}

type ClusterNameView struct {
	Name string `json:"name" binding:"required"`
}

type ClusterItem struct {
	Name              string     `json:"name" binding:"required"`
	Phase             string     `json:"phase" binding:"required" enums:"Registering,Online,Offline,Uninstalling,Uninstalled"`
	OperatorVersion   string     `json:"operatorVersion" binding:"required"`
	KubernetesVersion string     `json:"kubernetesVersion" binding:"required"`
	NodeCount         int        `json:"nodeCount" binding:"required"`
	ReadyNodeCount    int        `json:"readyNodeCount" binding:"required"`
	LastHeartbeatAt   *time.Time `json:"lastHeartbeatAt,omitempty" format:"date-time"`
	CreatedAt         time.Time  `json:"createdAt" binding:"required" format:"date-time"`
}

type ClusterView struct {
	Name              string            `json:"name" binding:"required"`
	ClusterUID        string            `json:"clusterUid" binding:"required"`
	Phase             string            `json:"phase" binding:"required" enums:"Registering,Online,Offline,Uninstalling,Uninstalled"`
	OperatorVersion   string            `json:"operatorVersion" binding:"required"`
	LastHeartbeatAt   *time.Time        `json:"lastHeartbeatAt,omitempty" format:"date-time"`
	KubernetesVersion string            `json:"kubernetesVersion" binding:"required"`
	APIServerAddress  string            `json:"apiServerAddress" binding:"required"`
	NodeCount         int               `json:"nodeCount" binding:"required"`
	ReadyNodeCount    int               `json:"readyNodeCount" binding:"required"`
	Labels            map[string]string `json:"labels" binding:"required"`
	CreatedAt         time.Time         `json:"createdAt" binding:"required" format:"date-time"`
	UpdatedAt         time.Time         `json:"updatedAt" binding:"required" format:"date-time"`
}

type InstallRecordItemView struct {
	ClusterName        string    `json:"clusterName" binding:"required"`
	Namespace          string    `json:"namespace" binding:"required"`
	WebhookRepository  string    `json:"webhookRepository" binding:"required"`
	WebhookTag         string    `json:"webhookTag" binding:"required"`
	OperatorRepository string    `json:"operatorRepository" binding:"required"`
	OperatorTag        string    `json:"operatorTag" binding:"required"`
	HighAvailability   bool      `json:"highAvailability" binding:"required"`
	GatewayAddress     string    `json:"gatewayAddress" binding:"required"`
	CreatedAt          time.Time `json:"createdAt" binding:"required" format:"date-time"`
}

type UninstallTaskView struct {
	ID          int64      `json:"id"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	RequestedAt time.Time  `json:"requestedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type UninstallTaskHTTPView struct {
	ID          string     `json:"id" binding:"required" pattern:"^[0-9]+$"`
	Status      string     `json:"status" binding:"required" enums:"Pending,Received,Running,Succeeded,Failed"`
	Error       string     `json:"error,omitempty"`
	RequestedAt time.Time  `json:"requestedAt" binding:"required" format:"date-time"`
	CompletedAt *time.Time `json:"completedAt,omitempty" format:"date-time"`
}

type UninstallTaskIDView struct {
	ID string `json:"id" binding:"required" pattern:"^[0-9]+$"`
}

type RetryUninstallTaskPath struct {
	// 卸载任务 ID
	TaskID string `uri:"taskID" binding:"required" pattern:"^[0-9]+$"`
}

type ClusterPageView struct {
	Items    []ClusterItem `json:"items" binding:"required"`
	Total    int           `json:"total" binding:"required"`
	Page     int           `json:"page" binding:"required"`
	PageSize int           `json:"pageSize" binding:"required" enums:"10,20,50,100"`
}

type InstallRecordPageView struct {
	Items    []InstallRecordItemView `json:"items" binding:"required"`
	Total    int                     `json:"total" binding:"required"`
	Page     int                     `json:"page" binding:"required"`
	PageSize int                     `json:"pageSize" binding:"required" enums:"10,20,50,100"`
}

type ClusterListHTTPResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    *ClusterPageView `json:"data"`
}

func (*ClusterListHTTPResponse) listClustersRes() {}

type ClusterDetailHTTPResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    ClusterView `json:"data"`
}

func (*ClusterDetailHTTPResponse) getClusterRes() {}

type ClusterCommandHTTPResponse struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    ClusterCommandView `json:"data"`
}

func (*ClusterCommandHTTPResponse) createClusterRes()       {}
func (*ClusterCommandHTTPResponse) reissueClusterTokenRes() {}

type ClusterNameHTTPResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    ClusterNameView `json:"data"`
}

func (*ClusterNameHTTPResponse) updateClusterRes() {}

type ClusterEmptyHTTPResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterEmptyHTTPResponse) deleteClusterRes() {}

type ClusterBadRequestHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterBadRequestHTTPError) HTTPStatus() int   { return 400 }
func (*ClusterBadRequestHTTPError) createClusterRes() {}

type ClusterForbiddenHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterForbiddenHTTPError) HTTPStatus() int   { return 403 }
func (*ClusterForbiddenHTTPError) deleteClusterRes() {}

type ClusterNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterNotFoundHTTPError) HTTPStatus() int         { return 404 }
func (*ClusterNotFoundHTTPError) getClusterRes()          {}
func (*ClusterNotFoundHTTPError) updateClusterRes()       {}
func (*ClusterNotFoundHTTPError) deleteClusterRes()       {}
func (*ClusterNotFoundHTTPError) reissueClusterTokenRes() {}

type ClusterConflictHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterConflictHTTPError) HTTPStatus() int         { return 409 }
func (*ClusterConflictHTTPError) createClusterRes()       {}
func (*ClusterConflictHTTPError) deleteClusterRes()       {}
func (*ClusterConflictHTTPError) reissueClusterTokenRes() {}

type ClusterInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterInternalServerHTTPError) HTTPStatus() int         { return 500 }
func (*ClusterInternalServerHTTPError) listClustersRes()        {}
func (*ClusterInternalServerHTTPError) createClusterRes()       {}
func (*ClusterInternalServerHTTPError) getClusterRes()          {}
func (*ClusterInternalServerHTTPError) updateClusterRes()       {}
func (*ClusterInternalServerHTTPError) deleteClusterRes()       {}
func (*ClusterInternalServerHTTPError) reissueClusterTokenRes() {}

type InstallParamsHTTPResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    *ClusterInstallInput `json:"data,omitempty"`
}

func (*InstallParamsHTTPResponse) getLatestInstallParamsRes() {}

type UninstallTaskHTTPResponse struct {
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Data    UninstallTaskHTTPView `json:"data"`
}

func (*UninstallTaskHTTPResponse) createUninstallTaskRes() {}

type UninstallTaskListHTTPResponse struct {
	Code    int                     `json:"code"`
	Message string                  `json:"message"`
	Data    []UninstallTaskHTTPView `json:"data"`
}

func (*UninstallTaskListHTTPResponse) listUninstallTasksRes() {}

type UninstallTaskIDHTTPResponse struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    UninstallTaskIDView `json:"data"`
}

func (*UninstallTaskIDHTTPResponse) retryUninstallTaskRes() {}

type InstallRecordListHTTPResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    *InstallRecordPageView `json:"data"`
}

func (*InstallRecordListHTTPResponse) listInstallRecordsRes() {}

type ClusterTaskNotFoundHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterTaskNotFoundHTTPError) HTTPStatus() int            { return 404 }
func (*ClusterTaskNotFoundHTTPError) getLatestInstallParamsRes() {}
func (*ClusterTaskNotFoundHTTPError) createUninstallTaskRes()    {}
func (*ClusterTaskNotFoundHTTPError) listUninstallTasksRes()     {}
func (*ClusterTaskNotFoundHTTPError) retryUninstallTaskRes()     {}

type ClusterTaskConflictHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterTaskConflictHTTPError) HTTPStatus() int         { return 409 }
func (*ClusterTaskConflictHTTPError) createUninstallTaskRes() {}
func (*ClusterTaskConflictHTTPError) retryUninstallTaskRes()  {}

type ClusterTaskInternalServerHTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (*ClusterTaskInternalServerHTTPError) HTTPStatus() int            { return 500 }
func (*ClusterTaskInternalServerHTTPError) getLatestInstallParamsRes() {}
func (*ClusterTaskInternalServerHTTPError) createUninstallTaskRes()    {}
func (*ClusterTaskInternalServerHTTPError) listUninstallTasksRes()     {}
func (*ClusterTaskInternalServerHTTPError) retryUninstallTaskRes()     {}
func (*ClusterTaskInternalServerHTTPError) listInstallRecordsRes()     {}
