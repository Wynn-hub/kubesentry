package helpers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gatewayPostgresDSNFmt matches the DSN baked into the
// kubesentry-postgres-credentials Secret by
// charts/kubesentry-gateway/templates/postgres/secret.yaml — same user,
// password, and database name, just pointed at a port-forwarded localhost
// port instead of the in-cluster Service DNS name.
const gatewayPostgresDSNFmt = "postgres://kubesentry:kubesentry@127.0.0.1:%d/kubesentry?sslmode=disable"

// freeLocalPort asks the OS for an unused TCP port on localhost.
func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// OpenGatewayDB port-forwards the gateway's Postgres Service to a free local
// port and opens a GORM connection to it. Callers must invoke the returned
// stop func to tear down the port-forward (and close the *gorm.DB's
// underlying connection pool via its sql.DB) once done.
//
// The gateway has zero CRDs now (all state lives in Postgres via GORM) and
// console-api has no REST route for cluster CRUD yet, so this is how the
// e2e harness seeds/reads Cluster rows directly — the same dbrepo package
// cmd/gateway and cmd/console already use.
func OpenGatewayDB(ctx context.Context) (*gorm.DB, func(), error) {
	localPort, err := freeLocalPort()
	if err != nil {
		return nil, nil, fmt.Errorf("find free port: %w", err)
	}
	cmd := exec.CommandContext(ctx, "kubectl", "-n", WebhookNamespace,
		"port-forward", "svc/kubesentry-postgres", fmt.Sprintf("%d:5432", localPort))
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start postgres port-forward: %w", err)
	}
	stop := func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }

	dsn := fmt.Sprintf(gatewayPostgresDSNFmt, localPort)
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err == nil {
			if sqlDB, sqlErr := db.DB(); sqlErr == nil && sqlDB.PingContext(ctx) == nil {
				return db, stop, nil
			}
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	stop()
	return nil, nil, fmt.Errorf("connect to gateway postgres via port-forward: %w", lastErr)
}

// WaitForConsoleHealthy port-forwards the console Service briefly and polls
// /healthz until it answers 200. helm install --wait already blocks on the
// console-api container's own readyz (which is DB-gated), so by the time
// this runs the initContainer chain (Postgres → migrate Job → console) has
// long finished — this is a cheap extra confirmation that the Service is
// actually reachable and serving before the rest of TestMain starts issuing
// real console-api writes over the same kind of port-forward.
func WaitForConsoleHealthy(ctx context.Context, timeout time.Duration) error {
	localPort, err := freeLocalPort()
	if err != nil {
		return fmt.Errorf("find free port: %w", err)
	}
	cmd := exec.CommandContext(ctx, "kubectl", "-n", WebhookNamespace,
		"port-forward", "svc/kubesentry-console", fmt.Sprintf("%d:8080", localPort))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start console port-forward: %w", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", localPort)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("console /healthz not reachable within %s", timeout)
}
