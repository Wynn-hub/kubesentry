package console

import (
	"context"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

func TestRevalidateAllPoliciesFlagsBrokenRego(t *testing.T) {
	repos := dbRepos(t)
	ctx := context.Background()

	good := &dbrepo.Policy{Name: "good", Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}", CompileOK: true}
	bad := &dbrepo.Policy{Name: "bad", Rego: "not valid rego {{{", CompileOK: true}
	if err := repos.Policies.Create(ctx, good); err != nil {
		t.Fatalf("create good: %v", err)
	}
	if err := repos.Policies.Create(ctx, bad); err != nil {
		t.Fatalf("create bad: %v", err)
	}

	if err := RevalidateAllPolicies(ctx, repos); err != nil {
		t.Fatalf("revalidate: %v", err)
	}

	gotGood, err := repos.Policies.GetByName(ctx, "good")
	if err != nil {
		t.Fatalf("get good: %v", err)
	}
	if !gotGood.CompileOK {
		t.Error("expected good policy to remain compile_ok=true")
	}
	if gotGood.CompileError != "" {
		t.Errorf("expected empty compile_error for good policy, got %q", gotGood.CompileError)
	}

	gotBad, err := repos.Policies.GetByName(ctx, "bad")
	if err != nil {
		t.Fatalf("get bad: %v", err)
	}
	if gotBad.CompileOK {
		t.Error("expected bad policy to be flagged compile_ok=false")
	}
	if gotBad.CompileError == "" {
		t.Error("expected compile_error to be populated for bad policy")
	}
}

// TestRevalidateAllPoliciesRecoversFixedPolicy proves the pass is
// bidirectional: a row someone hand-edited to compile_ok=false (e.g. a stale
// value from before a manual DB fix) gets corrected back to true once its
// Rego actually compiles, not just flagged one-way toward failure.
func TestRevalidateAllPoliciesRecoversFixedPolicy(t *testing.T) {
	repos := dbRepos(t)
	ctx := context.Background()

	p := &dbrepo.Policy{
		Name:         "fixed",
		Rego:         "package kubesentry\ndeny[msg]{false;msg:=\"x\"}",
		CompileOK:    false,
		CompileError: "stale error from before a manual fix",
	}
	if err := repos.Policies.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := RevalidateAllPolicies(ctx, repos); err != nil {
		t.Fatalf("revalidate: %v", err)
	}

	got, err := repos.Policies.GetByName(ctx, "fixed")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.CompileOK {
		t.Error("expected compile_ok=true after revalidation")
	}
	if got.CompileError != "" {
		t.Errorf("expected compile_error cleared, got %q", got.CompileError)
	}
}

// TestRevalidateAllPoliciesContinuesPastUpdateFailure proves the pass no
// longer fail-fasts on the first DB write error: one row's Update is forced
// to fail via a real Postgres trigger (simulating lock contention / a bad
// row), and every other policy must still be revalidated and persisted, with
// the failing row's name surfaced in the returned aggregate error.
func TestRevalidateAllPoliciesContinuesPastUpdateFailure(t *testing.T) {
	db := newTestDB(t)
	repos := gateway.Repos{Policies: dbrepo.NewPolicyRepo(db)}
	ctx := context.Background()

	victim := &dbrepo.Policy{Name: "victim", Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}", CompileOK: true}
	okPeer := &dbrepo.Policy{Name: "ok-peer", Rego: "package kubesentry\ndeny[msg]{false;msg:=\"x\"}", CompileOK: true}
	badPeer := &dbrepo.Policy{Name: "bad-peer", Rego: "not valid rego {{{", CompileOK: true}
	for _, p := range []*dbrepo.Policy{victim, okPeer, badPeer} {
		if err := repos.Policies.Create(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.Name, err)
		}
	}

	// Force every UPDATE on the "victim" row to fail at the DB level, like a
	// transient write failure (lock contention, one bad row) would.
	installErr := db.Exec(`
		CREATE OR REPLACE FUNCTION kubesentry.fail_victim_update() RETURNS trigger AS $$
		BEGIN
			IF NEW.name = 'victim' THEN
				RAISE EXCEPTION 'simulated write failure for %', NEW.name;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER fail_victim_update BEFORE UPDATE ON kubesentry.policies
		FOR EACH ROW EXECUTE FUNCTION kubesentry.fail_victim_update();
	`).Error
	if installErr != nil {
		t.Fatalf("install failing trigger: %v", installErr)
	}

	err := RevalidateAllPolicies(ctx, repos)
	if err == nil {
		t.Fatal("expected a non-nil aggregate error from the failing update")
	}
	if !strings.Contains(err.Error(), "victim") {
		t.Errorf("expected aggregate error to mention the failing policy name, got: %v", err)
	}

	// The peers must have been revalidated and persisted despite the victim
	// row's write failure — proving the scan did not abort early.
	gotOK, err := repos.Policies.GetByName(ctx, "ok-peer")
	if err != nil {
		t.Fatalf("get ok-peer: %v", err)
	}
	if !gotOK.CompileOK || gotOK.CompileError != "" {
		t.Errorf("expected ok-peer revalidated as compile_ok=true, got ok=%v err=%q", gotOK.CompileOK, gotOK.CompileError)
	}

	gotBad, err := repos.Policies.GetByName(ctx, "bad-peer")
	if err != nil {
		t.Fatalf("get bad-peer: %v", err)
	}
	if gotBad.CompileOK || gotBad.CompileError == "" {
		t.Errorf("expected bad-peer flagged compile_ok=false with an error, got ok=%v err=%q", gotBad.CompileOK, gotBad.CompileError)
	}
}
