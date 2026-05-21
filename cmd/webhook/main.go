package main

import (
	"context"
	"log/slog"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
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
			if p, ok := extractPolicy(obj); ok {
				policyCache.DeletePolicy(p.Name)
			}
		},
	})

	pgInformer, err := c.GetInformer(context.Background(), &v1alpha1.PolicyGroup{})
	if err != nil {
		slog.Error("get policygroup informer", "error", err)
		os.Exit(1)
	}
	pgInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { syncGroup(policyCache, obj) },
		UpdateFunc: func(_, obj interface{}) { syncGroup(policyCache, obj) },
		DeleteFunc: func(obj interface{}) {
			if g, ok := extractPolicyGroup(obj); ok {
				policyCache.DeleteGroup(g.Name)
			}
		},
	})

	exceptionCache := webhook.NewExceptionCache()
	exInformer, err := c.GetInformer(context.Background(), &v1alpha1.PolicyException{})
	if err != nil {
		slog.Error("get exception informer", "error", err)
		os.Exit(1)
	}
	exInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { syncException(exceptionCache, obj) },
		UpdateFunc: func(_, obj interface{}) { syncException(exceptionCache, obj) },
		DeleteFunc: func(obj interface{}) {
			if pex, ok := extractPolicyException(obj); ok {
				exceptionCache.Delete(pex.Name)
			}
		},
	})

	// API fallback for namespace lookups that miss the informer cache.
	// Without this, requests against a freshly-created namespace would
	// silently fail-open against any PolicyGroup whose namespaceSelector
	// is set (i.e. all built-in groups). Direct typed-client bypasses the
	// runtime cache, so it always hits the live API server.
	apiClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		slog.Error("create kubernetes client", "error", err)
		os.Exit(1)
	}
	namespaceCache := webhook.NewNamespaceCache()
	namespaceCache.SetFetcher(func(ctx context.Context, name string) (*corev1.Namespace, error) {
		return apiClient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	})
	nsInformer, err := c.GetInformer(context.Background(), &corev1.Namespace{})
	if err != nil {
		slog.Error("get namespace informer", "error", err)
		os.Exit(1)
	}
	nsInformer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { syncNamespace(namespaceCache, obj) },
		UpdateFunc: func(_, obj interface{}) { syncNamespace(namespaceCache, obj) },
		DeleteFunc: func(obj interface{}) {
			if ns, ok := extractNamespace(obj); ok {
				namespaceCache.Delete(ns.Name)
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
	exceptionCache.SetReady()
	slog.Info("exception cache ready")
	namespaceCache.SetReady()
	slog.Info("namespace cache ready")

	srv := webhook.NewServer(policyCache, exceptionCache, namespaceCache, webhook.ServerConfig{
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

func syncException(c *webhook.ExceptionCache, obj interface{}) {
	pex, ok := obj.(*v1alpha1.PolicyException)
	if !ok {
		return
	}
	c.Upsert(pex)
}

func syncNamespace(c *webhook.NamespaceCache, obj interface{}) {
	ns, ok := obj.(*corev1.Namespace)
	if !ok {
		return
	}
	c.Upsert(ns)
}

func extractNamespace(obj interface{}) (*corev1.Namespace, bool) {
	if ns, ok := obj.(*corev1.Namespace); ok {
		return ns, true
	}
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		if ns, ok := tombstone.Obj.(*corev1.Namespace); ok {
			return ns, true
		}
	}
	return nil, false
}

// extractPolicy unwraps a DeletedFinalStateUnknown tombstone so that delete
// events that arrive after a watch reconnect are still applied to the cache.
func extractPolicy(obj interface{}) (*v1alpha1.Policy, bool) {
	if p, ok := obj.(*v1alpha1.Policy); ok {
		return p, true
	}
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		if p, ok := tombstone.Obj.(*v1alpha1.Policy); ok {
			return p, true
		}
	}
	return nil, false
}

func extractPolicyException(obj interface{}) (*v1alpha1.PolicyException, bool) {
	if pex, ok := obj.(*v1alpha1.PolicyException); ok {
		return pex, true
	}
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		if pex, ok := tombstone.Obj.(*v1alpha1.PolicyException); ok {
			return pex, true
		}
	}
	return nil, false
}

func syncPolicy(c *webhook.PolicyCache, obj interface{}) {
	p, ok := obj.(*v1alpha1.Policy)
	if !ok {
		return
	}
	// Only PhaseReady policies are admissible. Any other state — Invalid
	// (validation failed), Syncing (operator hasn't processed yet), or empty
	// (just created, never reconciled) — must NOT participate in admission,
	// otherwise an unvalidated Rego could enter the decision path before the
	// operator has had a chance to reject it.
	if p.Status.Phase != v1alpha1.PhaseReady {
		c.DeletePolicy(p.Name)
		return
	}
	q, err := webhook.CompileRego(p.Spec.Rego)
	if err != nil {
		slog.Error("compile rego", "policy", p.Name, "error", err)
		c.DeletePolicy(p.Name)
		return
	}
	c.SetPolicy(p.Name, &webhook.CompiledPolicy{
		Name:         p.Name,
		Description:  p.Spec.Description,
		DefaultMode:  p.Spec.EnforcementMode,
		Match:        p.Spec.Match,
		Query:        q,
		ReferencedBy: append([]string(nil), p.Status.ReferencedBy...),
	})
}

func syncGroup(c *webhook.PolicyCache, obj interface{}) {
	g, ok := obj.(*v1alpha1.PolicyGroup)
	if !ok {
		return
	}
	cg, err := webhook.CompileGroup(g)
	if err != nil {
		slog.Error("compile group", "name", g.Name, "error", err)
		c.DeleteGroup(g.Name)
		return
	}
	c.SetGroup(g.Name, cg)
}

func extractPolicyGroup(obj interface{}) (*v1alpha1.PolicyGroup, bool) {
	if g, ok := obj.(*v1alpha1.PolicyGroup); ok {
		return g, true
	}
	if tombstone, ok := obj.(toolscache.DeletedFinalStateUnknown); ok {
		if g, ok := tombstone.Obj.(*v1alpha1.PolicyGroup); ok {
			return g, true
		}
	}
	return nil, false
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
	_ = corev1.AddToScheme(s)
	return s
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
