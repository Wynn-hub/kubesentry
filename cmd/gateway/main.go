// cmd/gateway/main.go — KubeSentry gateway: member-cluster registry + gRPC gateway.
package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/tlssetup"
)

func main() {
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true)))

	namespace := envOrDefault("NAMESPACE", "kubesentry-system")
	grpcAddr := envOrDefault("GRPC_ADDR", ":9443")
	tlsSecret := envOrDefault("TLS_SECRET_NAME", "kubesentry-gateway-tls")
	tlsDNS := envOrDefault("TLS_DNS_NAME", "kubesentry-gateway."+namespace+".svc")

	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		slog.Error("DATABASE_DSN is required")
		os.Exit(1)
	}
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	if err := dbmodel.Init(dbmodel.NodeIDFromPodName(os.Getenv("POD_NAME"))); err != nil {
		slog.Error("init id generator", "error", err)
		os.Exit(1)
	}
	repos := gateway.Repos{
		Clusters:      dbrepo.NewClusterRepo(db),
		Policies:      dbrepo.NewPolicyRepo(db),
		Groups:        dbrepo.NewGroupRepo(db),
		Distributions: dbrepo.NewDistributionRepo(db),
	}

	// ponytail: heartbeat state is in-memory only now — StatusFlusher (internal/gateway/heartbeats.go)
	// still persists via a k8s Cluster CRD client and has no DB-backed replacement yet.
	// Task 8's Connect/NotifyChange RPCs are expected to own liveness tracking against
	// dbrepo.Cluster (which already has Phase/OperatorVersion/LastHeartbeatAt columns) —
	// wire a DB-backed flusher there instead of resurrecting the CRD-based one.
	tracker := gateway.NewHeartbeatTracker()
	notifier := gateway.NewNotifier()

	cert, err := ensureTLS(namespace, tlsSecret, tlsDNS)
	if err != nil {
		slog.Error("ensure tls", "error", err)
		os.Exit(1)
	}
	grpcRun := &grpcRunnable{
		addr:   grpcAddr,
		cert:   cert,
		server: gateway.NewServer(repos, namespace, tracker, notifier),
	}

	ctx := ctrl.SetupSignalHandler()
	go startHealthServer(ctx, &grpcRun.ready)

	slog.Info("starting gateway", "grpc", grpcAddr, "namespace", namespace)
	if err := grpcRun.Start(ctx); err != nil {
		slog.Error("grpc server error", "error", err)
		os.Exit(1)
	}
}

// healthAddr matches the hardcoded containerPort 8081 (name: probe) in
// charts/kubesentry-gateway/templates/deployment.yaml's liveness/readiness
// probes — not exposed as an env var there, so no envOrDefault indirection.
const healthAddr = ":8081"

// startHealthServer serves the plain-HTTP probes the chart expects (the gRPC
// listener is TLS-only, so Kubernetes' native grpc probe type can't reach
// it). /healthz reports the process is up; /readyz reflects grpcRunnable's
// ready flag. Shuts down on ctx.Done(), same pattern as grpcRunnable.Start.
func startHealthServer(ctx context.Context, ready *atomic.Bool) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	srv := &http.Server{Addr: healthAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("health server error", "error", err)
	}
}

// grpcRunnable serves gRPC. It must run on every replica — like a
// webhook/health server, member clusters need an answer from whichever pod
// they reach.
//
// Liveness/readiness are checked over plain HTTP (not the grpc_health_v1
// protocol — Kubernetes' native `grpc` probe type only ever dials in
// plaintext, so it can never complete a handshake against this TLS-only
// listener); see startHealthServer, which reads the `ready` flag below.
type grpcRunnable struct {
	addr   string
	cert   tls.Certificate
	server *gateway.Server
	ready  atomic.Bool
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
	gatewaypb.RegisterHubServer(gs, g.server)
	go func() {
		<-ctx.Done()
		g.ready.Store(false)
		gs.GracefulStop()
	}()
	g.ready.Store(true)
	return gs.Serve(lis)
}

// ensureTLS loads the gateway server cert Secret, generating a self-signed
// bundle on first start (replicas=1, no leader race by design).
// ponytail: single-replica bootstrap; move to a pre-install job if gateway ever runs multi-replica.
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
		// No cert persisted yet — nothing was configured via the gateway chart's
		// tls.ca/cert/key values (those pre-create this same Secret via
		// tls-secret.yaml before the gateway pod ever starts), so fall back to
		// the hardcoded default bundle rather than generating a fresh random
		// one each first start.
		bundle := tlssetup.DefaultCertBundle()
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
		slog.Info("persisted default gateway TLS secret", "name", secretName, "dns", dnsName)
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
