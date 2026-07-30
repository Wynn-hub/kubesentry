package operator

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

type admissionEventSinkStub struct {
	leader      bool
	observed    int
	faultReason string
}

func (s *admissionEventSinkStub) IsLeader() bool { return s.leader }

func (s *admissionEventSinkStub) SubmitAdmissionEvents(*gatewaypb.AdmissionEventBatch) ([]*gatewaypb.AdmissionEventBatchAck_Result, bool) {
	return nil, true
}

func (s *admissionEventSinkStub) SubmitObservationReports(batch *gatewaypb.ObservationReportBatch) []*gatewaypb.ObservationReportBatchAck_Result {
	s.observed += len(batch.Reports)
	return []*gatewaypb.ObservationReportBatchAck_Result{{
		ReportId: batch.Reports[0].ReportId,
		Status:   gatewaypb.ObservationReportBatchAck_Result_ACCEPTED,
	}}
}

func (s *admissionEventSinkStub) RecordAdmissionObservationFailure(_ admissionobs.ReasonClass, reason string) {
	s.faultReason = reason
}

func TestAdmissionObservationEndpointAuthenticatesAndAcknowledges(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/admission-events/observations",
		strings.NewReader(`{"reports":[{"reportId":"r1"}]}`))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()

	server.handleObservations(response, request)
	if response.Code != http.StatusOK || sink.observed != 1 ||
		!strings.Contains(response.Body.String(), `"status":"ACCEPTED"`) {
		t.Fatalf("response = (%d, %q), observed = %d", response.Code, response.Body.String(), sink.observed)
	}
}

func TestAdmissionObservationEndpointRecordsAuthenticationFailure(t *testing.T) {
	sink := &admissionEventSinkStub{leader: true}
	server := &AdmissionEventServer{Token: "secret", Sink: sink}
	response := httptest.NewRecorder()

	server.handleObservations(response,
		httptest.NewRequest(http.MethodPost, "/internal/v1/admission-events/observations", strings.NewReader(`{}`)))
	if response.Code != http.StatusUnauthorized || sink.faultReason != admissionobs.ReasonAuthFailed {
		t.Fatalf("response = %d, fault = %q", response.Code, sink.faultReason)
	}
}
