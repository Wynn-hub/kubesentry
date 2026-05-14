package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
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
	"github.com/Wynn-hub/kubesentry/internal/operator"
	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tls-setup" {
		runTLSSetup()
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
	vwcName := envOrDefault("VWC_NAME", "kubesentry")
	tlsSecretName := envOrDefault("SECRET_NAME", "kubesentry-tls")
	tlsNamespace := envOrDefault("NAMESPACE", "kubesentry-system")
	if err := operator.NewWebhookConfigReconciler(mgr.GetClient(), vwcName, tlsSecretName, tlsNamespace).SetupWithManager(mgr); err != nil {
		slog.Error("setup webhookconfig reconciler", "error", err)
		os.Exit(1)
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
	_ = corev1.AddToScheme(s)
	return s
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
