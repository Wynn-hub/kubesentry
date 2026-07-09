package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// validRego is a minimal v0-syntax module satisfying the kubesentry contract.
const validRego = `package kubesentry

deny[msg] {
	input.request.object.spec.hostNetwork == true
	msg := "hostNetwork not allowed"
}`

type testEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

func newTestServer(t *testing.T, objs ...client.Object) (http.Handler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.Policy{}, &v1alpha1.PolicyGroup{}, &v1alpha1.PolicyException{}).
		WithObjects(objs...).
		Build()
	srv := NewServer(&Handlers{Client: c}, fstest.MapFS{}, func() bool { return true })
	return srv.Handler, c
}

// newTestServerWithHandlers is like newTestServer but returns the *Handlers
// itself too, for tests that need to poke at fields newTestServer doesn't
// expose (e.g. a fake SchemaFetcher).
func newTestServerWithHandlers(t *testing.T, objs ...client.Object) (http.Handler, *Handlers) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.Policy{}, &v1alpha1.PolicyGroup{}, &v1alpha1.PolicyException{}).
		WithObjects(objs...).
		Build()
	h := &Handlers{Client: c}
	srv := NewServer(h, fstest.MapFS{}, func() bool { return true })
	return srv.Handler, h
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, testEnvelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env testEnvelope
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode envelope: %v (body=%s)", err, rec.Body.String())
		}
	}
	return rec, env
}

func mustUnmarshal[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal data: %v (raw=%s)", err, raw)
	}
	return v
}
