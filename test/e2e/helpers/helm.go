package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// HelmInstallMemberWithHub installs the member chart with the hub connection
// fields already set. hub.address/caBundle/clusterName/registrationToken are
// chart-required (standalone deployment removed), so the member chart can
// only ever be installed this way — there is no bare install followed by a
// later upgrade.
func HelmInstallMemberWithHub(ctx context.Context, chartDir, valuesFile, caBundleB64, token string) error {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "helm", "install", "kubesentry", chartDir,
		"-f", valuesFile,
		"--namespace", "kubesentry-system",
		"--create-namespace",
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
		return fmt.Errorf("helm install member failed: %w\nOutput:\n%s", err, out.String())
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
		"--create-namespace",
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
