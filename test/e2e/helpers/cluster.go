package helpers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewayauth"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

// RegisterLocalCluster seeds the "local" Cluster row directly in the
// gateway's Postgres and returns the one-time registration token and
// base64 gateway CA bundle needed to install the member chart pre-pointed
// at the gateway. Must run after the gateway chart is installed and before
// the member chart is installed — gateway.address/caBundle/clusterName/
// registrationToken are chart-required, so there is no "install member
// bare, then helm upgrade with real values" path anymore.
//
// The gateway has zero CRDs (all data lives in Postgres via GORM) and
// console-api has no REST route for cluster CRUD yet — registration is a
// gRPC-only concept (see internal/gateway.Server.Register) that requires a
// pre-existing Cluster row with a known registration-token hash. This
// writes that row directly via the same dbrepo package cmd/gateway and
// cmd/console use, over a port-forward to the gateway's Postgres Service
// (see OpenGatewayDB) — the closest in-process equivalent to what
// scripts/deploy-local.sh does out-of-process via cmd/db's
// register-cluster subcommand (bash has no Go runtime to link dbrepo into).
func RegisterLocalCluster(ctx context.Context, c client.Client, name string) (token, caBundleB64 string, err error) {
	db, stop, err := OpenGatewayDB(ctx)
	if err != nil {
		return "", "", fmt.Errorf("open gateway db: %w", err)
	}
	defer stop()

	if err := dbmodel.Init(dbmodel.NodeIDFromPodName("e2e-test")); err != nil {
		return "", "", fmt.Errorf("init id generator: %w", err)
	}

	token, err = gatewayauth.NewToken()
	if err != nil {
		return "", "", fmt.Errorf("generate registration token: %w", err)
	}

	repo := dbrepo.NewClusterRepo(db)
	if _, getErr := repo.GetByName(ctx, name); getErr == nil {
		return "", "", fmt.Errorf("cluster %q already registered (stale state from a previous run?)", name)
	} else if !errors.Is(getErr, gorm.ErrRecordNotFound) {
		return "", "", fmt.Errorf("check existing cluster: %w", getErr)
	}
	cl := &dbrepo.Cluster{
		Name:         name,
		Phase:        gatewaymodel.ClusterPhaseRegistering,
		RegTokenHash: gatewayauth.HashToken(token),
	}
	if err := repo.Create(ctx, cl); err != nil {
		return "", "", fmt.Errorf("create cluster row: %w", err)
	}

	var tlsSec corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: WebhookNamespace, Name: "kubesentry-gateway-tls"}, &tlsSec); err != nil {
		return "", "", fmt.Errorf("read gateway tls secret: %w", err)
	}
	caBundleB64 = base64.StdEncoding.EncodeToString(tlsSec.Data["ca.crt"])
	return token, caBundleB64, nil
}

// WaitClusterOnline polls the gateway's Postgres until the named cluster
// reaches Online phase (set by the gateway's StatusFlusher once the
// member's hublink completes registration and starts heartbeating). There
// is no Cluster CRD to watch anymore, so this reads the DB directly, same
// as RegisterLocalCluster.
func WaitClusterOnline(ctx context.Context, name string, timeout time.Duration) error {
	db, stop, err := OpenGatewayDB(ctx)
	if err != nil {
		return fmt.Errorf("open gateway db: %w", err)
	}
	defer stop()
	repo := dbrepo.NewClusterRepo(db)
	return wait.PollUntilContextTimeout(ctx, 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		cl, err := repo.GetByName(ctx, name)
		if err != nil {
			return false, nil
		}
		return cl.Phase == gatewaymodel.ClusterPhaseOnline, nil
	})
}

// Cleanup removes all KubeSentry resources and test namespaces.
//
// Every command runs under a bounded per-call timeout (see run) rather than
// the caller's raw context: `kubectl delete crd` blocks on the CRD's
// customresourcecleanup finalizer, which itself blocks on every instance of
// that CRD actually being deleted — if a previous run's operator crashed
// (or was force-killed) while a CR still carried its own custom finalizer
// (e.g. PolicyGroup's kubesentry.io/policygroup-cleanup) with nothing left
// to remove it, that delete hangs forever. Cleanup used to be called with
// context.Background() (no deadline) from TestMain, so a single stuck
// finalizer silently ate the entire -timeout budget with no diagnostic
// output — observed firsthand while validating this task.
func Cleanup(ctx context.Context) {
	// Delete CR instances first, while the member operator (which owns
	// e.g. PolicyGroup's kubesentry.io/policygroup-cleanup finalizer) is
	// still running to actually process the finalizer removal — deleting
	// the operator (via helm uninstall kubesentry, below) first would
	// leave any CR that still carries that finalizer stuck forever with
	// nothing left to clear it, which is exactly what the CRD delete
	// loop's bounded --timeout=30s below now survives, but shouldn't need
	// to on a normal run.
	run(ctx, "kubectl", "delete", "policygroups.kubesentry.io", "--all", "--ignore-not-found", "--timeout=20s")
	run(ctx, "kubectl", "delete", "policies.kubesentry.io", "--all", "--ignore-not-found", "--timeout=20s")
	run(ctx, "kubectl", "delete", "policyexceptions.kubesentry.io", "--all", "--ignore-not-found", "--timeout=20s")
	run(ctx, "helm", "uninstall", "kubesentry-gateway", "-n", "kubesentry-system", "--ignore-not-found")
	run(ctx, "helm", "uninstall", "kubesentry", "-n", "kubesentry-system", "--ignore-not-found")
	run(ctx, "kubectl", "delete", "namespace", "kubesentry-system", "--ignore-not-found", "--timeout=90s")
	run(ctx, "kubectl", "delete", "namespace", TestNamespace, "--ignore-not-found", "--timeout=60s")
	// clusters.kubesentry.io / policydistributions.kubesentry.io no longer
	// exist — the gateway is fully database-backed (zero CRDs). The
	// remaining four are the member chart's own CRDs, unrelated to that
	// migration and still legitimately in use.
	for _, crd := range []string{
		"policies.kubesentry.io",
		"policygroups.kubesentry.io",
		"policyversions.kubesentry.io",
		"policyexceptions.kubesentry.io",
	} {
		run(ctx, "kubectl", "delete", "crd", crd, "--ignore-not-found", "--timeout=30s")
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
