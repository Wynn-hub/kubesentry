package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

const evalTimeout = 5 * time.Second

// PolicyStore is the read interface the handler needs from the cache.
type PolicyStore interface {
	MatchingPolicies(resource, apiGroup, operation, namespace string) []*CompiledPolicy
	IsReady() bool
}

// Handler processes admission review requests.
type Handler struct {
	store PolicyStore
}

func NewHandler(store PolicyStore) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.store.IsReady() {
		http.Error(w, "cache not ready", http.StatusServiceUnavailable)
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "missing admission request", http.StatusBadRequest)
		return
	}

	req := review.Request
	policies := h.store.MatchingPolicies(
		req.Resource.Resource,
		req.Resource.Group,
		string(req.Operation),
		req.Namespace,
	)

	resp := h.evaluate(r.Context(), req, policies)
	review.Response = resp
	review.Request = nil

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

func (h *Handler) evaluate(ctx context.Context, req *admissionv1.AdmissionRequest, policies []*CompiledPolicy) *admissionv1.AdmissionResponse {
	if len(policies) == 0 {
		return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
	}

	input := buildInput(req)

	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	var (
		mu      sync.Mutex
		denials []string
	)

	var wg sync.WaitGroup
	for _, p := range policies {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			msgs, err := EvaluatePolicy(evalCtx, p.Query, input)
			if err != nil {
				slog.Error("policy eval error", "policy", p.Name, "error", err)
				if p.EnforcementMode == v1alpha1.ModeEnforce {
					mu.Lock()
					denials = append(denials, "policy evaluation error: "+p.Name)
					mu.Unlock()
				}
				return
			}
			if len(msgs) == 0 {
				return
			}
			if p.EnforcementMode == v1alpha1.ModeAudit {
				slog.Warn("audit policy violation", "policy", p.Name, "denials", msgs)
				return
			}
			mu.Lock()
			denials = append(denials, msgs...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(denials) > 0 {
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Code:    http.StatusForbidden,
				Message: strings.Join(denials, "; "),
			},
		}
	}
	return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
}

func buildInput(req *admissionv1.AdmissionRequest) map[string]interface{} {
	var obj interface{}
	if len(req.Object.Raw) > 0 {
		json.Unmarshal(req.Object.Raw, &obj) //nolint:errcheck
	}
	return map[string]interface{}{
		"request": map[string]interface{}{
			"uid":       string(req.UID),
			"namespace": req.Namespace,
			"operation": string(req.Operation),
			"resource": map[string]interface{}{
				"group":    req.Resource.Group,
				"version":  req.Resource.Version,
				"resource": req.Resource.Resource,
			},
			"object":   obj,
			"userInfo": req.UserInfo,
		},
	}
}
