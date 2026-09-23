package builtins_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	dbmigrate "github.com/Wynn-hub/kubesentry/internal/database/migrate"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

// TestBuiltinPoliciesCompile runs the real migration pipeline (0002 seeds
// the built-in policies, spec D9) against a throwaway Postgres and verifies
// every seeded policy's rego compiles via the production webhook compiler.
// Catches v0 syntax mistakes (rego.v1 / some x in set) and broken sprintf
// signatures at PR time — the chart no longer renders any Policy CRs
// (spec D17), so this replaces the old `helm template` based check.
func TestBuiltinPoliciesCompile(t *testing.T) {
	ctx := context.Background()
	pgc, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("kubesentry"),
		postgres.WithUsername("kubesentry"),
		postgres.WithPassword("kubesentry"),
		testcontainers.WithWaitStrategy(tcwait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if err := dbmigrate.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name, rego FROM kubesentry.policies`)
	if err != nil {
		t.Fatalf("query policies: %v", err)
	}
	defer rows.Close()

	policyCount := 0
	for rows.Next() {
		var name, rego string
		if err := rows.Scan(&name, &rego); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if rego == "" {
			t.Errorf("policy %s: empty rego", name)
			continue
		}
		policyCount++
		if _, err := webhook.CompileRego(rego); err != nil {
			t.Errorf("policy %s rego compile: %v", name, err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if policyCount < 37 {
		t.Errorf("expected ≥37 seeded policies, got %d", policyCount)
	}
}
