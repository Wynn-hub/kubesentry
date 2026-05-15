//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestT1_WebhookDeploymentReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := helpers.WaitForDeploymentReady(ctx, k8sClient, helpers.WebhookNamespace, "kubesentry-webhook", 30*time.Second); err != nil {
		t.Fatalf("webhook deployment not ready: %v", err)
	}
}

func TestT1_OperatorDeploymentReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := helpers.WaitForDeploymentReady(ctx, k8sClient, helpers.WebhookNamespace, "kubesentry-operator", 30*time.Second); err != nil {
		t.Fatalf("operator deployment not ready: %v", err)
	}
}

func TestT1_CRDsRegistered(t *testing.T) {
	ctx := context.Background()
	for _, gvr := range []string{"policies", "policygroups", "policyversions"} {
		out, err := kubectlRun(ctx, "get", "crd", gvr+".kubesentry.io")
		if err != nil {
			t.Errorf("CRD %s.kubesentry.io not found: %s", gvr, out)
		}
	}
}

func TestT1_ValidatingWebhookConfigExists(t *testing.T) {
	ctx := context.Background()
	var vwc admregv1.ValidatingWebhookConfiguration
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "kubesentry"}, &vwc); err != nil {
		t.Fatalf("ValidatingWebhookConfiguration kubesentry not found: %v", err)
	}
	if len(vwc.Webhooks) == 0 {
		t.Fatal("ValidatingWebhookConfiguration has no webhooks")
	}
	if len(vwc.Webhooks[0].ClientConfig.CABundle) == 0 {
		t.Fatal("caBundle is empty — TLS setup may have failed")
	}
}

func TestT1_TLSSecretExists(t *testing.T) {
	ctx := context.Background()
	var secret corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: helpers.WebhookNamespace, Name: "kubesentry-tls"}, &secret); err != nil {
		t.Fatalf("TLS secret kubesentry-tls not found: %v", err)
	}
	if len(secret.Data["tls.crt"]) == 0 {
		t.Fatal("tls.crt is empty")
	}
	if len(secret.Data["tls.key"]) == 0 {
		t.Fatal("tls.key is empty")
	}
}

func kubectlRun(ctx context.Context, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	return out.String(), cmd.Run()
}
