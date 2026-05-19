package operator

import (
	"fmt"
	"reflect"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// CheckExceptionSpecMutation rejects mutations to immutable fields.
// Mutable: Duration, RetainAfterExpiry, Reason.
func CheckExceptionSpecMutation(oldSpec, newSpec *v1alpha1.PolicyExceptionSpec) error {
	if !slices.Equal(oldSpec.PolicyRefs, newSpec.PolicyRefs) {
		return fmt.Errorf("spec.policyRefs is immutable")
	}
	if !slices.Equal(oldSpec.PolicyGroupRefs, newSpec.PolicyGroupRefs) {
		return fmt.Errorf("spec.policyGroupRefs is immutable")
	}
	if oldSpec.AllPolicies != newSpec.AllPolicies {
		return fmt.Errorf("spec.allPolicies is immutable")
	}
	if !slices.Equal(oldSpec.Match.Namespaces, newSpec.Match.Namespaces) {
		return fmt.Errorf("spec.match.namespaces is immutable")
	}
	if !labelSelectorsEqual(oldSpec.Match.NamespaceSelector, newSpec.Match.NamespaceSelector) {
		return fmt.Errorf("spec.match.namespaceSelector is immutable")
	}
	if !labelSelectorsEqual(oldSpec.Match.ResourceSelector, newSpec.Match.ResourceSelector) {
		return fmt.Errorf("spec.match.resourceSelector is immutable")
	}
	return nil
}

// CheckExceptionExpiredFreeze rejects any spec change once status.phase == Expired.
func CheckExceptionExpiredFreeze(oldEx, newEx *v1alpha1.PolicyException) error {
	if oldEx.Status.Phase == v1alpha1.PhaseExpired {
		if !specsEqual(&oldEx.Spec, &newEx.Spec) {
			return fmt.Errorf("PolicyException %q is Expired; spec is frozen", oldEx.Name)
		}
	}
	return nil
}

func labelSelectorsEqual(a, b *metav1.LabelSelector) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}

func specsEqual(a, b *v1alpha1.PolicyExceptionSpec) bool {
	return CheckExceptionSpecMutation(a, b) == nil &&
		a.Duration == b.Duration &&
		a.RetainAfterExpiry == b.RetainAfterExpiry &&
		a.Reason == b.Reason
}
