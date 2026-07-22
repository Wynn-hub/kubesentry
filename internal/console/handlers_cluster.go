// internal/console/handlers_cluster.go
package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

type imageParams struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	PullPolicy string `json:"pullPolicy"`
}

type installParams struct {
	Namespace            string      `json:"namespace"`
	WebhookImage         imageParams `json:"webhookImage"`
	OperatorImage        imageParams `json:"operatorImage"`
	GatewayAddress       string      `json:"gatewayAddress"`
	GatewayTLSServerName string      `json:"gatewayTlsServerName,omitempty"`
	GatewayCABundle      string      `json:"gatewayCaBundle,omitempty"`
}

// dangerousShellChars are characters that could break out of a
// double-quoted `--set key="value"` argument in the generated helm command
// (which the user copies and runs verbatim in their own shell). Rejecting
// them outright is simpler and sufficient here — this is not a general
// shell-escaping facility, it's a guard against a free-text field silently
// producing a command that does something other than what it looks like.
const dangerousShellChars = "`$\"\\"

func validateInstallParams(p *installParams) error {
	if errs := validation.IsDNS1123Subdomain(p.Namespace); len(errs) > 0 {
		return fmt.Errorf("invalid namespace: %s", strings.Join(errs, "; "))
	}
	// PullPolicy is interpolated into the command WITHOUT quotes
	// (`--set ...pullPolicy=%s`, see buildInstallCommand) — unlike the
	// quoted fields below, an unvalidated value here has no quote boundary
	// to contain it at all, so it's checked against the exact Kubernetes
	// enum rather than just screened for shell metacharacters.
	validPullPolicy := map[string]bool{"Always": true, "IfNotPresent": true, "Never": true}
	if !validPullPolicy[p.WebhookImage.PullPolicy] || !validPullPolicy[p.OperatorImage.PullPolicy] {
		return fmt.Errorf("pullPolicy must be one of Always, IfNotPresent, Never")
	}
	fields := map[string]string{
		"webhookImage.repository":  p.WebhookImage.Repository,
		"webhookImage.tag":         p.WebhookImage.Tag,
		"operatorImage.repository": p.OperatorImage.Repository,
		"operatorImage.tag":        p.OperatorImage.Tag,
		"gatewayAddress":           p.GatewayAddress,
		"gatewayTlsServerName":     p.GatewayTLSServerName,
		"gatewayCaBundle":          p.GatewayCABundle,
	}
	for name, v := range fields {
		if strings.ContainsAny(v, dangerousShellChars) {
			return fmt.Errorf("%s contains an unsupported character (one of %q)", name, dangerousShellChars)
		}
	}
	if p.WebhookImage.Repository == "" || p.WebhookImage.Tag == "" ||
		p.OperatorImage.Repository == "" || p.OperatorImage.Tag == "" ||
		p.GatewayAddress == "" {
		return fmt.Errorf("webhookImage.repository/tag, operatorImage.repository/tag, and gatewayAddress are required")
	}
	return nil
}

// buildInstallCommand mirrors the exact --set flag list scripts/deploy-local.sh
// uses for `helm upgrade --install charts/kubesentry`. gateway.caBundle/
// tlsServerName are omitted entirely when empty — the operator falls back to
// tlssetup.DefaultCertBundle in code when no --set overrides them (see
// internal/operator/hublink/client.go's transportCredentials).
func buildInstallCommand(clusterName, token string, p *installParams) string {
	var b strings.Builder
	fmt.Fprintf(&b, "helm upgrade --install kubesentry charts/kubesentry \\\n")
	fmt.Fprintf(&b, "  --namespace %s --create-namespace \\\n", p.Namespace)
	fmt.Fprintf(&b, "  --set webhook.image.repository=%q \\\n", p.WebhookImage.Repository)
	fmt.Fprintf(&b, "  --set webhook.image.tag=%q \\\n", p.WebhookImage.Tag)
	fmt.Fprintf(&b, "  --set webhook.image.pullPolicy=%s \\\n", p.WebhookImage.PullPolicy)
	fmt.Fprintf(&b, "  --set operator.image.repository=%q \\\n", p.OperatorImage.Repository)
	fmt.Fprintf(&b, "  --set operator.image.tag=%q \\\n", p.OperatorImage.Tag)
	fmt.Fprintf(&b, "  --set operator.image.pullPolicy=%s \\\n", p.OperatorImage.PullPolicy)
	fmt.Fprintf(&b, "  --set gateway.address=%q \\\n", p.GatewayAddress)
	if p.GatewayTLSServerName != "" {
		fmt.Fprintf(&b, "  --set gateway.tlsServerName=%q \\\n", p.GatewayTLSServerName)
	}
	if p.GatewayCABundle != "" {
		fmt.Fprintf(&b, "  --set gateway.caBundle=%q \\\n", p.GatewayCABundle)
	}
	fmt.Fprintf(&b, "  --set gateway.clusterName=%q \\\n", clusterName)
	fmt.Fprintf(&b, "  --set gateway.registrationToken=%q \\\n", token)
	fmt.Fprintf(&b, "  --wait --timeout 5m\n")
	return b.String()
}

type createClusterRequest struct {
	Name          string            `json:"name"`
	Labels        map[string]string `json:"labels,omitempty"`
	InstallParams installParams     `json:"installParams"`
}

