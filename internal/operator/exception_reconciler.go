package operator

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// ExceptionReconciler enforces the PolicyException lifecycle state machine.
//
// State machine:
//
//	Pending → Active (validation passed, now < expiresAt)
//	Active  → Expired (now ≥ expiresAt)
//	Expired → Deleted (now ≥ expiresAt + retainAfterExpiry)
//
// Expired is terminal: any spec change is rejected by the VAP; reconciler
// never transitions back to Active even if a new duration would push
// expiresAt into the future.
type ExceptionReconciler struct {
	client client.Client
	now    func() time.Time
}

func NewExceptionReconciler(c client.Client) *ExceptionReconciler {
	return &ExceptionReconciler{client: c, now: time.Now}
}

func (r *ExceptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pex v1alpha1.PolicyException
	if err := r.client.Get(ctx, req.NamespacedName, &pex); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Terminal state: handle TTL-based deletion or stay put.
	if pex.Status.Phase == v1alpha1.PhaseExpired {
		return r.handleExpired(ctx, &pex)
	}

	// 1. Validate one-of target selection.
	if msg := validateOneOf(&pex.Spec); msg != "" {
		return ctrl.Result{}, r.setInvalid(ctx, &pex, msg)
	}
	// 2. Validate reason.
	if strings.TrimSpace(pex.Spec.Reason) == "" {
		return ctrl.Result{}, r.setInvalid(ctx, &pex, "spec.reason must not be empty or whitespace")
	}
	// 3. Parse duration.
	d, err := time.ParseDuration(pex.Spec.Duration)
	if err != nil {
		return ctrl.Result{}, r.setInvalid(ctx, &pex, fmt.Sprintf("spec.duration %q: %v", pex.Spec.Duration, err))
	}
	if d <= 0 {
		return ctrl.Result{}, r.setInvalid(ctx, &pex, fmt.Sprintf("spec.duration must be positive, got %q", pex.Spec.Duration))
	}
	// 4. Parse retainAfterExpiry (default 0).
	var retain time.Duration
	if pex.Spec.RetainAfterExpiry != "" {
		retain, err = time.ParseDuration(pex.Spec.RetainAfterExpiry)
		if err != nil || retain < 0 {
			return ctrl.Result{}, r.setInvalid(ctx, &pex, fmt.Sprintf("spec.retainAfterExpiry invalid: %v", err))
		}
	}
	// 5. Validate references exist.
	if msg, err := r.validateRefs(ctx, &pex); err != nil {
		return ctrl.Result{}, err
	} else if msg != "" {
		return ctrl.Result{}, r.setInvalid(ctx, &pex, msg)
	}
	// 6. Compute time window from creationTimestamp.
	creationTS := pex.CreationTimestamp
	expiresAt := metav1.NewTime(creationTS.Add(d))
	now := r.now()

	if now.Before(expiresAt.Time) {
		return r.setActive(ctx, &pex, &creationTS, &expiresAt)
	}
	// Past expiration — transition straight to Expired (skip Active).
	return r.setExpired(ctx, &pex, &creationTS, &expiresAt, retain)
}

func (r *ExceptionReconciler) handleExpired(ctx context.Context, pex *v1alpha1.PolicyException) (ctrl.Result, error) {
	var retain time.Duration
	if pex.Spec.RetainAfterExpiry != "" {
		retain, _ = time.ParseDuration(pex.Spec.RetainAfterExpiry)
	}
	if pex.Status.ExpiresAt == nil {
		// Defensive: shouldn't happen.
		now := metav1.NewTime(r.now())
		pex.Status.ExpiresAt = &now
		if err := r.client.Status().Update(ctx, pex); err != nil {
			return ctrl.Result{}, fmt.Errorf("update status (Expired, missing ExpiresAt): %w", err)
		}
	}
	deleteAt := pex.Status.ExpiresAt.Add(retain)
	now := r.now()
	if !now.Before(deleteAt) {
		if err := r.client.Delete(ctx, pex); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete expired PolicyException: %w", err)
		}
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: deleteAt.Sub(now)}, nil
}

