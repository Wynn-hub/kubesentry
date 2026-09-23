package observability

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultSilencePeriod = 60 * time.Second
	ActiveFreshness      = 2 * time.Minute
	DefaultPendingLimit  = 256
	MaxBatchSize         = 256
	MaxSampleEventIDs    = 5
	MaxDetailLength      = 200
	MaxIdentityLength    = 128
)

type Component string

const (
	ComponentWebhook  Component = "webhook"
	ComponentOperator Component = "operator"
	ComponentGateway  Component = "gateway"
)

func (c Component) Valid() bool {
	return c == ComponentWebhook || c == ComponentOperator || c == ComponentGateway
}

func (c Component) ValidMember() bool {
	return c == ComponentWebhook || c == ComponentOperator
}

const (
	ReasonQueueFull      = "queue_full"
	ReasonRetryExhausted = "retry_exhausted"
	ReasonACKTimeout     = "ack_timeout"
	ReasonAuthFailure    = "auth_failure"
	ReasonBatchFailure   = "batch_failure"
	ReasonWriteFailed    = "write_failed"
	ReasonReportDropped  = "report_dropped"
)

// Delivery result details remain more specific than the seven incident
// categories. Callers map these details to an incident reason only when the
// failure becomes user-actionable.
const (
	ReasonMarshalFailed          = "marshal_failed"
	ReasonRequestBuildFailed     = "request_build_failed"
	ReasonPostFailed             = "post_failed"
	ReasonACKReadFailed          = "ack_read_failed"
	ReasonACKInvalid             = "ack_invalid"
	ReasonHTTP4xx                = "http_4xx"
	ReasonHTTP5xx                = "http_5xx"
	ReasonOperatorEventQueueFull = "operator_event_queue_full"
	ReasonOperatorRetryable      = "operator_retryable"
	ReasonGatewayACKTimeout      = "gateway_ack_timeout"
	ReasonStoragePolicyNoRetain  = "storage_policy_no_retain"
	ReasonStoragePolicyNoMatch   = "storage_policy_no_match"
	ReasonRetentionElapsed       = "retention_already_elapsed"
	ReasonInvalidEvent           = "invalid_event"
	ReasonInvalidReport          = "invalid_report"
	ReasonNotLeader              = "not_leader"
	ReasonSyncStreamClosed       = "sync_stream_closed"
	ReasonSendFailed             = "send_failed"
	ReasonRegisterAuthFailed     = "register_auth_failed"
	ReasonSyncAuthFailed         = "sync_auth_failed"
)

func HTTPFailureReason(status int) string {
	if status >= 400 && status < 500 {
		return ReasonHTTP4xx
	}
	if status >= 500 && status < 600 {
		return ReasonHTTP5xx
	}
	return "unknown"
}

var validReasons = map[string]struct{}{
	ReasonQueueFull: {}, ReasonRetryExhausted: {}, ReasonACKTimeout: {},
	ReasonAuthFailure: {}, ReasonBatchFailure: {}, ReasonWriteFailed: {},
	ReasonReportDropped: {},
}

func ValidReason(reason string) bool {
	_, ok := validReasons[reason]
	return ok
}

type Incident struct {
	IncidentKey        string
	ReporterInstanceID string
	Revision           uint64
	Component          Component
	Reason             string
	StartedAt          time.Time
	LastSeenAt         time.Time
	AffectedCount      int64
	SampleEventIDs     []string
	Detail             string
	Closed             bool
}

func (i Incident) Validate() error {
	switch {
	case i.IncidentKey == "" || len(i.IncidentKey) > MaxIdentityLength:
		return errors.New("incident key is required and must not exceed 128 characters")
	case i.ReporterInstanceID == "" || len(i.ReporterInstanceID) > MaxIdentityLength:
		return errors.New("reporter instance id is required and must not exceed 128 characters")
	case i.Revision == 0:
		return errors.New("revision must be positive")
	case !i.Component.Valid():
		return fmt.Errorf("invalid component %q", i.Component)
	case !ValidReason(i.Reason):
		return fmt.Errorf("invalid reason %q", i.Reason)
	case i.StartedAt.IsZero() || i.LastSeenAt.IsZero():
		return errors.New("incident timestamps are required")
	case i.LastSeenAt.Before(i.StartedAt):
		return errors.New("last_seen_at must not precede started_at")
	case i.AffectedCount <= 0:
		return errors.New("affected_count must be positive")
	case len(i.SampleEventIDs) > MaxSampleEventIDs:
		return errors.New("too many sample event ids")
	case len(i.Detail) > MaxDetailLength:
		return errors.New("detail is too long")
	}
	return nil
}

func NewIdentity() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

type Tracker struct {
	mu         sync.Mutex
	component  Component
	instanceID string
	silence    time.Duration
	limit      int
	active     map[string]*Incident
	pending    map[string]*Incident
	order      []string
}

