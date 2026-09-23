// Package dbmigrate wraps golang-migrate with the project's embedded SQL
// migration files, so cmd/db and every testcontainers-based test call the
// exact same code path (spec D2/D16).
package migrate

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies all pending migrations against dsn, up to the latest
// version. Safe to call repeatedly (golang-migrate skips already-applied
// versions), matching the pre-upgrade hook's re-run-on-every-upgrade
// behavior (spec D17).
func Migrate(dsn string) error {
	// golang-migrate's own version-tracking table is pinned into the
	// "kubesentry" schema (see pinMigrationsTableSchema) rather than
	// "public", so it must exist before ensureVersionTable's
	// CREATE TABLE IF NOT EXISTS runs — which happens before any migration
	// file (including 0001's own CREATE SCHEMA) executes.
	if err := ensureSchema(dsn); err != nil {
		return err
	}

	src, err := iofs.New(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pinMigrationsTableSchema(dsn))
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func ensureSchema(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS kubesentry`); err != nil {
		return fmt.Errorf("create kubesentry schema: %w", err)
	}
	return nil
}

// pinMigrationsTableSchema forces golang-migrate's own version-tracking
// table to live in the "kubesentry" schema (same database, same schema as
// every application table — no separate database, no "public" spillover),
// regardless of the connecting role's search_path.
//
// Without an explicit pin, golang-migrate resolves its default schema via
// CURRENT_SCHEMA() on every new connection, which depends on the role's
// search_path (default `"$user", public`) and drifts depending on what
// schemas already exist and connection order. Pinning explicitly avoids that
// instability, matching the idempotency the pre-upgrade hook depends on
// (spec D17).
func pinMigrationsTableSchema(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn // malformed dsn surfaces as a clearer error from migrate itself
	}
	q := u.Query()
	q.Set("x-migrations-table-quoted", "1")
	q.Set("x-migrations-table", `"kubesentry"."schema_migrations"`)
	u.RawQuery = q.Encode()
	return u.String()
}
