package builtins_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

// TestBuiltinPoliciesCompile runs `helm template` against the chart and
// verifies that every Policy CR's spec.rego compiles via the production
// webhook compiler. Catches v0 syntax mistakes (rego.v1 / some x in set)
// and broken sprintf signatures at PR time.
func TestBuiltinPoliciesCompile(t *testing.T) {
	chart, err := filepath.Abs("../../charts/kubesentry")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("helm", "template", chart)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("helm template: %v\nstderr=%s", err, stderr.String())
	}

	docs := strings.Split(stdout.String(), "\n---\n")
	policyCount := 0
	for _, doc := range docs {
		if !strings.Contains(doc, "kind: Policy\n") {
			continue
		}
		var obj struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Rego string `yaml:"rego"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			t.Errorf("yaml unmarshal: %v\ndoc=%q", err, doc[:minInt(200, len(doc))])
			continue
		}
		if obj.Kind != "Policy" || obj.Spec.Rego == "" {
			continue
		}
		policyCount++
		if _, err := webhook.CompileRego(obj.Spec.Rego); err != nil {
			t.Errorf("policy %s rego compile: %v", obj.Metadata.Name, err)
		}
	}

	if policyCount < 37 {
		t.Errorf("expected ≥37 Policy CRs rendered, got %d", policyCount)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
