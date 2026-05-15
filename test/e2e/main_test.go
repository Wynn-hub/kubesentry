//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/test/e2e/helpers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	K8sClient      client.Client
	K8sClientset   *kubernetes.Clientset
	WebhookPodName string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	chartDir := filepath.Join("..", "..", "charts", "kubesentry")
	valuesFile := filepath.Join("testdata", "values-test.yaml")

	helpers.Cleanup(ctx)

	if err := helpers.HelmInstall(ctx, chartDir, valuesFile); err != nil {
		panic("helm install: " + err.Error())
	}

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic("build kubeconfig: " + err.Error())
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	K8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic("create k8s client: " + err.Error())
	}

	K8sClientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		panic("create clientset: " + err.Error())
	}

	if err := helpers.WaitForDeploymentReady(ctx, K8sClient, "kubesentry-system", "kubesentry-webhook", 2*time.Minute); err != nil {
		panic("webhook deployment not ready: " + err.Error())
	}
	if err := helpers.WaitForDeploymentReady(ctx, K8sClient, "kubesentry-system", "kubesentry-operator", 2*time.Minute); err != nil {
		panic("operator deployment not ready: " + err.Error())
	}

	if err := helpers.CreateNamespace(ctx, K8sClient, helpers.TestNamespace); err != nil {
		panic("create test namespace: " + err.Error())
	}

	if err := helpers.WaitForAllPoliciesReady(ctx, K8sClient, 2*time.Minute); err != nil {
		panic("policies not ready: " + err.Error())
	}

	WebhookPodName, err = helpers.GetWebhookPodName(ctx, K8sClient)
	if err != nil {
		panic("get webhook pod: " + err.Error())
	}

	code := m.Run()

	helpers.Cleanup(ctx)
	os.Exit(code)
}
