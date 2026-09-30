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

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewayauth"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

type ClusterService struct {
	repos    repository.Repositories
	notifier Notifier
}

func NewClusterService(repos repository.Repositories, notifier Notifier) *ClusterService {
	return &ClusterService{repos: repos, notifier: notifier}
}

func (s *ClusterService) List(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*dto.ClusterPageView, error) {
	options := dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"}
	principal := requestPrincipal(ctx)
	var rows []dbrepo.Cluster
	var total int64
	var err error
	if principal.Role == dbrepo.RoleSystemAdmin {
		rows, total, err = s.repos.Clusters.ListPage(ctx, options)
	} else if principal.TenantID != nil {
		rows, total, err = s.repos.Clusters.ListPageForTenant(ctx, *principal.TenantID, options)
	} else {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	items := make([]dto.ClusterItem, 0, len(rows))
	for i := range rows {
		c := rows[i]
		items = append(items, dto.ClusterItem{Name: c.Name, Phase: c.Phase, OperatorVersion: c.OperatorVersion, KubernetesVersion: c.KubernetesVersion, NodeCount: c.NodeCount, ReadyNodeCount: c.ReadyNodeCount, LastHeartbeatAt: c.LastHeartbeatAt, CreatedAt: c.CreatedAt, TenantID: c.TenantID})
	}
	return &dto.ClusterPageView{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (s *ClusterService) Get(ctx context.Context, name string) (*dto.ClusterView, error) {
	c, err := s.clusterForActor(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return &dto.ClusterView{Name: c.Name, ClusterUID: c.ClusterUID, Phase: c.Phase, OperatorVersion: c.OperatorVersion, LastHeartbeatAt: c.LastHeartbeatAt, KubernetesVersion: c.KubernetesVersion, APIServerAddress: c.APIServerAddress, NodeCount: c.NodeCount, ReadyNodeCount: c.ReadyNodeCount, Labels: c.Labels.Data(), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, TenantID: c.TenantID}, nil
}

func (s *ClusterService) Create(ctx context.Context, req dto.ClusterCreateInput) (string, string, error) {
	if err := requireSystemAdmin(ctx); err != nil {
		return "", "", err
	}
	if errs := validation.IsDNS1123Subdomain(req.Name); len(errs) > 0 {
		return "", "", &ValidationError{Msg: "invalid name: " + strings.Join(errs, "; ")}
	}
	if err := validateInstallParams(&req.InstallParams); err != nil {
		return "", "", err
	}
	token, err := gatewayauth.NewToken()
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	c := &dbrepo.Cluster{Name: req.Name, Labels: datatypes.NewJSONType(req.Labels), Phase: gatewaymodel.ClusterPhaseRegistering, RegTokenHash: gatewayauth.HashToken(token)}
	if err := s.repos.ClusterUOW.WithinTransaction(ctx, func(clusters repository.ClusterRepository, records repository.InstallRecordRepository, _ repository.UninstallTaskRepository) error {
		if err := clusters.Create(ctx, c); err != nil {
			return err
		}
		return records.Create(ctx, installRecord(req.Name, &req.InstallParams))
	}); err != nil {
		if dbrepo.IsUniqueViolation(err) {
			return "", "", newConflict("cluster %s already exists", req.Name)
		}
		return "", "", fmt.Errorf("create cluster: %w", err)
	}
	return c.Name, buildInstallCommand(c.Name, token, &req.InstallParams), nil
}

func (s *ClusterService) UpdateLabels(ctx context.Context, name string, req dto.ClusterLabelsInput) error {
	if err := requireSystemAdmin(ctx); err != nil {
		return err
	}
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
	if err := requireSystemAdmin(ctx); err != nil {
		return err
	}
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("get cluster: %w", err)
	}
	if c.Phase == gatewaymodel.ClusterPhaseOnline || c.Phase == gatewaymodel.ClusterPhaseUninstalling {
		return newConflict("online or uninstalling cluster cannot be deleted from here")
	}
	if err := s.repos.Clusters.Delete(ctx, c.ID); err != nil {
		return fmt.Errorf("delete cluster: %w", err)
	}
	return nil
}

func (s *ClusterService) ReissueToken(ctx context.Context, name string, req dto.ClusterTokenInput) (string, string, error) {
	if err := requireSystemAdmin(ctx); err != nil {
		return "", "", err
	}
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
	if c.Phase != gatewaymodel.ClusterPhaseRegistering {
		return "", "", newConflict("token can only be reissued while the cluster is Registering")
	}
	token, err := gatewayauth.NewToken()
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	if err := s.repos.ClusterUOW.WithinTransaction(ctx, func(clusters repository.ClusterRepository, records repository.InstallRecordRepository, _ repository.UninstallTaskRepository) error {
		if err := clusters.UpdateRegToken(ctx, c.ID, gatewayauth.HashToken(token)); err != nil {
			return err
		}
		return records.Create(ctx, installRecord(c.Name, &req.InstallParams))
	}); err != nil {
		return "", "", fmt.Errorf("reissue token: %w", err)
	}
	return c.Name, buildInstallCommand(c.Name, token, &req.InstallParams), nil
}

func (s *ClusterService) LatestInstallParams(ctx context.Context, name string) (*dto.ClusterInstallInput, error) {
	if err := requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	rec, err := s.repos.InstallRecords.LatestForCluster(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("latest install params: %w", err)
	}
	if rec == nil {
		return nil, nil
	}
	return &dto.ClusterInstallInput{Namespace: rec.Namespace, WebhookImage: dto.ClusterImageInput{Repository: rec.WebhookRepository, Tag: rec.WebhookTag, PullPolicy: rec.WebhookPullPolicy}, OperatorImage: dto.ClusterImageInput{Repository: rec.OperatorRepository, Tag: rec.OperatorTag, PullPolicy: rec.OperatorPullPolicy}, HighAvailability: rec.HighAvailability, GatewayAddress: rec.GatewayAddress, GatewayTLSServerName: rec.GatewayTLSServerName, GatewayCABundle: rec.GatewayCABundle}, nil
}

func (s *ClusterService) CreateUninstall(ctx context.Context, name string) (*dto.UninstallTaskView, error) {
	if err := requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	c, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if c.Phase != gatewaymodel.ClusterPhaseOnline {
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
	if err := s.repos.ClusterUOW.WithinTransaction(ctx, func(clusters repository.ClusterRepository, _ repository.InstallRecordRepository, tasks repository.UninstallTaskRepository) error {
		if err := tasks.Create(ctx, task); err != nil {
			return err
		}
		return clusters.UpdatePhase(ctx, c.ID, gatewaymodel.ClusterPhaseUninstalling)
	}); err != nil {
		return nil, fmt.Errorf("create uninstall task: %w", err)
	}
	s.notify(ctx, "cluster-uninstall")
	return &dto.UninstallTaskView{ID: task.ID, Status: task.Status, RequestedAt: task.RequestedAt}, nil
}

func (s *ClusterService) ListUninstall(ctx context.Context, name string) ([]dto.UninstallTaskView, error) {
	c, err := s.clusterForActor(ctx, name)
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
	out := make([]dto.UninstallTaskView, 0, len(rows))
	for _, t := range rows {
		out = append(out, dto.UninstallTaskView{ID: t.ID, Status: t.Status, Error: t.Error, RequestedAt: t.RequestedAt, CompletedAt: t.CompletedAt})
	}
	return out, nil
}

func (s *ClusterService) RetryUninstall(ctx context.Context, name string, taskID int64) error {
	if err := requireSystemAdmin(ctx); err != nil {
		return err
	}
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
	if err := s.repos.ClusterUOW.WithinTransaction(ctx, func(clusters repository.ClusterRepository, _ repository.InstallRecordRepository, tasks repository.UninstallTaskRepository) error {
		if err := tasks.Retry(ctx, taskID, time.Now().UTC()); err != nil {
			return err
		}
		return clusters.UpdatePhase(ctx, c.ID, gatewaymodel.ClusterPhaseUninstalling)
	}); err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			return newConflict("only a failed uninstall task can be retried")
		}
		return fmt.Errorf("retry uninstall task: %w", err)
	}
	s.notify(ctx, "cluster-uninstall-retry")
	return nil
}

func (s *ClusterService) ListInstallRecords(ctx context.Context, page, pageSize int, sortBy, sortOrder string) (*dto.InstallRecordPageView, error) {
	if err := requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	rows, total, err := s.repos.InstallRecords.ListPage(ctx, dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"})
	if err != nil {
		return nil, fmt.Errorf("list install records: %w", err)
	}
	items := make([]dto.InstallRecordItemView, 0, len(rows))
	for _, rec := range rows {
		items = append(items, dto.InstallRecordItemView{ClusterName: rec.ClusterName, Namespace: rec.Namespace, WebhookRepository: rec.WebhookRepository, WebhookTag: rec.WebhookTag, OperatorRepository: rec.OperatorRepository, OperatorTag: rec.OperatorTag, HighAvailability: rec.HighAvailability, GatewayAddress: rec.GatewayAddress, CreatedAt: rec.CreatedAt})
	}
	return &dto.InstallRecordPageView{Items: items, Total: int(total), Page: page, PageSize: pageSize}, nil
}

func (s *ClusterService) AssignTenant(ctx context.Context, name string, tenantID *int64) error {
	if err := requireSystemAdmin(ctx); err != nil {
		return err
	}
	cluster, err := s.repos.Clusters.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if tenantID != nil {
		if _, err := s.repos.Identity.GetTenant(ctx, *tenantID); err != nil {
			return ErrNotFound
		}
	}
	if err := s.repos.Clusters.AssignTenant(ctx, cluster.ID, tenantID); err != nil {
		if errors.Is(err, dbrepo.ErrClusterTenantInUse) {
			return newConflict("%s", err.Error())
		}
		return err
	}
	return nil
}

func (s *ClusterService) clusterForActor(ctx context.Context, name string) (*dbrepo.Cluster, error) {
	principal := requestPrincipal(ctx)
	if principal.Role == dbrepo.RoleSystemAdmin {
		return s.repos.Clusters.GetByName(ctx, name)
	}
	if principal.TenantID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.repos.Clusters.GetByNameForTenant(ctx, *principal.TenantID, name)
}

func (s *ClusterService) recordInstall(ctx context.Context, clusterName string, p *dto.ClusterInstallInput) error {
	return s.repos.InstallRecords.Create(ctx, installRecord(clusterName, p))
}

func installRecord(clusterName string, p *dto.ClusterInstallInput) *dbrepo.ClusterInstallRecord {
	return &dbrepo.ClusterInstallRecord{ClusterName: clusterName, Namespace: p.Namespace, WebhookRepository: p.WebhookImage.Repository, WebhookTag: p.WebhookImage.Tag, WebhookPullPolicy: p.WebhookImage.PullPolicy, OperatorRepository: p.OperatorImage.Repository, OperatorTag: p.OperatorImage.Tag, OperatorPullPolicy: p.OperatorImage.PullPolicy, HighAvailability: p.HighAvailability, GatewayAddress: p.GatewayAddress, GatewayTLSServerName: p.GatewayTLSServerName, GatewayCABundle: p.GatewayCABundle}
}

func (s *ClusterService) notify(ctx context.Context, topic string) {
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, topic)
	}
}

const dangerousShellChars = "`$\\\""

func validateInstallParams(p *dto.ClusterInstallInput) error {
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

func buildInstallCommand(clusterName, token string, p *dto.ClusterInstallInput) string {
	var b strings.Builder
	fmt.Fprint(&b, "helm upgrade --install kubesentry charts/kubesentry \\\n  --namespace "+p.Namespace+" --create-namespace \\\n")
	fmt.Fprintf(&b, "  --set webhook.image.repository=%q \\\n", p.WebhookImage.Repository)
	fmt.Fprintf(&b, "  --set webhook.image.tag=%q \\\n", p.WebhookImage.Tag)
	fmt.Fprintf(&b, "  --set webhook.image.pullPolicy=%s \\\n", p.WebhookImage.PullPolicy)
	fmt.Fprintf(&b, "  --set operator.image.repository=%q \\\n", p.OperatorImage.Repository)
	fmt.Fprintf(&b, "  --set operator.image.tag=%q \\\n", p.OperatorImage.Tag)
	fmt.Fprintf(&b, "  --set operator.image.pullPolicy=%s \\\n", p.OperatorImage.PullPolicy)
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
