package operator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type admissionEventSinkStub struct {
	leader          bool
	platformEnabled bool
	observed        int
	samples         int
	faultReason     string
}

func (s *admissionEventSinkStub) IsLeader() bool { return s.leader }

func (s *admissionEventSinkStub) SubmitAdmissionEvents(*gatewaypb.AdmissionEventBatch) ([]*gatewaypb.AdmissionEventBatchAck_Result, bool) {
	return nil, true
}

func (s *admissionEventSinkStub) SubmitLinkIncidents(batch *gatewaypb.LinkIncidentBatch) []*gatewaypb.LinkIncidentBatchAck_Result {
	s.observed += len(batch.Incidents)
	return []*gatewaypb.LinkIncidentBatchAck_Result{{
		IncidentKey: batch.Incidents[0].IncidentKey,
		Revision:    batch.Incidents[0].Revision,
		Status:      gatewaypb.LinkIncidentBatchAck_Result_ACCEPTED,
	}}
}

func (s *admissionEventSinkStub) SubmitComponentSamples(batch *gatewaypb.ComponentSampleBatch) bool {
	s.samples += len(batch.GetSamples())
	return s.leader
}
func (s *admissionEventSinkStub) AdmissionObservationEnabled() bool { return s.leader }

func (s *admissionEventSinkStub) PlatformObservationEnabled() bool { return s.platformEnabled }

func (s *admissionEventSinkStub) RecordLinkIncident(reason string, _ int64, _ string) {
	s.faultReason = reason
}

func TestAdmissionObservationEndpointAuthenticatesAndAcknowledges(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	now := timestamppb.Now()
	body, _ := protojson.Marshal(&gatewaypb.LinkIncidentBatch{Incidents: []*gatewaypb.LinkIncident{{
		IncidentKey: "r1", ReporterInstanceId: "webhook-a", Revision: 1,
		Component: "webhook", Reason: admissionobs.ReasonQueueFull,
		StartedAt: now, LastSeenAt: now, AffectedCount: 1,
	}}})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/admission-events/incidents", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	server.handleIncidents(response, request)
	if response.Code != http.StatusOK || sink.observed != 1 ||
		!strings.Contains(response.Body.String(), `"status":"ACCEPTED"`) {
		t.Fatalf("response = (%d, %q), observed = %d", response.Code, response.Body.String(), sink.observed)
	}
}

func TestAdmissionObservationEndpointRecordsAuthenticationFailure(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	response := httptest.NewRecorder()

	server.handleIncidents(response,
		httptest.NewRequest(http.MethodPost, "/internal/v1/admission-events/incidents", strings.NewReader(`{}`)))
	if response.Code != http.StatusUnauthorized || sink.faultReason != admissionobs.ReasonAuthFailure {
		t.Fatalf("response = %d, fault = %q", response.Code, sink.faultReason)
	}
}

func TestPlatformSampleEndpointAcceptsLeaderSamples(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true, platformEnabled: true}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	sample := platformobs.UnavailableSample(platformobs.ComponentWebhook, "webhook-0", time.Now())
	sample.ProcessStartedAt = time.Now().Add(-time.Hour)
	body, err := protojson.Marshal(&gatewaypb.ComponentSampleBatch{
		Samples: []*gatewaypb.ComponentSample{platformobs.ToProto(sample)},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/platform-samples", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	server.handlePlatformSamples(response, request)
	if response.Code != http.StatusAccepted || sink.samples != 1 {
		t.Fatalf("response = %d, samples = %d", response.Code, sink.samples)
	}
}

// Disabled must answer 204, not 503: the webhook treats 503 as a transient
// failure worth warning about, and would log every minute for as long as the
// feature stays off.
func TestPlatformSampleEndpointReportsDisabledAsNoContent(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true, platformEnabled: false}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/platform-samples", strings.NewReader("{}"))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	server.handlePlatformSamples(response, request)
	if response.Code != http.StatusNoContent || sink.samples != 0 {
		t.Fatalf("response = %d, samples = %d; want 204 and no samples", response.Code, sink.samples)
	}
}

func TestAdmissionObservationConfigReflectsLeaderAndSetting(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/admission-events/observations/config", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	server.handleObservationConfig(response, request)
	var state struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !state.Enabled {
		t.Fatalf("response = %d, state = %+v", response.Code, state)
	}
}
