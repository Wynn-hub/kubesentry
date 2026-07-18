// cmd/db — schema migration runner, invoked as a Helm hook Job (spec D17).
package main

import (
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Wynn-hub/kubesentry/internal/dbmigrate"
	"github.com/Wynn-hub/kubesentry/internal/logging"
)

// ponytail: fixed retry budget instead of a shell pg_isready probe — this
// binary ships on distroless/static (no shell, no postgres client tools)
// like every other kubesentry image, and the post-install hook Job that
// runs this fires right after Postgres's StatefulSet is submitted, not
// necessarily Ready (spec D17). 60 attempts * 5s covers first-boot/PVC
// provisioning latency; bump if that ever isn't enough.
const (
	migrateRetryAttempts = 60
	migrateRetryDelay    = 5 * time.Second
)

func main() {
	logging.Init()
	if len(os.Args) < 2 || os.Args[1] != "migrate" {
		slog.Error("usage: db migrate")
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

	// Connection is reachable — a migrate failure past this point is a real
	// problem (bad migration state, permissions, etc.), not a "still
	// starting up" transient. Fail fast, no retry.
	if err := dbmigrate.Migrate(dsn); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}
	slog.Info("migration complete")
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