func (r *ExceptionReconciler) setActive(ctx context.Context, pex *v1alpha1.PolicyException, effective, expires *metav1.Time) (ctrl.Result, error) {
	pex.Status.Phase = v1alpha1.PhaseActive
	pex.Status.Message = ""
	pex.Status.EffectiveAt = effective
	pex.Status.ExpiresAt = expires
	pex.Status.ObservedGeneration = pex.Generation
	if err := r.client.Status().Update(ctx, pex); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status (Active): %w", err)
	}
	return ctrl.Result{RequeueAfter: expires.Sub(r.now())}, nil
}

func (r *ExceptionReconciler) setExpired(ctx context.Context, pex *v1alpha1.PolicyException, effective, expires *metav1.Time, retain time.Duration) (ctrl.Result, error) {
	pex.Status.Phase = v1alpha1.PhaseExpired
	pex.Status.Message = ""
	pex.Status.EffectiveAt = effective
	pex.Status.ExpiresAt = expires
	pex.Status.ObservedGeneration = pex.Generation
	if err := r.client.Status().Update(ctx, pex); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status (Expired): %w", err)
	}
	// Deletion is deferred to the next reconcile through handleExpired so the
	// Expired phase is observable for at least one cycle even when retain==0.
	return ctrl.Result{Requeue: true}, nil
}

func (r *ExceptionReconciler) setInvalid(ctx context.Context, pex *v1alpha1.PolicyException, msg string) error {
	pex.Status.Phase = v1alpha1.PhaseInvalid
	pex.Status.Message = msg
	pex.Status.ObservedGeneration = pex.Generation
	if err := r.client.Status().Update(ctx, pex); err != nil {
		return fmt.Errorf("update status (Invalid): %w", err)
	}
	return nil
}

func validateOneOf(s *v1alpha1.PolicyExceptionSpec) string {
	count := 0
	if len(s.PolicyRefs) > 0 {
		count++
	}
	if len(s.PolicyGroupRefs) > 0 {
		count++
	}
	if s.AllPolicies {
		count++
	}
	if count != 1 {
		return fmt.Sprintf("exactly one of spec.policyRefs/policyGroupRefs/allPolicies must be set (got %d)", count)
	}
	return ""
}

// validateRefs returns (msg, err). msg!="" → set Invalid; err!=nil → API error.
func (r *ExceptionReconciler) validateRefs(ctx context.Context, pex *v1alpha1.PolicyException) (string, error) {
	for _, name := range pex.Spec.PolicyRefs {
		var p v1alpha1.Policy
		err := r.client.Get(ctx, types.NamespacedName{Name: name}, &p)
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("spec.policyRefs[%q] not found", name), nil
		}
		if err != nil {
			return "", fmt.Errorf("get Policy %q: %w", name, err)
		}
	}
	for _, name := range pex.Spec.PolicyGroupRefs {
		var g v1alpha1.PolicyGroup
		err := r.client.Get(ctx, types.NamespacedName{Name: name}, &g)
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("spec.policyGroupRefs[%q] not found", name), nil
		}
		if err != nil {
			return "", fmt.Errorf("get PolicyGroup %q: %w", name, err)
		}
	}
	return "", nil
}

func (r *ExceptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PolicyException{}).
		Watches(&v1alpha1.Policy{}, requeueAllExceptions(mgr.GetClient())).
		Watches(&v1alpha1.PolicyGroup{}, requeueAllExceptions(mgr.GetClient())).
		Complete(r)
}

// requeueAllExceptions returns an EventHandler that enqueues a reconcile for
// every PolicyException whenever a Policy/PolicyGroup changes — necessary so
// dangling references auto-recover and reachability re-validates.
func requeueAllExceptions(c client.Client) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
		var list v1alpha1.PolicyExceptionList
		if err := c.List(ctx, &list); err != nil {
			return nil
		}
		out := make([]reconcile.Request, 0, len(list.Items))
		for _, item := range list.Items {
			out = append(out, reconcile.Request{NamespacedName: types.NamespacedName{Name: item.Name}})
		}
		return out
	})
}
