package dto

import "time"

type ClusterImageInput struct {
	Repository string `json:"repository" binding:"required"`
	Tag        string `json:"tag" binding:"required"`
	PullPolicy string `json:"pullPolicy" binding:"required" enums:"Always,IfNotPresent,Never"`
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

type ClusterCreateInput struct {
	Name          string              `json:"name" binding:"required"`
	Labels        map[string]string   `json:"labels,omitempty"`
	InstallParams ClusterInstallInput `json:"installParams" binding:"required"`
}

type ClusterTokenInput struct {
	InstallParams ClusterInstallInput `json:"installParams" binding:"required"`
}

type ClusterLabelsInput struct {
	Labels map[string]string `json:"labels" binding:"required"`
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
