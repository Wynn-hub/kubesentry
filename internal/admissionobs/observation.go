package admissionobs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

type Component string

const (
	ComponentWebhook  Component = "webhook"
	ComponentOperator Component = "operator"
	ComponentGateway  Component = "gateway"
	ComponentCleanup  Component = "cleanup"
)

func (c Component) ValidMember() bool {
	return c == ComponentWebhook || c == ComponentOperator
}

func (c Component) ValidBucket() bool {
	return c.ValidMember() || c == ComponentGateway || c == ComponentCleanup
}

type ReasonClass string

const (
	ReasonFault    ReasonClass = "fault"
	ReasonPressure ReasonClass = "pressure"
	ReasonFilter   ReasonClass = "filter"
)

const (
	ReasonUnknown = "unknown"

	ReasonWebhookQueueFull     = "webhook_queue_full"
	ReasonMarshalFailed        = "marshal_failed"
	ReasonRequestBuildFailed   = "request_build_failed"
	ReasonPostFailed           = "post_failed"
	ReasonACKReadFailed        = "ack_read_failed"
	ReasonACKInvalid           = "ack_invalid"
	ReasonHTTP4xx              = "http_4xx"
	ReasonHTTP5xx              = "http_5xx"
	ReasonRetryExhausted       = "retry_exhausted"
	ReasonObservationQueueFull = "observation_queue_full"

	ReasonGatewayACKTimeout = "gateway_ack_timeout"
	ReasonSendFailed        = "send_failed"
	ReasonSyncStreamClosed  = "sync_stream_closed"
	ReasonAuthFailed        = "auth_failed"
	ReasonInvalidReport     = "invalid_report"
	ReasonNotLeader         = "not_leader"

	ReasonInvalidEvent           = "invalid_event"
	ReasonDBWriteFailed          = "db_write_failed"
	ReasonRegisterAuthFailed     = "register_auth_failed"
	ReasonSyncAuthFailed         = "sync_auth_failed"
	ReasonObservationWriteFailed = "observation_write_failed"

	ReasonOperatorEventQueueFull = "operator_event_queue_full"
	ReasonOperatorRetryable      = "operator_retryable"

	ReasonStoragePolicyNoRetain = "storage_policy_no_retain"
	ReasonStoragePolicyNoMatch  = "storage_policy_no_match"
	ReasonRetentionElapsed      = "retention_already_elapsed"
)

var (
	faultReasons = []string{
		ReasonWebhookQueueFull,
		ReasonMarshalFailed,
		ReasonRequestBuildFailed,
		ReasonPostFailed,
		ReasonACKReadFailed,
		ReasonACKInvalid,
		ReasonHTTP4xx,
		ReasonHTTP5xx,
		ReasonRetryExhausted,
		ReasonObservationQueueFull,
		ReasonGatewayACKTimeout,
		ReasonSendFailed,
		ReasonSyncStreamClosed,
		ReasonAuthFailed,
		ReasonInvalidReport,
		ReasonNotLeader,
		ReasonInvalidEvent,
		ReasonDBWriteFailed,
		ReasonRegisterAuthFailed,
		ReasonSyncAuthFailed,
		ReasonObservationWriteFailed,
		ReasonUnknown,
	}
	pressureReasons = []string{
		ReasonOperatorEventQueueFull,
		ReasonOperatorRetryable,
		ReasonUnknown,
	}
	filterReasons = []string{
		ReasonStoragePolicyNoRetain,
		ReasonStoragePolicyNoMatch,
		ReasonRetentionElapsed,
		ReasonUnknown,
	}
	faultReasonIndex    = indexReasons(faultReasons)
	pressureReasonIndex = indexReasons(pressureReasons)
	filterReasonIndex   = indexReasons(filterReasons)
)

func indexReasons(reasons []string) map[string]int {
	result := make(map[string]int, len(reasons))
	for index, reason := range reasons {
		result[reason] = index
	}
	return result
}

func NormalizeReason(class ReasonClass, reason string) string {
	var index map[string]int
	switch class {
	case ReasonFault:
		index = faultReasonIndex
	case ReasonPressure:
		index = pressureReasonIndex
	case ReasonFilter:
		index = filterReasonIndex
	default:
		return ReasonUnknown
	}
	if _, ok := index[reason]; ok {
		return reason
	}
	return ReasonUnknown
}

func ValidReason(class ReasonClass, reason string) bool {
	return NormalizeReason(class, reason) == reason
}

func HTTPFailureReason(status int) string {
	if status >= 400 && status < 500 {
		return ReasonHTTP4xx
	}
	if status >= 500 && status < 600 {
		return ReasonHTTP5xx
	}
	return ReasonUnknown
}

