package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ManagedByLabel marks member-cluster objects materialized from a gateway
// distribution. The member operator prunes labeled objects that are absent
// from the current snapshot; objects without the label are never touched.
const (
	ManagedByLabel    = "kubesentry.io/managed-by"
	ManagedByPlatform = "platform"
)

// LastTraceIDAnnotation records the trace_id (spec D18) of the most recent
// NotifyChange-triggered push that actually changed this managed object —
// ticker-driven resyncs carry an empty trace_id and must never clobber it.
const LastTraceIDAnnotation = "kubesentry.io/last-trace-id"

// Per-cluster distribution sync states.
const (
	DistributionSynced  = "Synced"
	DistributionPending = "Pending"
	DistributionError   = "Error"
)

// DistributionClusters selects target clusters: byName ∪ clusterSelector.
type DistributionClusters struct {
	ByName          []string              `json:"byName,omitempty"`
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
}

// DistributionOverride downgrades/upgrades every group of this distribution
// on one cluster. Cross-distribution conflicts resolve strictest-wins.
type DistributionOverride struct {
	Cluster         string `json:"cluster"`
	EnforcementMode string `json:"enforcementMode"`
}

type PolicyDistributionSpec struct {
	Clusters     DistributionClusters   `json:"clusters,omitempty"`
	PolicyGroups []string               `json:"policyGroups"`
	Overrides    []DistributionOverride `json:"overrides,omitempty"`
}

type DistributionClusterStatus struct {
	Name           string       `json:"name"`
	State          string       `json:"state"`
	AppliedVersion string       `json:"appliedVersion,omitempty"`
	Message        string       `json:"message,omitempty"`
	LastSyncTime   *metav1.Time `json:"lastSyncTime,omitempty"`
}

type PolicyDistributionStatus struct {
	Clusters []DistributionClusterStatus `json:"clusters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pdist
type PolicyDistribution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicyDistributionSpec   `json:"spec,omitempty"`
	Status            PolicyDistributionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PolicyDistributionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyDistribution `json:"items"`
}
