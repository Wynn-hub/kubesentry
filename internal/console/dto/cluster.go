package dto

import "time"

type ClusterImageInput struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	PullPolicy string `json:"pullPolicy"`
}

type ClusterInstallInput struct {
	Namespace            string            `json:"namespace"`
	WebhookImage         ClusterImageInput `json:"webhookImage"`
	OperatorImage        ClusterImageInput `json:"operatorImage"`
	HighAvailability     bool              `json:"highAvailability"`
	GatewayAddress       string            `json:"gatewayAddress"`
	GatewayTLSServerName string            `json:"gatewayTlsServerName,omitempty"`
	GatewayCABundle      string            `json:"gatewayCaBundle,omitempty"`
}

type ClusterCreateInput struct {
	Name          string              `json:"name"`
	Labels        map[string]string   `json:"labels,omitempty"`
	InstallParams ClusterInstallInput `json:"installParams"`
}

type ClusterTokenInput struct {
	InstallParams ClusterInstallInput `json:"installParams"`
}

type ClusterLabelsInput struct {
	Labels map[string]string `json:"labels"`
}

type ClusterItem struct {
	Name              string     `json:"name"`
	Phase             string     `json:"phase"`
	OperatorVersion   string     `json:"operatorVersion"`
	KubernetesVersion string     `json:"kubernetesVersion"`
	NodeCount         int        `json:"nodeCount"`
	ReadyNodeCount    int        `json:"readyNodeCount"`
	LastHeartbeatAt   *time.Time `json:"lastHeartbeatAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type ClusterView struct {
	Name              string            `json:"name"`
	ClusterUID        string            `json:"clusterUid"`
	Phase             string            `json:"phase"`
	OperatorVersion   string            `json:"operatorVersion"`
	LastHeartbeatAt   *time.Time        `json:"lastHeartbeatAt,omitempty"`
	KubernetesVersion string            `json:"kubernetesVersion"`
	APIServerAddress  string            `json:"apiServerAddress"`
	NodeCount         int               `json:"nodeCount"`
	ReadyNodeCount    int               `json:"readyNodeCount"`
	Labels            map[string]string `json:"labels"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

type InstallRecordItemView struct {
	ClusterName        string    `json:"clusterName"`
	Namespace          string    `json:"namespace"`
	WebhookRepository  string    `json:"webhookRepository"`
	WebhookTag         string    `json:"webhookTag"`
	OperatorRepository string    `json:"operatorRepository"`
	OperatorTag        string    `json:"operatorTag"`
	HighAvailability   bool      `json:"highAvailability"`
	GatewayAddress     string    `json:"gatewayAddress"`
	CreatedAt          time.Time `json:"createdAt"`
}

type UninstallTaskView struct {
	ID          int64      `json:"id"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	RequestedAt time.Time  `json:"requestedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type ClusterPageView struct {
	Items    []ClusterItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

type InstallRecordPageView struct {
	Items    []InstallRecordItemView `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
}
