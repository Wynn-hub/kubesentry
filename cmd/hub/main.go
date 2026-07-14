// cmd/hub/main.go — KubeSentry hub: member-cluster registry + gRPC gateway.
package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hub"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

func main() {
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true)))

	namespace := envOrDefault("NAMESPACE", "kubesentry-system")
	grpcAddr := envOrDefault("GRPC_ADDR", ":9443")
	tlsSecret := envOrDefault("TLS_SECRET_NAME", "kubesentry-hub-tls")
	tlsDNS := envOrDefault("TLS_DNS_NAME", "kubesentry-hub."+namespace+".svc")
	flushEvery := envDuration("HEARTBEAT_FLUSH_INTERVAL", 30*time.Second)
	offlineAfter := envDuration("OFFLINE_AFTER", 90*time.Second)

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Client: client.Options{
			Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}},
		},
		LeaderElection:   true,
		LeaderElectionID: "kubesentry-hub-leader",
	})
	if err != nil {
		slog.Error("create manager", "error", err)
		os.Exit(1)
	}

	if err := hub.NewClusterReconciler(mgr.GetClient(), namespace).SetupWithManager(mgr); err != nil {
		slog.Error("setup cluster reconciler", "error", err)
		os.Exit(1)
	}

	tracker := hub.NewHeartbeatTracker()
	if err := mgr.Add(hub.NewStatusFlusher(mgr.GetClient(), tracker, flushEvery, offlineAfter)); err != nil {
		slog.Error("add status flusher", "error", err)
		os.Exit(1)
	}

	cert, err := ensureTLS(namespace, tlsSecret, tlsDNS)
	if err != nil {
		slog.Error("ensure tls", "error", err)
		os.Exit(1)
	}
	if err := mgr.Add(&grpcRunnable{
		addr:   grpcAddr,
		cert:   cert,
		server: hub.NewServer(mgr.GetClient(), namespace, tracker),
	}); err != nil {
		slog.Error("add grpc runnable", "error", err)
		os.Exit(1)
	}

	slog.Info("starting hub", "grpc", grpcAddr, "namespace", namespace)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("manager error", "error", err)
		os.Exit(1)
	}
}

// grpcRunnable serves gRPC under the manager's lifecycle.
type grpcRunnable struct {
	addr   string
	cert   tls.Certificate
	server *hub.Server
}

func (g *grpcRunnable) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", g.addr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{g.cert},
		MinVersion:   tls.VersionTLS12,
	})))
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(gs, healthSrv)
	hubpb.RegisterHubServer(gs, g.server)
	go func() {
		<-ctx.Done()
		healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		gs.GracefulStop()
	}()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	return gs.Serve(lis)
}

// ensureTLS loads the hub server cert Secret, generating a self-signed
// bundle on first start (replicas=1, no leader race by design).
// ponytail: single-replica bootstrap; move to a pre-install job if hub ever runs multi-replica.
func ensureTLS(namespace, secretName, dnsName string) (tls.Certificate, error) {
	cfg := ctrl.GetConfigOrDie()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return tls.Certificate{}, err
	}
	ctx := context.Background()

	var sec corev1.Secret
	err = c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretName}, &sec)
	if apierrors.IsNotFound(err) {
		bundle, genErr := tlssetup.GenerateCerts(dnsName)
		if genErr != nil {
			return tls.Certificate{}, genErr
		}
		sec = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: secretName},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"ca.crt":  bundle.CACert,
				"tls.crt": bundle.TLSCert,
				"tls.key": bundle.TLSKey,
			},
		}
		if createErr := c.Create(ctx, &sec); createErr != nil {
			return tls.Certificate{}, createErr
		}
		slog.Info("generated hub TLS secret", "name", secretName, "dns", dnsName)
	} else if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(sec.Data["tls.crt"], sec.Data["tls.key"])
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		slog.Warn("invalid duration env, using default", "key", key, "default", def)
	}
	return def
}
