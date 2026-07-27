package operator

import (
	"context"
	"io"
	"net/http"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// AdmissionEventSink is intentionally small so the local HTTP service can be
// tested without a real Hub Link. IsLeader is mandatory: Service selectors are
// not an authority boundary during leader handover.
type AdmissionEventSink interface {
	IsLeader() bool
	SubmitAdmissionEvents(*gatewaypb.AdmissionEventBatch) ([]*gatewaypb.AdmissionEventBatchAck_Result, bool)
}
type AdmissionEventServer struct {
	Address, Token string
	Sink           AdmissionEventSink
}

func (s *AdmissionEventServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/admission-events", s.handle)
	server := &http.Server{Addr: s.Address, Handler: mux}
	go func() { <-ctx.Done(); _ = server.Shutdown(context.Background()) }()
	return server.ListenAndServe()
}

func (s *AdmissionEventServer) handle(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" || strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") != s.Token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.Sink == nil || !s.Sink.IsLeader() {
		http.Error(w, "event leader unavailable", http.StatusServiceUnavailable)
		return
	}
	var batch gatewaypb.AdmissionEventBatch
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read event batch", http.StatusBadRequest)
		return
	}
	if err = (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, &batch); err != nil {
		http.Error(w, "invalid event batch", http.StatusBadRequest)
		return
	}
	results, ok := s.Sink.SubmitAdmissionEvents(&batch)
	if !ok {
		http.Error(w, "event queue full", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encoded, _ := protojson.Marshal(&gatewaypb.AdmissionEventBatchAck{Results: results})
	_, _ = w.Write(encoded)
}