type Counters struct {
	Generated        int64 `json:"generated"`
	Received         int64 `json:"received"`
	Forwarded        int64 `json:"forwarded"`
	Written          int64 `json:"written"`
	Duplicate        int64 `json:"duplicate"`
	PolicyFiltered   int64 `json:"policyFiltered"`
	RetentionElapsed int64 `json:"retentionElapsed"`
	QueueRejected    int64 `json:"queueRejected"`
	Retry            int64 `json:"retry"`
	Dropped          int64 `json:"dropped"`
	RetryExhausted   int64 `json:"retryExhausted"`
	BatchFailure     int64 `json:"batchFailure"`
	AuthFailure      int64 `json:"authFailure"`
	ACKTimeout       int64 `json:"ackTimeout"`
	ReportDropped    int64 `json:"reportDropped"`
	CleanupRun       int64 `json:"cleanupRun"`
	CleanupDeleted   int64 `json:"cleanupDeleted"`
	CleanupFailure   int64 `json:"cleanupFailure"`
}

func (c Counters) Valid() bool {
	return c.Generated >= 0 &&
		c.Received >= 0 &&
		c.Forwarded >= 0 &&
		c.Written >= 0 &&
		c.Duplicate >= 0 &&
		c.PolicyFiltered >= 0 &&
		c.RetentionElapsed >= 0 &&
		c.QueueRejected >= 0 &&
		c.Retry >= 0 &&
		c.Dropped >= 0 &&
		c.RetryExhausted >= 0 &&
		c.BatchFailure >= 0 &&
		c.AuthFailure >= 0 &&
		c.ACKTimeout >= 0 &&
		c.ReportDropped >= 0 &&
		c.CleanupRun >= 0 &&
		c.CleanupDeleted >= 0 &&
		c.CleanupFailure >= 0
}

func (c Counters) Add(other Counters) Counters {
	return Counters{
		Generated:        c.Generated + other.Generated,
		Received:         c.Received + other.Received,
		Forwarded:        c.Forwarded + other.Forwarded,
		Written:          c.Written + other.Written,
		Duplicate:        c.Duplicate + other.Duplicate,
		PolicyFiltered:   c.PolicyFiltered + other.PolicyFiltered,
		RetentionElapsed: c.RetentionElapsed + other.RetentionElapsed,
		QueueRejected:    c.QueueRejected + other.QueueRejected,
		Retry:            c.Retry + other.Retry,
		Dropped:          c.Dropped + other.Dropped,
		RetryExhausted:   c.RetryExhausted + other.RetryExhausted,
		BatchFailure:     c.BatchFailure + other.BatchFailure,
		AuthFailure:      c.AuthFailure + other.AuthFailure,
		ACKTimeout:       c.ACKTimeout + other.ACKTimeout,
		ReportDropped:    c.ReportDropped + other.ReportDropped,
		CleanupRun:       c.CleanupRun + other.CleanupRun,
		CleanupDeleted:   c.CleanupDeleted + other.CleanupDeleted,
		CleanupFailure:   c.CleanupFailure + other.CleanupFailure,
	}
}

func (c Counters) Delta(previous Counters) (Counters, error) {
	delta := Counters{
		Generated:        c.Generated - previous.Generated,
		Received:         c.Received - previous.Received,
		Forwarded:        c.Forwarded - previous.Forwarded,
		Written:          c.Written - previous.Written,
		Duplicate:        c.Duplicate - previous.Duplicate,
		PolicyFiltered:   c.PolicyFiltered - previous.PolicyFiltered,
		RetentionElapsed: c.RetentionElapsed - previous.RetentionElapsed,
		QueueRejected:    c.QueueRejected - previous.QueueRejected,
		Retry:            c.Retry - previous.Retry,
		Dropped:          c.Dropped - previous.Dropped,
		RetryExhausted:   c.RetryExhausted - previous.RetryExhausted,
		BatchFailure:     c.BatchFailure - previous.BatchFailure,
		AuthFailure:      c.AuthFailure - previous.AuthFailure,
		ACKTimeout:       c.ACKTimeout - previous.ACKTimeout,
		ReportDropped:    c.ReportDropped - previous.ReportDropped,
		CleanupRun:       c.CleanupRun - previous.CleanupRun,
		CleanupDeleted:   c.CleanupDeleted - previous.CleanupDeleted,
		CleanupFailure:   c.CleanupFailure - previous.CleanupFailure,
	}
	if !delta.Valid() {
		return Counters{}, errors.New("counter snapshot moved backwards")
	}
	return delta, nil
}

