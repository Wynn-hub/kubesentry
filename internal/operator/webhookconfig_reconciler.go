package operator

import (
	"context"
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// WebhookConfigReconciler keeps the ValidatingWebhookConfiguration rules
// and caBundle in sync with all Ready Policy objects and the TLS secret.
type WebhookConfigReconciler struct {
	client        client.Client
	vwcName       string
	tlsSecretName string
	tlsNamespace  string
}

func NewWebhookConfigReconciler(c client.Client, vwcName, tlsSecretName, tlsNamespace string) *WebhookConfigReconciler {
	return &WebhookConfigReconciler{
		client:        c,
		vwcName:       vwcName,
		tlsSecretName: tlsSecretName,
		tlsNamespace:  tlsNamespace,
	}
}

func (r *WebhookConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var policyList v1alpha1.PolicyList
	if err := r.client.List(ctx, &policyList); err != nil {
		return ctrl.Result{}, fmt.Errorf("list policies: %w", err)
	}
	rules := aggregateRules(policyList.Items)

	var secret corev1.Secret
	if err := r.client.Get(ctx, types.NamespacedName{Name: r.tlsSecretName, Namespace: r.tlsNamespace}, &secret); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	caBundle := secret.Data["ca.crt"]

	var vwc admissionregv1.ValidatingWebhookConfiguration
	if err := r.client.Get(ctx, types.NamespacedName{Name: r.vwcName}, &vwc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patch := client.MergeFrom(vwc.DeepCopy())
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].Rules = rules
		vwc.Webhooks[i].ClientConfig.CABundle = caBundle
	}
	return ctrl.Result{}, r.client.Patch(ctx, &vwc, patch)
}

// aggregateRules merges all Ready policy match rules into webhook rules.
// Rules are deduplicated by apiGroup+resource+operations key.
func aggregateRules(policies []v1alpha1.Policy) []admissionregv1.RuleWithOperations {
	type key struct{ apiGroup, resource, ops string }
	seen := map[key]bool{}
	var rules []admissionregv1.RuleWithOperations

	for _, p := range policies {
		if p.Status.Phase != v1alpha1.PhaseReady {
			continue
		}
		for _, mr := range p.Spec.Match.Resources {
			for _, group := range mr.APIGroups {
				for _, res := range mr.Resources {
					k := key{group, res, joinOps(p.Spec.Match.Operations)}
					if seen[k] {
						continue
					}
					seen[k] = true
					scope := admissionregv1.AllScopes
					rules = append(rules, admissionregv1.RuleWithOperations{
						Operations: toWebhookOps(p.Spec.Match.Operations),
						Rule: admissionregv1.Rule{
							APIGroups:   []string{group},
							APIVersions: mr.APIVersions,
							Resources:   []string{res},
							Scope:       &scope,
						},
					})
				}
			}
		}
	}
	return rules
}

func toWebhookOps(ops []string) []admissionregv1.OperationType {
	out := make([]admissionregv1.OperationType, len(ops))
	for i, op := range ops {
		out[i] = admissionregv1.OperationType(op)
	}
	return out
}

func joinOps(ops []string) string {
	result := ""
	for _, op := range ops {
		result += op + ","
	}
	return result
}

func (r *WebhookConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueVWC := handler.EnqueueRequestsFromMapFunc(
		func(_ context.Context, _ client.Object) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: r.vwcName}},
			}
		},
	)
	return ctrl.NewControllerManagedBy(mgr).
		For(&admissionregv1.ValidatingWebhookConfiguration{}).
		Watches(&v1alpha1.Policy{}, enqueueVWC).
		Complete(r)
}
