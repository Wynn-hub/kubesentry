package hublink

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
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

// newTestDB starts a throwaway PG container, runs migrations, and returns a
// ready-to-use *gorm.DB. Mirrors internal/dbrepo/testhelper_test.go's
// newTestDB — this package's client_test.go spins up a real gateway.Server
// (the hub side of these end-to-end tests) which now needs real repos
// instead of a fake controller-runtime client.
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

// newTestRepos wraps a fresh newTestDB in the four repos gateway.Server needs.
func newTestRepos(t *testing.T) gateway.Repos {
	t.Helper()
	db := newTestDB(t)
	return gateway.Repos{
		Clusters:      dbrepo.NewClusterRepo(db),
		Policies:      dbrepo.NewPolicyRepo(db),
		Groups:        dbrepo.NewGroupRepo(db),
		Distributions: dbrepo.NewDistributionRepo(db),
	}
}
