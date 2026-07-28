package console

import (
	"encoding/json"
	"reflect"
	"testing"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestAdmissionEventItemFallsBackToViolationPolicyReferences(t *testing.T) {
	violations, err := json.Marshal([]string{"[host-ipc-set via default] host IPC is not allowed"})
	if err != nil {
		t.Fatal(err)
	}

	item := admissionEventItemOf(dbrepo.AdmissionEvent{Base: dbmodel.Base{ID: 2081582208838783001}, Violations: datatypes.JSON(violations)})
	if item.ID != "2081582208838783001" {
		t.Errorf("event ID = %q, want exact Snowflake ID", item.ID)
	}
	if want := []string{"host-ipc-set"}; !reflect.DeepEqual(item.PolicyNames, want) {
		t.Errorf("policy names = %#v, want %#v", item.PolicyNames, want)
	}
}

func TestAdmissionEventSettingsRoundTrip(t *testing.T) {
	server, _ := newDBTestServer(t)
	body := map[string]any{
		"defaultRetentionDays": 21,
		"retryEnabled":         true,
		"maxRetryAttempts":     5,
		"queueCapacity":        2048,
		"storagePolicies": []map[string]any{{
			"matchMode":     "exclude",
			"verdicts":      []string{"deny"},
			"clusters":      []string{},
			"namespaces":    []string{"kube-system"},
			"gvks":          []string{"pods", "apps/deployments"},
			"action":        "retain",
			"retentionDays": 30,
		}},
	}
	rec, env := doRequest(t, server, "PUT", "/api/v1/admission-event-settings", body)
	if rec.Code != 200 || !env.Success {
		t.Fatalf("update settings = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := mustUnmarshal[admissionSettingsResponse](t, env.Data)
	if got.DefaultRetentionDays != 21 || !got.RetryEnabled || got.MaxRetryAttempts != 5 || got.QueueCapacity != 2048 {
		t.Fatalf("settings = %+v", got)
	}
	if len(got.StoragePolicies) != 1 || got.StoragePolicies[0].MatchMode != dbrepo.AdmissionMatchModeExclude {
		t.Fatalf("storage policies = %+v", got.StoragePolicies)
	}
}

func TestAdmissionEventSettingsRejectInvalidMatchModeAndGVR(t *testing.T) {
	server, _ := newDBTestServer(t)
	base := map[string]any{
		"defaultRetentionDays": 14,
		"maxRetryAttempts":     3,
		"queueCapacity":        1024,
	}
	for _, test := range []struct {
		name string
		rule map[string]any
	}{
		{name: "match mode", rule: map[string]any{"matchMode": "not-in", "action": "discard"}},
		{name: "versioned GVR", rule: map[string]any{"matchMode": "include", "gvks": []string{"apps/v1/deployments"}, "action": "discard"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := make(map[string]any, len(base)+1)
			for key, value := range base {
				body[key] = value
			}
			body["storagePolicies"] = []map[string]any{test.rule}
			rec, _ := doRequest(t, server, "PUT", "/api/v1/admission-event-settings", body)
			if rec.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestValidAdmissionGVR(t *testing.T) {
	for _, value := range []string{"pods", "apps/deployments", "networking.k8s.io/ingresses"} {
		if !validAdmissionGVR(value) {
			t.Errorf("validAdmissionGVR(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "/pods", "apps/v1/deployments", "apps/", "Pods", " apps/deployments"} {
		if validAdmissionGVR(value) {
			t.Errorf("validAdmissionGVR(%q) = true, want false", value)
		}
	}
}
