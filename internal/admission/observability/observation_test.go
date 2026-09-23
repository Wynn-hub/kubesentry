package observability

import (
	"sync"
	"testing"
	"time"
)

func TestTrackerAggregatesClosesAndAcknowledgesByRevision(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	tracker := NewTracker(ComponentWebhook, "webhook-a")
	tracker.Record(ReasonQueueFull, 2, "event-a", "full", now)
	tracker.Record(ReasonQueueFull, 3, "event-b", "still full", now.Add(30*time.Second))

	open := tracker.Snapshot(now.Add(30 * time.Second))
	if len(open) != 1 || open[0].AffectedCount != 5 || open[0].Closed || open[0].Revision != 2 {
		t.Fatalf("open incident = %#v", open)
	}
	closed := tracker.Snapshot(now.Add(91 * time.Second))
	if len(closed) != 1 || !closed[0].Closed || closed[0].Revision != 3 {
		t.Fatalf("closed incident = %#v", closed)
	}
	tracker.Acknowledge(closed[0].IncidentKey, 2, true)
	if tracker.PendingCount() != 1 {
		t.Fatal("stale acknowledgement removed a newer revision")
	}
	tracker.Acknowledge(closed[0].IncidentKey, 3, true)
	if tracker.PendingCount() != 0 {
		t.Fatal("terminal acknowledgement did not remove closed incident")
	}
}

func TestTrackerUsesRandomKeysForSameSecondAndCapsSamples(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	first := NewTracker(ComponentWebhook, "replica-a")
	second := NewTracker(ComponentWebhook, "replica-b")
	for index, sample := range []string{"a", "b", "c", "d", "e", "f", "a"} {
		first.Record(ReasonQueueFull, 1, sample, "", now.Add(time.Duration(index)*time.Millisecond))
	}
	second.Record(ReasonQueueFull, 1, "z", "", now)
	left, right := first.Snapshot(now), second.Snapshot(now)
	if left[0].IncidentKey == right[0].IncidentKey {
		t.Fatal("same-second replicas generated the same incident key")
	}
	if len(left[0].SampleEventIDs) != MaxSampleEventIDs {
		t.Fatalf("samples = %v", left[0].SampleEventIDs)
	}
}

func TestTrackerPendingCapacityRecordsNonRecursiveDrop(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	tracker := NewTracker(ComponentOperator, "operator-a")
	tracker.SetPendingLimitForTest(1)
	tracker.Record(ReasonQueueFull, 1, "", "", now)
	tracker.Snapshot(now.Add(61 * time.Second))
	tracker.Record(ReasonACKTimeout, 1, "", "", now.Add(62*time.Second))
	items := tracker.Snapshot(now.Add(123 * time.Second))
	if tracker.PendingCount() != 1 {
		t.Fatalf("pending = %d, want bounded at one", tracker.PendingCount())
	}
	foundDrop := false
	for _, item := range items {
		foundDrop = foundDrop || item.Reason == ReasonReportDropped
	}
	if !foundDrop {
		t.Fatal("pending overflow did not record report_dropped")
	}
}

// A drop happens when the snapshot evicts, so it must be stamped with the
// snapshot time. Stamping it with the closing incident's older last_seen made it
// born already past the silence period, so the very next scan closed it and the
// overflow after that minted a second incident_key instead of aggregating.
func TestTrackerOverflowDropIsStampedAtSnapshotTime(t *testing.T) {
	now := time.Date(2026, 8, 3, 1, 0, 0, 0, time.UTC)
	tracker := NewTracker(ComponentOperator, "operator-a")
	tracker.SetPendingLimitForTest(1)
	tracker.Record(ReasonQueueFull, 1, "", "", now)
	tracker.Snapshot(now.Add(61 * time.Second))
	tracker.Record(ReasonACKTimeout, 1, "", "", now.Add(2*time.Minute))

	// This scan closes ack_timeout, which overflows pending and drops queue_full.
	evictedAt := now.Add(2*time.Minute + 61*time.Second)
	var drop *Incident
	for _, item := range tracker.Snapshot(evictedAt) {
		if item.Reason == ReasonReportDropped {
			drop = &item
		}
	}
	if drop == nil {
		t.Fatal("pending overflow did not record report_dropped")
	}
	if !drop.StartedAt.Equal(evictedAt) || !drop.LastSeenAt.Equal(evictedAt) {
		t.Fatalf("drop stamped %s/%s, want the snapshot time %s", drop.StartedAt, drop.LastSeenAt, evictedAt)
	}
	if drop.Closed {
		t.Fatal("drop was born already closed; it can never aggregate a later overflow")
	}
}

func TestTrackerConcurrentRecord(t *testing.T) {
	tracker := NewTracker(ComponentWebhook, "webhook-a")
	now := time.Now().UTC()
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Record(ReasonQueueFull, 1, "event", "full", now)
		}()
	}
	wg.Wait()
	items := tracker.Snapshot(now)
	if len(items) != 1 || items[0].AffectedCount != 100 {
		t.Fatalf("incident = %#v", items)
	}
}

func TestMergeSampleEventIDsUsesStableUnion(t *testing.T) {
	got := MergeSampleEventIDs([]string{"a", "b"}, []string{"b", "c", "d", "e", "f"})
	want := []string{"a", "b", "c", "d", "e"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("samples = %v", got)
		}
	}
}
