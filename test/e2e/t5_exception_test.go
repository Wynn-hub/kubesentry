//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const pexNamespace = "pex-hr-system"

func setupPexNamespace(t *testing.T, ctx context.Context) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: pexNamespace}}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})
}

func deletePex(ctx context.Context, name string) {
	pex := &v1alpha1.PolicyException{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(ctx, pex)
}

// TestT5_ValidExemption: valid exemption admits violating pod; control namespace still blocks.
func TestT5_ValidExemption(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	out, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-valid.yaml")
	if err != nil {
		t.Fatalf("apply pex-valid: %v\n%s", err, out)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-valid-hr") })

	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-valid-hr", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatalf("pex never became Active: %v", err)
	}

	// Violating pod is admitted in the exempted namespace.
	out, err = helpers.KubectlApplyStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
	if err != nil {
		t.Fatalf("violating pod should be admitted under exemption, got: %v\n%s", err, out)
	}
	helpers.KubectlDeleteStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))

	// Control: same pod in a non-exempted namespace MUST be blocked.
	ctrlNs := "pex-ctrl-ns"
	_ = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ctrlNs}})
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ctrlNs}})
	})

	swapped := strings.ReplaceAll(helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"), pexNamespace, ctrlNs)
	out, err = helpers.KubectlApplyStdin(ctx, swapped)
	if err == nil {
		helpers.KubectlDeleteStdin(ctx, swapped)
		t.Errorf("control namespace pod should be blocked, but was admitted: %s", out)
	}
}

// TestT5_GroupExemption: PolicyGroup-level exemption.
func TestT5_GroupExemption(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	out, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-by-group.yaml")
	if err != nil {
		t.Fatalf("apply pex-by-group: %v\n%s", err, out)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-by-group") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-by-group", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	out, err = helpers.KubectlApplyStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
	if err != nil {
		t.Fatalf("group exemption should admit privileged pod, got: %v\n%s", err, out)
	}
	helpers.KubectlDeleteStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
}

// TestT5_AllPoliciesExemption: allPolicies whitelist.
func TestT5_AllPoliciesExemption(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	out, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-all-policies.yaml")
	if err != nil {
		t.Fatalf("apply pex-all-policies: %v\n%s", err, out)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-all-policies") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-all-policies", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	out, err = helpers.KubectlApplyStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
	if err != nil {
		t.Fatalf("allPolicies should admit anything; got: %v\n%s", err, out)
	}
	helpers.KubectlDeleteStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
}

// TestT5_ExtendDuration: extend duration → expiresAt recomputed, effectiveAt unchanged.
func TestT5_ExtendDuration(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	if _, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-valid.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-valid-hr") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-valid-hr", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	var pex v1alpha1.PolicyException
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pex-valid-hr"}, &pex); err != nil {
		t.Fatal(err)
	}
	origEffective := pex.Status.EffectiveAt
	origExpires := pex.Status.ExpiresAt
	patch := client.MergeFrom(pex.DeepCopy())
	pex.Spec.Duration = "1h"
	if err := k8sClient.Patch(ctx, &pex, patch); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	var got v1alpha1.PolicyException
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: "pex-valid-hr"}, &got)
	if !got.Status.EffectiveAt.Equal(origEffective) {
		t.Errorf("EffectiveAt changed: %v → %v", origEffective, got.Status.EffectiveAt)
	}
	if got.Status.ExpiresAt.Equal(origExpires) {
		t.Errorf("ExpiresAt should have been recomputed")
	}
}

// TestT5_ImmutableFieldRejected: immutable field patch is rejected.
func TestT5_ImmutableFieldRejected(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	if _, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-valid.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-valid-hr") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-valid-hr", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	var pex v1alpha1.PolicyException
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: "pex-valid-hr"}, &pex)
	patch := client.MergeFrom(pex.DeepCopy())
	pex.Spec.Match.Namespaces = []string{"other-ns"}
	if err := k8sClient.Patch(ctx, &pex, patch); err == nil {
		t.Error("expected immutability rejection")
	}
}

// TestT5_EmptyReasonRejected: empty reason → API rejects via CRD validation.
func TestT5_EmptyReasonRejected(t *testing.T) {
	ctx := context.Background()
	if _, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-invalid-empty-reason.yaml"); err == nil {
		t.Error("expected reject; reason was empty")
		deletePex(ctx, "pex-empty-reason")
	}
}

// TestT5_OneOfViolationRejected: oneOf violation → API rejects.
func TestT5_OneOfViolationRejected(t *testing.T) {
	ctx := context.Background()
	if _, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-invalid-oneof.yaml"); err == nil {
		t.Error("expected reject; both policyRefs and allPolicies were set")
		deletePex(ctx, "pex-oneof")
	}
}

