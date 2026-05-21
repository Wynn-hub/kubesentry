package operator

import (
	"context"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

const (
	condTypeMissingMember   = "MissingMember"
	condTypeInvalidSelector = "InvalidSelector"
)

// PolicyGroupReconciler resolves a PolicyGroup's members from byName + bySelector,
// computes effective mode for each, and writes status to the group + every
// referenced Policy.
type PolicyGroupReconciler struct {
	client client.Client
}

func NewPolicyGroupReconciler(c client.Client) *PolicyGroupReconciler {
	return &PolicyGroupReconciler{client: c}
}

func (r *PolicyGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("policygroup", req.Name)

	var pg v1alpha1.PolicyGroup
	if err := r.client.Get(ctx, req.NamespacedName, &pg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Snapshot previous resolved set before any mutation, so we can compute the
	// referencedBy diff at the end of the reconcile.
	prev := snapshotResolved(pg.Status.ResolvedPolicies)

	if !pg.Spec.Enabled {
		// Clear status, then refresh referencedBy on previously-resolved policies.
		pg.Status.Phase = v1alpha1.PhaseDisabled
		pg.Status.ResolvedCount = 0
		pg.Status.ResolvedPolicies = nil
		pg.Status.Conditions = nil
		pg.Status.ObservedGeneration = pg.Generation
		if err := r.client.Status().Update(ctx, &pg); err != nil {
			return ctrl.Result{}, fmt.Errorf("update group status: %w", err)
		}
		return ctrl.Result{}, r.refreshReferencedBy(ctx, prev, map[string]struct{}{})
	}

	resolved := map[string]v1alpha1.EffectiveMember{}
	var conditions []metav1.Condition

	// ---- byName path (takes precedence on conflict with bySelector) ----
	for _, ref := range pg.Spec.Policies.ByName {
		var pol v1alpha1.Policy
		err := r.client.Get(ctx, types.NamespacedName{Name: ref.Name}, &pol)
		if apierrors.IsNotFound(err) {
			conditions = append(conditions, metav1.Condition{
				Type:               condTypeMissingMember,
				Status:             metav1.ConditionTrue,
				Reason:             "PolicyNotFound",
				Message:            ref.Name,
				LastTransitionTime: metav1.Now(),
			})
			continue
		}
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("get Policy %q: %w", ref.Name, err)
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

	// ---- bySelector path ----
	if pg.Spec.Policies.BySelector != nil {
		sel, err := metav1.LabelSelectorAsSelector(pg.Spec.Policies.BySelector)
		if err != nil {
			conditions = append(conditions, metav1.Condition{
				Type:               condTypeInvalidSelector,
				Status:             metav1.ConditionTrue,
				Reason:             "MalformedSelector",
				Message:            err.Error(),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			var allPolicies v1alpha1.PolicyList
			if err := r.client.List(ctx, &allPolicies); err != nil {
				return ctrl.Result{}, fmt.Errorf("list Policies: %w", err)
			}
			for i := range allPolicies.Items {
				pol := &allPolicies.Items[i]
				if !sel.Matches(labels.Set(pol.Labels)) {
					continue
				}
				if _, alreadyByName := resolved[pol.Name]; alreadyByName {
					continue // byName precedence
				}
				mode := pg.Spec.SelectorEnforcementMode
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

	// Sort resolved by name for stable status.
	members := make([]v1alpha1.EffectiveMember, 0, len(resolved))
	for _, m := range resolved {
		members = append(members, m)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })

	phase := v1alpha1.PhaseReady
	if len(conditions) > 0 {
		phase = v1alpha1.PhaseDegraded
	}

	pg.Status.Phase = phase
	pg.Status.ObservedGeneration = pg.Generation
	pg.Status.ResolvedCount = int32(len(members))
	pg.Status.ResolvedPolicies = members
	pg.Status.Conditions = conditions

	if err := r.client.Status().Update(ctx, &pg); err != nil {
		return ctrl.Result{}, fmt.Errorf("update group status: %w", err)
	}
	logger.V(1).Info("reconciled", "members", len(members), "phase", phase)

	// Build current set for referencedBy diff.
	current := map[string]struct{}{}
	for _, m := range members {
		current[m.Name] = struct{}{}
	}
	return ctrl.Result{}, r.refreshReferencedBy(ctx, prev, current)
}

func snapshotResolved(in []v1alpha1.EffectiveMember) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, m := range in {
		out[m.Name] = struct{}{}
	}
	return out
}

// refreshReferencedBy recomputes Policy.status.referencedBy for the union of
// (previously-resolved union currently-resolved) Policies.
func (r *PolicyGroupReconciler) refreshReferencedBy(ctx context.Context, prev, current map[string]struct{}) error {
	touch := make(map[string]struct{}, len(prev)+len(current))
	for n := range prev {
		touch[n] = struct{}{}
	}
	for n := range current {
		touch[n] = struct{}{}
	}

	// Need the full PolicyGroup list to recompute each touched Policy's referencedBy.
	var groups v1alpha1.PolicyGroupList
	if err := r.client.List(ctx, &groups); err != nil {
		return fmt.Errorf("list PolicyGroups for referencedBy diff: %w", err)
	}

	for name := range touch {
		// Collect groups whose status.resolvedPolicies currently include `name`.
		var refs []string
		for _, g := range groups.Items {
			for _, m := range g.Status.ResolvedPolicies {
				if m.Name == name {
					refs = append(refs, g.Name)
					break
				}
			}
		}
		sort.Strings(refs)

		var pol v1alpha1.Policy
		if err := r.client.Get(ctx, types.NamespacedName{Name: name}, &pol); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("get Policy %q for referencedBy update: %w", name, err)
		}
		if stringSlicesEqual(pol.Status.ReferencedBy, refs) {
			continue // no diff, skip write
		}
		pol.Status.ReferencedBy = refs
		if err := r.client.Status().Update(ctx, &pol); err != nil {
			return fmt.Errorf("update Policy %q referencedBy: %w", name, err)
		}
	}
	return nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// referencesPolicy checks whether group g would include policy pol via byName or bySelector.
func referencesPolicy(g *v1alpha1.PolicyGroup, pol *v1alpha1.Policy) bool {
	for _, ref := range g.Spec.Policies.ByName {
		if ref.Name == pol.Name {
			return true
		}
	}
	if g.Spec.Policies.BySelector != nil {
		if sel, err := metav1.LabelSelectorAsSelector(g.Spec.Policies.BySelector); err == nil {
			if sel.Matches(labels.Set(pol.Labels)) {
				return true
			}
		}
	}
	return false
}

func (r *PolicyGroupReconciler) findGroupsReferencing(ctx context.Context, obj client.Object) []reconcile.Request {
	pol, ok := obj.(*v1alpha1.Policy)
	if !ok {
		return nil
	}
	var groups v1alpha1.PolicyGroupList
	if err := r.client.List(ctx, &groups); err != nil {
		return nil
	}
	out := make([]reconcile.Request, 0, len(groups.Items))
	for i := range groups.Items {
		g := &groups.Items[i]
		if referencesPolicy(g, pol) {
			out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Name: g.Name}})
			continue
		}
		for _, m := range g.Status.ResolvedPolicies {
			if m.Name == pol.Name {
				out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Name: g.Name}})
				break
			}
		}
	}
	return out
}

func (r *PolicyGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PolicyGroup{}).
		Watches(
			&v1alpha1.Policy{},
			handler.EnqueueRequestsFromMapFunc(r.findGroupsReferencing),
		).
		Complete(r)
}
