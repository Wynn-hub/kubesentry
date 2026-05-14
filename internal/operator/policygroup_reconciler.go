package operator

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/builtins"
)

// PolicyGroupReconciler reconciles PolicyGroup objects.
type PolicyGroupReconciler struct {
	client client.Client
}

func NewPolicyGroupReconciler(c client.Client) *PolicyGroupReconciler {
	return &PolicyGroupReconciler{client: c}
}

func (r *PolicyGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pg v1alpha1.PolicyGroup
	if err := r.client.Get(ctx, req.NamespacedName, &pg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pg.Spec.Enabled {
		return ctrl.Result{}, r.deleteOwnedPolicies(ctx, &pg)
	}

	var (
		active   int
		skipped  int
		newConds []metav1.Condition
	)

	for _, entry := range pg.Spec.Policies {
		if entry.Enabled != nil && !*entry.Enabled {
			if err := r.ensurePolicyAbsent(ctx, entry.Key, &pg); err != nil {
				logger.Error(err, "delete disabled policy", "key", entry.Key)
			}
			continue
		}

		builtin, found := builtins.Library[entry.Key]
		rego := entry.Rego
		match := entry.Match
		mode := entry.Mode
		desc := ""

		if found {
			if rego == "" {
				rego = builtin.Rego
			}
			if match == nil {
				m := builtin.Match
				match = &m
			}
			if mode == "" {
				mode = builtin.DefaultMode
			}
			desc = builtin.Description
		} else if rego == "" {
			newConds = append(newConds, metav1.Condition{
				Type:               "InvalidPolicy",
				Status:             metav1.ConditionTrue,
				Reason:             "KeyNotFound",
				Message:            fmt.Sprintf("key %q not in built-in library and no rego provided", entry.Key),
				LastTransitionTime: metav1.Now(),
			})
			continue
		}

		if match == nil {
			newConds = append(newConds, metav1.Condition{
				Type:               "InvalidPolicy",
				Status:             metav1.ConditionTrue,
				Reason:             "MatchRequired",
				Message:            fmt.Sprintf("key %q has no match spec", entry.Key),
				LastTransitionTime: metav1.Now(),
			})
			continue
		}

		didSkip, err := r.reconcilePolicy(ctx, &pg, entry.Key, rego, *match, mode, desc)
		if err != nil {
			logger.Error(err, "reconcile policy", "key", entry.Key)
			continue
		}
		if didSkip {
			skipped++
			newConds = append(newConds, metav1.Condition{
				Type:               "CustomOverride",
				Status:             metav1.ConditionTrue,
				Reason:             "CustomPolicyExists",
				Message:            fmt.Sprintf("key %q is overridden by a custom Policy", entry.Key),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			active++
		}
	}

	phase := v1alpha1.PhaseReady
	if len(newConds) > 0 {
		phase = v1alpha1.PhaseDegraded
	}

	pg.Status.Phase = phase
	pg.Status.ActivePolicies = active
	pg.Status.SkippedPolicies = skipped
	pg.Status.Conditions = newConds
	return ctrl.Result{}, r.client.Status().Update(ctx, &pg)
}

// reconcilePolicy creates or updates the Policy for a given key.
// Returns (skipped, error) where skipped=true means a custom policy takes priority.
func (r *PolicyGroupReconciler) reconcilePolicy(
	ctx context.Context, pg *v1alpha1.PolicyGroup,
	key, rego string, match v1alpha1.PolicyMatch, mode, desc string,
) (bool, error) {
	var existing v1alpha1.PolicyList
	if err := r.client.List(ctx, &existing, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{v1alpha1.LabelKey: key}),
	}); err != nil {
		return false, fmt.Errorf("list policies by key: %w", err)
	}

	source := v1alpha1.SourceBuiltin
	if _, inLib := builtins.Library[key]; !inLib {
		source = v1alpha1.SourceCustom
	}

	desired := v1alpha1.PolicySpec{
		EnforcementMode: mode,
		Rego:            rego,
		Match:           match,
		Description:     desc,
	}

	if len(existing.Items) > 0 {
		found := existing.Items[0]
		if !isOwnedByGroup(&found, pg) {
			return true, nil // custom policy wins
		}
		// owned by this group → patch if changed
		patch := client.MergeFrom(found.DeepCopy())
		found.Spec = desired
		return false, r.client.Patch(ctx, &found, patch)
	}

	// create
	policy := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name: CamelToKebab(key),
			Labels: map[string]string{
				v1alpha1.LabelKey:    key,
				v1alpha1.LabelGroup:  pg.Name,
				v1alpha1.LabelSource: source,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha1.GroupVersion.String(),
					Kind:       "PolicyGroup",
					Name:       pg.Name,
					UID:        pg.UID,
					Controller: pgBoolRef(true),
				},
			},
		},
		Spec: desired,
	}
	return false, r.client.Create(ctx, policy)
}

func (r *PolicyGroupReconciler) deleteOwnedPolicies(ctx context.Context, pg *v1alpha1.PolicyGroup) error {
	var pList v1alpha1.PolicyList
	if err := r.client.List(ctx, &pList); err != nil {
		return err
	}
	for i := range pList.Items {
		p := &pList.Items[i]
		if isOwnedByGroup(p, pg) {
			if err := r.client.Delete(ctx, p); err != nil {
				return err
			}
		}
	}
	pg.Status.Phase = v1alpha1.PhaseDisabled
	pg.Status.ActivePolicies = 0
	return r.client.Status().Update(ctx, pg)
}

func (r *PolicyGroupReconciler) ensurePolicyAbsent(ctx context.Context, key string, pg *v1alpha1.PolicyGroup) error {
	var pList v1alpha1.PolicyList
	if err := r.client.List(ctx, &pList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{v1alpha1.LabelKey: key}),
	}); err != nil {
		return err
	}
	for i := range pList.Items {
		p := &pList.Items[i]
		if isOwnedByGroup(p, pg) {
			return r.client.Delete(ctx, p)
		}
	}
	return nil
}

func isOwnedByGroup(p *v1alpha1.Policy, pg *v1alpha1.PolicyGroup) bool {
	for _, ref := range p.OwnerReferences {
		if ref.Kind == "PolicyGroup" && ref.Name == pg.Name && ref.UID == pg.UID {
			return true
		}
	}
	return false
}

// CamelToKebab converts a camelCase key to a kebab-case Kubernetes name.
// e.g. "runAsPrivileged" → "run-as-privileged"
func CamelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// pgBoolRef returns a pointer to the given bool value.
// Named pgBoolRef to avoid collisions with any boolRef defined in other files.
func pgBoolRef(b bool) *bool { return &b }

func (r *PolicyGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PolicyGroup{}).
		Owns(&v1alpha1.Policy{}).
		Complete(r)
}