// TestT5_DanglingRefRecovers: dangling policyRef → Invalid phase.
func TestT5_DanglingRefRecovers(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)
	if _, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-dangling-ref.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-dangling") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-dangling", v1alpha1.PhaseInvalid, 30*time.Second); err != nil {
		t.Fatalf("did not reach Invalid: %v", err)
	}
}

// TestT5_ExpiryAndAfter: short duration → Expired; post-expiry pod blocked.
func TestT5_ExpiryAndAfter(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	if _, err := helpers.KubectlApplyClusterScoped(ctx, "testdata/exceptions/pex-with-expiry.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-short") })

	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-short", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-short", v1alpha1.PhaseExpired, 60*time.Second); err != nil {
		t.Fatalf("never reached Expired: %v", err)
	}

	// Patch spec on Expired object → rejected.
	var got v1alpha1.PolicyException
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "pex-short"}, &got); err == nil {
		patch := client.MergeFrom(got.DeepCopy())
		got.Spec.Duration = "1h"
		if err := k8sClient.Patch(ctx, &got, patch); err == nil {
			t.Error("Expired terminal lock: patching spec should be rejected")
		}
	}

	// Violating pod blocked after exemption expired.
	out, err := helpers.KubectlApplyStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
	if err == nil {
		helpers.KubectlDeleteStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
		t.Errorf("post-expiry pod was admitted; should have been blocked: %s", out)
	}
}

// TestT5_MatchingNamespaceAndOR: OR semantics across multiple exceptions.
func TestT5_MatchingNamespaceAndOR(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	pex1 := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: "pex-or-a"},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-privileged"},
			Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{pexNamespace}},
			Duration:   "5m",
			Reason:     "or-a",
		},
	}
	pex2 := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: "pex-or-b"},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-root-allowed"},
			Match:      v1alpha1.PolicyExceptionMatch{Namespaces: []string{pexNamespace}},
			Duration:   "5m",
			Reason:     "or-b",
		},
	}
	for _, p := range []*v1alpha1.PolicyException{pex1, pex2} {
		if err := k8sClient.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		defer deletePex(context.Background(), p.Name)
	}
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-or-a", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-or-b", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	out, err := helpers.KubectlApplyStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
	if err != nil {
		t.Fatalf("OR semantics broken: pod blocked: %v\n%s", err, out)
	}
	helpers.KubectlDeleteStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
}

// TestT5_NamespaceSelectorMatch: namespaceSelector matching.
func TestT5_NamespaceSelectorMatch(t *testing.T) {
	ctx := context.Background()
	nsName := "pex-labeled-ns"
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   nsName,
		Labels: map[string]string{"env": "legacy"},
	}}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), ns) })

	pex := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: "pex-selector"},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-privileged"},
			Match: v1alpha1.PolicyExceptionMatch{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"env": "legacy"}},
			},
			Duration: "5m",
			Reason:   "selector",
		},
	}
	if err := k8sClient.Create(ctx, pex); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-selector") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-selector", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	body := strings.ReplaceAll(helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"), pexNamespace, nsName)
	if _, err := helpers.KubectlApplyStdin(ctx, body); err != nil {
		t.Fatalf("namespaceSelector failed to admit: %v", err)
	}
	helpers.KubectlDeleteStdin(ctx, body)
}

// TestT5_ResourceSelectorMatch: resourceSelector — same ns, different label, different outcome.
func TestT5_ResourceSelectorMatch(t *testing.T) {
	ctx := context.Background()
	setupPexNamespace(t, ctx)

	pex := &v1alpha1.PolicyException{
		ObjectMeta: metav1.ObjectMeta{Name: "pex-reslabel"},
		Spec: v1alpha1.PolicyExceptionSpec{
			PolicyRefs: []string{"run-as-privileged"},
			Match: v1alpha1.PolicyExceptionMatch{
				Namespaces:       []string{pexNamespace},
				ResourceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "legacy-billing"}},
			},
			Duration: "5m",
			Reason:   "reslabel",
		},
	}
	if err := k8sClient.Create(ctx, pex); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deletePex(context.Background(), "pex-reslabel") })
	if err := helpers.WaitForExceptionPhase(ctx, k8sClient, "pex-reslabel", v1alpha1.PhaseActive, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	// Pod with matching label → admitted.
	out, err := helpers.KubectlApplyStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))
	if err != nil {
		t.Fatalf("matching label should be admitted: %v\n%s", err, out)
	}
	helpers.KubectlDeleteStdin(ctx, helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"))

	// Pod with non-matching label → blocked.
	noLabel := strings.ReplaceAll(helpers.MustReadFile(t, "testdata/exceptions/privileged-pod.yaml"),
		"app: legacy-billing", "app: modern")
	out, err = helpers.KubectlApplyStdin(ctx, noLabel)
	if err == nil {
		helpers.KubectlDeleteStdin(ctx, noLabel)
		t.Errorf("non-matching label pod should have been blocked: %s", out)
	}
}
