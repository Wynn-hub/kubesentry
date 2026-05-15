package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// HelmInstall installs the kubesentry chart with the given values file.
func HelmInstall(ctx context.Context, chartDir, valuesFile string) error {
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
