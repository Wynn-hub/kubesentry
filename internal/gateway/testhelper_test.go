package gateway

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/dbmigrate"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// gatewayNS is the namespace used across gateway package tests for Server
// construction; shared here since deleting cluster_reconciler_test.go (its
// original home) orphaned every other test file that referenced it.
const gatewayNS = "kubesentry-gateway-system"

// newTestRepos wraps a fresh newTestDB in the four repos the Server needs.
func newTestRepos(t *testing.T) Repos {
	t.Helper()
	db := newTestDB(t)
	return Repos{
		Clusters:       dbrepo.NewClusterRepo(db),
		Policies:       dbrepo.NewPolicyRepo(db),
		Groups:         dbrepo.NewGroupRepo(db),
		Distributions:  dbrepo.NewDistributionRepo(db),
		UninstallTasks: dbrepo.NewUninstallTaskRepo(db),
		Admission:      dbrepo.NewAdmissionRepo(db),
		Observability:  dbrepo.NewObservabilityRepo(db),
		Platform:       dbrepo.NewPlatformObservabilityRepo(db),
	}
}

// newTestDB starts a throwaway PG container, runs migrations, and returns a
// ready-to-use *gorm.DB. Mirrors internal/dbrepo/testhelper_test.go's
// newTestDB exactly — kept as a separate copy since it's unexported test
// helper and this package cannot import dbrepo_test.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	if err := dbmodel.Init(1); err != nil {
		t.Fatalf("dbmodel.Init: %v", err)
	}
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return db
}

// enableAdmissionObservation turns on the pipeline funnel switch, which lives
// with the admission event settings it instruments.
func enableAdmissionObservation(ctx context.Context, repos Repos) error {
	settings, err := repos.Admission.GetSettings(ctx)
	if err != nil {
		return err
	}
	settings.ObservationEnabled = true
	return repos.Admission.UpdateSettingsAndPolicies(ctx, settings, nil)
}
