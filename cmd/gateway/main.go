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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
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
		Clusters:       dbrepo.NewClusterRepo(db),
		Policies:       dbrepo.NewPolicyRepo(db),
		Groups:         dbrepo.NewGroupRepo(db),
		Exceptions:     dbrepo.NewExceptionRepo(db),
		Distributions:  dbrepo.NewDistributionRepo(db),
		UninstallTasks: dbrepo.NewUninstallTaskRepo(db),
	}

	tracker := gateway.NewHeartbeatTracker()
	notifier := gateway.NewNotifier()
	sharedToken := os.Getenv("GATEWAY_SHARED_TOKEN")

	ctx := ctrl.SetupSignalHandler()
	var ready atomic.Bool
	go startHealthServer(ctx, &ready)
	flushInterval := envDurationOrDefault("HEARTBEAT_FLUSH_INTERVAL", 15*time.Second)
	offlineAfter := envDurationOrDefault("HEARTBEAT_OFFLINE_AFTER", 90*time.Second)

	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("create kubernetes client", "error", err)
		os.Exit(1)
	}
	identity := envOrDefault("POD_NAME", hostname())
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace,
		"kubesentry-gateway-leader",
		clientset.CoreV1(),
		clientset.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: identity},
	)
	if err != nil {
		slog.Error("create gateway leader-election lock", "error", err)
		os.Exit(1)
	}

	leaderElector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				cert, err := ensureTLS(namespace, tlsSecret, tlsDNS)
				if err != nil {
					slog.Error("ensure tls as gateway leader", "error", err)
					// A leader that cannot start gRPC must relinquish the Lease;
					// otherwise every standby remains blocked behind a permanently
					// NotReady leader. Match the former startup-failure behavior and
					// let Kubernetes restart this Pod.
					os.Exit(1)
				}
				grpcRun := &grpcRunnable{
					addr:   grpcAddr,
					cert:   cert,
					server: gateway.NewServer(repos, namespace, tracker, notifier, sharedToken),
					ready:  &ready,
				}
				flusher := gateway.NewStatusFlusher(repos.Clusters, tracker, flushInterval, offlineAfter)
				go func() {
					if err := flusher.Start(leaderCtx); err != nil {
						slog.Error("status flusher error", "error", err)
					}
				}()

				slog.Info("gateway became leader; starting gRPC service", "grpc", grpcAddr, "namespace", namespace)
				if err := grpcRun.Start(leaderCtx); err != nil && leaderCtx.Err() == nil {
					slog.Error("grpc server error", "error", err)
					os.Exit(1)
				}
			},
			OnStoppedLeading: func() {
				// Exclude a former leader from EndpointSlices before its gRPC
				// server finishes draining existing long-lived streams.
				ready.Store(false)
				slog.Info("gateway leadership lost; gRPC service is unavailable")
			},
			OnNewLeader: func(newIdentity string) {
				slog.Info("gateway leader changed", "leader", newIdentity)
			},
		},
	})
	if err != nil {
		slog.Error("create gateway leader elector", "error", err)
		os.Exit(1)
	}
	slog.Info("starting gateway leader election", "identity", identity, "namespace", namespace)
	leaderElector.Run(ctx)
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return "gateway-unknown"
	}
	return host
}

// healthAddr matches the hardcoded containerPort 8081 (name: probe) in
// charts/kubesentry-gateway/templates/deployment.yaml's liveness/readiness
// probes — not exposed as an env var there, so no envOrDefault indirection.
const healthAddr = ":8081"

// startHealthServer serves the plain-HTTP probes the chart expects (the gRPC
// listener is TLS-only, so Kubernetes' native grpc probe type can't reach
// it). /healthz reports the process is up; /readyz reflects the leader-owned
// gRPC server's ready flag. Shuts down on ctx.Done(), same pattern as
// grpcRunnable.Start.
func startHealthServer(ctx context.Context, ready *atomic.Bool) {
	srv := &http.Server{Addr: healthAddr, Handler: healthHandler(ready)}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("health server error", "error", err)
	}
}

func healthHandler(ready *atomic.Bool) http.Handler {
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
	return mux
}

// grpcRunnable serves gRPC only while this pod owns the gateway Lease. Its
// readiness flag is shared with the health endpoint, ensuring the Service's
// EndpointSlice contains exactly the current leader.
//
// Liveness/readiness are checked over plain HTTP (not the grpc_health_v1
// protocol — Kubernetes' native `grpc` probe type only ever dials in
// plaintext, so it can never complete a handshake against this TLS-only
// listener); see startHealthServer, which reads the `ready` flag below.
type grpcRunnable struct {
	addr   string
	cert   tls.Certificate
	server *gateway.Server
	ready  *atomic.Bool
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

func envDurationOrDefault(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
