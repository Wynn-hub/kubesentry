package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

// ClusterImage describes one member-plane image used by the generated Helm
// command. It deliberately contains no credentials or registration token.
type ClusterImage struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	PullPolicy string `json:"pullPolicy"`
}

type ClusterInstallParams struct {
	Namespace            string       `json:"namespace"`
	WebhookImage         ClusterImage `json:"webhookImage"`
	OperatorImage        ClusterImage `json:"operatorImage"`
	HighAvailability     bool         `json:"highAvailability"`
	GatewayAddress       string       `json:"gatewayAddress"`
	GatewayTLSServerName string       `json:"gatewayTlsServerName,omitempty"`
	GatewayCABundle      string       `json:"gatewayCaBundle,omitempty"`
}

type CreateClusterRequest struct {
	Name          string               `json:"name"`
	Labels        map[string]string    `json:"labels,omitempty"`
	InstallParams ClusterInstallParams `json:"installParams"`
}

type ReissueClusterTokenRequest struct {
	InstallParams ClusterInstallParams `json:"installParams"`
}

type UpdateClusterLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}

type ClusterListItem struct {
	Name              string     `json:"name"`
	Phase             string     `json:"phase"`
	OperatorVersion   string     `json:"operatorVersion"`
	KubernetesVersion string     `json:"kubernetesVersion"`
	NodeCount         int        `json:"nodeCount"`
	ReadyNodeCount    int        `json:"readyNodeCount"`
	LastHeartbeatAt   *time.Time `json:"lastHeartbeatAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type ClusterDetail struct {
	Name              string            `json:"name"`
	ClusterUID        string            `json:"clusterUid"`
	Phase             string            `json:"phase"`
	OperatorVersion   string            `json:"operatorVersion"`
	LastHeartbeatAt   *time.Time        `json:"lastHeartbeatAt,omitempty"`
	KubernetesVersion string            `json:"kubernetesVersion"`
	APIServerAddress  string            `json:"apiServerAddress"`
	NodeCount         int               `json:"nodeCount"`
	ReadyNodeCount    int               `json:"readyNodeCount"`
	Labels            map[string]string `json:"labels"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

type InstallRecordItem struct {
	ClusterName        string    `json:"clusterName"`
	Namespace          string    `json:"namespace"`
	WebhookRepository  string    `json:"webhookRepository"`
	WebhookTag         string    `json:"webhookTag"`
	OperatorRepository string    `json:"operatorRepository"`
	OperatorTag        string    `json:"operatorTag"`
	HighAvailability   bool      `json:"highAvailability"`
	GatewayAddress     string    `json:"gatewayAddress"`
	CreatedAt          time.Time `json:"createdAt"`
}

type UninstallTaskItem struct {
	ID          int64      `json:"id"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	RequestedAt time.Time  `json:"requestedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type ClusterPage struct {
	Items    []ClusterListItem `json:"items"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

type InstallRecordPage struct {
	Items    []InstallRecordItem `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

type ClusterService struct {
	repos    repository.Repositories
	notifier Notifier
}

func NewClusterService(repos repository.Repositories, notifier Notifier) *ClusterService {
	return &ClusterService{repos: repos, notifier: notifier}
}

func (s *ClusterService) List(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*ClusterPage, error) {
	rows, total, err := s.repos.Clusters.ListPage(ctx, dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"})
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	items := make([]ClusterListItem, 0, len(rows))
	for i := range rows {
		c := rows[i]
		items = append(items, ClusterListItem{Name: c.Name, Phase: c.Phase, OperatorVersion: c.OperatorVersion, KubernetesVersion: c.KubernetesVersion, NodeCount: c.NodeCount, ReadyNodeCount: c.ReadyNodeCount, LastHeartbeatAt: c.LastHeartbeatAt, CreatedAt: c.CreatedAt})
	}
	return &ClusterPage{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (s *ClusterService) Get(ctx context.Context, name string) (*ClusterDetail, error) {
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return &ClusterDetail{Name: c.Name, ClusterUID: c.ClusterUID, Phase: c.Phase, OperatorVersion: c.OperatorVersion, LastHeartbeatAt: c.LastHeartbeatAt, KubernetesVersion: c.KubernetesVersion, APIServerAddress: c.APIServerAddress, NodeCount: c.NodeCount, ReadyNodeCount: c.ReadyNodeCount, Labels: c.Labels.Data(), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}, nil
}

func (s *ClusterService) Create(ctx context.Context, req CreateClusterRequest) (string, string, error) {
	if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
		return "", "", &ValidationError{Msg: "invalid name: " + strings.Join(errs, "; ")}
	}
	if err := validateInstallParams(&req.InstallParams); err != nil {
		return "", "", err
	}
	token, err := gateway.NewToken()
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	c := &dbrepo.Cluster{Name: req.Name, Labels: datatypes.NewJSONType(req.Labels), Phase: v1alpha1.ClusterPhaseRegistering, RegTokenHash: gateway.HashToken(token)}
	if err := s.repos.Clusters.Create(ctx, c); err != nil {
		if dbrepo.IsUniqueViolation(err) {
			return "", "", newConflict("cluster %s already exists", req.Name)
		}
		return "", "", fmt.Errorf("create cluster: %w", err)
	}
	if err := s.recordInstall(ctx, req.Name, &req.InstallParams); err != nil {
		return "", "", fmt.Errorf("record install: %w", err)
	}
	return c.Name, buildInstallCommand(c.Name, token, &req.InstallParams), nil
}

func (s *ClusterService) UpdateLabels(ctx context.Context, name string, req UpdateClusterLabelsRequest) error {
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("get cluster: %w", err)
	}
	if err := s.repos.Clusters.UpdateLabels(ctx, c.ID, req.Labels); err != nil {
		return fmt.Errorf("update cluster labels: %w", err)
	}
	return nil
}

func (s *ClusterService) Delete(ctx context.Context, name string) error {
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("get cluster: %w", err)
	}
	if c.Phase == v1alpha1.ClusterPhaseOnline || c.Phase == v1alpha1.ClusterPhaseUninstalling {
		return newConflict("online or uninstalling cluster cannot be deleted from here")
	}
	if err := s.repos.Clusters.Delete(ctx, c.ID); err != nil {
		return fmt.Errorf("delete cluster: %w", err)
	}
	return nil
}

func (s *ClusterService) ReissueToken(ctx context.Context, name string, req ReissueClusterTokenRequest) (string, string, error) {
	if err := validateInstallParams(&req.InstallParams); err != nil {
		return "", "", err
	}
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrNotFound
		}
		return "", "", fmt.Errorf("get cluster: %w", err)
	}
	if c.Phase != v1alpha1.ClusterPhaseRegistering {
		return "", "", newConflict("token can only be reissued while the cluster is Registering")
	}
	token, err := gateway.NewToken()
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	if err := s.repos.Clusters.UpdateRegToken(ctx, c.ID, gateway.HashToken(token)); err != nil {
		return "", "", fmt.Errorf("reissue token: %w", err)
	}
	if err := s.recordInstall(ctx, c.Name, &req.InstallParams); err != nil {
		return "", "", fmt.Errorf("record install: %w", err)
	}
	return c.Name, buildInstallCommand(c.Name, token, &req.InstallParams), nil
}

