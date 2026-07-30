package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

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
	if len(os.Args) > 1 && os.Args[1] == "apply-crds" {
		runApplyCRDs()
		return
	}
	runOperator()
}

func runOperator() {
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true)))
	scheme := buildScheme()
	kubeConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
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
		linkConfig := hublink.Config{
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
			Instance:        os.Getenv("POD_NAME"),
			LeaderElected:   mgr.Elected(),
		}
		// Cluster inventory is best-effort: a member that cannot build the
		// discovery/nodes clients still reports policy counts and heartbeats.
		if discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeConfig); err != nil {
			slog.Warn("build discovery client for cluster inventory", "error", err)
		} else {
			linkConfig.Discovery = discoveryClient
		}
		if nodesClient, err := kubernetes.NewForConfig(kubeConfig); err != nil {
			slog.Warn("build nodes client for cluster inventory", "error", err)
		} else {
			linkConfig.NodeLister = clientsetNodeLister{core: nodesClient.CoreV1()}
		}
		linkConfig.APIServerAddress = kubeConfig.Host
		link := hublink.New(linkConfig)
		if err := mgr.Add(link); err != nil {
			slog.Error("add gateway link", "error", err)
			os.Exit(1)
		}
		if token := os.Getenv("ADMISSION_EVENT_TOKEN"); token != "" {
			if err := mgr.Add(&operator.AdmissionEventServer{Address: envOrDefault("ADMISSION_EVENT_ADDR", ":8082"), Token: token, Sink: link}); err != nil {
				slog.Error("add admission event server", "error", err)
				os.Exit(1)
			}
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

// runApplyCRDs makes an interrupted uninstall recoverable on the next Helm
// install or upgrade. Helm intentionally skips crds/ during upgrades.
func runApplyCRDs() {
	cfg, err := buildConfig()
	if err != nil {
		slog.Error("build config", "error", err)
		os.Exit(1)
	}
	c, err := client.New(cfg, client.Options{Scheme: buildScheme()})
	if err != nil {
		slog.Error("build CRD client", "error", err)
		os.Exit(1)
	}
	paths, err := filepath.Glob(filepath.Join(envOrDefault("CRD_MANIFEST_DIR", "/crds"), "*.yaml"))
	if err != nil || len(paths) == 0 {
		slog.Error("find CRD manifests", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Error("read CRD manifest", "path", path, "error", err)
			os.Exit(1)
		}
		var desired apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal(data, &desired); err != nil || desired.Name == "" {
			slog.Error("decode CRD manifest", "path", path, "error", err)
			os.Exit(1)
		}
		if err := applyCRD(ctx, c, &desired); err != nil {
			slog.Error("apply CRD", "name", desired.Name, "error", err)
			os.Exit(1)
		}
		if err := waitForCRDEstablished(ctx, c, desired.Name); err != nil {
			slog.Error("wait for CRD establishment", "name", desired.Name, "error", err)
			os.Exit(1)
		}
	}
	slog.Info("KubeSentry CRDs applied", "count", len(paths))
}

func applyCRD(ctx context.Context, c client.Client, desired *apiextensionsv1.CustomResourceDefinition) error {
	var existing apiextensionsv1.CustomResourceDefinition
	err := c.Get(ctx, types.NamespacedName{Name: desired.Name}, &existing)
	switch {
	case apierrors.IsNotFound(err):
		return c.Create(ctx, desired)
	case err != nil:
		return err
	default:
		desired.ResourceVersion = existing.ResourceVersion
		return c.Update(ctx, desired)
	}
}

func waitForCRDEstablished(ctx context.Context, c client.Client, name string) error {
	return wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := c.Get(ctx, types.NamespacedName{Name: name}, &crd); err != nil {
			return false, err
		}
		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
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

// clientsetNodeLister adapts the typed core client to hublink.NodeLister.
type clientsetNodeLister struct {
	core corev1client.CoreV1Interface
}

func (c clientsetNodeLister) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	list, err := c.core.Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
