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

func TestSeedDataLoaded(t *testing.T) {
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
		t.Errorf("expected 37 seeded policies, got %d", policyCount)
	}

	// Spot-check specific well-known policies actually landed with real
	// content, not just empty rows.
	for _, name := range []string{"run-as-privileged", "automount-service-account-token"} {
		var rego string
		if err := db.QueryRow(`SELECT rego FROM kubesentry.policies WHERE name = $1`, name).Scan(&rego); err != nil {
			t.Fatalf("query policy %q: %v", name, err)
		}
		if rego == "" {
			t.Errorf("expected non-empty rego for policy %q", name)
		}
	}

	var groupExists, distExists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM kubesentry.policy_groups WHERE name = 'default')`).Scan(&groupExists); err != nil {
		t.Fatalf("check default group: %v", err)
	}
	if !groupExists {
		t.Error("expected a 'default' policy_groups row")
	}
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM kubesentry.policy_distributions WHERE name = 'default')`).Scan(&distExists); err != nil {
		t.Fatalf("check default distribution: %v", err)
	}
	if !distExists {
		t.Error("expected a 'default' policy_distributions row")
	}
}
