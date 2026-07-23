package console

import (
	"cmp"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
)

// defaultGroupName is the platform baseline PolicyGroup, preloaded by the
// gateway chart and referenced by the reserved "default" PolicyDistribution —
// deleting it would silently drop baseline enforcement for every registered
// cluster.
const defaultGroupName = "default"

type groupListItem struct {
	Name          string `json:"name"`
	Source        string `json:"source"`
	DisplayName   string `json:"displayName"`
	Enabled       bool   `json:"enabled"`
	Phase         string `json:"phase"`
	ResolvedCount int    `json:"resolvedCount"`
}

// groupStatus mirrors the CRD-era status subresource, but per spec D7 it is
// never stored — always computed live from groupresolve.Resolve against the
// current policy list, same as the summary endpoint.
type groupStatus struct {
	Phase            string                     `json:"phase"`
	ResolvedCount    int                        `json:"resolvedCount"`
	ResolvedPolicies []v1alpha1.EffectiveMember `json:"resolvedPolicies"`
}

type groupDetail struct {
	Name            string                   `json:"name"`
	Source          string                   `json:"source"`
	ResourceVersion string                   `json:"resourceVersion"`
	Spec            v1alpha1.PolicyGroupSpec `json:"spec"`
	Status          groupStatus              `json:"status"`
	Scope           clusterScope             `json:"scope"`
}

// groupPhase reports "Ready" unless resolution produced a condition (a
// missing byName reference or a malformed selector) — mirrors the CRD-era
// PolicyGroupReconciler's status.phase computation, now derived at read time.
func groupPhase(conditions []metav1.Condition) string {
	if len(conditions) > 0 {
		return v1alpha1.PhaseInvalid
	}
	return v1alpha1.PhaseReady
}

// groupResourceVersion derives an opaque optimistic-concurrency token from
// the row's UpdatedAt, mirroring the CRD-era resourceVersion contract
// (client echoes it back on update; a mismatch means someone else wrote in
// between) without needing a dedicated column.
func groupResourceVersion(g *dbrepo.PolicyGroup) string {
	return strconv.FormatInt(g.UpdatedAt.UnixNano(), 10)
}

func toGroupDetail(g *dbrepo.PolicyGroup, policies []v1alpha1.Policy) groupDetail {
	spec := g.Spec.Data()
	members, conditions := groupresolve.Resolve(spec, policies)
	return groupDetail{
		Name:            g.Name,
		Source:          policySource(g.Labels.Data()),
		ResourceVersion: groupResourceVersion(g),
		Spec:            spec,
		Status: groupStatus{
			Phase:            groupPhase(conditions),
			ResolvedCount:    len(members),
			ResolvedPolicies: members,
		},
	}
}

func (h *Handlers) listGroups(w http.ResponseWriter, r *http.Request) {
	query, err := parsePageQuery(r.URL.Query(), pageSpec{
		DefaultSortBy:    "name",
		DefaultSortOrder: "asc",
		AllowedSort:      stringSet("name", "displayName", "enabled", "phase", "resolvedCount"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	groups, err := h.Repos.Groups.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list groups: "+err.Error())
		return
	}
	policies, err := h.Repos.Policies.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	resolvePolicies := resolvablePolicies(policies)
	items := make([]groupListItem, 0, len(groups))
	for i := range groups {
		g := &groups[i]
		spec := g.Spec.Data()
		members, conditions := groupresolve.Resolve(spec, resolvePolicies)
		items = append(items, groupListItem{
			Name:          g.Name,
			Source:        policySource(g.Labels.Data()),
			DisplayName:   spec.DisplayName,
			Enabled:       spec.Enabled,
			Phase:         groupPhase(conditions),
			ResolvedCount: len(members),
		})
	}
	sortPageItems(items, query, func(a, b groupListItem) int {
		switch query.SortBy {
		case "displayName":
			return cmp.Compare(a.DisplayName, b.DisplayName)
		case "enabled":
			if a.Enabled == b.Enabled {
				return 0
			}
			if !a.Enabled {
				return -1
			}
			return 1
		case "phase":
			return cmp.Compare(a.Phase, b.Phase)
		case "resolvedCount":
			return cmp.Compare(a.ResolvedCount, b.ResolvedCount)
		default:
			return cmp.Compare(a.Name, b.Name)
		}
	}, func(a, b groupListItem) int {
		return cmp.Compare(a.Name, b.Name)
	})
	writeOK(w, paginate(items, query))
}

func (h *Handlers) getGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := h.fetchGroup(w, r)
	if !ok {
		return
	}
	policies, err := h.Repos.Policies.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	detail := toGroupDetail(g, resolvablePolicies(policies))
	scope, err := h.getScopeBinding(r.Context(), scopeOwnerGroup, g.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get policy group scope: "+err.Error())
		return
	}
	detail.Scope = scope
	writeOK(w, detail)
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
	if err := h.validateScopeTargets(r.Context(), req.AllClusters, req.ClusterNames); err != nil {
		writeErr(w, http.StatusBadRequest, "set policy group scope: "+err.Error())
		return
	}
	if err := h.Repos.Groups.WithTx(r.Context(), func(groups *dbrepo.GroupRepo, distributions *dbrepo.DistributionRepo) error {
		if err := groups.Create(r.Context(), g); err != nil {
			return err
		}
		return syncScopeBinding(r.Context(), distributions, scopeOwnerGroup, g.Name, req.AllClusters, req.ClusterNames)
	}); err != nil {
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
	if g.Name == defaultGroupName {
		writeErr(w, http.StatusForbidden, "default policy group scope is managed; use the enabled endpoint to disable it")
		return
	}
	if req.ResourceVersion != "" && req.ResourceVersion != groupResourceVersion(g) {
		writeErr(w, http.StatusConflict, "policygroup has been modified, refresh and retry")
		return
	}
	if err := h.validateScopeTargets(r.Context(), req.AllClusters, req.ClusterNames); err != nil {
		writeErr(w, http.StatusBadRequest, "set policy group scope: "+err.Error())
		return
	}
	// expectedUpdatedAt anchors the compare-and-swap to the exact row state
	// fetchGroup just read, closing the check-then-act race.
	expectedUpdatedAt := g.UpdatedAt
	g.Spec = datatypes.NewJSONType(groupSpecFrom(&req))
	err := h.Repos.Groups.WithTx(r.Context(), func(groups *dbrepo.GroupRepo, distributions *dbrepo.DistributionRepo) error {
		if err := groups.UpdateCAS(r.Context(), g.ID, expectedUpdatedAt, map[string]any{"spec": g.Spec}); err != nil {
			return err
		}
		return syncScopeBinding(r.Context(), distributions, scopeOwnerGroup, g.Name, req.AllClusters, req.ClusterNames)
	})
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
	if err := h.Repos.Groups.WithTx(r.Context(), func(groups *dbrepo.GroupRepo, distributions *dbrepo.DistributionRepo) error {
		if err := groups.Delete(r.Context(), g.ID); err != nil {
			return err
		}
		return deleteScopeBinding(r.Context(), distributions, scopeOwnerGroup, g.Name)
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete policygroup: "+err.Error())
		return
	}
	h.notifyChange(r, "group")
	writeOK(w, nil)
}
