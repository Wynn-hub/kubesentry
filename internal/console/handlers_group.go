package console

import (
	"encoding/json"
	"net/http"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

type groupListItem struct {
	Name          string `json:"name"`
	DisplayName   string `json:"displayName"`
	Source        string `json:"source"`
	Enabled       bool   `json:"enabled"`
	Phase         string `json:"phase"`
	ResolvedCount int32  `json:"resolvedCount"`
}

type groupDetail struct {
	Name            string                     `json:"name"`
	Source          string                     `json:"source"`
	ResourceVersion string                     `json:"resourceVersion"`
	Spec            v1alpha1.PolicyGroupSpec   `json:"spec"`
	Status          v1alpha1.PolicyGroupStatus `json:"status"`
}

func (h *Handlers) listGroups(w http.ResponseWriter, r *http.Request) {
	var list v1alpha1.PolicyGroupList
	if err := h.Client.List(r.Context(), &list); err != nil {
		writeErr(w, http.StatusInternalServerError, "list policygroups: "+err.Error())
		return
	}
	items := make([]groupListItem, 0, len(list.Items))
	for i := range list.Items {
		g := &list.Items[i]
		items = append(items, groupListItem{
			Name:          g.Name,
			DisplayName:   g.Spec.DisplayName,
			Source:        sourceOf(g.Labels),
			Enabled:       g.Spec.Enabled,
			Phase:         g.Status.Phase,
			ResolvedCount: g.Status.ResolvedCount,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeOK(w, items)
}

func (h *Handlers) getGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := h.fetchGroup(w, r)
	if !ok {
		return
	}
	writeOK(w, groupDetail{
		Name:            g.Name,
		Source:          sourceOf(g.Labels),
		ResourceVersion: g.ResourceVersion,
		Spec:            g.Spec,
		Status:          g.Status,
	})
}

func (h *Handlers) fetchGroup(w http.ResponseWriter, r *http.Request) (*v1alpha1.PolicyGroup, bool) {
	name := r.PathValue("name")
	var g v1alpha1.PolicyGroup
	if err := h.Client.Get(r.Context(), client.ObjectKey{Name: name}, &g); err != nil {
		if apierrors.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "policygroup "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get policygroup: "+err.Error())
		}
		return nil, false
	}
	return &g, true
}

func (h *Handlers) createGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateGroupRequest(&req, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	g := &v1alpha1.PolicyGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:   req.Name,
			Labels: map[string]string{v1alpha1.LabelSource: v1alpha1.SourceCustom},
		},
		Spec: v1alpha1.PolicyGroupSpec{
			DisplayName:             req.DisplayName,
			Description:             req.Description,
			Enabled:                 req.Enabled,
			NamespaceSelector:       req.NamespaceSelector,
			Policies:                req.Policies,
			SelectorEnforcementMode: req.SelectorEnforcementMode,
		},
	}
	if err := h.Client.Create(r.Context(), g); err != nil {
		if apierrors.IsAlreadyExists(err) {
			writeErr(w, http.StatusConflict, "policygroup "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create policygroup: "+err.Error())
		}
		return
	}
	writeOK(w, map[string]string{"name": g.Name})
}

func (h *Handlers) updateGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateGroupRequest(&req, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	g, ok := h.fetchGroup(w, r)
	if !ok {
		return
	}
	if req.ResourceVersion != "" && req.ResourceVersion != g.ResourceVersion {
		writeErr(w, http.StatusConflict, "policygroup has been modified, refresh and retry")
		return
	}
	g.Spec.DisplayName = req.DisplayName
	g.Spec.Description = req.Description
	g.Spec.Enabled = req.Enabled
	g.Spec.NamespaceSelector = req.NamespaceSelector
	g.Spec.Policies = req.Policies
	g.Spec.SelectorEnforcementMode = req.SelectorEnforcementMode
	if err := h.Client.Update(r.Context(), g); err != nil {
		if apierrors.IsConflict(err) {
			writeErr(w, http.StatusConflict, "policygroup has been modified, refresh and retry")
		} else {
			writeErr(w, http.StatusInternalServerError, "update policygroup: "+err.Error())
		}
		return
	}
	writeOK(w, map[string]string{"name": g.Name})
}

// setGroupEnabled toggles spec.enabled via merge patch so the quick switch in
// the list view cannot conflict with a concurrent full edit of other fields.
func (h *Handlers) setGroupEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Enabled == nil {
		writeErr(w, http.StatusBadRequest, "enabled is required")
		return
	}
	g, ok := h.fetchGroup(w, r)
	if !ok {
		return
	}
	if g.Spec.Enabled != *req.Enabled {
		patch := client.MergeFrom(g.DeepCopy())
		g.Spec.Enabled = *req.Enabled
		if err := h.Client.Patch(r.Context(), g, patch); err != nil {
			writeErr(w, http.StatusInternalServerError, "patch policygroup: "+err.Error())
			return
		}
	}
	writeOK(w, map[string]bool{"enabled": *req.Enabled})
}

func (h *Handlers) deleteGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := h.fetchGroup(w, r)
	if !ok {
		return
	}
	if err := h.Client.Delete(r.Context(), g); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete policygroup: "+err.Error())
		return
	}
	writeOK(w, nil)
}
