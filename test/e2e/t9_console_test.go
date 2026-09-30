//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
)

const (
	consoleBase          = "http://127.0.0.1:18080"
	consoleAdminPassword = "kubesentry-e2e-admin-password"
)

var consoleClient = http.DefaultClient

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
				authenticateConsole(t)
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
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func consoleDo(t *testing.T, method, path string, body any) (int, apiEnvelope) {
	return consoleRequest(t, consoleClient, method, path, body)
}

func consoleRequest(t *testing.T, client *http.Client, method, path string, body any) (int, apiEnvelope) {
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
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set("X-KubeSentry-CSRF", "1")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var env apiEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	return resp.StatusCode, env
}

func authenticateConsole(t *testing.T) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	code, env := consoleRequest(t, client, http.MethodGet, "/api/v1/setup/status", nil)
	if code != http.StatusOK {
		t.Fatalf("setup status: %d %+v", code, env)
	}
	var status struct {
		Required bool `json:"required"`
	}
	if err := json.Unmarshal(env.Data, &status); err != nil {
		t.Fatal(err)
	}
	if status.Required {
		if code, env = consoleRequest(t, client, http.MethodPost, "/api/v1/setup/admin", map[string]string{"password": consoleAdminPassword}); code != http.StatusOK {
			t.Fatalf("setup admin: %d %+v", code, env)
		}
	}
	if code, env = consoleRequest(t, client, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": "admin", "password": consoleAdminPassword}); code != http.StatusOK {
		t.Fatalf("login admin: %d %+v", code, env)
	}
	consoleClient = client
}

func loginConsole(t *testing.T, username, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if code, env := consoleRequest(t, client, http.MethodPost, "/api/v1/auth/login", map[string]string{"username": username, "password": password}); code != http.StatusOK {
		t.Fatalf("login %s: %d %+v", username, code, env)
	}
	return client
}

func TestT9_MultitenantAccess(t *testing.T) {
	stop := startPortForward(t)
	defer stop()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	tenantName, username := "e2e-tenant-"+suffix, "e2e-viewer-"+suffix
	code, env := consoleDo(t, http.MethodPost, "/api/v1/tenants", map[string]string{"name": tenantName})
	if code != http.StatusOK {
		t.Fatalf("create tenant: %d %+v", code, env)
	}
	var tenant struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &tenant); err != nil {
		t.Fatal(err)
	}
	defer consoleDo(t, http.MethodPut, "/api/v1/tenants/"+strconv.FormatInt(tenant.ID, 10)+"/status", map[string]string{"status": "disabled"})
	code, env = consoleDo(t, http.MethodPost, "/api/v1/users", map[string]any{"username": username, "password": consoleAdminPassword, "role": "viewer", "tenantId": tenant.ID})
	if code != http.StatusOK {
		t.Fatalf("create viewer: %d %+v", code, env)
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &user); err != nil {
		t.Fatal(err)
	}
	defer consoleDo(t, http.MethodDelete, "/api/v1/users/"+strconv.FormatInt(user.ID, 10), nil)
	if code, env = consoleDo(t, http.MethodPut, "/api/v1/clusters/local/tenant", map[string]any{"tenantId": tenant.ID}); code != http.StatusOK {
		t.Fatalf("assign cluster: %d %+v", code, env)
	}
	defer consoleDo(t, http.MethodPut, "/api/v1/clusters/local/tenant", map[string]any{})
	viewer := loginConsole(t, username, consoleAdminPassword)
	if code, env = consoleRequest(t, viewer, http.MethodGet, "/api/v1/clusters/local", nil); code != http.StatusOK {
		t.Fatalf("viewer cluster read: %d %+v", code, env)
	}
	if code, _ = consoleRequest(t, viewer, http.MethodPost, "/api/v1/clusters", map[string]string{"name": "forbidden"}); code != http.StatusForbidden {
		t.Fatalf("viewer cluster create = %d, want 403", code)
	}
}

