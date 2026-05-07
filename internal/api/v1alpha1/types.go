package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	PhaseReady   = "Ready"
	PhaseInvalid = "Invalid"
	PhaseSyncing = "Syncing"

	ModeEnforce = "enforce"
	ModeAudit   = "audit"
)

// MatchResource defines a resource rule for policy matching.
type MatchResource struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Resources   []string `json:"resources"`
}

// PolicyMatch defines which requests a policy applies to.
type PolicyMatch struct {
	Operations        []string              `json:"operations"`
	Resources         []MatchResource       `json:"resources"`
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// RollbackTo triggers a rollback to a specific version.
type RollbackTo struct {
	Version int64 `json:"version"`
}

// PolicySpec defines the desired state of a Policy.
type PolicySpec struct {
	Match           PolicyMatch `json:"match"`
	EnforcementMode string      `json:"enforcementMode"`
	Rego            string      `json:"rego"`
	RollbackTo      *RollbackTo `json:"rollbackTo,omitempty"`
}

// PolicyVersionSummary is a brief history entry stored in Policy.status.
type PolicyVersionSummary struct {
	Version   int64       `json:"version"`
	CreatedAt metav1.Time `json:"createdAt"`
	Phase     string      `json:"phase"`
}

// PolicyStatus defines the observed state of a Policy.
type PolicyStatus struct {
	Phase              string                 `json:"phase,omitempty"`
	Message            string                 `json:"message,omitempty"`
	CurrentVersion     int64                  `json:"currentVersion,omitempty"`
	ObservedGeneration int64                  `json:"observedGeneration,omitempty"`
	LastSyncTime       *metav1.Time           `json:"lastSyncTime,omitempty"`
	VersionHistory     []PolicyVersionSummary `json:"versionHistory,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pol
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicySpec   `json:"spec,omitempty"`
	Status            PolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Policy `json:"items"`
}

// PolicyVersionSpec is an immutable snapshot of a Policy at a given version.
type PolicyVersionSpec struct {
	PolicyRef       string      `json:"policyRef"`
	Version         int64       `json:"version"`
	Rego            string      `json:"rego"`
	Match           PolicyMatch `json:"match"`
	EnforcementMode string      `json:"enforcementMode"`
	CreatedAt       metav1.Time `json:"createdAt"`
	CreatedBy       string      `json:"createdBy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=pv
type PolicyVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicyVersionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type PolicyVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyVersion `json:"items"`
}
