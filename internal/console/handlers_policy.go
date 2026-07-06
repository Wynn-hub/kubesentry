package console

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

const cursorAnnotation = "kubesentry.io/logical-cursor"

type policyListItem struct {
	Name            string   `json:"name"`
	Source          string   `json:"source"`
	EnforcementMode string   `json:"enforcementMode"`
	Phase           string   `json:"phase"`
	Description     string   `json:"description"`
	ReferencedBy    []string `json:"referencedBy"`
	CurrentVersion  int64    `json:"currentVersion"`
}

type policyDetail struct {
	Name            string                `json:"name"`
	Source          string                `json:"source"`
	Labels          map[string]string     `json:"labels"`
	ResourceVersion string                `json:"resourceVersion"`
	Spec            v1alpha1.PolicySpec   `json:"spec"`
	Status          v1alpha1.PolicyStatus `json:"status"`
}

func sourceOf(labels map[string]string) string {
	if s := labels[v1alpha1.LabelSource]; s != "" {
		return s
	}
	return v1alpha1.SourceCustom
}

func (h *Handlers) listPolicies(w http.ResponseWriter, r *http.Request) {
	var list v1alpha1.PolicyList
	if err := h.Client.List(r.Context(), &list); err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	q := r.URL.Query()
	source, phase := q.Get("source"), q.Get("phase")
	keyword := strings.ToLower(q.Get("keyword"))

	items := make([]policyListItem, 0, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
		if source != "" && sourceOf(p.Labels) != source {
			continue
		}
		if phase != "" && p.Status.Phase != phase {
			continue
		}
		if keyword != "" &&
			!strings.Contains(strings.ToLower(p.Name), keyword) &&
			!strings.Contains(strings.ToLower(p.Spec.Description), keyword) {
			continue
		}
		items = append(items, policyListItem{
			Name:            p.Name,
			Source:          sourceOf(p.Labels),
			EnforcementMode: p.Spec.EnforcementMode,
			Phase:           p.Status.Phase,
			Description:     p.Spec.Description,
			ReferencedBy:    p.Status.ReferencedBy,
			CurrentVersion:  p.Status.CurrentVersion,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeOK(w, items)
}

func (h *Handlers) getPolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	writeOK(w, policyDetail{
		Name:            p.Name,
		Source:          sourceOf(p.Labels),
		Labels:          p.Labels,
		ResourceVersion: p.ResourceVersion,
		Spec:            p.Spec,
		Status:          p.Status,
	})
}

// fetchPolicy loads the {name} path policy, writing 404/500 on failure.
func (h *Handlers) fetchPolicy(w http.ResponseWriter, r *http.Request) (*v1alpha1.Policy, bool) {
	name := r.PathValue("name")
	var p v1alpha1.Policy
	if err := h.Client.Get(r.Context(), client.ObjectKey{Name: name}, &p); err != nil {
		if apierrors.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "policy "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get policy: "+err.Error())
		}
		return nil, false
	}
	return &p, true
}

func (h *Handlers) createPolicy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1 MB limit
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validatePolicyRequest(&req, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	labels := map[string]string{v1alpha1.LabelSource: v1alpha1.SourceCustom}
	for k, v := range req.Labels {
		labels[k] = v
	}
	p := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Labels: labels},
		Spec: v1alpha1.PolicySpec{
			Match:           req.Match,
			EnforcementMode: req.EnforcementMode,
			Rego:            req.Rego,
			Description:     req.Description,
		},
	}
	if err := h.Client.Create(r.Context(), p); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeErr(w, http.StatusConflict, "policy "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create policy: "+err.Error())
		}
		return
	}
	writeOK(w, map[string]string{"name": p.Name})
}

func (h *Handlers) updatePolicy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1 MB limit
	var req policyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validatePolicyRequest(&req, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	// Prevent edits to builtin policies
	if sourceOf(p.Labels) == v1alpha1.SourceBuiltin {
		writeErr(w, http.StatusForbidden, "cannot edit builtin policy")
		return
	}
	if req.ResourceVersion != "" && req.ResourceVersion != p.ResourceVersion {
		writeErr(w, http.StatusConflict, "policy has been modified, refresh and retry")
		return
	}
	p.Spec.Match = req.Match
	p.Spec.EnforcementMode = req.EnforcementMode
	p.Spec.Rego = req.Rego
	p.Spec.Description = req.Description
	if req.Labels != nil {
		if p.Labels == nil {
			p.Labels = map[string]string{}
		}
		// Only allow custom labels; preserve source label
		for k, v := range req.Labels {
			if k != v1alpha1.LabelSource {
				p.Labels[k] = v
			}
		}
	}
	delete(p.Annotations, cursorAnnotation) // console edit resets the timeline cursor
	if err := h.Client.Update(r.Context(), p); err != nil {
		if apierrors.IsConflict(err) {
			writeErr(w, http.StatusConflict, "policy has been modified, refresh and retry")
		} else {
			writeErr(w, http.StatusInternalServerError, "update policy: "+err.Error())
		}
		return
	}
	writeOK(w, map[string]string{"name": p.Name})
}

func (h *Handlers) validatePolicy(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1 MB limit
	var req struct {
		Rego string `json:"rego"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if _, err := webhook.CompileRego(req.Rego); err != nil {
		writeErr(w, http.StatusBadRequest, "rego compile failed: "+err.Error())
		return
	}
	writeOK(w, nil)
}
