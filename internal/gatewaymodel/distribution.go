package gatewaymodel

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	DistributionSynced  = "Synced"
	DistributionPending = "Pending"
	DistributionError   = "Error"
)

type DistributionClusters struct {
	ByName          []string              `json:"byName,omitempty"`
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`
}

type DistributionOverride struct {
	Cluster         string `json:"cluster"`
	EnforcementMode string `json:"enforcementMode"`
}

type PolicyDistributionSpec struct {
	Clusters         DistributionClusters   `json:"clusters,omitempty"`
	PolicyGroups     []string               `json:"policyGroups"`
	PolicyExceptions []string               `json:"policyExceptions,omitempty"`
	Overrides        []DistributionOverride `json:"overrides,omitempty"`
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

type PolicyDistribution struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicyDistributionSpec   `json:"spec,omitempty"`
	Status            PolicyDistributionStatus `json:"status,omitempty"`
}
