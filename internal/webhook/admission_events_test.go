package webhook_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func TestAdmissionEventReferencesExtractsAndDeduplicatesHeaders(t *testing.T) {
	policies, groups := webhook.AdmissionEventReferences([]string{
		"[host-ipc-set via baseline, security] host IPC is not allowed\n  描述：Blocks host IPC",
		"[run-as-privileged via security] privileged containers are not allowed",
		"[host-ipc-set via baseline, security] another container violates the same policy",
		"unstructured evaluation error",
	})

	if want := []string{"host-ipc-set", "run-as-privileged"}; !reflect.DeepEqual(policies, want) {
		t.Errorf("policies = %#v, want %#v", policies, want)
	}
	if want := []string{"baseline", "security"}; !reflect.DeepEqual(groups, want) {
		t.Errorf("groups = %#v, want %#v", groups, want)
	}
}

func TestAdmissionEventReferencesIgnoresMalformedHeaders(t *testing.T) {
	policies, groups := webhook.AdmissionEventReferences([]string{"", "[missing end", "[] empty", "  描述：[not a violation header]"})
	if len(policies) != 0 || len(groups) != 0 {
		t.Errorf("references = (%#v, %#v), want no references", policies, groups)
	}
}

// A persistently RETRYABLE operator (typically "operator_event_queue_full")
// must not make the webhook re-post the same event forever: the retry budget is
// bounded, otherwise the webhook keeps hammering an already saturated operator.
func TestReporterStopsRetryingAfterAttemptBudget(t *testing.T) {
	var posts int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&posts, 1)
		w.Header().Set("Content-Type", "application/json")
		// Reply RETRYABLE for whatever event id was posted.
		w.Write([]byte(`{"results":[{"eventId":"","status":"RETRYABLE","reason":"operator_event_queue_full"}]}`)) //nolint:errcheck
	}))
	defer server.Close()

	reporter := webhook.NewHTTPAdmissionEventReporter(server.URL, "token")
	reporter.Report(
		&admissionv1.AdmissionRequest{UID: "uid-1", Operation: admissionv1.Create},
		&admissionv1.AdmissionResponse{Allowed: false, Result: &metav1.Status{Message: "[p via g] denied"}},
	)

	// Attempts are spaced by a 1s retry delay; wait past the budget then assert
	// the count stopped growing instead of climbing without bound.
	time.Sleep(5 * time.Second)
	settled := atomic.LoadInt64(&posts)
	if settled == 0 {
		t.Fatal("reporter never posted the event")
	}
	if settled > 4 {
		t.Fatalf("posts = %d, want the bounded retry budget (<= 4)", settled)
	}
	time.Sleep(2 * time.Second)
	if grew := atomic.LoadInt64(&posts); grew != settled {
		t.Errorf("posts kept growing: %d -> %d, want retries to stop", settled, grew)
	}
}