type Snapshot struct {
	Counters             Counters         `json:"counters"`
	FaultReasonCounts    map[string]int64 `json:"faultReasonCounts"`
	PressureReasonCounts map[string]int64 `json:"pressureReasonCounts"`
	FilterReasonCounts   map[string]int64 `json:"filterReasonCounts"`
}

func (s Snapshot) Valid() bool {
	return s.Counters.Valid() &&
		validReasonCounts(ReasonFault, s.FaultReasonCounts) &&
		validReasonCounts(ReasonPressure, s.PressureReasonCounts) &&
		validReasonCounts(ReasonFilter, s.FilterReasonCounts)
}

func validReasonCounts(class ReasonClass, counts map[string]int64) bool {
	for reason, count := range counts {
		if count < 0 || !ValidReason(class, reason) {
			return false
		}
	}
	return true
}

func NormalizeSnapshot(snapshot Snapshot) Snapshot {
	snapshot.FaultReasonCounts = normalizeReasonCounts(ReasonFault, snapshot.FaultReasonCounts)
	snapshot.PressureReasonCounts = normalizeReasonCounts(ReasonPressure, snapshot.PressureReasonCounts)
	snapshot.FilterReasonCounts = normalizeReasonCounts(ReasonFilter, snapshot.FilterReasonCounts)
	return snapshot
}

func normalizeReasonCounts(class ReasonClass, counts map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(counts))
	for reason, count := range counts {
		result[NormalizeReason(class, reason)] += count
	}
	return result
}

func (s Snapshot) Delta(previous Snapshot) (Snapshot, error) {
	counters, err := s.Counters.Delta(previous.Counters)
	if err != nil {
		return Snapshot{}, err
	}
	fault, err := deltaReasonCounts(s.FaultReasonCounts, previous.FaultReasonCounts)
	if err != nil {
		return Snapshot{}, err
	}
	pressure, err := deltaReasonCounts(s.PressureReasonCounts, previous.PressureReasonCounts)
	if err != nil {
		return Snapshot{}, err
	}
	filter, err := deltaReasonCounts(s.FilterReasonCounts, previous.FilterReasonCounts)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Counters: counters, FaultReasonCounts: fault, PressureReasonCounts: pressure, FilterReasonCounts: filter}, nil
}

func deltaReasonCounts(current, previous map[string]int64) (map[string]int64, error) {
	result := make(map[string]int64, len(current))
	keys := make(map[string]struct{}, len(current)+len(previous))
	for key := range current {
		keys[key] = struct{}{}
	}
	for key := range previous {
		keys[key] = struct{}{}
	}
	for key := range keys {
		delta := current[key] - previous[key]
		if delta < 0 {
			return nil, fmt.Errorf("reason %q moved backwards", key)
		}
		if delta > 0 {
			result[key] = delta
		}
	}
	return result, nil
}

type AtomicCounters struct {
	generated        atomic.Int64
	received         atomic.Int64
	forwarded        atomic.Int64
	written          atomic.Int64
	duplicate        atomic.Int64
	policyFiltered   atomic.Int64
	retentionElapsed atomic.Int64
	queueRejected    atomic.Int64
	retry            atomic.Int64
	dropped          atomic.Int64
	retryExhausted   atomic.Int64
	batchFailure     atomic.Int64
	authFailure      atomic.Int64
	ackTimeout       atomic.Int64
	reportDropped    atomic.Int64
	cleanupRun       atomic.Int64
	cleanupDeleted   atomic.Int64
	cleanupFailure   atomic.Int64
	faultReasons     []atomic.Int64
	pressureReasons  []atomic.Int64
	filterReasons    []atomic.Int64
}

func NewAtomicCounters() *AtomicCounters {
	return &AtomicCounters{
		faultReasons:    make([]atomic.Int64, len(faultReasons)),
		pressureReasons: make([]atomic.Int64, len(pressureReasons)),
		filterReasons:   make([]atomic.Int64, len(filterReasons)),
	}
}

func (c *AtomicCounters) Add(delta Counters) {
	c.generated.Add(delta.Generated)
	c.received.Add(delta.Received)
	c.forwarded.Add(delta.Forwarded)
	c.written.Add(delta.Written)
	c.duplicate.Add(delta.Duplicate)
	c.policyFiltered.Add(delta.PolicyFiltered)
	c.retentionElapsed.Add(delta.RetentionElapsed)
	c.queueRejected.Add(delta.QueueRejected)
	c.retry.Add(delta.Retry)
	c.dropped.Add(delta.Dropped)
	c.retryExhausted.Add(delta.RetryExhausted)
	c.batchFailure.Add(delta.BatchFailure)
	c.authFailure.Add(delta.AuthFailure)
	c.ackTimeout.Add(delta.ACKTimeout)
	c.reportDropped.Add(delta.ReportDropped)
	c.cleanupRun.Add(delta.CleanupRun)
	c.cleanupDeleted.Add(delta.CleanupDeleted)
	c.cleanupFailure.Add(delta.CleanupFailure)
}

