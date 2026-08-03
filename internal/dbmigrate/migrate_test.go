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
		"cluster_install_records", "cluster_uninstall_tasks",
		"admission_event_settings", "admission_storage_policies", "admission_events",
		"platform_observability_settings", "admission_link_incidents",
		"platform_component_samples", "platform_leadership_events",
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

	var retryEnabled bool
	var maxRetryAttempts, queueCapacity, incidentRetentionDays int
	if err := db.QueryRow(`SELECT retry_enabled, max_retry_attempts, queue_capacity, incident_retention_days FROM kubesentry.admission_event_settings WHERE id = 1`).
		Scan(&retryEnabled, &maxRetryAttempts, &queueCapacity, &incidentRetentionDays); err != nil {
		t.Fatalf("query admission event settings defaults: %v", err)
	}
	if retryEnabled || maxRetryAttempts != 3 || queueCapacity != 1024 || incidentRetentionDays != 7 {
		t.Errorf("admission delivery defaults = (%v, %d, %d, %d), want (false, 3, 1024, 7)", retryEnabled, maxRetryAttempts, queueCapacity, incidentRetentionDays)
	}

	// Both observability switches default off, and each lives with the thing it
	// instruments: the pipeline funnel with the admission event settings, the
	// component sampling with the platform settings.
	var observationEnabled bool
	if err := db.QueryRow(`SELECT observation_enabled FROM kubesentry.admission_event_settings WHERE id = 1`).
		Scan(&observationEnabled); err != nil {
		t.Fatalf("query admission observation default: %v", err)
	}
	if observationEnabled {
		t.Error("admission pipeline observation must default to disabled")
	}

	var platformEnabled bool
	var platformRetentionDays int
	if err := db.QueryRow(
		`SELECT enabled, retention_days FROM kubesentry.platform_observability_settings WHERE id = 1`).
		Scan(&platformEnabled, &platformRetentionDays); err != nil {
		t.Fatalf("query platform observability settings defaults: %v", err)
	}
	if platformEnabled || platformRetentionDays != 1 {
		t.Errorf("platform observability defaults = (%v, %d), want (false, 1)",
			platformEnabled, platformRetentionDays)
	}

	// The ceiling is a CHECK constraint, not just application validation, so a
	// direct DB edit cannot set a retention the cleanup path would not honour.
	if _, err := db.Exec(
		`UPDATE kubesentry.platform_observability_settings SET retention_days = 30 WHERE id = 1`); err == nil {
		t.Error("retention_days above the maximum was accepted by the database")
	}

	// Migrate must be idempotent — re-running against an already-migrated
	// DB (the pre-upgrade hook re-runs it on every helm upgrade, spec D17)
	// must not error.
	if err := dbmigrate.Migrate(dsn); err != nil {
		t.Fatalf("second migrate call: %v", err)
	}
}
