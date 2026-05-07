package webhook_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
