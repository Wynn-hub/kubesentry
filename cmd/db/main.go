// cmd/db — schema migration runner, invoked as a Helm hook Job (spec D17).
package main

import (
	"log/slog"
	"os"
	"time"

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

	var err error
	for attempt := 1; attempt <= migrateRetryAttempts; attempt++ {
		if err = dbmigrate.Migrate(dsn); err == nil {
			slog.Info("migration complete")
			return
		}
		if attempt < migrateRetryAttempts {
			slog.Warn("migrate attempt failed, retrying", "attempt", attempt, "error", err)
			time.Sleep(migrateRetryDelay)
		}
	}
	slog.Error("migrate", "error", err)
	os.Exit(1)
}