// createClusterResponse's Command is the only place a registration token's
// plaintext ever leaves the server (embedded in the full helm command) —
// returned once by createCluster and once more by reissueClusterToken,
// never included in clusterDetail/clusterListItem.
type createClusterResponse struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type reissueTokenRequest struct {
	InstallParams installParams `json:"installParams"`
}

type updateClusterLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}

type installRecordItem struct {
	ClusterName        string    `json:"clusterName"`
	Namespace          string    `json:"namespace"`
	WebhookRepository  string    `json:"webhookRepository"`
	WebhookTag         string    `json:"webhookTag"`
	OperatorRepository string    `json:"operatorRepository"`
	OperatorTag        string    `json:"operatorTag"`
	GatewayAddress     string    `json:"gatewayAddress"`
	CreatedAt          time.Time `json:"createdAt"`
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
	query, err := parsePageQuery(r.URL.Query(), pageSpec{
		DefaultSortBy:    "name",
		DefaultSortOrder: "asc",
		AllowedSort:      stringSet("name", "phase", "operatorVersion", "lastHeartbeatAt", "createdAt"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	clusters, total, err := h.Repos.Clusters.ListPage(r.Context(), dbrepo.PageOptions{
		Limit: query.PageSize, Offset: query.offset(), SortBy: query.SortBy, Desc: query.SortOrder == "desc",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list clusters: "+err.Error())
		return
	}
	items := make([]clusterListItem, 0, len(clusters))
	for i := range clusters {
		items = append(items, toClusterListItem(&clusters[i]))
	}
	writeOK(w, pageResult[clusterListItem]{
		Items: items, Total: int(total), Page: query.Page, PageSize: query.PageSize,
	})
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
	if err := validateInstallParams(&req.InstallParams); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
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
	if err := h.recordInstall(r.Context(), req.Name, &req.InstallParams); err != nil {
		writeErr(w, http.StatusInternalServerError, "record install: "+err.Error())
		return
	}
	writeOK(w, createClusterResponse{Name: c.Name, Command: buildInstallCommand(c.Name, token, &req.InstallParams)})
}

// recordInstall persists the non-secret install params — never the token.
func (h *Handlers) recordInstall(ctx context.Context, clusterName string, p *installParams) error {
	return h.Repos.InstallRecords.Create(ctx, &dbrepo.ClusterInstallRecord{
		ClusterName:          clusterName,
		Namespace:            p.Namespace,
		WebhookRepository:    p.WebhookImage.Repository,
		WebhookTag:           p.WebhookImage.Tag,
		WebhookPullPolicy:    p.WebhookImage.PullPolicy,
		OperatorRepository:   p.OperatorImage.Repository,
		OperatorTag:          p.OperatorImage.Tag,
		OperatorPullPolicy:   p.OperatorImage.PullPolicy,
		GatewayAddress:       p.GatewayAddress,
		GatewayTLSServerName: p.GatewayTLSServerName,
		GatewayCABundle:      p.GatewayCABundle,
	})
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
	var req reissueTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := validateInstallParams(&req.InstallParams); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
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
	if err := h.recordInstall(r.Context(), c.Name, &req.InstallParams); err != nil {
		writeErr(w, http.StatusInternalServerError, "record install: "+err.Error())
		return
	}
	writeOK(w, createClusterResponse{Name: c.Name, Command: buildInstallCommand(c.Name, token, &req.InstallParams)})
}

func (h *Handlers) getLatestInstallParams(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	rec, err := h.Repos.InstallRecords.LatestForCluster(r.Context(), name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "latest install params: "+err.Error())
		return
	}
	if rec == nil {
		writeOK(w, nil)
		return
	}
	writeOK(w, installParams{
		Namespace:            rec.Namespace,
		WebhookImage:         imageParams{Repository: rec.WebhookRepository, Tag: rec.WebhookTag, PullPolicy: rec.WebhookPullPolicy},
		OperatorImage:        imageParams{Repository: rec.OperatorRepository, Tag: rec.OperatorTag, PullPolicy: rec.OperatorPullPolicy},
		GatewayAddress:       rec.GatewayAddress,
		GatewayTLSServerName: rec.GatewayTLSServerName,
		GatewayCABundle:      rec.GatewayCABundle,
	})
}

func (h *Handlers) listInstallRecords(w http.ResponseWriter, r *http.Request) {
	query, err := parsePageQuery(r.URL.Query(), pageSpec{
		DefaultSortBy:    "createdAt",
		DefaultSortOrder: "desc",
		AllowedSort:      stringSet("clusterName", "namespace", "createdAt"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	records, total, err := h.Repos.InstallRecords.ListPage(r.Context(), dbrepo.PageOptions{
		Limit: query.PageSize, Offset: query.offset(), SortBy: query.SortBy, Desc: query.SortOrder == "desc",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list install records: "+err.Error())
		return
	}
	items := make([]installRecordItem, 0, len(records))
	for i := range records {
		rec := &records[i]
		items = append(items, installRecordItem{
			ClusterName:        rec.ClusterName,
			Namespace:          rec.Namespace,
			WebhookRepository:  rec.WebhookRepository,
			WebhookTag:         rec.WebhookTag,
			OperatorRepository: rec.OperatorRepository,
			OperatorTag:        rec.OperatorTag,
			GatewayAddress:     rec.GatewayAddress,
			CreatedAt:          rec.CreatedAt,
		})
	}
	writeOK(w, pageResult[installRecordItem]{
		Items: items, Total: int(total), Page: query.Page, PageSize: query.PageSize,
	})
}
