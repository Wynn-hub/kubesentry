package dbmodel

import "testing"

func TestBaseBeforeCreateAssignsID(t *testing.T) {
	if err := Init(1); err != nil {
		t.Fatalf("init: %v", err)
	}
	b := &Base{}
	if err := b.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if b.ID == 0 {
		t.Fatalf("expected BeforeCreate to assign a non-zero snowflake ID")
	}
}

func TestBaseBeforeCreateDoesNotOverwriteExistingID(t *testing.T) {
	if err := Init(1); err != nil {
		t.Fatalf("init: %v", err)
	}
	b := &Base{ID: 42}
	if err := b.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if b.ID != 42 {
		t.Fatalf("expected existing ID 42 to be preserved, got %d", b.ID)
	}
}
