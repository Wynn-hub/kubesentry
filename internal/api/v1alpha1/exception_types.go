package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PolicyException phases.
const (
	PhasePending = "Pending"
	PhaseActive  = "Active"
	PhaseExpired = "Expired"
	// PhaseInvalid is reused from policy phases (already declared in types.go).
)

// PolicyExceptionMatch selects which admission requests this exception applies to.
// Dimensions are AND-ed; values within a list are OR-ed.
type PolicyExceptionMatch struct {
	// Namespaces is an exact list of namespace names (no glob).
	Namespaces []string `json:"namespaces,omitempty"`
	// NamespaceSelector matches metadata.labels of the Namespace object.
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	// ResourceSelector matches metadata.labels of the admitted object itself.
	ResourceSelector *metav1.LabelSelector `json:"resourceSelector,omitempty"`
}

// PolicyExceptionSpec defines the desired state of a PolicyException.
type PolicyExceptionSpec struct {
	// Exactly one of these three target selectors must be set.
	PolicyRefs      []string `json:"policyRefs,omitempty"`
	PolicyGroupRefs []string `json:"policyGroupRefs,omitempty"`
	AllPolicies     bool     `json:"allPolicies,omitempty"`

	Match PolicyExceptionMatch `json:"match"`

	// Duration is required and parsed by time.ParseDuration; must be > 0.
	Duration string `json:"duration"`
	// RetainAfterExpiry: 0s deletes immediately at expiry; default 0s.
	RetainAfterExpiry string `json:"retainAfterExpiry,omitempty"`
	// Reason is required; trim() length must be >= 1.
	Reason string `json:"reason"`
}

// PolicyExceptionStatus reflects the observed state.
type PolicyExceptionStatus struct {
	Phase              string       `json:"phase,omitempty"`
	Message            string       `json:"message,omitempty"`
	EffectiveAt        *metav1.Time `json:"effectiveAt,omitempty"`
	ExpiresAt          *metav1.Time `json:"expiresAt,omitempty"`
	ObservedGeneration int64        `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=pex
type PolicyException struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicyExceptionSpec   `json:"spec,omitempty"`
	Status            PolicyExceptionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type PolicyExceptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyException `json:"items"`
}