func NewTracker(component Component, instanceID string) *Tracker {
	if !component.Valid() {
		panic("invalid incident tracker component")
	}
	if instanceID == "" {
		var err error
		instanceID, err = NewIdentity()
		if err != nil {
			panic("generate incident reporter instance id: " + err.Error())
		}
	}
	return &Tracker{
		component: component, instanceID: truncate(instanceID, MaxIdentityLength),
		silence: DefaultSilencePeriod, limit: DefaultPendingLimit,
		active: make(map[string]*Incident), pending: make(map[string]*Incident),
	}
}

func (t *Tracker) Record(reason string, affected int64, sampleEventID, detail string, now time.Time) {
	if !ValidReason(reason) {
		return
	}
	if affected <= 0 {
		affected = 1
	}
	now = now.UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	current := t.active[reason]
	if current != nil && now.Sub(current.LastSeenAt) > t.silence {
		t.closeLocked(reason, current, now)
		current = nil
	}
	if current == nil {
		key, err := NewIdentity()
		if err != nil {
			return
		}
		current = &Incident{
			IncidentKey: key, ReporterInstanceID: t.instanceID, Revision: 1,
			Component: t.component, Reason: reason, StartedAt: now, LastSeenAt: now,
			AffectedCount: affected, Detail: truncate(detail, MaxDetailLength),
		}
		t.active[reason] = current
	} else {
		current.Revision++
		current.LastSeenAt = now
		current.AffectedCount += affected
		current.Detail = truncate(detail, MaxDetailLength)
	}
	current.SampleEventIDs = appendSample(current.SampleEventIDs, sampleEventID)
}

func (t *Tracker) Snapshot(now time.Time) []Incident {
	now = now.UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	// Collect first, close after: closeLocked may insert a report_dropped
	// incident into t.active, and an entry created while ranging over a map is
	// produced or skipped at the runtime's discretion.
	expired := make([]string, 0, len(t.active))
	for reason, incident := range t.active {
		if now.Sub(incident.LastSeenAt) > t.silence {
			expired = append(expired, reason)
		}
	}
	for _, reason := range expired {
		t.closeLocked(reason, t.active[reason], now)
	}
	result := make([]Incident, 0, len(t.active)+len(t.pending))
	for _, incident := range t.active {
		result = append(result, cloneIncident(incident))
	}
	for _, key := range t.order {
		if incident := t.pending[key]; incident != nil {
			result = append(result, cloneIncident(incident))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].IncidentKey < result[j].IncidentKey
		}
		return result[i].StartedAt.Before(result[j].StartedAt)
	})
	return result
}

func (t *Tracker) Acknowledge(key string, revision uint64, terminal bool) {
	if !terminal {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	incident := t.pending[key]
	if incident == nil || incident.Revision > revision {
		return
	}
	delete(t.pending, key)
	for index, value := range t.order {
		if value == key {
			t.order = append(t.order[:index], t.order[index+1:]...)
			break
		}
	}
}

func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = make(map[string]*Incident)
	t.pending = make(map[string]*Incident)
	t.order = nil
}

func (t *Tracker) PendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

func (t *Tracker) SetPendingLimitForTest(limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if limit > 0 {
		t.limit = limit
	}
}

func (t *Tracker) closeLocked(reason string, incident *Incident, now time.Time) {
	delete(t.active, reason)
	incident.Closed = true
	incident.Revision++
	if len(t.pending) >= t.limit {
		droppedKey := t.order[0]
		dropped := t.pending[droppedKey]
		delete(t.pending, droppedKey)
		t.order = t.order[1:]
		if dropped != nil && dropped.Reason != ReasonReportDropped && reason != ReasonReportDropped {
			t.recordDropLocked(now)
		}
	}
	t.pending[incident.IncidentKey] = incident
	t.order = append(t.order, incident.IncidentKey)
}

func (t *Tracker) recordDropLocked(now time.Time) {
	current := t.active[ReasonReportDropped]
	if current == nil {
		key, err := NewIdentity()
		if err != nil {
			return
		}
		current = &Incident{
			IncidentKey: key, ReporterInstanceID: t.instanceID, Revision: 1,
			Component: t.component, Reason: ReasonReportDropped,
			StartedAt: now, LastSeenAt: now, AffectedCount: 1,
			Detail: "pending incident queue reached capacity",
		}
		t.active[ReasonReportDropped] = current
		return
	}
	current.Revision++
	current.LastSeenAt = now
	current.AffectedCount++
}

func cloneIncident(value *Incident) Incident {
	result := *value
	result.SampleEventIDs = append([]string(nil), value.SampleEventIDs...)
	return result
}

func appendSample(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || len(values) >= MaxSampleEventIDs {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func MergeSampleEventIDs(oldValues, newValues []string) []string {
	result := append([]string(nil), oldValues...)
	for _, value := range newValues {
		result = appendSample(result, value)
	}
	return result
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
