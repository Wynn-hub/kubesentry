package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Cleanup removes all KubeSentry resources and test namespaces.
func Cleanup(ctx context.Context) {
	run(ctx, "helm", "uninstall", "kubesentry", "-n", "kubesentry-system", "--ignore-not-found")
	run(ctx, "kubectl", "delete", "namespace", "kubesentry-system", "--ignore-not-found", "--timeout=90s")
	run(ctx, "kubectl", "delete", "namespace", TestNamespace, "--ignore-not-found", "--timeout=60s")
	for _, crd := range []string{
		"policies.kubesentry.io",
		"policygroups.kubesentry.io",
		"policyversions.kubesentry.io",
	} {
		run(ctx, "kubectl", "delete", "crd", crd, "--ignore-not-found")
	}
	run(ctx, "kubectl", "delete", "validatingwebhookconfiguration", "kubesentry", "--ignore-not-found")
	// cluster-scoped RBAC fixtures left over from T3 tests
	for _, name := range []string{
		"test-safe-clusterrole",
		"test-safe-clusterrolebinding",
		"test-safe-clusterrolebinding-admin",
		"test-dangerous-clusterrole",
	} {
		run(ctx, "kubectl", "delete", "clusterrole", name, "--ignore-not-found")
		run(ctx, "kubectl", "delete", "clusterrolebinding", name, "--ignore-not-found")
	}
}

// WaitForDeploymentReady polls until Deployment has all expected replicas Ready.
func WaitForDeploymentReady(ctx context.Context, c client.Client, ns, name string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		var dep appsv1.Deployment
		if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &dep); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if dep.Spec.Replicas == nil {
			return false, nil
		}
		return dep.Status.ReadyReplicas >= *dep.Spec.Replicas, nil
	})
}

// CreateNamespace creates a namespace, ignoring AlreadyExists.
func CreateNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{}
	ns.Name = name
	if err := c.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) {
	_ = exec.CommandContext(ctx, name, args...).Run()
}
