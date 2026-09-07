// cmd/db — schema migration runner, invoked as a Helm hook Job (spec D17).
// Also ships a "register-cluster" subcommand: a one-off Job that seeds a
// Cluster row + one-time registration token directly in the gateway's
// Postgres, for environments (deploy-local.sh) that have no Go runtime and
// no console-api REST route for cluster CRUD yet.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Wynn-hub/kubesentry/internal/dbmigrate"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/logging"
)

// ponytail: fixed retry budget rather than an external pg_isready probe. The
// post-install hook Job fires right after Postgres's StatefulSet is submitted,
// not necessarily Ready (spec D17). 60 attempts * 5s covers first-boot/PVC
// provisioning latency; bump if that ever isn't enough.
const (
	migrateRetryAttempts = 60
	migrateRetryDelay    = 5 * time.Second
)

func main() {
	logging.Init()
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "migrate":
		runMigrate()
	case "register-cluster":
		runRegisterCluster(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	slog.Error("usage: db migrate | db register-cluster --name=<cluster-name>")
	os.Exit(1)
}

func runMigrate() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		slog.Error("DATABASE_DSN is required")
		os.Exit(1)
	}

	if err := waitForConnection(dsn); err != nil {
		slog.Error("db unreachable", "error", err)
		os.Exit(1)
	}

	// Connection is reachable — a migrate failure past this point is a real
	// problem (bad migration state, permissions, etc.), not a "still
	// starting up" transient. Fail fast, no retry.
	if err := dbmigrate.Migrate(dsn); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}
	slog.Info("migration complete")
}

// runRegisterCluster seeds (or re-issues a token for) a Cluster row and
// prints the plaintext one-time registration token to stdout — and only
// the token, nothing else — so callers (deploy-local.sh, via `kubectl
// logs`) can capture it directly without touching the database themselves.
func runRegisterCluster(args []string) {
	fs := flag.NewFlagSet("register-cluster", flag.ExitOnError)
	name := fs.String("name", "", "cluster name to register")
	_ = fs.Parse(args)
	if *name == "" {
		slog.Error("register-cluster: --name is required")
		os.Exit(1)
	}
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		slog.Error("DATABASE_DSN is required")
		os.Exit(1)
	}
	if err := waitForConnection(dsn); err != nil {
		slog.Error("db unreachable", "error", err)
		os.Exit(1)
	}

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	if err := dbmodel.Init(dbmodel.NodeIDFromPodName(os.Getenv("POD_NAME"))); err != nil {
		slog.Error("init id generator", "error", err)
		os.Exit(1)
	}

	token, err := gateway.NewToken()
	if err != nil {
		slog.Error("generate token", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	repo := dbrepo.NewClusterRepo(db)
	existing, err := repo.GetByName(ctx, *name)
	switch {
	case err == nil:
		// Idempotent re-run: reissue a fresh one-time token for the
		// existing row rather than failing on a unique-name conflict.
		existing.RegTokenHash = gateway.HashToken(token)
		if err := repo.Update(ctx, existing); err != nil {
			slog.Error("update cluster", "error", err)
			os.Exit(1)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		cl := &dbrepo.Cluster{
			Name:         *name,
			Phase:        gatewaymodel.ClusterPhaseRegistering,
			RegTokenHash: gateway.HashToken(token),
		}
		if err := repo.Create(ctx, cl); err != nil {
			slog.Error("create cluster", "error", err)
			os.Exit(1)
		}
	default:
		slog.Error("get cluster", "error", err)
		os.Exit(1)
	}

	fmt.Println(token)
}

// waitForConnection retries a lightweight ping until Postgres accepts
// connections, up to migrateRetryAttempts. This only proves the server is
// reachable and the DSN is well-formed — it does not run any migrations.
func waitForConnection(dsn string) error {
	var err error
	for attempt := 1; attempt <= migrateRetryAttempts; attempt++ {
		if err = ping(dsn); err == nil {
			return nil
		}
		if attempt < migrateRetryAttempts {
			slog.Warn("db connection attempt failed, retrying", "attempt", attempt, "error", err)
			time.Sleep(migrateRetryDelay)
		}
	}
	return err
}

func ping(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}
