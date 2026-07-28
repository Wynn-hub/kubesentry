package webhook_test

import (
	"reflect"
	"testing"

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
