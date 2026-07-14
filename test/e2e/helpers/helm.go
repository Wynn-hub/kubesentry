package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// HelmInstall installs the kubesentry chart with the given values file.
// CRDs live in the chart's crds/ directory, so a plain install on a fresh
// cluster works — exactly the path an end user takes.
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

// HelmInstallHub installs the hub chart (hub + console) alongside the member release.
func HelmInstallHub(ctx context.Context, chartDir, valuesFile string) error {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "helm", "install", "kubesentry-hub", chartDir,
		"-f", valuesFile,
		"--namespace", "kubesentry-system",
		"--wait",
		"--timeout", "180s",
	)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install hub failed: %w\nOutput:\n%s", err, out.String())
	}
	return nil
}

// HelmUpgradeMemberWithHub points the member operator at the local hub.
func HelmUpgradeMemberWithHub(ctx context.Context, chartDir, caBundleB64, token string) error {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "helm", "upgrade", "kubesentry", chartDir,
		"--namespace", "kubesentry-system",
		"--reuse-values",
		"--set", "hub.address=kubesentry-hub.kubesentry-system.svc:9443",
		"--set", "hub.caBundle="+caBundleB64,
		"--set", "hub.clusterName=local",
		"--set", "hub.registrationToken="+token,
		"--wait",
		"--timeout", "180s",
	)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm upgrade member failed: %w\nOutput:\n%s", err, out.String())
	}
	return nil
}
