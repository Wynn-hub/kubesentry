package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Cluster lifecycle phases (gateway-side view of a member cluster).
const (
	ClusterPhaseRegistering     = "Registering"
	ClusterPhaseOnline          = "Online"
	ClusterPhaseOffline         = "Offline"
	ClusterPhaseUninstalling    = "Uninstalling"
	ClusterPhaseUninstalled     = "Uninstalled"
	ClusterPhaseUninstallFailed = "UninstallFailed"
)

// ClusterSpec defines a member cluster registered on the gateway.
type ClusterSpec struct {
	DisplayName string `json:"displayName,omitempty"`
	// Labels drive PolicyDistribution clusterSelector matching (SP2).
	Labels map[string]string `json:"labels,omitempty"`
}

// ClusterStatus is maintained by the gateway.
// SP2/SP3 add policySummary/baselineStatus/drift incrementally.
type ClusterStatus struct {
	Phase string `json:"phase,omitempty"`
	// ClusterUID is the member's kube-system namespace UID — the
	// machine identity that binds tokens to one physical cluster.
	ClusterUID      string       `json:"clusterUID,omitempty"`
	LastHeartbeat   *metav1.Time `json:"lastHeartbeat,omitempty"`
	OperatorVersion string       `json:"operatorVersion,omitempty"`
	// PolicySummary is the member-reported Ready/Invalid policy counts,
	// persisted by StatusFlusher alongside heartbeat state.
	PolicySummary *PolicySummary `json:"policySummary,omitempty"`
}

// PolicySummary counts member-cluster policies by phase.
type PolicySummary struct {
	Ready   int32 `json:"ready"`
	Invalid int32 `json:"invalid"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mcl
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterSpec   `json:"spec,omitempty"`
	Status            ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}
