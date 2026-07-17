package dbmodel

import (
	"testing"
)

func TestNewIDPanicsWhenNotInitialized(t *testing.T) {
	// Save the current node state
	savedNode := node
	defer func() {
		node = savedNode
	}()

	// Set node to nil and verify panic
	node = nil
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when NewID called before Init, but got none")
		} else if msg, ok := r.(string); !ok || msg != "dbmodel: Init must be called before NewID" {
			t.Fatalf("expected panic message 'dbmodel: Init must be called before NewID', got %v", r)
		}
	}()
	NewID()
}

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
