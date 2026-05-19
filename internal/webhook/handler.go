package webhook

import (
	"context"
	"encoding/json"
	"fmt"
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

// ExceptionStore is the read interface the handler needs from ExceptionCache.
type ExceptionStore interface {
	ExemptedKeys(namespace string, resourceLabels map[string]string, policies []*CompiledPolicy) map[string]bool
	IsReady() bool
}

type noExemptions struct{}

func (noExemptions) ExemptedKeys(string, map[string]string, []*CompiledPolicy) map[string]bool {
	return nil
}
func (noExemptions) IsReady() bool { return true }

// Handler processes admission review requests.
type Handler struct {
	store      PolicyStore
	exceptions ExceptionStore
}

func NewHandler(store PolicyStore) *Handler {
	return &Handler{store: store, exceptions: noExemptions{}}
}

func NewHandlerWithExceptions(store PolicyStore, exceptions ExceptionStore) *Handler {
	if exceptions == nil {
		exceptions = noExemptions{}
	}
	return &Handler{store: store, exceptions: exceptions}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.store.IsReady() || !h.exceptions.IsReady() {
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

	if len(policies) > 0 {
		objLabels := extractObjectLabels(req)
		exempt := h.exceptions.ExemptedKeys(req.Namespace, objLabels, policies)
		if len(exempt) > 0 {
			filtered := make([]*CompiledPolicy, 0, len(policies))
			for _, p := range policies {
				if !exempt[p.Name] {
					filtered = append(filtered, p)
				}
			}
			policies = filtered
		}
	}

	resp := h.evaluate(r.Context(), req, policies)
	review.Response = resp
	review.Request = nil

	allowed := resp.Allowed
	if allowed {
		slog.Info("admission allowed",
			"resource", req.Resource.Resource,
			"namespace", req.Namespace,
			"name", req.Name,
			"operation", string(req.Operation),
			"user", req.UserInfo.Username,
		)
	} else {
		slog.Warn("admission denied",
			"resource", req.Resource.Resource,
			"namespace", req.Namespace,
			"name", req.Name,
			"operation", string(req.Operation),
			"user", req.UserInfo.Username,
			"reason", resp.Result.Message,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

type policyResult struct {
	policy *CompiledPolicy
	msgs   []string
}

func formatViolation(p *CompiledPolicy, msg string) string {
	groupKey := p.Name
	if p.Key != "" {
		groupKey = p.Key
		if p.GroupName != "" {
			groupKey = p.GroupName + "/" + p.Key
		}
	}
	s := fmt.Sprintf("[%s] %s", groupKey, msg)
	if p.Description != "" {
		s += "\n  描述：" + p.Description
	}
	return s
}

func (h *Handler) evaluate(ctx context.Context, req *admissionv1.AdmissionRequest, policies []*CompiledPolicy) *admissionv1.AdmissionResponse {
	if len(policies) == 0 {
		return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
	}

	input := buildInput(req)

	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	var (
		mu             sync.Mutex
		enforceResults []policyResult
		auditResults   []policyResult
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
					enforceResults = append(enforceResults, policyResult{p, []string{"policy evaluation error: " + p.Name}})
					mu.Unlock()
				}
				return
			}
			if len(msgs) == 0 {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if p.EnforcementMode == v1alpha1.ModeAudit {
				auditResults = append(auditResults, policyResult{p, msgs})
			} else {
				enforceResults = append(enforceResults, policyResult{p, msgs})
			}
		}()
	}
	wg.Wait()

	if len(enforceResults) > 0 {
		var parts []string
		for _, r := range enforceResults {
			for _, msg := range r.msgs {
				parts = append(parts, formatViolation(r.policy, msg))
			}
		}
		for _, r := range auditResults {
			for _, msg := range r.msgs {
				parts = append(parts, formatViolation(r.policy, msg))
			}
		}
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Code:    http.StatusForbidden,
				Message: strings.Join(parts, "\n\n"),
			},
		}
	}

	if len(auditResults) > 0 {
		var warnings []string
		for _, r := range auditResults {
			slog.Warn("audit violation", "policy", r.policy.Key, "group", r.policy.GroupName, "count", len(r.msgs))
			for _, msg := range r.msgs {
				warnings = append(warnings, formatViolation(r.policy, msg))
			}
		}
		return &admissionv1.AdmissionResponse{
			UID:      req.UID,
			Allowed:  true,
			Warnings: warnings,
		}
	}

	return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
}

func extractObjectLabels(req *admissionv1.AdmissionRequest) map[string]string {
	if len(req.Object.Raw) == 0 {
		return nil
	}
	var meta struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(req.Object.Raw, &meta); err != nil {
		return nil
	}
	return meta.Metadata.Labels
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
