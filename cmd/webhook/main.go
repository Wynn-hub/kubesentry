package main

import (
	"context"
	"log/slog"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	toolscache "k8s.io/client-go/tools/cache"
	runtimecache "sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func main() {
	cfg, err := buildConfig()
	if err != nil {
		slog.Error("build kubeconfig", "error", err)
		os.Exit(1)
	}

	scheme := buildScheme()
	c, err := runtimecache.New(cfg, runtimecache.Options{Scheme: scheme})
	if err != nil {
		slog.Error("create cache", "error", err)
		os.Exit(1)
	}

	policyCache := webhook.NewPolicyCache()
	informer, err := c.GetInformer(context.Background(), &v1alpha1.Policy{})
	if err != nil {
		slog.Error("get informer", "error", err)
		os.Exit(1)
	}
	informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { syncPolicy(policyCache, obj) },
		UpdateFunc: func(_, obj interface{}) { syncPolicy(policyCache, obj) },
		DeleteFunc: func(obj interface{}) {
			if p, ok := obj.(*v1alpha1.Policy); ok {
				policyCache.Delete(p.Name)
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := c.Start(ctx); err != nil {
			slog.Error("cache start", "error", err)
		}
	}()
	c.WaitForCacheSync(ctx)
	policyCache.SetReady()
	slog.Info("policy cache ready")

	srv := webhook.NewServer(policyCache, webhook.ServerConfig{
		Addr:     envOrDefault("ADDR", ":8443"),
		CertFile: "/tls/tls.crt",
		KeyFile:  "/tls/tls.key",
	})
	slog.Info("starting webhook server", "addr", ":8443")
	if err := srv.ListenAndServeTLS(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func syncPolicy(c *webhook.PolicyCache, obj interface{}) {
	p, ok := obj.(*v1alpha1.Policy)
	if !ok {
		return
	}
	if p.Status.Phase == v1alpha1.PhaseInvalid {
		c.Delete(p.Name)
		return
	}
	q, err := webhook.CompileRego(p.Spec.Rego)
	if err != nil {
		slog.Error("compile rego", "policy", p.Name, "error", err)
		c.Delete(p.Name)
		return
	}
	c.Set(p.Name, &webhook.CompiledPolicy{
		Name:            p.Name,
		Key:             p.Labels[v1alpha1.LabelKey],
		GroupName:       p.Labels[v1alpha1.LabelGroup],
		Description:     p.Spec.Description,
		EnforcementMode: p.Spec.EnforcementMode,
		Match:           p.Spec.Match,
		Query:           q,
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
	return s
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
