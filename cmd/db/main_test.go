package main

import (
	"context"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

// ping is the primitive the fail-fast fix hinges on: it must succeed once
// Postgres actually accepts connections, and fail (without hanging) when it
// doesn't — that's what lets waitForConnection retry only the "not up yet"
// case and let a real dbmigrate.Migrate error surface immediately.
func TestPingSucceedsAgainstLiveDB(t *testing.T) {
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

	if err := ping(dsn); err != nil {
		t.Fatalf("ping against live db: %v", err)
	}
}

// A connection-refused target (nothing listening) must fail fast, not hang
// — this is the "not reachable yet" case waitForConnection retries.
func TestPingFailsFastWhenUnreachable(t *testing.T) {
	if err := ping("postgres://user:pass@127.0.0.1:1/db?sslmode=disable"); err == nil {
		t.Fatal("expected ping to fail against an unreachable address")
	}
}

// A malformed DSN must fail immediately at ping, not hang or panic.
func TestPingFailsFastOnMalformedDSN(t *testing.T) {
	if err := ping("://not-a-valid-dsn"); err == nil {
		t.Fatal("expected ping to fail on malformed DSN")
	}
}
