//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
)

const consoleBase = "http://127.0.0.1:18080"

// startPortForward runs kubectl port-forward for the console service and
// waits until /healthz answers. Callers must invoke the returned stop func.
func startPortForward(t *testing.T) func() {
	t.Helper()
	cmd := exec.Command("kubectl", "-n", helpers.WebhookNamespace,
		"port-forward", "svc/kubesentry-console", "18080:8080")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start port-forward: %v", err)
	}
	stop := func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(consoleBase + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return stop
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	stop()
	t.Fatal("console /healthz not reachable via port-forward")
	return nil
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *string         `json:"error"`
}

func consoleDo(t *testing.T, method, path string, body any) (int, apiEnvelope) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, consoleBase+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var env apiEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp.StatusCode, env
}

func TestT9_ConsoleListsBuiltinPolicies(t *testing.T) {
	stop := startPortForward(t)
	defer stop()

	code, env := consoleDo(t, "GET", "/api/v1/policies", nil)
	if code != 200 || !env.Success {
		t.Fatalf("code=%d env=%+v", code, env)
	}
	var items []map[string]any
	if err := json.Unmarshal(env.Data, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) < 37 {
		t.Fatalf("policies = %d, want >= 37", len(items))
	}
}

func TestT9_ConsolePolicyLifecycle(t *testing.T) {
	stop := startPortForward(t)
	defer stop()

	const name = "e2e-console-policy"
	rego := "package kubesentry\n\ndeny[msg] {\n\tinput.request.object.spec.hostNetwork == true\n\tmsg := \"hostNetwork not allowed\"\n}"
	body := map[string]any{
		"name":            name,
		"enforcementMode": "audit",
		"rego":            rego,
		"match": map[string]any{
			"operations": []string{"CREATE"},
			"resources": []map[string]any{
				{"apiGroups": []string{""}, "apiVersions": []string{"v1"}, "resources": []string{"pods"}},
			},
		},
	}

	// create → operator 生成 v1
	if code, env := consoleDo(t, "POST", "/api/v1/policies", body); code != 200 {
		t.Fatalf("create: code=%d env=%+v", code, env)
	}
	defer consoleDo(t, "DELETE", "/api/v1/policies/"+name+"?force=true", nil)
	waitCurrentVersion(t, name, 1)

	// update → v2
	body["enforcementMode"] = "enforce"
	if code, env := consoleDo(t, "PUT", "/api/v1/policies/"+name, body); code != 200 {
		t.Fatalf("update: code=%d env=%+v", code, env)
	}
	waitCurrentVersion(t, name, 2)

	// rollback prev → v3（内容 == v1），游标应指向 1
	if code, env := consoleDo(t, "POST", "/api/v1/policies/"+name+"/rollback",
		map[string]string{"direction": "prev"}); code != 200 {
		t.Fatalf("rollback: code=%d env=%+v", code, env)
	}
	waitCurrentVersion(t, name, 3)

	code, env := consoleDo(t, "GET", "/api/v1/policies/"+name+"/versions", nil)
	if code != 200 {
		t.Fatalf("versions: code=%d", code)
	}
	var tl struct {
		Cursor      int64 `json:"cursor"`
		NextEnabled bool  `json:"nextEnabled"`
	}
	if err := json.Unmarshal(env.Data, &tl); err != nil {
		t.Fatal(err)
	}
	if tl.Cursor != 1 || !tl.NextEnabled {
		t.Fatalf("timeline = %+v, want cursor=1 nextEnabled=true", tl)
	}

	// delete
	if code, _ := consoleDo(t, "DELETE", "/api/v1/policies/"+name+"?force=true", nil); code != 200 {
		t.Fatalf("delete: code=%d", code)
	}
}

// waitCurrentVersion polls the console API until the policy reaches version v.
func waitCurrentVersion(t *testing.T, name string, v int64) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		code, env := consoleDo(t, "GET", "/api/v1/policies/"+name, nil)
		if code == 200 {
			var d struct {
				Status struct {
					CurrentVersion int64  `json:"currentVersion"`
					Phase          string `json:"phase"`
				} `json:"status"`
			}
			if err := json.Unmarshal(env.Data, &d); err == nil &&
				d.Status.CurrentVersion == v && d.Status.Phase == "Ready" {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("policy %s did not reach version %d within 60s", name, v)
}
