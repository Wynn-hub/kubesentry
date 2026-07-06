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
		for k, v := range req.Labels {
			p.Labels[k] = v
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

func (h *Handlers) deletePolicy(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	if len(p.Status.ReferencedBy) > 0 && r.URL.Query().Get("force") != "true" {
		writeErrData(w, http.StatusConflict,
			"policy is referenced by policy groups",
			map[string][]string{"referencedBy": p.Status.ReferencedBy})
		return
	}
	if err := h.Client.Delete(r.Context(), p); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete policy: "+err.Error())
		return
	}
	writeOK(w, nil)
}

type versionEntry struct {
	Version         int64                `json:"version"`
	CreatedAt       metav1.Time          `json:"createdAt"`
	Phase           string               `json:"phase"`
	Rego            string               `json:"rego"`
	Match           v1alpha1.PolicyMatch `json:"match"`
	EnforcementMode string               `json:"enforcementMode"`
	IsCurrent       bool                 `json:"isCurrent"`
}

type versionTimeline struct {
	CurrentVersion int64          `json:"currentVersion"`
	Cursor         int64          `json:"cursor"`
	Head           int64          `json:"head"`
	InFlight       bool           `json:"inFlight"`
	PrevEnabled    bool           `json:"prevEnabled"`
	NextEnabled    bool           `json:"nextEnabled"`
	Versions       []versionEntry `json:"versions"`
}

// listPolicyVersions returns snapshots for a policy, newest first.
func (h *Handlers) listPolicyVersions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.fetchPolicy(w, r)
	if !ok {
		return
	}
	var pvList v1alpha1.PolicyVersionList
	if err := h.Client.List(r.Context(), &pvList,
		client.MatchingLabels{"kubesentry/policy": p.Name}); err != nil {
		writeErr(w, http.StatusInternalServerError, "list versions: "+err.Error())
		return
	}

	phaseByVersion := map[int64]string{}
	for _, s := range p.Status.VersionHistory {
		phaseByVersion[s.Version] = s.Phase
	}

	cursor, head, inFlight := resolveCursor(p)
	if p.Spec.RollbackTo != nil {
		inFlight = true
	}

	exists := map[int64]bool{}
	entries := make([]versionEntry, 0, len(pvList.Items))
	for i := range pvList.Items {
		pv := &pvList.Items[i]
		exists[pv.Spec.Version] = true
		entries = append(entries, versionEntry{
			Version:         pv.Spec.Version,
			CreatedAt:       pv.Spec.CreatedAt,
			Phase:           phaseByVersion[pv.Spec.Version],
			Rego:            pv.Spec.Rego,
			Match:           pv.Spec.Match,
			EnforcementMode: pv.Spec.EnforcementMode,
			IsCurrent:       pv.Spec.Version == cursor,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Version > entries[j].Version })

	writeOK(w, versionTimeline{
		CurrentVersion: p.Status.CurrentVersion,
		Cursor:         cursor,
		Head:           head,
		InFlight:       inFlight,
		PrevEnabled:    !inFlight && cursor > 1 && exists[cursor-1],
		NextEnabled:    !inFlight && cursor < head && exists[cursor+1],
		Versions:       entries,
	})
}
