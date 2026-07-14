// cmd/hub/main.go — KubeSentry hub: member-cluster registry + gRPC gateway.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/hub"
	"github.com/Wynn-hub/kubesentry/internal/hubpb"
	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

var errGRPCNotReady = errors.New("grpc listener not yet serving")

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
		LeaderElection:         true,
		LeaderElectionID:       "kubesentry-hub-leader",
		HealthProbeBindAddress: envOrDefault("HEALTH_PROBE_ADDR", ":8081"),
	})
	if err != nil {
		slog.Error("create manager", "error", err)
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		slog.Error("add healthz check", "error", err)
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
	grpcRun := &grpcRunnable{
		addr:   grpcAddr,
		cert:   cert,
		server: hub.NewServer(mgr.GetClient(), namespace, tracker),
	}
	if err := mgr.Add(grpcRun); err != nil {
		slog.Error("add grpc runnable", "error", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", func(_ *http.Request) error {
		if !grpcRun.ready.Load() {
			return errGRPCNotReady
		}
		return nil
	}); err != nil {
		slog.Error("add readyz check", "error", err)
		os.Exit(1)
	}

	slog.Info("starting hub", "grpc", grpcAddr, "namespace", namespace)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("manager error", "error", err)
		os.Exit(1)
	}
}

// grpcRunnable serves gRPC under the manager's lifecycle. It must run on
// every replica regardless of leader status — like a webhook/health server,
// member clusters need an answer from whichever pod they reach. Without
// NeedLeaderElection()==false, controller-runtime's default treats a plain
// Runnable as leader-gated: on a single-replica rolling update, the new pod
// can't become ready (gRPC never starts) until it's leader, and it can't
// become leader until the old pod (still serving, still Ready) releases the
// lease — a deadlock the Deployment can never resolve on its own. Only the
// stateful writers (ClusterReconciler, StatusFlusher) should stay leader-only.
//
// Liveness/readiness are checked over the manager's plain HTTP health probe
// port (see mgr.AddReadyzCheck below), not the grpc_health_v1 protocol —
// Kubernetes' native `grpc` probe type only ever dials in plaintext, so it
// can never complete a handshake against this TLS-only listener.
type grpcRunnable struct {
	addr   string
	cert   tls.Certificate
	server *hub.Server
	ready  atomic.Bool
}

func (g *grpcRunnable) NeedLeaderElection() bool { return false }

func (g *grpcRunnable) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", g.addr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{g.cert},
		MinVersion:   tls.VersionTLS12,
	})))
	hubpb.RegisterHubServer(gs, g.server)
	go func() {
		<-ctx.Done()
		g.ready.Store(false)
		gs.GracefulStop()
	}()
	g.ready.Store(true)
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
