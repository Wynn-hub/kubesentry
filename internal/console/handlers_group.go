package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// defaultGroupName is the platform baseline PolicyGroup, preloaded by the
// gateway chart and referenced by the reserved "default" PolicyDistribution —
// deleting it would silently drop baseline enforcement for every registered
// cluster.
const defaultGroupName = "default"

type groupListItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
}

type groupDetail struct {
	Name            string                   `json:"name"`
	ResourceVersion string                   `json:"resourceVersion"`
	Spec            v1alpha1.PolicyGroupSpec `json:"spec"`
}

// groupResourceVersion derives an opaque optimistic-concurrency token from
// the row's UpdatedAt, mirroring the CRD-era resourceVersion contract
// (client echoes it back on update; a mismatch means someone else wrote in
// between) without needing a dedicated column.
func groupResourceVersion(g *dbrepo.PolicyGroup) string {
	return strconv.FormatInt(g.UpdatedAt.UnixNano(), 10)
}

func toGroupDetail(g *dbrepo.PolicyGroup) groupDetail {
	return groupDetail{
		Name:            g.Name,
		ResourceVersion: groupResourceVersion(g),
		Spec:            g.Spec.Data(),
	}
}

func (h *Handlers) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.Repos.Groups.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list groups: "+err.Error())
		return
	}
	items := make([]groupListItem, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		spec := g.Spec.Data()
		items = append(items, groupListItem{
			Name:        g.Name,
			DisplayName: spec.DisplayName,
			Enabled:     spec.Enabled,
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
	writeOK(w, toGroupDetail(g))
}

// fetchGroup loads the {name} path policy group, writing 404/500 on failure.
func (h *Handlers) fetchGroup(w http.ResponseWriter, r *http.Request) (*dbrepo.PolicyGroup, bool) {
	name := r.PathValue("name")
	g, err := h.Repos.Groups.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, http.StatusNotFound, "policygroup "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get policygroup: "+err.Error())
		}
		return nil, false
	}
	return g, true
}

func groupSpecFrom(req *groupRequest) v1alpha1.PolicyGroupSpec {
	return v1alpha1.PolicyGroupSpec{
		DisplayName:             req.DisplayName,
		Description:             req.Description,
		Enabled:                 req.Enabled,
		NamespaceSelector:       req.NamespaceSelector,
		Policies:                req.Policies,
		SelectorEnforcementMode: req.SelectorEnforcementMode,
	}
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
	g := &dbrepo.PolicyGroup{Name: req.Name, Spec: datatypes.NewJSONType(groupSpecFrom(&req))}
	if err := h.Repos.Groups.Create(r.Context(), g); err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "policygroup "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create policygroup: "+err.Error())
		}
		return
	}
	h.notifyChange(r, "group")
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
	if req.ResourceVersion != "" && req.ResourceVersion != groupResourceVersion(g) {
		writeErr(w, http.StatusConflict, "policygroup has been modified, refresh and retry")
		return
	}
	// expectedUpdatedAt anchors the compare-and-swap to the exact row state
	// fetchGroup just read, closing the check-then-act race.
	expectedUpdatedAt := g.UpdatedAt
	g.Spec = datatypes.NewJSONType(groupSpecFrom(&req))
	err := h.Repos.Groups.UpdateCAS(r.Context(), g.ID, expectedUpdatedAt, map[string]any{"spec": g.Spec})
	if err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			writeErr(w, http.StatusConflict, "policygroup has been modified, refresh and retry")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update policygroup: "+err.Error())
		return
	}
	h.notifyChange(r, "group")
	writeOK(w, map[string]string{"name": g.Name})
}

// setGroupEnabled toggles spec.enabled — a narrower write than updateGroup so
// the quick switch in the list view cannot conflict with a concurrent full
// edit of other fields.
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
	spec := g.Spec.Data()
	if spec.Enabled != *req.Enabled {
		expectedUpdatedAt := g.UpdatedAt
		spec.Enabled = *req.Enabled
		g.Spec = datatypes.NewJSONType(spec)
		err := h.Repos.Groups.UpdateCAS(r.Context(), g.ID, expectedUpdatedAt, map[string]any{"spec": g.Spec})
		if err != nil {
			if errors.Is(err, dbrepo.ErrConflict) {
				writeErr(w, http.StatusConflict, "policygroup has been modified, refresh and retry")
				return
			}
			writeErr(w, http.StatusInternalServerError, "update policygroup: "+err.Error())
			return
		}
		h.notifyChange(r, "group")
	}
	writeOK(w, map[string]bool{"enabled": *req.Enabled})
}

func (h *Handlers) deleteGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := h.fetchGroup(w, r)
	if !ok {
		return
	}
	if g.Name == defaultGroupName {
		writeErr(w, http.StatusForbidden, "default policy group cannot be deleted")
		return
	}
	if err := h.Repos.Groups.Delete(r.Context(), g.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete policygroup: "+err.Error())
		return
	}
	h.notifyChange(r, "group")
	writeOK(w, nil)
}