func (s *ClusterService) LatestInstallParams(ctx context.Context, name string) (*ClusterInstallParams, error) {
	rec, err := s.repos.InstallRecords.LatestForCluster(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("latest install params: %w", err)
	}
	if rec == nil {
		return nil, nil
	}
	return &ClusterInstallParams{Namespace: rec.Namespace, WebhookImage: ClusterImage{Repository: rec.WebhookRepository, Tag: rec.WebhookTag, PullPolicy: rec.WebhookPullPolicy}, OperatorImage: ClusterImage{Repository: rec.OperatorRepository, Tag: rec.OperatorTag, PullPolicy: rec.OperatorPullPolicy}, HighAvailability: rec.HighAvailability, GatewayAddress: rec.GatewayAddress, GatewayTLSServerName: rec.GatewayTLSServerName, GatewayCABundle: rec.GatewayCABundle}, nil
}

func (s *ClusterService) CreateUninstall(ctx context.Context, name string) (*UninstallTaskItem, error) {
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if c.Phase != v1alpha1.ClusterPhaseOnline {
		return nil, newConflict("only an online cluster can be uninstalled")
	}
	active, err := s.repos.UninstallTasks.HasActive(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("check uninstall task: %w", err)
	}
	if active {
		return nil, newConflict("cluster already has an active uninstall task")
	}
	task := &dbrepo.UninstallTask{ClusterID: c.ID}
	if err := s.repos.UninstallTasks.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("create uninstall task: %w", err)
	}
	if err := s.repos.Clusters.UpdatePhase(ctx, c.ID, v1alpha1.ClusterPhaseUninstalling); err != nil {
		return nil, fmt.Errorf("mark cluster uninstalling: %w", err)
	}
	s.notify("cluster-uninstall")
	return &UninstallTaskItem{ID: task.ID, Status: task.Status, RequestedAt: task.RequestedAt}, nil
}

