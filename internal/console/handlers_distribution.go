// internal/console/handlers_distribution.go
package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// defaultDistributionName is the platform baseline PolicyDistribution
// (empty cluster selector = matches every cluster, references the
// "default" PolicyGroup) — deleting it would silently drop baseline
// enforcement for every registered cluster.
const defaultDistributionName = "default"

type distributionListItem struct {
	Name         string   `json:"name"`
	PolicyGroups []string `json:"policyGroups"`
}

type distributionDetail struct {
	Name            string                          `json:"name"`
	ResourceVersion string                          `json:"resourceVersion"`
	Spec            v1alpha1.PolicyDistributionSpec `json:"spec"`
}

type distributionRequest struct {
	Name            string                          `json:"name,omitempty"`
	Spec            v1alpha1.PolicyDistributionSpec `json:"spec"`
	ResourceVersion string                          `json:"resourceVersion,omitempty"`
}

func toDistributionListItem(d *dbrepo.PolicyDistribution) distributionListItem {
	return distributionListItem{
		Name:         d.Name,
		PolicyGroups: d.Spec.Data().PolicyGroups,
	}
}

// distributionResourceVersion derives an opaque optimistic-concurrency
// token from the row's UpdatedAt, mirroring groupResourceVersion.
func distributionResourceVersion(d *dbrepo.PolicyDistribution) string {
	return strconv.FormatInt(d.UpdatedAt.UnixNano(), 10)
}

func toDistributionDetail(d *dbrepo.PolicyDistribution) distributionDetail {
	return distributionDetail{
		Name:            d.Name,
		ResourceVersion: distributionResourceVersion(d),
		Spec:            d.Spec.Data(),
	}
}

func (h *Handlers) listDistributions(w http.ResponseWriter, r *http.Request) {
	query, err := parsePageQuery(r.URL.Query(), pageSpec{
		DefaultSortBy:    "name",
		DefaultSortOrder: "asc",
		AllowedSort:      stringSet("name"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	dists, total, err := h.Repos.Distributions.ListPage(r.Context(), dbrepo.PageOptions{
		Limit: query.PageSize, Offset: query.offset(), SortBy: query.SortBy, Desc: query.SortOrder == "desc",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list policy distributions: "+err.Error())
		return
	}
	items := make([]distributionListItem, 0, len(dists))
	for i := range dists {
		items = append(items, toDistributionListItem(&dists[i]))
	}
	writeOK(w, pageResult[distributionListItem]{
		Items: items, Total: int(total), Page: query.Page, PageSize: query.PageSize,
	})
}

func (h *Handlers) getDistribution(w http.ResponseWriter, r *http.Request) {
	d, ok := h.fetchDistribution(w, r)
	if !ok {
		return
	}
	writeOK(w, toDistributionDetail(d))
}

// fetchDistribution loads the {name} path distribution, writing 404/500 on failure.
func (h *Handlers) fetchDistribution(w http.ResponseWriter, r *http.Request) (*dbrepo.PolicyDistribution, bool) {
	name := r.PathValue("name")
	d, err := h.Repos.Distributions.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, http.StatusNotFound, "policydistribution "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get policydistribution: "+err.Error())
		}
		return nil, false
	}
	return d, true
}

// validateDistributionRequest checks structural validity only — it
// deliberately does NOT verify that req.Spec.PolicyGroups entries reference
// existing PolicyGroups. See this plan's Global Constraints: dangling group
// references are tolerated at write time (matching validateGroupRequest's
// treatment of Policies.ByName), and internal/gateway/render.go's Render()
// already surfaces an unresolvable reference as a per-distribution error at
// render time without blocking other distributions.
func validateDistributionRequest(req *distributionRequest, isCreate bool) error {
	if isCreate {
		if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
			return errors.New("invalid name: " + strings.Join(errs, "; "))
		}
	}
	for _, g := range req.Spec.PolicyGroups {
		if g == "" {
			return errors.New("policyGroups entries must not be empty")
		}
	}
	return nil
}

func (h *Handlers) createDistribution(w http.ResponseWriter, r *http.Request) {
	var req distributionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateDistributionRequest(&req, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d := &dbrepo.PolicyDistribution{Name: req.Name, Spec: datatypes.NewJSONType(req.Spec)}
	if err := h.Repos.Distributions.Create(r.Context(), d); err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "policydistribution "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create policydistribution: "+err.Error())
		}
		return
	}
	h.notifyChange(r, "distribution")
	writeOK(w, map[string]string{"name": d.Name})
}

func (h *Handlers) updateDistribution(w http.ResponseWriter, r *http.Request) {
	var req distributionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateDistributionRequest(&req, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d, ok := h.fetchDistribution(w, r)
	if !ok {
		return
	}
	if req.ResourceVersion != "" && req.ResourceVersion != distributionResourceVersion(d) {
		writeErr(w, http.StatusConflict, "policydistribution has been modified, refresh and retry")
		return
	}
	expectedUpdatedAt := d.UpdatedAt
	d.Spec = datatypes.NewJSONType(req.Spec)
	err := h.Repos.Distributions.UpdateCAS(r.Context(), d.ID, expectedUpdatedAt, map[string]any{"spec": d.Spec})
	if err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			writeErr(w, http.StatusConflict, "policydistribution has been modified, refresh and retry")
			return
		}
		writeErr(w, http.StatusInternalServerError, "update policydistribution: "+err.Error())
		return
	}
	h.notifyChange(r, "distribution")
	writeOK(w, map[string]string{"name": d.Name})
}

func (h *Handlers) deleteDistribution(w http.ResponseWriter, r *http.Request) {
	d, ok := h.fetchDistribution(w, r)
	if !ok {
		return
	}
	if d.Name == defaultDistributionName {
		writeErr(w, http.StatusForbidden, "default policy distribution cannot be deleted")
		return
	}
	if err := h.Repos.Distributions.Delete(r.Context(), d.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete policydistribution: "+err.Error())
		return
	}
	h.notifyChange(r, "distribution")
	writeOK(w, nil)
}
