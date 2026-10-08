package repository

import (
	"context"
	"database/sql"
	"fmt"
)

const policyReferenceLockID int64 = 0x4b53454e545259

func withPolicyReferenceLock(ctx context.Context, dbProvider interface{ DB() (*sql.DB, error) }, fn func() error) error {
	db, err := dbProvider.DB()
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", policyReferenceLockID); err != nil {
		return fmt.Errorf("lock policy references: %w", err)
	}
	// ponytail: one global lock keeps selector-based references correct; partition
	// it only if policy/group write contention becomes measurable.
	defer conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", policyReferenceLockID)
	return fn()
}

func (r *PolicyRepo) WithPolicyReferenceLock(ctx context.Context, fn func() error) error {
	return withPolicyReferenceLock(ctx, r.db, fn)
}

func (r *GroupRepo) WithPolicyReferenceLock(ctx context.Context, fn func() error) error {
	return withPolicyReferenceLock(ctx, r.db, fn)
}