func (s *ClusterService) ListUninstall(ctx context.Context, name string) ([]UninstallTaskItem, error) {
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.repos.UninstallTasks.ListByClusterID(ctx, c.ID)
	if err != nil {
		return nil, fmt.Errorf("list uninstall tasks: %w", err)
	}
	out := make([]UninstallTaskItem, 0, len(rows))
	for _, t := range rows {
		out = append(out, UninstallTaskItem{ID: t.ID, Status: t.Status, Error: t.Error, RequestedAt: t.RequestedAt, CompletedAt: t.CompletedAt})
	}
	return out, nil
}

func (s *ClusterService) RetryUninstall(ctx context.Context, name string, taskID int64) error {
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	task, err := s.repos.UninstallTasks.GetByID(ctx, taskID)
	if err != nil || task.ClusterID != c.ID {
		return ErrNotFound
	}
	if err := s.repos.UninstallTasks.Retry(ctx, taskID, time.Now().UTC()); err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			return newConflict("only a failed uninstall task can be retried")
		}
		return fmt.Errorf("retry uninstall task: %w", err)
	}
	if err := s.repos.Clusters.UpdatePhase(ctx, c.ID, v1alpha1.ClusterPhaseUninstalling); err != nil {
		return fmt.Errorf("mark cluster uninstalling: %w", err)
	}
	s.notify("cluster-uninstall-retry")
	return nil
}

