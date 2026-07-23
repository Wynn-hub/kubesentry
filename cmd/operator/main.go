package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/logging"
	"github.com/Wynn-hub/kubesentry/internal/operator"
	"github.com/Wynn-hub/kubesentry/internal/operator/hublink"
	"github.com/Wynn-hub/kubesentry/internal/operator/uninstall"
	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

func main() {
	logging.Init()
	if len(os.Args) > 1 && os.Args[1] == "tls-setup" {
		runTLSSetup()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "uninstall" {
		runUninstall()
		return
	}
	runOperator()
}

func runOperator() {
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true)))
	scheme := buildScheme()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{&corev1.Secret{}},
			},
		},
		LeaderElection:   true,
		LeaderElectionID: "kubesentry-operator-leader",
	})
	if err != nil {
		slog.Error("create manager", "error", err)
		os.Exit(1)
	}

	historyLimit := 20
	if err := operator.NewPolicyReconciler(mgr.GetClient(), historyLimit).SetupWithManager(mgr); err != nil {
		slog.Error("setup policy reconciler", "error", err)
		os.Exit(1)
	}
	if err := operator.NewPolicyGroupReconciler(mgr.GetClient()).SetupWithManager(mgr); err != nil {
		slog.Error("setup policygroup reconciler", "error", err)
		os.Exit(1)
	}
	if err := operator.NewExceptionReconciler(mgr.GetClient()).SetupWithManager(mgr); err != nil {
		slog.Error("setup exception reconciler", "error", err)
		os.Exit(1)
	}
	vwcName := envOrDefault("VWC_NAME", "kubesentry")
	tlsSecretName := envOrDefault("SECRET_NAME", "kubesentry-tls")
	tlsNamespace := envOrDefault("NAMESPACE", "kubesentry-system")
	if err := operator.NewWebhookConfigReconciler(mgr.GetClient(), vwcName, tlsSecretName, tlsNamespace).SetupWithManager(mgr); err != nil {
		slog.Error("setup webhookconfig reconciler", "error", err)
		os.Exit(1)
	}

	if gatewayAddr := os.Getenv("GATEWAY_ADDRESS"); gatewayAddr != "" {
		link := hublink.New(hublink.Config{
			HubAddress:      gatewayAddr,
			TLSCAFile:       envOrDefault("GATEWAY_CA_FILE", "/etc/kubesentry/hub-ca/ca.crt"),
			TLSServerName:   os.Getenv("GATEWAY_TLS_SERVER_NAME"),
			ClusterName:     envOrDefault("CLUSTER_NAME", "local"),
			Namespace:       envOrDefault("NAMESPACE", "kubesentry-system"),
			TLSSecretName:   envOrDefault("SECRET_NAME", "kubesentry-tls"),
			OperatorVersion: envOrDefault("OPERATOR_VERSION", "dev"),
			Reader:          mgr.GetAPIReader(),
			Client:          mgr.GetClient(),
			HeartbeatEvery:  15 * time.Second,
		})
		if err := mgr.Add(link); err != nil {
			slog.Error("add gateway link", "error", err)
			os.Exit(1)
		}
		slog.Info("gateway link enabled", "gateway", gatewayAddr)
	}

	slog.Info("starting operator")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("manager error", "error", err)
		os.Exit(1)
	}
}

func runTLSSetup() {
	namespace := envOrDefault("NAMESPACE", "kubesentry-system")
	secretName := envOrDefault("SECRET_NAME", "kubesentry-tls")
	vwcName := envOrDefault("VWC_NAME", "kubesentry")
	svcName := fmt.Sprintf("kubesentry-webhook.%s.svc", namespace)

	cfg, err := buildConfig()
	if err != nil {
		slog.Error("build config", "error", err)
		os.Exit(1)
	}

	c, err := client.New(cfg, client.Options{Scheme: buildScheme()})
	if err != nil {
		slog.Error("build client", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()

	var existing corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, &existing); err == nil {
		slog.Info("TLS secret already exists, skipping generation")
		return
	} else if !apierrors.IsNotFound(err) {
		slog.Error("get secret", "error", err)
		os.Exit(1)
	}

	bundle, err := tlssetup.GenerateCerts(svcName)
	if err != nil {
		slog.Error("generate certs", "error", err)
		os.Exit(1)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: secretName},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"ca.crt":  bundle.CACert,
			"tls.crt": bundle.TLSCert,
			"tls.key": bundle.TLSKey,
		},
	}
	if err := c.Create(ctx, secret); err != nil {
		slog.Error("create secret", "error", err)
		os.Exit(1)
	}
	slog.Info("TLS secret created", "name", secretName)

	var vwc admissionregv1.ValidatingWebhookConfiguration
	if err := c.Get(ctx, types.NamespacedName{Name: vwcName}, &vwc); err != nil {
		if apierrors.IsNotFound(err) {
			// VWC is deployed after this pre-install hook; the operator will sync caBundle on startup.
			slog.Info("VWC not found, caBundle will be synced by operator")
			return
		}
		slog.Error("get VWC", "error", err)
		os.Exit(1)
	}
	patch := client.MergeFrom(vwc.DeepCopy())
	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = bundle.CACert
	}
	if err := c.Patch(ctx, &vwc, patch); err != nil {
		slog.Error("patch VWC caBundle", "error", err)
		os.Exit(1)
	}
	slog.Info("VWC caBundle patched")
}

// runUninstall is used by the chart's post-delete hook. It removes every
// KubeSentry-owned component (including CR instances and CRDs) while keeping
// the namespace intact, so a colocated gateway/control-plane is not removed.
func runUninstall() {
	cfg, err := buildConfig()
	if err != nil {
		slog.Error("build config", "error", err)
		os.Exit(1)
	}
	c, err := client.New(cfg, client.Options{Scheme: buildScheme()})
	if err != nil {
		slog.Error("build uninstall client", "error", err)
		os.Exit(1)
	}
	runner := uninstall.Runner{
		Client:        c,
		Namespace:     envOrDefault("NAMESPACE", "kubesentry-system"),
		TLSSecretName: envOrDefault("SECRET_NAME", "kubesentry-tls"),
	}
	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		slog.Error("uninstall KubeSentry resources", "error", err)
		os.Exit(1)
	}
	if err := runner.Finalize(ctx); err != nil {
		slog.Error("finalize KubeSentry uninstall", "error", err)
		os.Exit(1)
	}
	slog.Info("KubeSentry resources uninstalled")
}

func buildConfig() (*rest.Config, error) {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return clientcmd.BuildConfigFromFlags("", kc)
	}
	return rest.InClusterConfig()
}

func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = admissionregv1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	_ = coordinationv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = apiextensionsv1.AddToScheme(s)
	return s
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
