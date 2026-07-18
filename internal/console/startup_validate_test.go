package console

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
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
