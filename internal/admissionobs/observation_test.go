package admissionobs

import (
	"testing"
)

func TestAtomicCountersNormalizeUnknownReasons(t *testing.T) {
	counters := NewAtomicCounters()
	counters.Add(Counters{Generated: 2, Dropped: 1})
	counters.AddReason(ReasonFault, "future_fault", 3)
	counters.AddReason(ReasonPressure, "future_pressure", 4)
	counters.AddReason(ReasonFilter, "future_filter", 5)

	snapshot := counters.Snapshot()
	if snapshot.Counters.Generated != 2 || snapshot.Counters.Dropped != 1 {
		t.Fatalf("unexpected counters: %+v", snapshot.Counters)
	}
	if snapshot.FaultReasonCounts[ReasonUnknown] != 3 {
		t.Fatalf("fault unknown = %d", snapshot.FaultReasonCounts[ReasonUnknown])
	}
	if snapshot.PressureReasonCounts[ReasonUnknown] != 4 {
		t.Fatalf("pressure unknown = %d", snapshot.PressureReasonCounts[ReasonUnknown])
	}
	if snapshot.FilterReasonCounts[ReasonUnknown] != 5 {
		t.Fatalf("filter unknown = %d", snapshot.FilterReasonCounts[ReasonUnknown])
	}
}

func TestSnapshotDelta(t *testing.T) {
	previous := Snapshot{
		Counters:             Counters{Generated: 2, Retry: 1},
		FaultReasonCounts:    map[string]int64{ReasonPostFailed: 1},
		PressureReasonCounts: map[string]int64{},
		FilterReasonCounts:   map[string]int64{},
	}
	current := Snapshot{
		Counters:             Counters{Generated: 7, Retry: 3},
		FaultReasonCounts:    map[string]int64{ReasonPostFailed: 2},
		PressureReasonCounts: map[string]int64{ReasonOperatorRetryable: 4},
		FilterReasonCounts:   map[string]int64{},
	}
	delta, err := current.Delta(previous)
	if err != nil {
		t.Fatal(err)
	}
	if delta.Counters.Generated != 5 || delta.Counters.Retry != 2 {
		t.Fatalf("unexpected delta: %+v", delta.Counters)
	}
	if delta.FaultReasonCounts[ReasonPostFailed] != 1 {
		t.Fatalf("post_failed delta = %d", delta.FaultReasonCounts[ReasonPostFailed])
	}
	if delta.PressureReasonCounts[ReasonOperatorRetryable] != 4 {
		t.Fatalf("operator_retryable delta = %d", delta.PressureReasonCounts[ReasonOperatorRetryable])
	}
}

func TestSnapshotDeltaRejectsRollback(t *testing.T) {
	_, err := (Snapshot{
		Counters:             Counters{Generated: 1},
		FaultReasonCounts:    map[string]int64{},
		PressureReasonCounts: map[string]int64{},
		FilterReasonCounts:   map[string]int64{},
	}).Delta(Snapshot{
		Counters:             Counters{Generated: 2},
		FaultReasonCounts:    map[string]int64{},
		PressureReasonCounts: map[string]int64{},
		FilterReasonCounts:   map[string]int64{},
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
}

func TestQueueGaugeSnapshotResetsPeakToCurrentDepth(t *testing.T) {
	var gauge QueueGauge
	gauge.Observe(9, 256)
	gauge.Observe(2, 256)
	if got := gauge.SnapshotAndReset(); got.DepthPeak != 9 || got.Capacity != 256 {
		t.Fatalf("snapshot = %#v, want peak 9 and capacity 256", got)
	}
	if got := gauge.SnapshotAndReset().DepthPeak; got != 2 {
		t.Fatalf("peak = %d, want current depth 2", got)
	}
}

func TestQueueGaugeNewPeakAppearsInNextSnapshot(t *testing.T) {
	var gauge QueueGauge
	gauge.Observe(4, 256)
	if got := gauge.SnapshotAndReset().DepthPeak; got != 4 {
		t.Fatalf("first peak = %d, want 4", got)
	}
	gauge.Observe(9, 256)
	if got := gauge.SnapshotAndReset().DepthPeak; got != 9 {
		t.Fatalf("next peak = %d, want 9", got)
	}
}

func TestHTTPFailureReasonIsBounded(t *testing.T) {
	for status, want := range map[int]string{
		400: ReasonHTTP4xx,
		429: ReasonHTTP4xx,
		500: ReasonHTTP5xx,
		503: ReasonHTTP5xx,
		302: ReasonUnknown,
	} {
		if got := HTTPFailureReason(status); got != want {
			t.Fatalf("status %d: got %q, want %q", status, got, want)
		}
	}
}

func TestNewSeriesIDHasSixteenBytesOfEntropy(t *testing.T) {
	first, err := NewSeriesID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewSeriesID()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || first == second {
		t.Fatalf("unexpected series IDs %q and %q", first, second)
	}
}
