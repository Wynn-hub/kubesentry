package console

import (
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

func TestAdmissionEventSettingsRoundTrip(t *testing.T) {
	server, _ := newDBTestServer(t)
	body := map[string]any{
		"defaultRetentionDays":  21,
		"incidentRetentionDays": 11,
		"retryEnabled":          true,
		"maxRetryAttempts":      5,
		"queueCapacity":         2048,
		"storagePolicies": []map[string]any{{
			"matchMode":  "exclude",
			"verdicts":   []string{"deny"},
			"clusters":   []string{},
			"namespaces": []string{"kube-system"},
			"gvks":       []string{"pods", "apps/deployments"},
		}},
	}
	rec, env := doRequest(t, server, "PUT", "/api/v1/admission-event-settings", body)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("update settings = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := mustUnmarshal[dto.AdmissionSettingsView](t, env.Data)
	if got.DefaultRetentionDays != 21 || got.IncidentRetentionDays != 11 || !got.RetryEnabled || got.MaxRetryAttempts != 5 || got.QueueCapacity != 2048 {
		t.Fatalf("settings = %+v", got)
	}
	if len(got.StoragePolicies) != 1 || got.StoragePolicies[0].MatchMode != dbrepo.AdmissionMatchModeExclude {
		t.Fatalf("storage policies = %+v", got.StoragePolicies)
	}
}

func TestAdmissionEventSettingsRejectInvalidMatchModeAndGVR(t *testing.T) {
	server, _ := newDBTestServer(t)
	base := map[string]any{
		"defaultRetentionDays":  14,
		"retryEnabled":          true,
		"maxRetryAttempts":      3,
		"queueCapacity":         1024,
		"incidentRetentionDays": 7,
		"observationEnabled":    false,
	}
	for _, test := range []struct {
		name string
		rule map[string]any
	}{
		{name: "match mode", rule: map[string]any{"matchMode": "not-in", "verdicts": []string{"deny"}, "clusters": []string{}, "gvks": []string{"pods"}, "namespaces": []string{}}},
		{name: "versioned GVR", rule: map[string]any{"matchMode": "include", "verdicts": []string{"deny"}, "clusters": []string{}, "gvks": []string{"apps/v1/deployments"}, "namespaces": []string{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := make(map[string]any, len(base)+1)
			for key, value := range base {
				body[key] = value
			}
			body["storagePolicies"] = []map[string]any{test.rule}
			rec, env := doRequest(t, server, "PUT", "/api/v1/admission-event-settings", body)
			if rec.Code != 400 {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			want := "matchMode"
			if test.name == "versioned GVR" {
				want = "gvk"
			}
			if !strings.Contains(env.Message, want) {
				t.Fatalf("message = %q, want specific %q validation", env.Message, want)
			}
		})
	}
}

// The settings write must notify the gateway with the exact topic the gateway
// matches on to run a full cleanup pass. A drifting literal on either side
// would silently stop narrowed rules from dropping already-stored events.
func TestAdmissionEventSettingsNotifyGatewayWithCleanupTopic(t *testing.T) {
	server, _, handlers := newDBTestServerWithHandlers(t)
	fake := &fakeHubClient{}
	handlers.Gateway = fake

	rec, env := doRequest(t, server, "PUT", "/api/v1/admission-event-settings", map[string]any{
		"defaultRetentionDays": 14,
		"maxRetryAttempts":     3,
		"queueCapacity":        1024,
	})
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("update settings = %d, body=%s", rec.Code, rec.Body.String())
	}
	topics, _ := fake.calls()
	if len(topics) != 1 || topics[0] != gatewaypb.AdmissionEventSettingsTopic {
		t.Fatalf("NotifyChange topics = %v, want [%s]", topics, gatewaypb.AdmissionEventSettingsTopic)
	}
}