func (s *ClusterService) ListInstallRecords(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*InstallRecordPage, error) {
	rows, total, err := s.repos.InstallRecords.ListPage(ctx, dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"})
	if err != nil {
		return nil, fmt.Errorf("list install records: %w", err)
	}
	items := make([]InstallRecordItem, 0, len(rows))
	for _, rec := range rows {
		items = append(items, InstallRecordItem{ClusterName: rec.ClusterName, Namespace: rec.Namespace, WebhookRepository: rec.WebhookRepository, WebhookTag: rec.WebhookTag, OperatorRepository: rec.OperatorRepository, OperatorTag: rec.OperatorTag, HighAvailability: rec.HighAvailability, GatewayAddress: rec.GatewayAddress, CreatedAt: rec.CreatedAt})
	}
	return &InstallRecordPage{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (s *ClusterService) recordInstall(ctx context.Context, clusterName string, p *ClusterInstallParams) error {
	return s.repos.InstallRecords.Create(ctx, &dbrepo.ClusterInstallRecord{ClusterName: clusterName, Namespace: p.Namespace, WebhookRepository: p.WebhookImage.Repository, WebhookTag: p.WebhookImage.Tag, WebhookPullPolicy: p.WebhookImage.PullPolicy, OperatorRepository: p.OperatorImage.Repository, OperatorTag: p.OperatorImage.Tag, OperatorPullPolicy: p.OperatorImage.PullPolicy, HighAvailability: p.HighAvailability, GatewayAddress: p.GatewayAddress, GatewayTLSServerName: p.GatewayTLSServerName, GatewayCABundle: p.GatewayCABundle})
}

func (s *ClusterService) notify(topic string) {
	if s.notifier != nil {
		s.notifier.NotifyChange(context.Background(), topic)
	}
}

const dangerousShellChars = "`$\\\""

func validateInstallParams(p *ClusterInstallParams) error {
	if errs := validation.IsDNS1123Subdomain(p.Namespace); len(errs) > 0 {
		return &ValidationError{Msg: "invalid namespace: " + strings.Join(errs, "; ")}
	}
	valid := map[string]bool{"Always": true, "IfNotPresent": true, "Never": true}
	if !valid[p.WebhookImage.PullPolicy] || !valid[p.OperatorImage.PullPolicy] {
		return &ValidationError{Msg: "pullPolicy must be one of Always, IfNotPresent, Never"}
	}
	fields := map[string]string{"webhookImage.repository": p.WebhookImage.Repository, "webhookImage.tag": p.WebhookImage.Tag, "operatorImage.repository": p.OperatorImage.Repository, "operatorImage.tag": p.OperatorImage.Tag, "gatewayAddress": p.GatewayAddress, "gatewayTlsServerName": p.GatewayTLSServerName, "gatewayCaBundle": p.GatewayCABundle}
	for name, value := range fields {
		if strings.ContainsAny(value, dangerousShellChars) {
			return &ValidationError{Msg: fmt.Sprintf("%s contains an unsupported character (one of %q)", name, dangerousShellChars)}
		}
	}
	if p.WebhookImage.Repository == "" || p.WebhookImage.Tag == "" || p.OperatorImage.Repository == "" || p.OperatorImage.Tag == "" || p.GatewayAddress == "" {
		return &ValidationError{Msg: "webhookImage.repository/tag, operatorImage.repository/tag, and gatewayAddress are required"}
	}
	return nil
}

func buildInstallCommand(clusterName, token string, p *ClusterInstallParams) string {
	var b strings.Builder
	fmt.Fprint(&b, "helm upgrade --install kubesentry charts/kubesentry \\\n+  --namespace "+p.Namespace+" --create-namespace \\\n+")
	fmt.Fprintf(&b, "  --set webhook.image.repository=%q \\\n+", p.WebhookImage.Repository)
	fmt.Fprintf(&b, "  --set webhook.image.tag=%q \\\n+", p.WebhookImage.Tag)
	fmt.Fprintf(&b, "  --set webhook.image.pullPolicy=%s \\\n+", p.WebhookImage.PullPolicy)
	fmt.Fprintf(&b, "  --set operator.image.repository=%q \\\n+", p.OperatorImage.Repository)
	fmt.Fprintf(&b, "  --set operator.image.tag=%q \\\n+", p.OperatorImage.Tag)
	fmt.Fprintf(&b, "  --set operator.image.pullPolicy=%s \\\n+", p.OperatorImage.PullPolicy)
	if p.HighAvailability {
		fmt.Fprint(&b, "  --set webhook.highAvailability.enabled=true \\\n  --set operator.highAvailability.enabled=true \\\n")
	}
	fmt.Fprintf(&b, "  --set gateway.address=%q \\\n", p.GatewayAddress)
	if p.GatewayTLSServerName != "" {
		fmt.Fprintf(&b, "  --set gateway.tlsServerName=%q \\\n", p.GatewayTLSServerName)
	}
	if p.GatewayCABundle != "" {
		fmt.Fprintf(&b, "  --set gateway.caBundle=%q \\\n", p.GatewayCABundle)
	}
	fmt.Fprintf(&b, "  --set gateway.clusterName=%q \\\n  --set gateway.registrationToken=%q \\\n  --wait --timeout 5m\n", clusterName, token)
	return b.String()
}
