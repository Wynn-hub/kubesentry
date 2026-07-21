// internal/console/handlers_cluster.go
package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

type clusterListItem struct {
	Name            string     `json:"name"`
	Phase           string     `json:"phase"`
	OperatorVersion string     `json:"operatorVersion"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type clusterDetail struct {
	Name            string            `json:"name"`
	ClusterUID      string            `json:"clusterUid"`
	Phase           string            `json:"phase"`
	OperatorVersion string            `json:"operatorVersion"`
	LastHeartbeatAt *time.Time        `json:"lastHeartbeatAt,omitempty"`
	Labels          map[string]string `json:"labels"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

type createClusterRequest struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// createClusterResponse is the only place a registration token's plaintext
// ever leaves the server — returned once by createCluster and once more by
// reissueClusterToken, never included in clusterDetail/clusterListItem.
type createClusterResponse struct {
	Name  string `json:"name"`
	Token string `json:"token"`
}

type updateClusterLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}

func toClusterListItem(c *dbrepo.Cluster) clusterListItem {
	return clusterListItem{
		Name:            c.Name,
		Phase:           c.Phase,
		OperatorVersion: c.OperatorVersion,
		LastHeartbeatAt: c.LastHeartbeatAt,
		CreatedAt:       c.CreatedAt,
	}
}

func toClusterDetail(c *dbrepo.Cluster) clusterDetail {
	return clusterDetail{
		Name:            c.Name,
		ClusterUID:      c.ClusterUID,
		Phase:           c.Phase,
		OperatorVersion: c.OperatorVersion,
		LastHeartbeatAt: c.LastHeartbeatAt,
		Labels:          c.Labels.Data(),
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

func (h *Handlers) listClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := h.Repos.Clusters.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list clusters: "+err.Error())
		return
	}
	items := make([]clusterListItem, 0, len(clusters))
	for i := range clusters {
		items = append(items, toClusterListItem(&clusters[i]))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	writeOK(w, items)
}

func (h *Handlers) getCluster(w http.ResponseWriter, r *http.Request) {
	c, ok := h.fetchCluster(w, r)
	if !ok {
		return
	}
	writeOK(w, toClusterDetail(c))
}

// fetchCluster loads the {name} path cluster, writing 404/500 on failure.
func (h *Handlers) fetchCluster(w http.ResponseWriter, r *http.Request) (*dbrepo.Cluster, bool) {
	name := r.PathValue("name")
	c, err := h.Repos.Clusters.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, http.StatusNotFound, "cluster "+name+" not found")
		} else {
			writeErr(w, http.StatusInternalServerError, "get cluster: "+err.Error())
		}
		return nil, false
	}
	return c, true
}

func (h *Handlers) createCluster(w http.ResponseWriter, r *http.Request) {
	var req createClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
		writeErr(w, http.StatusBadRequest, "invalid name: "+strings.Join(errs, "; "))
		return
	}
	token, err := gateway.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "generate token: "+err.Error())
		return
	}
	c := &dbrepo.Cluster{
		Name:         req.Name,
		Labels:       datatypes.NewJSONType(req.Labels),
		Phase:        v1alpha1.ClusterPhaseRegistering,
		RegTokenHash: gateway.HashToken(token),
	}
	if err := h.Repos.Clusters.Create(r.Context(), c); err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "cluster "+req.Name+" already exists")
		} else {
			writeErr(w, http.StatusInternalServerError, "create cluster: "+err.Error())
		}
		return
	}
	writeOK(w, createClusterResponse{Name: c.Name, Token: token})
}

// updateClusterLabels only ever writes the labels column — see
// ClusterRepo.UpdateLabels for why a full-row update is unsafe here.
func (h *Handlers) updateClusterLabels(w http.ResponseWriter, r *http.Request) {
	var req updateClusterLabelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	c, ok := h.fetchCluster(w, r)
	if !ok {
		return
	}
	if err := h.Repos.Clusters.UpdateLabels(r.Context(), c.ID, req.Labels); err != nil {
		writeErr(w, http.StatusInternalServerError, "update cluster labels: "+err.Error())
		return
	}
	writeOK(w, map[string]string{"name": c.Name})
}

func (h *Handlers) deleteCluster(w http.ResponseWriter, r *http.Request) {
	c, ok := h.fetchCluster(w, r)
	if !ok {
		return
	}
	if c.Phase == v1alpha1.ClusterPhaseOnline {
		writeErr(w, http.StatusConflict, "online cluster cannot be deleted from here")
		return
	}
	if err := h.Repos.Clusters.Delete(r.Context(), c.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete cluster: "+err.Error())
		return
	}
	writeOK(w, nil)
}

func (h *Handlers) reissueClusterToken(w http.ResponseWriter, r *http.Request) {
	c, ok := h.fetchCluster(w, r)
	if !ok {
		return
	}
	if c.Phase != v1alpha1.ClusterPhaseRegistering {
		writeErr(w, http.StatusConflict, "token can only be reissued while the cluster is Registering")
		return
	}
	token, err := gateway.NewToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "generate token: "+err.Error())
		return
	}
	if err := h.Repos.Clusters.UpdateRegToken(r.Context(), c.ID, gateway.HashToken(token)); err != nil {
		writeErr(w, http.StatusInternalServerError, "reissue token: "+err.Error())
		return
	}
	writeOK(w, createClusterResponse{Name: c.Name, Token: token})
}
