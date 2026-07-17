package dbmodel

import "testing"

func TestNewIDUnique(t *testing.T) {
	if err := Init(1); err != nil {
		t.Fatalf("init: %v", err)
	}
	a := NewID()
	b := NewID()
	if a == b {
		t.Fatalf("expected unique ids, got %d twice", a)
	}
	if a == 0 || b == 0 {
		t.Fatalf("expected non-zero ids, got %d and %d", a, b)
	}
}

func TestNodeIDFromPodNameDeterministic(t *testing.T) {
	a := NodeIDFromPodName("gateway-abc123")
	b := NodeIDFromPodName("gateway-abc123")
	if a != b {
		t.Fatalf("expected deterministic hash, got %d and %d", a, b)
	}
	if NodeIDFromPodName("") != 0 {
		t.Fatalf("expected 0 for empty pod name")
	}
	if NodeIDFromPodName("gateway-abc123") == NodeIDFromPodName("gateway-def456") {
		t.Fatalf("expected different pod names to (usually) hash differently")
	}
}
