// cmd/console/main.go
package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console"
	"github.com/Wynn-hub/kubesentry/web"
)

func main() {
	addr := os.Getenv("CONSOLE_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	cfg, err := buildConfig()
	if err != nil {
		slog.Error("build kubeconfig", "error", err)
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		slog.Error("add scheme", "error", err)
		os.Exit(1)
	}

	cl, err := cluster.New(cfg, func(o *cluster.Options) { o.Scheme = scheme })
	if err != nil {
		slog.Error("create cluster", "error", err)
		os.Exit(1)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		slog.Error("build discovery client", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	go func() {
		if err := cl.Start(ctx); err != nil {
			slog.Error("start cache", "error", err)
			os.Exit(1)
		}
	}()

	readyCh := make(chan struct{})
	go func() {
		cl.GetCache().WaitForCacheSync(ctx)
		close(readyCh)
	}()
	ready := func() bool {
		select {
		case <-readyCh:
			return true
		default:
			return false
		}
	}

	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		slog.Error("sub dist fs", "error", err)
		os.Exit(1)
	}

	h := &console.Handlers{Client: cl.GetClient(), Discovery: disc}
	srv := console.NewServer(h, dist, ready)

	slog.Info("console listening", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Handler); err != nil {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}

func buildConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
