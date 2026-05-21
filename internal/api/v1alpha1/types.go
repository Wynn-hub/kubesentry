package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	PhaseReady    = "Ready"
	PhaseInvalid  = "Invalid"
	PhaseSyncing  = "Syncing"
	PhaseDisabled = "Disabled"
	PhaseDegraded = "Degraded"

	ModeEnforce = "enforce"
	ModeAudit   = "audit"
)

// Label keys used on Policy objects.
const (
	LabelSource   = "kubesentry.io/source"
	LabelCategory = "kubesentry.io/category"

	SourceBuiltin    = "builtin"
	SourceCustom     = "custom"
	SourceByName     = "byName"
	SourceBySelector = "bySelector"
)

// MatchResource defines a resource rule for policy matching.
type MatchResource struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Resources   []string `json:"resources"`
}

// PolicyMatch defines which requests a policy applies to.
type PolicyMatch struct {
	Operations []string        `json:"operations"`
	Resources  []MatchResource `json:"resources"`
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
	ReferencedBy       []string               `json:"referencedBy,omitempty"`
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

// PolicyRef references a Policy by name, with an optional enforcement mode override.
type PolicyRef struct {
	Name            string `json:"name"`
	EnforcementMode string `json:"enforcementMode,omitempty"`
}

// PolicyGroupPolicies holds the two ways to reference policies.
type PolicyGroupPolicies struct {
	ByName     []PolicyRef           `json:"byName,omitempty"`
	BySelector *metav1.LabelSelector `json:"bySelector,omitempty"`
}

// PolicyGroupSpec defines the desired state of a PolicyGroup.
type PolicyGroupSpec struct {
	DisplayName             string                `json:"displayName,omitempty"`
	Description             string                `json:"description,omitempty"`
	Enabled                 bool                  `json:"enabled"`
	NamespaceSelector       *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	Policies                PolicyGroupPolicies   `json:"policies,omitempty"`
	SelectorEnforcementMode string                `json:"selectorEnforcementMode,omitempty"`
}

// EffectiveMember describes a single resolved member of a PolicyGroup.
type EffectiveMember struct {
	Name            string `json:"name"`
	EnforcementMode string `json:"enforcementMode"`
	Source          string `json:"source"`
}

// PolicyGroupStatus defines the observed state of a PolicyGroup.
type PolicyGroupStatus struct {
	Phase              string             `json:"phase,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	ResolvedPolicies   []EffectiveMember  `json:"resolvedPolicies,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
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
