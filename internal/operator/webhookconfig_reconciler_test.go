package operator_test

import (
	"context"
	"testing"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/operator"
)

func buildSchemeWithAdmission() *runtime.Scheme {
	s := buildScheme()
	_ = admissionregv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

func makeTLSSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubesentry-tls",
			Namespace: "kubesentry-system",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("fake-ca-bundle"),
		},
	}
}

func TestWebhookConfigReconcile(t *testing.T) {
	policies := []v1alpha1.Policy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "policy-a"},
			Spec: v1alpha1.PolicySpec{
				Match: v1alpha1.PolicyMatch{
					Operations: []string{"CREATE"},
					Resources: []v1alpha1.MatchResource{
						{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
					},
				},
			},
			Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "policy-b"},
			Spec: v1alpha1.PolicySpec{
				Match: v1alpha1.PolicyMatch{
					Operations: []string{"CREATE", "UPDATE"},
					Resources: []v1alpha1.MatchResource{
						{APIGroups: []string{"apps"}, APIVersions: []string{"v1"}, Resources: []string{"deployments"}},
					},
				},
			},
			Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady},
		},
	}

	failPolicy := admissionregv1.Fail
	sideEffects := admissionregv1.SideEffectClassNone
	vwc := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "kubesentry"},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "validate.kubesentry.io",
				FailurePolicy:           &failPolicy,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
				ClientConfig:            admissionregv1.WebhookClientConfig{},
			},
		},
	}

	objs := []runtime.Object{vwc, &policies[0], &policies[1], makeTLSSecret()}
	c := fake.NewClientBuilder().WithScheme(buildSchemeWithAdmission()).WithRuntimeObjects(objs...).Build()
	r := operator.NewWebhookConfigReconciler(c, "kubesentry", "kubesentry-tls", "kubesentry-system")

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "kubesentry"}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var updated admissionregv1.ValidatingWebhookConfiguration
	if err := c.Get(context.Background(), types.NamespacedName{Name: "kubesentry"}, &updated); err != nil {
		t.Fatal(err)
	}

	rules := updated.Webhooks[0].Rules
	if len(rules) == 0 {
		t.Fatal("expected webhook rules to be populated")
	}
}

func TestWebhookConfigSkipsInvalidPolicies(t *testing.T) {
	policies := []v1alpha1.Policy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-policy"},
			Spec: v1alpha1.PolicySpec{
				Match: v1alpha1.PolicyMatch{
					Operations: []string{"CREATE"},
					Resources:  []v1alpha1.MatchResource{{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}},
				},
			},
			Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseInvalid},
		},
	}

	failPolicy := admissionregv1.Fail
	sideEffects := admissionregv1.SideEffectClassNone
	vwc := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "kubesentry"},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "validate.kubesentry.io",
				FailurePolicy:           &failPolicy,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	objs := []runtime.Object{vwc, &policies[0], makeTLSSecret()}
	c := fake.NewClientBuilder().WithScheme(buildSchemeWithAdmission()).WithRuntimeObjects(objs...).Build()
	r := operator.NewWebhookConfigReconciler(c, "kubesentry", "kubesentry-tls", "kubesentry-system")

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "kubesentry"}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var updated admissionregv1.ValidatingWebhookConfiguration
	if err := c.Get(context.Background(), types.NamespacedName{Name: "kubesentry"}, &updated); err != nil {
		t.Fatal(err)
	}

	if len(updated.Webhooks[0].Rules) != 0 {
		t.Errorf("expected 0 rules for invalid policy, got %d", len(updated.Webhooks[0].Rules))
	}
}

func TestWebhookConfigDeduplicatesRules(t *testing.T) {
	// Two policies with identical match rules should result in single aggregated rule
	policies := []v1alpha1.Policy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "policy-1"},
			Spec: v1alpha1.PolicySpec{
				Match: v1alpha1.PolicyMatch{
					Operations: []string{"CREATE"},
					Resources: []v1alpha1.MatchResource{
						{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
					},
				},
			},
			Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "policy-2"},
			Spec: v1alpha1.PolicySpec{
				Match: v1alpha1.PolicyMatch{
					Operations: []string{"CREATE"},
					Resources: []v1alpha1.MatchResource{
						{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
					},
				},
			},
			Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady},
		},
	}

	failPolicy := admissionregv1.Fail
	sideEffects := admissionregv1.SideEffectClassNone
	vwc := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "kubesentry"},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "validate.kubesentry.io",
				FailurePolicy:           &failPolicy,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	objs := []runtime.Object{vwc, &policies[0], &policies[1], makeTLSSecret()}
	c := fake.NewClientBuilder().WithScheme(buildSchemeWithAdmission()).WithRuntimeObjects(objs...).Build()
	r := operator.NewWebhookConfigReconciler(c, "kubesentry", "kubesentry-tls", "kubesentry-system")

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "kubesentry"}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var updated admissionregv1.ValidatingWebhookConfiguration
	if err := c.Get(context.Background(), types.NamespacedName{Name: "kubesentry"}, &updated); err != nil {
		t.Fatal(err)
	}

	// Should have exactly 1 rule (deduplicated from 2 identical rules)
	rules := updated.Webhooks[0].Rules
	if len(rules) != 1 {
		t.Errorf("expected 1 deduplicated rule, got %d", len(rules))
	}
}

func TestWebhookConfigVWCNotFound(t *testing.T) {
	// When VWC doesn't exist, reconcile should not error
	policy := v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-exists"},
		Spec: v1alpha1.PolicySpec{
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources:  []v1alpha1.MatchResource{{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}},
			},
		},
		Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady},
	}

	// Only policy, no VWC, no TLS secret
	objs := []runtime.Object{&policy}
	c := fake.NewClientBuilder().WithScheme(buildSchemeWithAdmission()).WithRuntimeObjects(objs...).Build()
	r := operator.NewWebhookConfigReconciler(c, "kubesentry", "kubesentry-tls", "kubesentry-system")

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "kubesentry"}})
	if err != nil {
		t.Fatalf("reconcile should not error when VWC not found: %v", err)
	}
}

func TestWebhookConfigSecretNotFound(t *testing.T) {
	// When TLS Secret doesn't exist, reconcile should not error (secret may not be ready yet)
	failPolicy := admissionregv1.Fail
	sideEffects := admissionregv1.SideEffectClassNone
	vwc := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "kubesentry"},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name:                    "validate.kubesentry.io",
				FailurePolicy:           &failPolicy,
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	policy := v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-exists"},
		Spec: v1alpha1.PolicySpec{
			Match: v1alpha1.PolicyMatch{
				Operations: []string{"CREATE"},
				Resources:  []v1alpha1.MatchResource{{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}},
			},
		},
		Status: v1alpha1.PolicyStatus{Phase: v1alpha1.PhaseReady},
	}

	// VWC and policy exist, but no TLS secret
	objs := []runtime.Object{vwc, &policy}
	c := fake.NewClientBuilder().WithScheme(buildSchemeWithAdmission()).WithRuntimeObjects(objs...).Build()
	r := operator.NewWebhookConfigReconciler(c, "kubesentry", "kubesentry-tls", "kubesentry-system")

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "kubesentry"}})
	if err != nil {
		t.Fatalf("reconcile should not error when Secret not found: %v", err)
	}
}
