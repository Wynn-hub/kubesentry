// cmd/db — schema migration runner, invoked as a Helm hook Job (spec D17).
package main

import (
	"log/slog"
	"os"

	"github.com/Wynn-hub/kubesentry/internal/dbmigrate"
	"github.com/Wynn-hub/kubesentry/internal/logging"
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
	if err := dbmigrate.Migrate(dsn); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}
	slog.Info("migration complete")
}
