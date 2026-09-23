package gateway

import (
	"context"
	"testing"
	"time"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// A storage-rule change must kick a full cleanup pass. Console reaches the
// gateway only through NotifyChange, so if this wiring breaks, a narrowed rule
// set silently waits out the daily ticker before dropping the events it now
// excludes.
func TestNotifyChangeTriggersFullAdmissionCleanup(t *testing.T) {
	cleaner := NewAdmissionCleaner(&dbrepo.AdmissionRepo{}, time.Hour)
	srv := NewServer(Repos{}, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	srv.SetAdmissionCleaner(cleaner)

	if _, err := srv.NotifyChange(context.Background(), &gatewaypb.NotifyChangeRequest{Topic: "policy"}); err != nil {
		t.Fatalf("notify policy change: %v", err)
	}
	if len(cleaner.full) != 0 {
		t.Fatalf("unrelated topic queued a cleanup pass")
	}

	req := &gatewaypb.NotifyChangeRequest{Topic: gatewaypb.AdmissionEventSettingsTopic}
	if _, err := srv.NotifyChange(context.Background(), req); err != nil {
		t.Fatalf("notify settings change: %v", err)
	}
	if len(cleaner.full) != 1 {
		t.Fatalf("settings change did not queue a cleanup pass")
	}

	// Repeated saves coalesce rather than block the gRPC handler on a full
	// trigger channel.
	if _, err := srv.NotifyChange(context.Background(), req); err != nil {
		t.Fatalf("second notify: %v", err)
	}
	if len(cleaner.full) != 1 {
		t.Fatalf("full = %d pending passes, want 1 (coalesced)", len(cleaner.full))
	}
}

// The gateway runs a cleaner only on the elected leader, and tests construct
// servers without one — NotifyChange must stay a no-op rather than panic.
func TestNotifyChangeToleratesMissingAdmissionCleaner(t *testing.T) {
	srv := NewServer(Repos{}, gatewayNS, NewHeartbeatTracker(), NewNotifier(), "test-token")
	req := &gatewaypb.NotifyChangeRequest{Topic: gatewaypb.AdmissionEventSettingsTopic}
	if _, err := srv.NotifyChange(context.Background(), req); err != nil {
		t.Fatalf("notify without cleaner: %v", err)
	}
}
