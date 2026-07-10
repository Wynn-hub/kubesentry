package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// crdNames are the CRDs managed under templates/crds/ in the chart.
var crdNames = []string{
	"policies.kubesentry.io",
	"policygroups.kubesentry.io",
	"policyversions.kubesentry.io",
	"policyexceptions.kubesentry.io",
}

// applyCRDs pre-installs the chart CRDs and marks them Helm-owned so the
// subsequent helm install adopts them. CRDs live in templates/, so a
// single-shot helm install on a fresh cluster fails manifest validation:
// the builtin Policy CRs in the same release reference a kind the API
// server does not know yet. Same adoption pattern as make deploy-local.
func applyCRDs(ctx context.Context, chartDir string) error {
	crdDir := filepath.Join(chartDir, "templates", "crds")
	if out, err := exec.CommandContext(ctx, "kubectl", "apply", "-f", crdDir).CombinedOutput(); err != nil {
		return fmt.Errorf("apply crds: %w\nOutput:\n%s", err, out)
	}
	for _, crd := range crdNames {
		if out, err := exec.CommandContext(ctx, "kubectl", "wait", "--for=condition=Established",
			"crd/"+crd, "--timeout=60s").CombinedOutput(); err != nil {
			return fmt.Errorf("wait crd %s established: %w\nOutput:\n%s", crd, err, out)
		}
		run(ctx, "kubectl", "label", "crd", crd, "app.kubernetes.io/managed-by=Helm", "--overwrite")
		run(ctx, "kubectl", "annotate", "crd", crd,
			"meta.helm.sh/release-name=kubesentry",
			"meta.helm.sh/release-namespace=kubesentry-system", "--overwrite")
	}
	return nil
}

// HelmInstall installs the kubesentry chart with the given values file.
func HelmInstall(ctx context.Context, chartDir, valuesFile string) error {
	if err := applyCRDs(ctx, chartDir); err != nil {
		return err
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "helm", "install", "kubesentry", chartDir,
		"-f", valuesFile,
		"--namespace", "kubesentry-system",
		"--create-namespace",
		"--wait",
		"--timeout", "120s",
	)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install failed: %w\nOutput:\n%s", err, out.String())
	}
	return nil
}

// HelmUninstall removes the kubesentry release (best-effort).
func HelmUninstall(ctx context.Context) {
	_ = exec.CommandContext(ctx, "helm", "uninstall", "kubesentry", "-n", "kubesentry-system", "--ignore-not-found").Run()
}
