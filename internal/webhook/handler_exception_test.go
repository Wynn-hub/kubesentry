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

type stubExceptionStore struct {
	exempted map[string]bool
	ready    bool
}

func (s *stubExceptionStore) ExemptedKeys(namespace string, namespaceLabels, resourceLabels map[string]string, policies []*webhook.CompiledPolicy) map[string]bool {
	return s.exempted
}
func (s *stubExceptionStore) IsReady() bool { return s.ready }

type stubNamespaceLister struct {
	labels map[string]map[string]string
}

func (s *stubNamespaceLister) GetLabels(name string) (map[string]string, bool) {
	if s == nil {
		return nil, false
	}
	l, ok := s.labels[name]
	return l, ok
}

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
	resolved := []*webhook.Resolved{{
		Policy: &webhook.CompiledPolicy{Name: "run-as-privileged", Query: q},
		Mode:   v1alpha1.ModeEnforce,
		Groups: []string{"security"},
	}}
	h := webhook.NewHandlerWithExceptions(
		&stubStore{resolved: resolved, ready: true},
		&stubExceptionStore{exempted: map[string]bool{"run-as-privileged": true}, ready: true},
		nil,
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
	resolved := []*webhook.Resolved{{
		Policy: &webhook.CompiledPolicy{Name: "run-as-privileged", Query: q},
		Mode:   v1alpha1.ModeEnforce,
		Groups: []string{"security"},
	}}
	h := webhook.NewHandlerWithExceptions(
		&stubStore{resolved: resolved, ready: true},
		&stubExceptionStore{exempted: nil, ready: true},
		nil,
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

// Verifies the Handler resolves namespace labels via NamespaceLister and
// passes them to the ExceptionStore. Uses a recording stub to capture what
// the Handler actually passes through.
type recordingExceptionStore struct {
	gotNamespaceLabels map[string]string
	exempted           map[string]bool
}

func (s *recordingExceptionStore) ExemptedKeys(_ string, nsLabels, _ map[string]string, _ []*webhook.CompiledPolicy) map[string]bool {
	s.gotNamespaceLabels = nsLabels
	return s.exempted
}
func (s *recordingExceptionStore) IsReady() bool { return true }

func TestHandlerPassesNamespaceLabelsToStore(t *testing.T) {
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}
	resolved := []*webhook.Resolved{{
		Policy: &webhook.CompiledPolicy{Name: "run-as-privileged", Query: q},
		Mode:   v1alpha1.ModeEnforce,
		Groups: []string{"security"},
	}}
	rec := &recordingExceptionStore{}
	lister := &stubNamespaceLister{labels: map[string]map[string]string{
		"hr": {"env": "legacy"},
	}}
	h := webhook.NewHandlerWithExceptions(&stubStore{resolved: resolved, ready: true}, rec, lister)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(makePodReview(t, true)))
	h.ServeHTTP(rr, req)

	if rec.gotNamespaceLabels == nil {
		t.Fatal("expected non-nil namespace labels passed through")
	}
	if rec.gotNamespaceLabels["env"] != "legacy" {
		t.Errorf("expected env=legacy, got %v", rec.gotNamespaceLabels)
	}
}

func TestHandlerPassesNilWhenNamespaceUnknown(t *testing.T) {
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}
	resolved := []*webhook.Resolved{{
		Policy: &webhook.CompiledPolicy{Name: "run-as-privileged", Query: q},
		Mode:   v1alpha1.ModeEnforce,
		Groups: []string{"security"},
	}}
	rec := &recordingExceptionStore{}
	lister := &stubNamespaceLister{labels: map[string]map[string]string{}} // empty cache
	h := webhook.NewHandlerWithExceptions(&stubStore{resolved: resolved, ready: true}, rec, lister)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(makePodReview(t, true)))
	h.ServeHTTP(rr, req)

	if rec.gotNamespaceLabels != nil {
		t.Errorf("expected nil (unresolved) namespace labels, got %v", rec.gotNamespaceLabels)
	}
}

func TestHandlerNotReadyWhenExceptionCacheNotReady(t *testing.T) {
	resolved := []*webhook.Resolved{{
		Policy: &webhook.CompiledPolicy{Name: "x"},
		Mode:   v1alpha1.ModeEnforce,
		Groups: []string{},
	}}
	h := webhook.NewHandlerWithExceptions(
		&stubStore{resolved: resolved, ready: true},
		&stubExceptionStore{ready: false},
		nil,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(makePodReview(t, true)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}
