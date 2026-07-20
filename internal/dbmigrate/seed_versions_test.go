package dbmigrate_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/Wynn-hub/kubesentry/internal/dbmigrate"
)

// TestSeedBuiltinPoliciesHaveInitialVersion guards against the regression
// where migration 0002 seeded 37 built-in policies but zero policy_versions
// rows, silently breaking rollback for any built-in policy edited later
// (see 0005_seed_builtin_policy_versions.up.sql).
func TestSeedBuiltinPoliciesHaveInitialVersion(t *testing.T) {
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

	var policyCount int
	if err := db.QueryRow(`SELECT count(*) FROM kubesentry.policies`).Scan(&policyCount); err != nil {
		t.Fatalf("count policies: %v", err)
	}
	if policyCount != 37 {
		t.Fatalf("expected 37 seeded policies, got %d", policyCount)
	}

	rows, err := db.Query(`
		SELECT p.name, p.rego, count(v.id), max(v.version), coalesce(bool_and(v.rego = p.rego), false)
		FROM kubesentry.policies p
		LEFT JOIN kubesentry.policy_versions v ON v.policy_id = p.id
		GROUP BY p.name, p.rego
	`)
	if err != nil {
		t.Fatalf("query policy/version join: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var name, rego string
		var count int
		var maxVersion sql.NullInt64
		var regoMatches bool
		if err := rows.Scan(&name, &rego, &count, &maxVersion, &regoMatches); err != nil {
			t.Fatalf("scan: %v", err)
		}
		checked++
		if count != 1 {
			t.Errorf("policy %q: expected exactly 1 policy_versions row, got %d", name, count)
			continue
		}
		if !maxVersion.Valid || maxVersion.Int64 != 1 {
			t.Errorf("policy %q: expected version=1, got %v", name, maxVersion)
		}
		if !regoMatches {
			t.Errorf("policy %q: policy_versions.rego does not match policies.rego", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rows: %v", err)
	}
	if checked != 37 {
		t.Fatalf("expected to check 37 policies, checked %d", checked)
	}
}
