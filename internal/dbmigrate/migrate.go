// Package dbmigrate wraps golang-migrate with the project's embedded SQL
// migration files, so cmd/db and every testcontainers-based test call the
// exact same code path (spec D2/D16).
package dbmigrate

import (
	"embed"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies all pending migrations against dsn, up to the latest
// version. Safe to call repeatedly (golang-migrate skips already-applied
// versions), matching the pre-upgrade hook's re-run-on-every-upgrade
// behavior (spec D17).
func Migrate(dsn string) error {
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

// pinMigrationsTableSchema forces golang-migrate's own version-tracking
// table to live in the "public" schema, regardless of the connecting role's
// search_path.
//
// Without this, golang-migrate resolves its default schema via
// CURRENT_SCHEMA() on every new connection. Postgres's default search_path
// is `"$user", public` — since our DB role and our application schema are
// both named "kubesentry", once the 0001 migration creates the "kubesentry"
// schema, CURRENT_SCHEMA() on any later connection silently resolves to
// "kubesentry" instead of "public". golang-migrate would then look for
// schema_migrations in the wrong schema and re-run migration 0001 from
// scratch, breaking the idempotency the pre-upgrade hook depends on
// (spec D17).
func pinMigrationsTableSchema(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn // malformed dsn surfaces as a clearer error from migrate itself
	}
	q := u.Query()
	q.Set("x-migrations-table-quoted", "1")
	q.Set("x-migrations-table", `"public"."schema_migrations"`)
	u.RawQuery = q.Encode()
	return u.String()
}
