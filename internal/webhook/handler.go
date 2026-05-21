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
	MatchingForRequest(resource, apiGroup, operation, namespace string, namespaceLabels map[string]string) []*Resolved
	IsReady() bool
}

// ExceptionStore is the read interface the handler needs from ExceptionCache.
type ExceptionStore interface {
	ExemptedKeys(namespace string, namespaceLabels, resourceLabels map[string]string, policies []*CompiledPolicy) map[string]bool
	IsReady() bool
}

// NamespaceLister resolves a namespace name to its metadata.labels. Used by
// the Handler so PolicyExceptions with `match.namespaceSelector` can be
// evaluated. Implementations should be backed by an informer cache and return
// (nil, false) when the namespace is not in the local cache.
type NamespaceLister interface {
	GetLabels(name string) (map[string]string, bool)
}

type noExemptions struct{}

func (noExemptions) ExemptedKeys(string, map[string]string, map[string]string, []*CompiledPolicy) map[string]bool {
	return nil
}
func (noExemptions) IsReady() bool { return true }

type noNamespaceLister struct{}

func (noNamespaceLister) GetLabels(string) (map[string]string, bool) { return nil, false }

// Handler processes admission review requests.
type Handler struct {
	store      PolicyStore
	exceptions ExceptionStore
	nsLister   NamespaceLister
}

func NewHandler(store PolicyStore) *Handler {
	return &Handler{store: store, exceptions: noExemptions{}, nsLister: noNamespaceLister{}}
}

func NewHandlerWithExceptions(store PolicyStore, exceptions ExceptionStore, nsLister NamespaceLister) *Handler {
	if exceptions == nil {
		exceptions = noExemptions{}
	}
	if nsLister == nil {
		nsLister = noNamespaceLister{}
	}
	return &Handler{store: store, exceptions: exceptions, nsLister: nsLister}
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
	var nsLabels map[string]string
	if req.Namespace != "" {
		if l, ok := h.nsLister.GetLabels(req.Namespace); ok {
			nsLabels = l
			if nsLabels == nil {
				// Distinguish "namespace has no labels" from "namespace not
				// found": an empty map is non-nil and means "evaluated".
				nsLabels = map[string]string{}
			}
		}
	}
	resolved := h.store.MatchingForRequest(
		req.Resource.Resource,
		req.Resource.Group,
		string(req.Operation),
		req.Namespace,
		nsLabels,
	)

	if len(resolved) > 0 {
		objLabels := extractObjectLabels(req)
		// Build the legacy []*CompiledPolicy slice the exception store expects.
		policies := make([]*CompiledPolicy, 0, len(resolved))
		for _, r := range resolved {
			policies = append(policies, r.Policy)
		}
		exempt := h.exceptions.ExemptedKeys(req.Namespace, nsLabels, objLabels, policies)
		if len(exempt) > 0 {
			filtered := make([]*Resolved, 0, len(resolved))
			for _, r := range resolved {
				if !exempt[r.Policy.Name] {
					filtered = append(filtered, r)
				}
			}
			resolved = filtered
		}
	}

	resp := h.evaluate(r.Context(), req, resolved)
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

func formatViolation(r *Resolved, msg string) string {
	// Header: [<policyName> via <group1,group2>] <msg>
	via := ""
	if len(r.Groups) > 0 {
		via = " via " + strings.Join(r.Groups, ",")
	}
	s := fmt.Sprintf("[%s%s] %s", r.Policy.Name, via, msg)
	if r.Policy.Description != "" {
		s += "\n  描述：" + r.Policy.Description
	}
	return s
}

func (h *Handler) evaluate(ctx context.Context, req *admissionv1.AdmissionRequest, resolved []*Resolved) *admissionv1.AdmissionResponse {
	if len(resolved) == 0 {
		return &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}
	}

	input := buildInput(req)

	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	type resolvedResult struct {
		resolved *Resolved
		msgs     []string
	}

	var (
		mu             sync.Mutex
		enforceResults []resolvedResult
		auditResults   []resolvedResult
	)

	var wg sync.WaitGroup
	for _, r := range resolved {
		r := r
		wg.Add(1)
		go func() {
			defer wg.Done()
			msgs, err := EvaluatePolicy(evalCtx, r.Policy.Query, input)
			if err != nil {
				slog.Error("policy eval error", "policy", r.Policy.Name, "error", err)
				if r.Mode == v1alpha1.ModeEnforce {
					mu.Lock()
					enforceResults = append(enforceResults, resolvedResult{r, []string{"policy evaluation error: " + r.Policy.Name}})
					mu.Unlock()
				}
				return
			}
			if len(msgs) == 0 {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if r.Mode == v1alpha1.ModeAudit {
				auditResults = append(auditResults, resolvedResult{r, msgs})
			} else {
				enforceResults = append(enforceResults, resolvedResult{r, msgs})
			}
		}()
	}
	wg.Wait()

	if len(enforceResults) > 0 {
		var parts []string
		for _, rr := range enforceResults {
			for _, msg := range rr.msgs {
				parts = append(parts, formatViolation(rr.resolved, msg))
			}
		}
		for _, rr := range auditResults {
			for _, msg := range rr.msgs {
				parts = append(parts, formatViolation(rr.resolved, msg))
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
		for _, rr := range auditResults {
			slog.Warn("audit violation", "policy", rr.resolved.Policy.Name, "groups", rr.resolved.Groups, "count", len(rr.msgs))
			for _, msg := range rr.msgs {
				warnings = append(warnings, formatViolation(rr.resolved, msg))
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
