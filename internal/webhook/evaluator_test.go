package webhook_test

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

const denyPrivilegedRego = `
package kubesentry

deny[msg] {
  input.request.object.spec.containers[_].securityContext.privileged == true
  msg := "privileged containers are not allowed"
}
`

const alwaysAllowRego = `
package kubesentry
`

func TestCompileValidRego(t *testing.T) {
	_, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatalf("expected valid rego to compile, got: %v", err)
	}
}

func TestCompileInvalidRego(t *testing.T) {
	_, err := webhook.CompileRego("this is not rego !!!!")
	if err == nil {
		t.Fatal("expected invalid rego to fail compilation")
	}
}

func TestEvaluateDeny(t *testing.T) {
	q, err := webhook.CompileRego(denyPrivilegedRego)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]interface{}{
		"request": map[string]interface{}{
			"object": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"securityContext": map[string]interface{}{
								"privileged": true,
							},
						},
					},
				},
			},
		},
	}

	denials, err := webhook.EvaluatePolicy(context.Background(), q, input)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if len(denials) != 1 || denials[0] != "privileged containers are not allowed" {
		t.Errorf("expected one denial, got: %v", denials)
	}
}

func TestEvaluateAllow(t *testing.T) {
	q, err := webhook.CompileRego(alwaysAllowRego)
	if err != nil {
		t.Fatal(err)
	}

	denials, err := webhook.EvaluatePolicy(context.Background(), q, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(denials) != 0 {
		t.Errorf("expected no denials, got: %v", denials)
	}
}