func TestT9_ConsoleListsBuiltinPolicies(t *testing.T) {
	stop := startPortForward(t)
	defer stop()

	code, env := consoleDo(t, "GET", "/api/v1/policies?pageSize=100", nil)
	if code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", code, env)
	}
	var page struct {
		Items    []map[string]any `json:"items"`
		Total    int              `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"pageSize"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 100 {
		t.Fatalf("pagination = page %d size %d, want page 1 size 100", page.Page, page.PageSize)
	}
	if page.Total < 37 || len(page.Items) < 37 {
		t.Fatalf("policies = total %d items %d, want >= 37", page.Total, len(page.Items))
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

	// rollback prev → v3 (content == v1). Rollback is roll-forward only in
	// the GORM-era API (see handlers_policy.go's rollbackPolicy comment) —
	// there is no logical-cursor concept to assert on anymore, just that a
	// fresh v3 was appended whose rego matches the original v1 content.
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
		CurrentVersion int64 `json:"currentVersion"`
		Versions       []struct {
			Version int    `json:"version"`
			Rego    string `json:"rego"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(env.Data, &tl); err != nil {
		t.Fatal(err)
	}
	if tl.CurrentVersion != 3 {
		t.Fatalf("timeline = %+v, want currentVersion=3", tl)
	}
	var v1Rego, v3Rego string
	for _, v := range tl.Versions {
		switch v.Version {
		case 1:
			v1Rego = v.Rego
		case 3:
			v3Rego = v.Rego
		}
	}
	if v1Rego == "" || v3Rego != v1Rego {
		t.Fatalf("v3 rego does not match v1 rego (rollback did not restore original content): v1=%q v3=%q", v1Rego, v3Rego)
	}

	// delete
	if code, _ := consoleDo(t, "DELETE", "/api/v1/policies/"+name+"?force=true", nil); code != 200 {
		t.Fatalf("delete: code=%d", code)
	}
}

// waitCurrentVersion polls the console API until the policy reaches version
// v. policyDetail (GET /api/v1/policies/{name}) reports Phase as a flat
// top-level field with no currentVersion; the version count only lives in
// the /versions timeline response — so this checks both endpoints.
func waitCurrentVersion(t *testing.T, name string, v int64) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if vCode, vEnv := consoleDo(t, "GET", "/api/v1/policies/"+name+"/versions", nil); vCode == 200 {
			var tl struct {
				CurrentVersion int64 `json:"currentVersion"`
			}
			if err := json.Unmarshal(vEnv.Data, &tl); err == nil && tl.CurrentVersion == v {
				if pCode, pEnv := consoleDo(t, "GET", "/api/v1/policies/"+name, nil); pCode == 200 {
					var pd struct {
						Phase string `json:"phase"`
					}
					if err := json.Unmarshal(pEnv.Data, &pd); err == nil && pd.Phase == "Ready" {
						return
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("policy %s did not reach version %d within 60s", name, v)
}

func TestT9_ResourceSuggestions(t *testing.T) {
	stop := startPortForward(t)
	defer stop()

	code, env := consoleDo(t, "GET", "/api/v1/policies/resource-suggestions", nil)
	if code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", code, env)
	}
	var resp struct {
		APIGroups   []string `json:"apiGroups"`
		APIVersions []string `json:"apiVersions"`
		Resources   []string `json:"resources"`
	}
	if err := json.Unmarshal(env.Data, &resp); err != nil {
		t.Fatal(err)
	}
	// 内置策略至少覆盖了 core group 的 pods 和 v1 版本
	if len(resp.Resources) == 0 || len(resp.APIVersions) == 0 {
		t.Fatalf("resp = %+v, want non-empty suggestions (37+ builtin policies should populate these)", resp)
	}
}

func TestT9_SchemaFields(t *testing.T) {
	stop := startPortForward(t)
	defer stop()

	code, env := consoleDo(t, "GET", "/api/v1/schema/fields?group=&version=v1&resource=pods", nil)
	if code != 200 || env.Code != 0 {
		t.Fatalf("code=%d env=%+v", code, env)
	}
	var fields []struct {
		Name     string `json:"name"`
		Children []struct {
			Name string `json:"name"`
		} `json:"children"`
	}
	if err := json.Unmarshal(env.Data, &fields); err != nil {
		t.Fatal(err)
	}

	// Find spec and metadata fields
	var specNode *struct {
		Name     string `json:"name"`
		Children []struct {
			Name string `json:"name"`
		} `json:"children"`
	}
	names := map[string]bool{}
	for i, f := range fields {
		names[f.Name] = true
		if f.Name == "spec" {
			specNode = &fields[i]
		}
	}
	if !names["spec"] || !names["metadata"] {
		t.Fatalf("fields = %+v, want top-level spec/metadata", fields)
	}

	// Strengthened assertion: spec field must have non-empty children (guards against
	// regression of the allOf-wrapped $ref resolution bug fixed in schema.go)
	if specNode == nil || len(specNode.Children) == 0 {
		t.Fatalf("spec field has no children, want nested fields like containers, hostNetwork, etc. (regression of allOf resolution bug)")
	}

	if code, _ := consoleDo(t, "GET", "/api/v1/schema/fields?group=&version=v1", nil); code != 400 {
		t.Fatalf("missing resource param: code=%d, want 400", code)
	}
}
