package operator

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

// PolicyReconciler reconciles Policy objects.
type PolicyReconciler struct {
	client              client.Client
	versionHistoryLimit int
}

func NewPolicyReconciler(c client.Client, historyLimit int) *PolicyReconciler {
	return &PolicyReconciler{client: c, versionHistoryLimit: historyLimit}
}

func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var policy v1alpha1.Policy
	if err := r.client.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle rollback request first — requeue to run normal reconcile after patching spec.
	if policy.Spec.RollbackTo != nil {
		return r.handleRollback(ctx, &policy)
	}

	// Already reconciled at this generation — nothing to do.
	if policy.Status.ObservedGeneration == policy.Generation && policy.Status.Phase == v1alpha1.PhaseReady {
		return ctrl.Result{}, nil
	}

	// Validate Rego.
	if _, err := webhook.CompileRego(policy.Spec.Rego); err != nil {
		return ctrl.Result{}, r.setInvalid(ctx, &policy, err.Error())
	}

	// Create PolicyVersion snapshot (idempotent: already-exists is fine if a
	// previous reconcile created it but failed before updating status).
	nextVersion := policy.Status.CurrentVersion + 1
	pv := r.buildPolicyVersion(&policy, nextVersion)
	if err := r.client.Create(ctx, pv); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, fmt.Errorf("create PolicyVersion: %w", err)
	}

	// Cleanup old versions.
	if err := r.pruneVersions(ctx, policy.Name); err != nil {
		logger.Error(err, "prune versions")
	}

	// Update status.
	now := metav1.NewTime(time.Now())
	policy.Status.Phase = v1alpha1.PhaseReady
	policy.Status.Message = ""
	policy.Status.CurrentVersion = nextVersion
	policy.Status.ObservedGeneration = policy.Generation
	policy.Status.LastSyncTime = &now
	policy.Status.VersionHistory = r.buildHistory(policy.Status.VersionHistory, nextVersion, v1alpha1.PhaseReady)

	if err := r.client.Status().Update(ctx, &policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *PolicyReconciler) handleRollback(ctx context.Context, policy *v1alpha1.Policy) (ctrl.Result, error) {
	targetVersion := policy.Spec.RollbackTo.Version

	var pvList v1alpha1.PolicyVersionList
	if err := r.client.List(ctx, &pvList, client.MatchingLabels{
		"kubesentry/policy":  policy.Name,
		"kubesentry/version": strconv.FormatInt(targetVersion, 10),
	}); err != nil {
		return ctrl.Result{}, err
	}
	if len(pvList.Items) == 0 {
		return ctrl.Result{}, r.setInvalid(ctx, policy, fmt.Sprintf("rollback target version %d not found", targetVersion))
	}

	pv := pvList.Items[0]
	patch := client.MergeFrom(policy.DeepCopy())
	policy.Spec.Rego = pv.Spec.Rego
	policy.Spec.Match = pv.Spec.Match
	policy.Spec.EnforcementMode = pv.Spec.EnforcementMode
	policy.Spec.RollbackTo = nil
	return ctrl.Result{Requeue: true}, r.client.Patch(ctx, policy, patch)
}

func (r *PolicyReconciler) setInvalid(ctx context.Context, policy *v1alpha1.Policy, msg string) error {
	policy.Status.Phase = v1alpha1.PhaseInvalid
	policy.Status.Message = msg
	return r.client.Status().Update(ctx, policy)
}

func (r *PolicyReconciler) buildPolicyVersion(policy *v1alpha1.Policy, version int64) *v1alpha1.PolicyVersion {
	return &v1alpha1.PolicyVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-v%d", policy.Name, version),
			Labels: map[string]string{
				"kubesentry/policy":  policy.Name,
				"kubesentry/version": strconv.FormatInt(version, 10),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha1.GroupVersion.String(),
					Kind:       "Policy",
					Name:       policy.Name,
					UID:        policy.UID,
				},
			},
		},
		Spec: v1alpha1.PolicyVersionSpec{
			PolicyRef:       policy.Name,
			Version:         version,
			Rego:            policy.Spec.Rego,
			Match:           policy.Spec.Match,
			EnforcementMode: policy.Spec.EnforcementMode,
			CreatedAt:       metav1.NewTime(time.Now()),
		},
	}
}

func (r *PolicyReconciler) pruneVersions(ctx context.Context, policyName string) error {
	var pvList v1alpha1.PolicyVersionList
	if err := r.client.List(ctx, &pvList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{"kubesentry/policy": policyName}),
	}); err != nil {
		return err
	}
	if len(pvList.Items) <= r.versionHistoryLimit {
		return nil
	}
	sort.Slice(pvList.Items, func(i, j int) bool {
		vi, _ := strconv.ParseInt(pvList.Items[i].Labels["kubesentry/version"], 10, 64)
		vj, _ := strconv.ParseInt(pvList.Items[j].Labels["kubesentry/version"], 10, 64)
		return vi < vj
	})
	toDelete := pvList.Items[:len(pvList.Items)-r.versionHistoryLimit]
	for i := range toDelete {
		if err := r.client.Delete(ctx, &toDelete[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *PolicyReconciler) buildHistory(existing []v1alpha1.PolicyVersionSummary, version int64, phase string) []v1alpha1.PolicyVersionSummary {
	entry := v1alpha1.PolicyVersionSummary{
		Version:   version,
		CreatedAt: metav1.NewTime(time.Now()),
		Phase:     phase,
	}
	history := append([]v1alpha1.PolicyVersionSummary{entry}, existing...)
	if len(history) > 10 {
		history = history[:10]
	}
	return history
}

func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Policy{}).
		Complete(r)
}
