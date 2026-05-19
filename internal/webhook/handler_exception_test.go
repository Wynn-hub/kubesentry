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

	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

type stubExceptionStore struct {
	exempted map[string]bool
	ready    bool
}

func (s *stubExceptionStore) ExemptedKeys(namespace string, labels map[string]string, policies []*webhook.CompiledPolicy) map[string]bool {
	return s.exempted
}
func (s *stubExceptionStore) IsReady() bool { return s.ready }

func makePodReview(t *testing.T, privileged bool) []byte {
	t.Helper()
	pod := map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"name": "p", "namespace": "hr"},
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name": "c", "image": "nginx",
					"securityContext": map[string]interface{}{"privileged": privileged},
				},
			},
		},
	}
	raw, _ := json.Marshal(pod)
	rev := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       "uid",
			Namespace: "hr",
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
			Resource: metav1.GroupVersionResource{
				Group: "", Version: "v1", Resource: "pods",
			},
		},
	}
	b, _ := json.Marshal(rev)
	return b
}

func TestHandlerSkipsExemptedPolicy(t *testing.T) {
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}
	policies := []*webhook.CompiledPolicy{{Name: "run-as-privileged", Query: q, EnforcementMode: "enforce"}}
	h := webhook.NewHandlerWithExceptions(
		&stubStore{policies: policies, ready: true},
		&stubExceptionStore{exempted: map[string]bool{"run-as-privileged": true}, ready: true},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(makePodReview(t, true)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	var rev admissionv1.AdmissionReview
	if err := json.NewDecoder(rec.Body).Decode(&rev); err != nil {
		t.Fatal(err)
	}
	if !rev.Response.Allowed {
		t.Errorf("expected allowed (policy exempted), got denied: %v", rev.Response.Result)
	}
}

func TestHandlerEvaluatesNonExempted(t *testing.T) {
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}
	policies := []*webhook.CompiledPolicy{{Name: "run-as-privileged", Query: q, EnforcementMode: "enforce"}}
	h := webhook.NewHandlerWithExceptions(
		&stubStore{policies: policies, ready: true},
		&stubExceptionStore{exempted: nil, ready: true},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(makePodReview(t, true)))
	h.ServeHTTP(rec, req)

	var rev admissionv1.AdmissionReview
	_ = json.NewDecoder(rec.Body).Decode(&rev)
	if rev.Response.Allowed {
		t.Error("expected denied; exemption was empty")
	}
}

func TestHandlerNotReadyWhenExceptionCacheNotReady(t *testing.T) {
	policies := []*webhook.CompiledPolicy{{Name: "x"}}
	h := webhook.NewHandlerWithExceptions(
		&stubStore{policies: policies, ready: true},
		&stubExceptionStore{ready: false},
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(makePodReview(t, true)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}
