package gateway

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// ClusterReconciler issues one-time registration tokens for new (or
// re-onboarding) member clusters. Deleting a cluster's auth Secret is the
// operational entry point for re-registration: the next reconcile issues
// a fresh registration token.
type ClusterReconciler struct {
	client    client.Client
	namespace string // gateway namespace where token Secrets live
}

func NewClusterReconciler(c client.Client, namespace string) *ClusterReconciler {
	return &ClusterReconciler{client: c, namespace: namespace}
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cl v1alpha1.Cluster
	if err := r.client.Get(ctx, req.NamespacedName, &cl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Already registered → nothing to issue.
	var auth corev1.Secret
	err := r.client.Get(ctx,
		types.NamespacedName{Namespace: r.namespace, Name: AuthSecretName(cl.Name)}, &auth)
	if err == nil {
		return ctrl.Result{}, nil
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("get auth secret: %w", err)
	}

	// Ensure a registration token Secret exists. This check is independently
	// idempotent from the phase update below: a retry after a partial
	// failure (e.g. Create succeeded but Status().Update below failed) must
	// still reach the phase-setting code, not short-circuit on it.
	var reg corev1.Secret
	err = r.client.Get(ctx,
		types.NamespacedName{Namespace: r.namespace, Name: RegTokenSecretName(cl.Name)}, &reg)
	switch {
	case err == nil:
		// already issued on a prior reconcile — fall through to the phase check
	case apierrors.IsNotFound(err):
		token, tokenErr := NewToken()
		if tokenErr != nil {
			return ctrl.Result{}, tokenErr
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.namespace,
				Name:      RegTokenSecretName(cl.Name),
				Labels:    map[string]string{"kubesentry.io/cluster": cl.Name},
			},
			Data: map[string][]byte{KeyToken: []byte(token)},
		}
		if createErr := r.client.Create(ctx, secret); createErr != nil {
			return ctrl.Result{}, fmt.Errorf("create reg-token secret: %w", createErr)
		}
	default:
		return ctrl.Result{}, fmt.Errorf("get reg-token secret: %w", err)
	}

	if cl.Status.Phase == "" {
		cl.Status.Phase = v1alpha1.ClusterPhaseRegistering
		if err := r.client.Status().Update(ctx, &cl); err != nil {
			return ctrl.Result{}, fmt.Errorf("update cluster status: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Cluster{}).
		Complete(r)
}
