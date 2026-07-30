//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
)

type observationOverview struct {
	Health           string `json:"health"`
	Generated        int64  `json:"generated"`
	OperatorReceived int64  `json:"operatorReceived"`
	GatewayReceived  int64  `json:"gatewayReceived"`
	Written          int64  `json:"written"`
}

func TestT12_AdmissionObservationThreeHopFlow(t *testing.T) {
	stop := startPortForward(t)
	defer stop()

	baseline := readObservationOverview(t)
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: observation-denied-%d
  namespace: %s
spec:
  hostIPC: true
  containers:
    - name: pause
      image: registry.k8s.io/pause:3.10
`, time.Now().Unix(), helpers.TestNamespace)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := helpers.KubectlApplyStdin(ctx, manifest); err == nil {
		helpers.KubectlDeleteStdin(context.Background(), manifest)
		t.Fatalf("violating pod was unexpectedly accepted: %s", output)
	}

	deadline := time.Now().Add(2 * time.Minute)
	var current observationOverview
	for time.Now().Before(deadline) {
		current = readObservationOverview(t)
		if current.Generated > baseline.Generated &&
			current.OperatorReceived > baseline.OperatorReceived &&
			current.GatewayReceived > baseline.GatewayReceived &&
			current.Written > baseline.Written {
			if current.Health != "normal" {
				t.Fatalf("observation flow advanced but health = %q, want normal: baseline=%+v current=%+v", current.Health, baseline, current)
			}
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("observation flow did not advance across all three hops: baseline=%+v current=%+v", baseline, current)
}

func readObservationOverview(t *testing.T) observationOverview {
	t.Helper()
	now := time.Now().UTC()
	query := url.Values{
		"cluster": {"local"},
		"from":    {now.Add(-time.Hour).Format(time.RFC3339)},
		"to":      {now.Format(time.RFC3339)},
	}
	code, env := consoleDo(t, "GET", "/api/v1/observability/admission/overview?"+query.Encode(), nil)
	if code != 200 || !env.Success {
		t.Fatalf("read observation overview: code=%d env=%+v", code, env)
	}
	var result observationOverview
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
