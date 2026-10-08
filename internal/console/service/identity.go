package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

var (
	ErrForbidden        = errors.New("forbidden")
	ErrIdentityNotFound = errors.New("not found")
	ErrInvalidIdentity  = errors.New("invalid identity request")
	ErrDefaultAdmin     = errors.New("default administrator cannot be deleted")
)

type IdentityService struct {
	repo repository.IdentityRepository
	now  func() time.Time
}

func NewIdentityService(repo repository.IdentityRepository) *IdentityService {
	return &IdentityService{repo: repo, now: time.Now}
}

func (s *IdentityService) ListTenants(ctx context.Context, actor dbrepo.Principal) ([]dto.TenantView, error) {
	if actor.Role == dbrepo.RoleViewer {
		return nil, ErrForbidden
	}
	var tenants []dbrepo.Tenant
	var err error
	if actor.Role == dbrepo.RoleSystemAdmin {
		tenants, err = s.repo.ListTenants(ctx)
	} else if actor.TenantID != nil {
		var tenant *dbrepo.Tenant
		tenant, err = s.repo.GetTenant(ctx, *actor.TenantID)
		if err == nil {
			tenants = []dbrepo.Tenant{*tenant}
		}
	} else {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	out := make([]dto.TenantView, 0, len(tenants))
	for _, tenant := range tenants {
		out = append(out, tenantView(tenant))
	}
	return out, nil
}

func (s *IdentityService) CreateTenant(ctx context.Context, actor dbrepo.Principal, req dto.CreateTenantRequest) (*dto.TenantView, error) {
	if actor.Role != dbrepo.RoleSystemAdmin {
		return nil, ErrForbidden
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrInvalidIdentity
	}
	tenant := &dbrepo.Tenant{Name: name, Status: dbrepo.UserStatusActive}
	if err := s.repo.CreateTenant(ctx, tenant); err != nil {
		return nil, err
	}
	view := tenantView(*tenant)
	return &view, nil
}

func (s *IdentityService) SetTenantStatus(ctx context.Context, actor dbrepo.Principal, tenantID int64, status dbrepo.UserStatus) error {
	if actor.Role != dbrepo.RoleSystemAdmin {
		return ErrForbidden
	}
	if !validStatus(status) {
		return ErrInvalidIdentity
	}
	return s.repo.SetTenantStatusAndRevoke(ctx, tenantID, status, s.now())
}

func (s *IdentityService) ListUsers(ctx context.Context, actor dbrepo.Principal) ([]dto.UserView, error) {
	var users []dbrepo.User
	var err error
	switch actor.Role {
	case dbrepo.RoleSystemAdmin:
		users, err = s.repo.ListUsers(ctx)
	case dbrepo.RoleTenantAdmin:
		if actor.TenantID == nil {
			return nil, ErrForbidden
		}
		users, err = s.repo.ListUsersByTenant(ctx, *actor.TenantID)
	default:
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	out := make([]dto.UserView, 0, len(users))
	for _, user := range users {
		out = append(out, userView(user))
	}
	return out, nil
}

func (s *IdentityService) CreateUser(ctx context.Context, actor dbrepo.Principal, req dto.CreateUserRequest) (*dto.UserView, error) {
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" || len(req.Password) < MinimumPasswordLength {
		return nil, ErrInvalidIdentity
	}
	if actor.Role == dbrepo.RoleTenantAdmin {
		if actor.TenantID == nil || req.Role != dbrepo.RoleViewer || req.TenantID == nil || *req.TenantID != *actor.TenantID {
			return nil, ErrForbidden
		}
	} else if actor.Role != dbrepo.RoleSystemAdmin {
		return nil, ErrForbidden
	}
	if (req.Role == dbrepo.RoleSystemAdmin) != (req.TenantID == nil) || (req.Role != dbrepo.RoleSystemAdmin && req.Role != dbrepo.RoleTenantAdmin && req.Role != dbrepo.RoleViewer) {
		return nil, ErrInvalidIdentity
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &dbrepo.User{Username: username, PasswordHash: string(hash), Role: req.Role, Status: dbrepo.UserStatusActive, TenantID: req.TenantID}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	view := userView(*user)
	return &view, nil
}

func (s *IdentityService) SetUserStatus(ctx context.Context, actor dbrepo.Principal, userID int64, status dbrepo.UserStatus) error {
	target, err := s.authorizedUser(ctx, actor, userID)
	if err != nil {
		return err
	}
	if !validStatus(status) {
		return ErrInvalidIdentity
	}
	if actor.Role == dbrepo.RoleTenantAdmin && target.Role != dbrepo.RoleViewer {
		return ErrForbidden
	}
	return s.repo.SetUserStatusAndRevoke(ctx, userID, status, s.now())
}

func (s *IdentityService) ResetPassword(ctx context.Context, actor dbrepo.Principal, userID int64, password string) error {
	target, err := s.authorizedUser(ctx, actor, userID)
	if err != nil {
		return err
	}
	if len(password) < MinimumPasswordLength {
		return ErrInvalidPassword
	}
	if actor.Role == dbrepo.RoleTenantAdmin && target.Role != dbrepo.RoleViewer {
		return ErrForbidden
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.SetUserPasswordAndRevoke(ctx, userID, string(hash), s.now())
}

func (s *IdentityService) DeleteUser(ctx context.Context, actor dbrepo.Principal, userID int64) error {
	target, err := s.authorizedUser(ctx, actor, userID)
	if err != nil {
		return err
	}
	if target.Username == "admin" {
		return ErrDefaultAdmin
	}
	if actor.Role == dbrepo.RoleTenantAdmin && target.Role != dbrepo.RoleViewer {
		return ErrForbidden
	}
	return s.repo.DeleteUserAndRevoke(ctx, userID, s.now())
}

func (s *IdentityService) authorizedUser(ctx context.Context, actor dbrepo.Principal, userID int64) (*dbrepo.User, error) {
	if actor.Role == dbrepo.RoleViewer {
		return nil, ErrForbidden
	}
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIdentityNotFound
		}
		return nil, err
	}
	if actor.Role == dbrepo.RoleTenantAdmin && (actor.TenantID == nil || user.TenantID == nil || *actor.TenantID != *user.TenantID) {
		return nil, ErrIdentityNotFound
	}
	return user, nil
}

func tenantView(tenant dbrepo.Tenant) dto.TenantView {
	return dto.TenantView{ID: tenant.ID, Name: tenant.Name, Status: tenant.Status}
}

func userView(user dbrepo.User) dto.UserView {
	return dto.UserView{ID: user.ID, Username: user.Username, Role: user.Role, Status: user.Status, TenantID: user.TenantID}
}

func validStatus(status dbrepo.UserStatus) bool {
	return status == dbrepo.UserStatusActive || status == dbrepo.UserStatusDisabled
}
