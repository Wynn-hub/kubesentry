package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// HelmInstallMemberWithGateway installs the member chart with the gateway
// connection fields already set. gateway.address/caBundle/clusterName/
// registrationToken are chart-required (standalone deployment removed), so
// the member chart can only ever be installed this way — there is no bare
// install followed by a later upgrade.
func HelmInstallMemberWithGateway(ctx context.Context, chartDir, valuesFile, caBundleB64, token string) error {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "helm", "install", "kubesentry", chartDir,
		"-f", valuesFile,
		"--namespace", "kubesentry-system",
		"--create-namespace",
		"--set", "gateway.address=kubesentry-gateway.kubesentry-system.svc:9443",
		"--set", "gateway.caBundle="+caBundleB64,
		"--set", "gateway.clusterName=local",
		"--set", "gateway.registrationToken="+token,
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

// HelmInstallGateway installs the gateway chart (gateway + console) alongside the member release.
func HelmInstallGateway(ctx context.Context, chartDir, valuesFile string) error {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "helm", "install", "kubesentry-gateway", chartDir,
		"-f", valuesFile,
		"--namespace", "kubesentry-system",
		"--create-namespace",
		"--wait",
		"--timeout", "180s",
	)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install gateway failed: %w\nOutput:\n%s", err, out.String())
	}
	return nil
}
