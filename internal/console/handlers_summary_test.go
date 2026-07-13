package console

import (
	"testing"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func TestSummary(t *testing.T) {
	h, _ := newTestServer(t,
		testPolicy("p1", "builtin", "enforce", "Ready"),
		testPolicy("p2", "custom", "audit", "Invalid"),
		testGroup("g1", true),
	)
	rec, env := doRequest(t, h, "GET", "/api/v1/summary", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	s := mustUnmarshal[summaryData](t, env.Data)
	if s.Totals["policies"] != 2 || s.Totals["policygroups"] != 1 || s.Totals["exceptions"] != 0 {
		t.Fatalf("totals = %+v", s.Totals)
	}
	if s.PolicyPhases["Ready"] != 1 || s.PolicyPhases["Invalid"] != 1 {
		t.Fatalf("policyPhases = %+v", s.PolicyPhases)
	}
	if s.PolicyModes["enforce"] != 1 || s.PolicyModes["audit"] != 1 {
		t.Fatalf("policyModes = %+v", s.PolicyModes)
	}
}

func TestSummaryExtended(t *testing.T) {
	group := testGroup("g1", true)
	group.Spec.NamespaceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"env": "prod"},
	}
	group.Status.ResolvedPolicies = []v1alpha1.EffectiveMember{
		{Name: "p1", EnforcementMode: "enforce", Source: "byName"},
		{Name: "p2", EnforcementMode: "audit", Source: "bySelector"},
	}

	older := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	newer := metav1.NewTime(time.Now().Add(-1 * time.Hour))

	h, _ := newTestServer(t,
		testPolicy("p1", "builtin", "enforce", "Ready"),
		testPolicy("p2", "custom", "audit", "Ready"),
		group,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "ns-prod", Labels: map[string]string{"env": "prod"},
		}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-dev"}},
		&admissionregv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "kubesentry"},
			Webhooks: []admissionregv1.ValidatingWebhook{{
				Name: "validate.kubesentry.io",
				ClientConfig: admissionregv1.WebhookClientConfig{
					CABundle: []byte("fake-ca"),
				},
				Rules: []admissionregv1.RuleWithOperations{{}, {}},
			}},
		},
		&v1alpha1.PolicyVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "p1-v1"},
			Spec: v1alpha1.PolicyVersionSpec{
				PolicyRef: "p1", Version: 1, CreatedAt: older,
			},
		},
		&v1alpha1.PolicyVersion{
			ObjectMeta: metav1.ObjectMeta{Name: "p1-v2"},
			Spec: v1alpha1.PolicyVersionSpec{
				PolicyRef: "p1", Version: 2, CreatedAt: newer,
			},
		},
	)

	rec, env := doRequest(t, h, "GET", "/api/v1/summary", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d body = %s", rec.Code, rec.Body.String())
	}
	s := mustUnmarshal[summaryData](t, env.Data)

	if !s.Webhook.Configured || !s.Webhook.CABundleSet || s.Webhook.RuleCount != 2 {
		t.Fatalf("webhook = %+v", s.Webhook)
	}
	if s.NamespaceCoverage.Protected != 1 || s.NamespaceCoverage.Total != 2 {
		t.Fatalf("namespaceCoverage = %+v", s.NamespaceCoverage)
	}
	if len(s.Groups) != 1 {
		t.Fatalf("groups = %+v", s.Groups)
	}
	g := s.Groups[0]
	if g.Name != "g1" || !g.Enabled || g.EnforceCount != 1 || g.AuditCount != 1 ||
		g.ResolvedCount != 2 || g.MatchedNamespaces != 1 {
		t.Fatalf("group summary = %+v", g)
	}
	if len(s.RecentVersions) != 2 ||
		s.RecentVersions[0].Version != 2 || s.RecentVersions[0].Policy != "p1" {
		t.Fatalf("recentVersions = %+v", s.RecentVersions)
	}
}

func TestSummaryNilSelectorMatchesAllNamespaces(t *testing.T) {
	group := testGroup("g1", true) // NamespaceSelector 为 nil
	h, _ := newTestServer(t,
		group,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
	)
	rec, env := doRequest(t, h, "GET", "/api/v1/summary", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	s := mustUnmarshal[summaryData](t, env.Data)
	if s.NamespaceCoverage.Protected != 2 || s.NamespaceCoverage.Total != 2 {
		t.Fatalf("namespaceCoverage = %+v", s.NamespaceCoverage)
	}
	if s.Groups[0].MatchedNamespaces != 2 {
		t.Fatalf("matchedNamespaces = %d", s.Groups[0].MatchedNamespaces)
	}
	// VWC 不存在 → configured=false，接口不报错
	if s.Webhook.Configured {
		t.Fatalf("webhook should be unconfigured: %+v", s.Webhook)
	}
}