func (c *AtomicCounters) AddReason(class ReasonClass, reason string, delta int64) {
	if delta <= 0 {
		return
	}
	var values []atomic.Int64
	var index map[string]int
	switch class {
	case ReasonFault:
		values, index = c.faultReasons, faultReasonIndex
	case ReasonPressure:
		values, index = c.pressureReasons, pressureReasonIndex
	case ReasonFilter:
		values, index = c.filterReasons, filterReasonIndex
	default:
		return
	}
	position, ok := index[reason]
	if !ok {
		position = index[ReasonUnknown]
	}
	values[position].Add(delta)
}

func (c *AtomicCounters) Snapshot() Snapshot {
	return Snapshot{
		Counters: Counters{
			Generated:        c.generated.Load(),
			Received:         c.received.Load(),
			Forwarded:        c.forwarded.Load(),
			Written:          c.written.Load(),
			Duplicate:        c.duplicate.Load(),
			PolicyFiltered:   c.policyFiltered.Load(),
			RetentionElapsed: c.retentionElapsed.Load(),
			QueueRejected:    c.queueRejected.Load(),
			Retry:            c.retry.Load(),
			Dropped:          c.dropped.Load(),
			RetryExhausted:   c.retryExhausted.Load(),
			BatchFailure:     c.batchFailure.Load(),
			AuthFailure:      c.authFailure.Load(),
			ACKTimeout:       c.ackTimeout.Load(),
			ReportDropped:    c.reportDropped.Load(),
			CleanupRun:       c.cleanupRun.Load(),
			CleanupDeleted:   c.cleanupDeleted.Load(),
			CleanupFailure:   c.cleanupFailure.Load(),
		},
		FaultReasonCounts:    snapshotReasons(faultReasons, c.faultReasons),
		PressureReasonCounts: snapshotReasons(pressureReasons, c.pressureReasons),
		FilterReasonCounts:   snapshotReasons(filterReasons, c.filterReasons),
	}
}

func snapshotReasons(reasons []string, values []atomic.Int64) map[string]int64 {
	result := make(map[string]int64)
	for index, reason := range reasons {
		if value := values[index].Load(); value > 0 {
			result[reason] = value
		}
	}
	return result
}

type GaugeSnapshot struct {
	DepthPeak int64
	Capacity  int64
}

type QueueGauge struct {
	depth    atomic.Int64
	peak     atomic.Int64
	capacity atomic.Int64
}

func (g *QueueGauge) Observe(depth, capacity int64) {
	if depth < 0 {
		depth = 0
	}
	if capacity < 0 {
		capacity = 0
	}
	g.depth.Store(depth)
	g.capacity.Store(capacity)
	for current := g.peak.Load(); depth > current; current = g.peak.Load() {
		if g.peak.CompareAndSwap(current, depth) {
			break
		}
	}
}

func (g *QueueGauge) SnapshotAndReset() GaugeSnapshot {
	for {
		peak := g.peak.Load()
		snapshot := GaugeSnapshot{DepthPeak: peak, Capacity: g.capacity.Load()}
		if g.peak.CompareAndSwap(peak, g.depth.Load()) {
			return snapshot
		}
	}
}

func NewSeriesID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func ReportID(seriesID string, sequence uint64) string {
	return fmt.Sprintf("%s-%d", seriesID, sequence)
}

type Report struct {
	ReportID      string
	SeriesID      string
	Sequence      uint64
	ObservedAt    time.Time
	Component     Component
	Snapshot      Snapshot
	QueueDepthMax int64
	QueueCapacity int64
}

func (r Report) Validate() error {
	if r.ReportID == "" || r.SeriesID == "" || r.Sequence == 0 {
		return errors.New("report identity is required")
	}
	if !r.Component.ValidMember() {
		return fmt.Errorf("invalid member component %q", r.Component)
	}
	if r.ObservedAt.IsZero() {
		return errors.New("observed_at is required")
	}
	if r.QueueDepthMax < 0 || r.QueueCapacity < 0 {
		return errors.New("queue gauges must be non-negative")
	}
	if !r.Snapshot.Valid() {
		return errors.New("invalid counter snapshot")
	}
	return nil
}

func SortedReasonKeys(counts map[string]int64) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
