package webhook

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	admissionv1 "k8s.io/api/admission/v1"
)

// AdmissionEventReporter is asynchronous by contract: reporting failures must
// never alter the admission response.
type AdmissionEventReporter interface {
	Report(*admissionv1.AdmissionRequest, *admissionv1.AdmissionResponse)
}

// maxWebhookDeliveryAttempts bounds how many times the webhook re-posts one
// event to the operator. Without it a persistently RETRYABLE reply (most often
// "operator_event_queue_full") loops forever at retryDelay, which keeps pressure
// on the operator exactly when it is already saturated.
const (
	maxWebhookDeliveryAttempts = 4
	retryDelay                 = time.Second
)

// queuedEvent tracks per-event delivery attempts. The webhook queue is
// intentionally in-memory and lossy; attempts are not persisted.
type queuedEvent struct {
	event    *gatewaypb.AdmissionEvent
	attempts int
}

type HTTPAdmissionEventReporter struct {
	endpoint string
	token    string
	queue    chan *queuedEvent
	client   *http.Client
}

func NewHTTPAdmissionEventReporter(endpoint, token string) *HTTPAdmissionEventReporter {
	r := &HTTPAdmissionEventReporter{endpoint: endpoint, token: token, queue: make(chan *queuedEvent, 256), client: &http.Client{Timeout: 2 * time.Second}}
	go r.run()
	return r
}

func (r *HTTPAdmissionEventReporter) Report(req *admissionv1.AdmissionRequest, response *admissionv1.AdmissionResponse) {
	verdict := ""
	violations := response.Warnings
	if !response.Allowed {
		verdict = "deny"
		if response.Result != nil {
			violations = []string{response.Result.Message}
		}
	} else if len(violations) > 0 {
		verdict = "audit"
	}
	if verdict == "" {
		return
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(req.Object.Raw, &raw)
	policies, groups := AdmissionEventReferences(violations)
	event := &gatewaypb.AdmissionEvent{EventId: newAdmissionEventULID(), RequestUid: string(req.UID), OccurredAt: timestamppb.Now(), Verdict: verdict, ApiGroup: req.Resource.Group, ApiVersion: req.Resource.Version, Resource: req.Resource.Resource, Operation: string(req.Operation), Namespace: req.Namespace, ResourceName: req.Name, Username: req.UserInfo.Username, UserGroups: req.UserInfo.Groups, PolicyNames: policies, PolicyGroupNames: groups, Violations: violations, MetadataSnapshot: raw["metadata"], SpecSnapshot: raw["spec"]}
	select {
	case r.queue <- &queuedEvent{event: event}:
	default:
		slog.Warn("admission event dropped", "event", event.EventId, "reason", "webhook_queue_full")
	}
}

func (r *HTTPAdmissionEventReporter) run() {
	for queued := range r.queue {
		queued.attempts++
		reason, delivered := r.deliver(queued.event)
		if delivered {
			continue
		}
		if queued.attempts >= maxWebhookDeliveryAttempts {
			slog.Warn("admission event delivery dropped",
				"event", queued.event.EventId, "attempts", queued.attempts, "reason", reason)
			continue
		}
		slog.Warn("admission event delivery will retry",
			"event", queued.event.EventId, "attempt", queued.attempts, "max_attempts", maxWebhookDeliveryAttempts, "reason", reason)
		time.AfterFunc(retryDelay, func() {
			select {
			case r.queue <- queued:
			default:
				slog.Warn("admission event dropped", "event", queued.event.EventId, "reason", "webhook_queue_full")
			}
		})
	}
}

// deliver posts one event and reports whether it reached a terminal state.
// ACCEPTED and DISCARDED are terminal; RETRYABLE, a transport error, a non-2xx
// status, or an unreadable/mismatched ack all mean "try again".
func (r *HTTPAdmissionEventReporter) deliver(event *gatewaypb.AdmissionEvent) (reason string, delivered bool) {
	body, err := protojson.Marshal(&gatewaypb.AdmissionEventBatch{Events: []*gatewaypb.AdmissionEvent{event}})
	if err != nil {
		// Marshal failures are deterministic; retrying cannot help.
		slog.Warn("admission event delivery dropped", "event", event.EventId, "reason", "marshal_failed")
		return "marshal_failed", true
	}
	req, err := http.NewRequest(http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		slog.Warn("admission event delivery dropped", "event", event.EventId, "reason", "request_build_failed")
		return "request_build_failed", true
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)
	response, err := r.client.Do(req)
	if err != nil {
		return "post_failed", false
	}
	defer response.Body.Close() //nolint:errcheck
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return admissionobs.HTTPFailureReason(response.StatusCode), false
	}
	data, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return "ack_read_failed", false
	}
	var ack gatewaypb.AdmissionEventBatchAck
	if protojson.Unmarshal(data, &ack) != nil || len(ack.Results) != 1 || ack.Results[0].EventId != event.EventId {
		return "ack_invalid", false
	}
	switch ack.Results[0].Status {
	case gatewaypb.AdmissionEventBatchAck_Result_ACCEPTED, gatewaypb.AdmissionEventBatchAck_Result_DISCARDED:
		return "", true
	}
	if ackReason := ack.Results[0].Reason; ackReason != "" {
		return ackReason, false
	}
	return "operator_retryable", false
}

const crockford32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newAdmissionEventULID() string {
	var random [10]byte
	_, _ = rand.Read(random[:])
	ms := uint64(time.Now().UnixMilli())
	out := make([]byte, 26)
	for i := 9; i >= 0; i-- {
		out[i] = crockford32[ms&31]
		ms >>= 5
	}
	var value uint64
	bits, pos := 0, 10
	for _, b := range random {
		value = value<<8 | uint64(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[pos] = crockford32[(value>>bits)&31]
			pos++
		}
	}
	return string(out)
}

// AdmissionEventReferences extracts the policy and policy-group references
// embedded by formatViolation. Keeping this next to the formatter's producer
// makes the event payload structured without changing the admission response
// users and Kubernetes already receive.
func AdmissionEventReferences(violations []string) ([]string, []string) {
	policies := make([]string, 0)
	groups := make([]string, 0)
	seenPolicies := make(map[string]struct{})
	seenGroups := make(map[string]struct{})

	add := func(values *[]string, seen map[string]struct{}, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		*values = append(*values, value)
	}

	for _, violation := range violations {
		for _, line := range strings.Split(violation, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "[") {
				continue
			}
			end := strings.IndexByte(line, ']')
			if end <= 1 {
				continue
			}
			policy, rawGroups, hasGroups := strings.Cut(line[1:end], " via ")
			add(&policies, seenPolicies, policy)
			if !hasGroups {
				continue
			}
			for _, group := range strings.Split(rawGroups, ",") {
				add(&groups, seenGroups, group)
			}
		}
	}
	return policies, groups
}
