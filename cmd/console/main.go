// cmd/console/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Wynn-hub/kubesentry/internal/console"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/database/model"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/logging"
)

// Startup revalidation is a one-shot local DB scan; readyz pings must fail
// fast so a hung DB connection doesn't wedge probes indefinitely.
const (
	startupRevalidateTimeout = 30 * time.Second
	readyzPingTimeout        = 2 * time.Second
)

func main() {
	logging.Init()
	addr := os.Getenv("CONSOLE_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// Policy/PolicyGroup/PolicyException/summary handlers read/write the
	// same Postgres store as cmd/gateway (Task 9 + Task 10: console-api
	// GORM handlers). Discovery below still talks to the live cluster only
	// for handlers_schema.go's OpenAPI field lookups.
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
	if err := model.Init(model.NodeIDFromPodName(os.Getenv("POD_NAME"))); err != nil {
		slog.Error("init id generator", "error", err)
		os.Exit(1)
	}
	repos := repository.New(db)

	// console-api's k8s footprint is intentionally minimal: only a discovery
	// client, kept for handlers_schema.go's OpenAPI field lookups. Every
	// other handler is DB-backed (Task 9-11), so there is no cache to build
	// or informers to run here anymore.
	cfg, err := buildConfig()
	if err != nil {
		slog.Error("build kubeconfig", "error", err)
		os.Exit(1)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		slog.Error("build discovery client", "error", err)
		os.Exit(1)
	}

	// gatewayCtx backs only the long-lived Connect registration loop below,
	// which retries/reconnects for the life of the process. It must stay
	// separate from the bounded, one-shot contexts used for startup
	// revalidation and readyz pings (see startupRevalidateTimeout,
	// readyzPingTimeout) — reusing one context.Background() for both let a
	// hung DB connection block startup and every readyz probe forever.
	gatewayCtx := context.Background()

	// gatewayClient talks NotifyChange for the write handlers (Task 11) and
	// backs the Connect registration loop below. Both are a soft
	// dependency — GATEWAY_ADDRESS unset, or any dial-time setup error,
	// just leaves gatewayClient nil; console-api's REST API must keep
	// working either way (see console.Handlers.Gateway).
	var gatewayClient gatewaypb.HubClient
	if gatewayAddr := os.Getenv("GATEWAY_ADDRESS"); gatewayAddr != "" {
		creds, err := credentials.NewClientTLSFromFile(
			envOrDefault("GATEWAY_CA_FILE", "/etc/kubesentry/gateway-ca/ca.crt"),
			os.Getenv("GATEWAY_TLS_SERVER_NAME"))
		if err != nil {
			slog.Error("load gateway ca, continuing without gateway link", "error", err)
		} else if conn, err := grpc.NewClient(gatewayAddr, grpc.WithTransportCredentials(creds)); err != nil {
			slog.Error("dial gateway, continuing without gateway link", "error", err)
		} else {
			gatewayClient = gatewaypb.NewHubClient(conn)
			sampleSource := console.NewPlatformSampleSource(
				envOrDefault("POD_NAME", "console-api"),
				envOrDefault("CONSOLE_WEB_URL", "http://127.0.0.1:8080/"),
			)
			go console.RunGatewayLinkWithSamples(
				gatewayCtx, gatewayClient, "console-api",
				os.Getenv("GATEWAY_SHARED_TOKEN"), sampleSource)
			slog.Info("gateway link enabled", "gateway", gatewayAddr)
		}
	} else {
		slog.Info("GATEWAY_ADDRESS not set; console-api will not notify the gateway of writes")
	}

	// Defense against someone editing the database directly, bypassing the
	// API's synchronous Rego validation (spec D8). Runs synchronously before
	// the server starts serving — a single pass of local DB round-trips, not
	// worth backgrounding — but a failure here is logged and never fatal:
	// console-api must still come up even if revalidation itself errors.
	revalidateCtx, revalidateCancel := context.WithTimeout(context.Background(), startupRevalidateTimeout)
	err = console.RevalidateAllPolicies(revalidateCtx, repos.Policies)
	revalidateCancel()
	if err != nil {
		slog.Error("revalidate policies at startup", "error", err)
	} else {
		slog.Info("revalidated all policies at startup")
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("get underlying sql.DB for readiness check", "error", err)
		os.Exit(1)
	}
	ready := func() bool {
		pingCtx, cancel := context.WithTimeout(context.Background(), readyzPingTimeout)
		defer cancel()
		return sqlDB.PingContext(pingCtx) == nil
	}

	sessionAbsolute, err := time.ParseDuration(envOrDefault("CONSOLE_SESSION_ABSOLUTE_DURATION", "24h"))
	if err != nil || sessionAbsolute <= 0 {
		slog.Error("invalid CONSOLE_SESSION_ABSOLUTE_DURATION")
		os.Exit(1)
	}
	sessionIdle, err := time.ParseDuration(envOrDefault("CONSOLE_SESSION_IDLE_DURATION", "30m"))
	if err != nil || sessionIdle <= 0 {
		slog.Error("invalid CONSOLE_SESSION_IDLE_DURATION")
		os.Exit(1)
	}
	if sessionIdle > sessionAbsolute {
		slog.Error("CONSOLE_SESSION_IDLE_DURATION must not exceed CONSOLE_SESSION_ABSOLUTE_DURATION")
		os.Exit(1)
	}
	cookieSecure, err := strconv.ParseBool(envOrDefault("CONSOLE_SESSION_COOKIE_SECURE", "false"))
	if err != nil {
		slog.Error("invalid CONSOLE_SESSION_COOKIE_SECURE")
		os.Exit(1)
	}

	h := &console.Handlers{Repos: repos, Discovery: disc, Gateway: gatewayClient, SessionAbsoluteDuration: sessionAbsolute, SessionIdleDuration: sessionIdle, SessionCookieSecure: cookieSecure}
	srv, err := console.NewServer(h, ready)
	if err != nil {
		slog.Error("build console server", "error", err)
		os.Exit(1)
	}

	slog.Info("console listening", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Handler); err != nil {
		slog.Error("serve", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
