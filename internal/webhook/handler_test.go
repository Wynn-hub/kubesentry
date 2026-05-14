package webhook_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

// stubStore implements webhook.PolicyStore for testing.
type stubStore struct {
	policies []*webhook.CompiledPolicy
	ready    bool
}

func (s *stubStore) MatchingPolicies(resource, apiGroup, operation, namespace string) []*webhook.CompiledPolicy {
	return s.policies
}

func (s *stubStore) IsReady() bool { return s.ready }

func buildAdmissionRequest(op string, privileged bool) []byte {
	ar := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Operation: admissionv1.Operation(op),
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			Namespace: "default",
			Object: runtime.RawExtension{
				Raw: mustJSON(map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"securityContext": map[string]interface{}{"privileged": privileged},
							},
						},
					},
				}),
			},
		},
	}
	data, _ := json.Marshal(ar)
	return data
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func compiledEnforcePolicy(t *testing.T) *webhook.CompiledPolicy {
	t.Helper()
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}
	return &webhook.CompiledPolicy{
		Name:            "deny-privileged",
		EnforcementMode: v1alpha1.ModeEnforce,
		Match: v1alpha1.PolicyMatch{
			Operations: []string{"CREATE"},
			Resources:  []v1alpha1.MatchResource{{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}}},
		},
		Query: q,
	}
}

func TestHandlerNotReady(t *testing.T) {
	h := webhook.NewHandler(&stubStore{ready: false})
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", false)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandlerAllowsCleanPod(t *testing.T) {
	store := &stubStore{ready: true, policies: []*webhook.CompiledPolicy{compiledEnforcePolicy(t)}}
	h := webhook.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", false)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp admissionv1.AdmissionReview
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Response.Allowed {
		t.Errorf("expected allowed=true, got false: %s", resp.Response.Result.Message)
	}
}

func TestHandlerDeniesPrivilegedPod(t *testing.T) {
	store := &stubStore{ready: true, policies: []*webhook.CompiledPolicy{compiledEnforcePolicy(t)}}
	h := webhook.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", true)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp admissionv1.AdmissionReview
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Response.Allowed {
		t.Error("expected allowed=false for privileged pod")
	}
	if resp.Response.Result == nil || resp.Response.Result.Message == "" {
		t.Error("expected denial message")
	}
}

func TestHandlerAuditModeAllows(t *testing.T) {
	q, _ := webhook.CompileRego(denyPrivilegedRego)
	auditPolicy := &webhook.CompiledPolicy{
		Name:            "audit-privileged",
		EnforcementMode: v1alpha1.ModeAudit,
		Query:           q,
	}
	store := &stubStore{ready: true, policies: []*webhook.CompiledPolicy{auditPolicy}}
	h := webhook.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", true)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp admissionv1.AdmissionReview
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Response.Allowed {
		t.Error("audit mode should allow even on deny")
	}
}

// Compile-time check: PolicyCache satisfies PolicyStore.
var _ webhook.PolicyStore = (*webhook.PolicyCache)(nil)

func TestCompiledPolicyMetadataFields(t *testing.T) {
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}
	p := &webhook.CompiledPolicy{
		Name:            "run-as-privileged",
		Key:             "runAsPrivileged",
		GroupName:       "security",
		Description:     "Fails when privileged is true",
		EnforcementMode: v1alpha1.ModeEnforce,
		Query:           q,
	}
	if p.Key != "runAsPrivileged" {
		t.Error("Key field not set")
	}
	if p.GroupName != "security" {
		t.Error("GroupName field not set")
	}
	if p.Description == "" {
		t.Error("Description field not set")
	}
}

func TestHandlerEnforceMessageContainsGroupKey(t *testing.T) {
	q, _ := webhook.CompileRego(denyPrivilegedRego)
	policy := &webhook.CompiledPolicy{
		Name:            "run-as-privileged",
		Key:             "runAsPrivileged",
		GroupName:       "security",
		Description:     "Fails when privileged is true",
		EnforcementMode: v1alpha1.ModeEnforce,
		Query:           q,
	}
	store := &stubStore{ready: true, policies: []*webhook.CompiledPolicy{policy}}
	h := webhook.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", true)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp admissionv1.AdmissionReview
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Response.Allowed {
		t.Fatal("expected denied")
	}
	msg := resp.Response.Result.Message
	if !strings.Contains(msg, "[security/runAsPrivileged]") {
		t.Errorf("message missing [group/key] prefix, got: %q", msg)
	}
	if !strings.Contains(msg, "Fails when privileged is true") {
		t.Errorf("message missing description, got: %q", msg)
	}
}

func TestHandlerAuditModeReturnsWarnings(t *testing.T) {
	q, _ := webhook.CompileRego(denyPrivilegedRego)
	policy := &webhook.CompiledPolicy{
		Name:            "host-network-set",
		Key:             "hostNetworkSet",
		GroupName:       "security",
		Description:     "Fails when hostNetwork is configured",
		EnforcementMode: v1alpha1.ModeAudit,
		Query:           q,
	}
	store := &stubStore{ready: true, policies: []*webhook.CompiledPolicy{policy}}
	h := webhook.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", true)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp admissionv1.AdmissionReview
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Response.Allowed {
		t.Fatal("audit mode should allow")
	}
	if len(resp.Response.Warnings) == 0 {
		t.Error("expected warnings for audit violation")
	}
	if !strings.Contains(resp.Response.Warnings[0], "[security/hostNetworkSet]") {
		t.Errorf("warning missing [group/key], got: %q", resp.Response.Warnings[0])
	}
}

func TestHandlerEnforceAndAuditBothInMessage(t *testing.T) {
	q, _ := webhook.CompileRego(denyPrivilegedRego)
	enforcePolicy := &webhook.CompiledPolicy{
		Name:            "run-as-privileged",
		Key:             "runAsPrivileged",
		GroupName:       "security",
		Description:     "enforce description",
		EnforcementMode: v1alpha1.ModeEnforce,
		Query:           q,
	}
	auditPolicy := &webhook.CompiledPolicy{
		Name:            "host-network-set",
		Key:             "hostNetworkSet",
		GroupName:       "security",
		Description:     "audit description",
		EnforcementMode: v1alpha1.ModeAudit,
		Query:           q,
	}
	store := &stubStore{ready: true, policies: []*webhook.CompiledPolicy{enforcePolicy, auditPolicy}}
	h := webhook.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(buildAdmissionRequest("CREATE", true)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp admissionv1.AdmissionReview
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Response.Allowed {
		t.Fatal("expected denied")
	}
	msg := resp.Response.Result.Message
	if !strings.Contains(msg, "runAsPrivileged") {
		t.Errorf("enforce policy missing from message: %q", msg)
	}
	if !strings.Contains(msg, "hostNetworkSet") {
		t.Errorf("audit policy missing from message: %q", msg)
	}
}
