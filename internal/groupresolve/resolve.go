// Package groupresolve computes a PolicyGroup's effective members from its
// spec against an in-memory Policy list. It is shared by the member-side
// operator reconciler and the gateway-side snapshot renderer — the gateway cannot
// use PolicyGroup.status because a pure management cluster runs no operator.
package groupresolve

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// Condition types mirrored from the original reconciler behavior.
const (
	CondTypeMissingMember   = "MissingMember"
	CondTypeInvalidSelector = "InvalidSelector"
)

// Resolve returns members sorted by name: byName ∪ bySelector − exclude,
// byName precedence on conflict, per-entry mode > selector mode > policy default.
func Resolve(spec v1alpha1.PolicyGroupSpec, policies []v1alpha1.Policy) ([]v1alpha1.EffectiveMember, []metav1.Condition) {
	byName := make(map[string]*v1alpha1.Policy, len(policies))
	for i := range policies {
		byName[policies[i].Name] = &policies[i]
	}

	resolved := map[string]v1alpha1.EffectiveMember{}
	var conditions []metav1.Condition

	for _, ref := range spec.Policies.ByName {
		pol, ok := byName[ref.Name]
		if !ok {
			conditions = append(conditions, metav1.Condition{
				Type:               CondTypeMissingMember,
				Status:             metav1.ConditionTrue,
				Reason:             "PolicyNotFound",
				Message:            ref.Name,
				LastTransitionTime: metav1.Now(),
			})
			continue
		}
		mode := ref.EnforcementMode
		if mode == "" {
			mode = pol.Spec.EnforcementMode
		}
		resolved[ref.Name] = v1alpha1.EffectiveMember{
			Name:            ref.Name,
			EnforcementMode: mode,
			Source:          v1alpha1.SourceByName,
		}
	}

	if spec.Policies.BySelector != nil {
		sel, err := metav1.LabelSelectorAsSelector(spec.Policies.BySelector)
		if err != nil {
			conditions = append(conditions, metav1.Condition{
				Type:               CondTypeInvalidSelector,
				Status:             metav1.ConditionTrue,
				Reason:             "MalformedSelector",
				Message:            err.Error(),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			for i := range policies {
				pol := &policies[i]
				if !sel.Matches(labels.Set(pol.Labels)) {
					continue
				}
				if _, alreadyByName := resolved[pol.Name]; alreadyByName {
					continue // byName precedence
				}
				mode := spec.SelectorEnforcementMode
				if mode == "" {
					mode = pol.Spec.EnforcementMode
				}
				resolved[pol.Name] = v1alpha1.EffectiveMember{
					Name:            pol.Name,
					EnforcementMode: mode,
					Source:          v1alpha1.SourceBySelector,
				}
			}
		}
	}

	for _, name := range spec.Policies.Exclude {
		delete(resolved, name)
	}

	members := make([]v1alpha1.EffectiveMember, 0, len(resolved))
	for _, m := range resolved {
		members = append(members, m)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	return members, conditions
}
