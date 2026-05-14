package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	PhaseReady     = "Ready"
	PhaseInvalid   = "Invalid"
	PhaseSyncing   = "Syncing"
	PhaseDisabled  = "Disabled"
	PhaseDegraded  = "Degraded"

	ModeEnforce = "enforce"
	ModeAudit   = "audit"
)

// Label keys used on Policy objects created by PolicyGroupReconciler.
const (
	LabelKey    = "kubesentry.io/key"
	LabelGroup  = "kubesentry.io/group"
	LabelSource = "kubesentry.io/source"

	SourceBuiltin = "builtin"
	SourceCustom  = "custom"
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
	Description     string      `json:"description,omitempty"`
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

// PolicyInGroup is a single policy entry inside a PolicyGroup spec.
type PolicyInGroup struct {
	Key     string       `json:"key"`
	Enabled *bool        `json:"enabled,omitempty"` // nil = true
	Mode    string       `json:"mode,omitempty"`    // "enforce"|"audit"; empty = library default
	Rego    string       `json:"rego,omitempty"`    // required if key not in built-in library
	Match   *PolicyMatch `json:"match,omitempty"`   // required if key not in built-in library
}

// PolicyGroupSpec defines the desired state of a PolicyGroup.
type PolicyGroupSpec struct {
	DisplayName string          `json:"displayName,omitempty"`
	Description string          `json:"description,omitempty"`
	Enabled     bool            `json:"enabled"`
	Policies    []PolicyInGroup `json:"policies,omitempty"`
}

// PolicyGroupStatus defines the observed state of a PolicyGroup.
type PolicyGroupStatus struct {
	Phase           string             `json:"phase,omitempty"`
	ActivePolicies  int                `json:"activePolicies,omitempty"`
	SkippedPolicies int                `json:"skippedPolicies,omitempty"`
	Conditions      []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pg
type PolicyGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicyGroupSpec   `json:"spec,omitempty"`
	Status            PolicyGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PolicyGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyGroup `json:"items"`
}
