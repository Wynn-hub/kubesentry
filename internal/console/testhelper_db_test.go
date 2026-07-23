// internal/console/testhelper_db_test.go
package console

import (
	"context"
	"net/http"
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
// ready-to-use *gorm.DB. Mirrors internal/gateway/testhelper_test.go's
// newTestDB exactly — kept as a separate copy since it's an unexported test
// helper and this package cannot import gateway's internal test file.
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

// dbRepos wraps a fresh newTestDB in the repos the policy/group/exception
// handlers need.
func dbRepos(t *testing.T) gateway.Repos {
	t.Helper()
	db := newTestDB(t)
	return gateway.Repos{
		Clusters:       dbrepo.NewClusterRepo(db),
		Policies:       dbrepo.NewPolicyRepo(db),
		Groups:         dbrepo.NewGroupRepo(db),
		Exceptions:     dbrepo.NewExceptionRepo(db),
		InstallRecords: dbrepo.NewInstallRecordRepo(db),
		Distributions:  dbrepo.NewDistributionRepo(db),
		UninstallTasks: dbrepo.NewUninstallTaskRepo(db),
	}
}

// newDBTestServer builds a console server backed by a real (testcontainers)
// Postgres via gateway.Repos, for the GORM-backed policy/group/exception
// handler tests. Distinct from newTestServer (handlers_test.go), which still
// backs the not-yet-converted summary/schema handlers with a fake
// controller-runtime client.
func newDBTestServer(t *testing.T) (http.Handler, gateway.Repos) {
	t.Helper()
	repos := dbRepos(t)
	h := &Handlers{Repos: repos}
	srv := NewServer(h, func() bool { return true })
	return srv.Handler, repos
}

// newDBTestServerWithHandlers is like newDBTestServer but also returns the
// *Handlers, so a test can set h.Gateway (e.g. to a fakeHubClient) before
// issuing requests — used to verify NotifyChange best-effort wiring (Task
// 11) without needing a real gateway.
func newDBTestServerWithHandlers(t *testing.T) (http.Handler, gateway.Repos, *Handlers) {
	t.Helper()
	repos := dbRepos(t)
	h := &Handlers{Repos: repos}
	srv := NewServer(h, func() bool { return true })
	return srv.Handler, repos, h
}
