// Package gatewaymodel defines gateway-plane models that are persisted and
// rendered by the gateway but are not Kubernetes custom resources.
package gatewaymodel

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	ClusterPhaseRegistering     = "Registering"
	ClusterPhaseOnline          = "Online"
	ClusterPhaseOffline         = "Offline"
	ClusterPhaseUninstalling    = "Uninstalling"
	ClusterPhaseUninstalled     = "Uninstalled"
	ClusterPhaseUninstallFailed = "UninstallFailed"
)

type ClusterSpec struct {
	DisplayName string            `json:"displayName,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type ClusterStatus struct {
	Phase           string         `json:"phase,omitempty"`
	ClusterUID      string         `json:"clusterUID,omitempty"`
	LastHeartbeat   *metav1.Time   `json:"lastHeartbeat,omitempty"`
	OperatorVersion string         `json:"operatorVersion,omitempty"`
	PolicySummary   *PolicySummary `json:"policySummary,omitempty"`
}

type PolicySummary struct {
	Ready   int32 `json:"ready"`
	Invalid int32 `json:"invalid"`
}

type Cluster struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterSpec   `json:"spec,omitempty"`
	Status            ClusterStatus `json:"status,omitempty"`
}
