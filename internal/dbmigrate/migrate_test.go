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

func TestMigrateCreatesAllTables(t *testing.T) {
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

	wantTables := []string{
		"clusters", "policies", "policy_versions", "policy_groups",
		"policy_exceptions", "policy_distributions", "distribution_cluster_status",
		"cluster_install_records",
	}
	for _, name := range wantTables {
		var exists bool
		q := `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'kubesentry' AND table_name = $1)`
		if err := db.QueryRow(q, name).Scan(&exists); err != nil {
			t.Fatalf("query table %q: %v", name, err)
		}
		if !exists {
			t.Errorf("expected table %q to exist after migrate", name)
		}
	}

	// Migrate must be idempotent — re-running against an already-migrated
	// DB (the pre-upgrade hook re-runs it on every helm upgrade, spec D17)
	// must not error.
	if err := dbmigrate.Migrate(dsn); err != nil {
		t.Fatalf("second migrate call: %v", err)
	}
}
