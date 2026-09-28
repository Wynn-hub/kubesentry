package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInactiveSession = errors.New("session is expired, revoked, or unavailable")

type IdentityRepo struct{ db *gorm.DB }

func NewIdentityRepo(db *gorm.DB) *IdentityRepo { return &IdentityRepo{db: db} }

func (r *IdentityRepo) CreateTenant(ctx context.Context, tenant *Tenant) error {
	return r.db.WithContext(ctx).Create(tenant).Error
}

func (r *IdentityRepo) GetTenant(ctx context.Context, id int64) (*Tenant, error) {
	var tenant Tenant
	if err := r.db.WithContext(ctx).First(&tenant, id).Error; err != nil {
		return nil, fmt.Errorf("get tenant %d: %w", id, err)
	}
	return &tenant, nil
}

func (r *IdentityRepo) ListTenants(ctx context.Context) ([]Tenant, error) {
	var tenants []Tenant
	err := r.db.WithContext(ctx).Order("name ASC").Find(&tenants).Error
	return tenants, err
}

func (r *IdentityRepo) UpdateTenantStatus(ctx context.Context, id int64, status UserStatus) error {
	result := r.db.WithContext(ctx).Model(&Tenant{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *IdentityRepo) CreateUser(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *IdentityRepo) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("get user %q: %w", username, err)
	}
	return &user, nil
}

func (r *IdentityRepo) GetUser(ctx context.Context, id int64) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, fmt.Errorf("get user %d: %w", id, err)
	}
	return &user, nil
}

func (r *IdentityRepo) ListUsers(ctx context.Context) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Order("username ASC").Find(&users).Error
	return users, err
}

func (r *IdentityRepo) ListUsersByTenant(ctx context.Context, tenantID int64) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("username ASC").Find(&users).Error
	return users, err
}

func (r *IdentityRepo) UpdateUser(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *IdentityRepo) InitializeAdmin(ctx context.Context, passwordHash string, at time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&User{}).
		Where("username = ? AND role = ? AND password_hash = ''", "admin", RoleSystemAdmin).
		Updates(map[string]any{"password_hash": passwordHash, "updated_at": at})
	return result.RowsAffected == 1, result.Error
}

func (r *IdentityRepo) CreateSession(ctx context.Context, session *Session) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *IdentityRepo) GetActiveSession(ctx context.Context, tokenHash string, now time.Time) (*Session, *User, *Tenant, error) {
	var session Session
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, now).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrInactiveSession
		}
		return nil, nil, nil, err
	}
	var user User
	if err := r.db.WithContext(ctx).First(&user, session.UserID).Error; err != nil || user.Status != UserStatusActive {
		return nil, nil, nil, ErrInactiveSession
	}
	var tenant *Tenant
	if user.TenantID != nil {
		tenant = &Tenant{}
		if err := r.db.WithContext(ctx).First(tenant, *user.TenantID).Error; err != nil || tenant.Status != UserStatusActive {
			return nil, nil, nil, ErrInactiveSession
		}
	}
	return &session, &user, tenant, nil
}

func (r *IdentityRepo) TouchSession(ctx context.Context, id int64, at time.Time) error {
	return r.db.WithContext(ctx).Model(&Session{}).Where("id = ? AND revoked_at IS NULL", id).Update("last_seen_at", at).Error
}

func (r *IdentityRepo) RevokeSession(ctx context.Context, id int64, at time.Time) error {
	return r.db.WithContext(ctx).Model(&Session{}).Where("id = ?", id).Update("revoked_at", at).Error
}

func (r *IdentityRepo) RevokeUserSessions(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", time.Now()).Error
}

func (r *IdentityRepo) SetUserStatusAndRevoke(ctx context.Context, userID int64, status UserStatus, at time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// ponytail: one global identity-admin lock; split per tenant only if admin mutation throughput matters.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(2081000000000000001)).Error; err != nil {
			return err
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		if user.Role == RoleSystemAdmin && status == UserStatusDisabled {
			var enabled []User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role = ? AND status = ?", RoleSystemAdmin, UserStatusActive).Find(&enabled).Error; err != nil {
				return err
			}
			if len(enabled) <= 1 {
				return ErrLastSystemAdmin
			}
		}
		if err := tx.Model(&user).Updates(map[string]any{"status": status, "updated_at": at}).Error; err != nil {
			return err
		}
		return tx.Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", at).Error
	})
}

func (r *IdentityRepo) SetUserPasswordAndRevoke(ctx context.Context, userID int64, hash string, at time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{"password_hash": hash, "updated_at": at})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", at).Error
	})
}

func (r *IdentityRepo) DeleteUserAndRevoke(ctx context.Context, userID int64, at time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Keep deletion under the same lock as status changes so the last-admin check cannot race.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(2081000000000000001)).Error; err != nil {
			return err
		}
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		if user.Role == RoleSystemAdmin && user.Status == UserStatusActive {
			var enabled []User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("role = ? AND status = ?", RoleSystemAdmin, UserStatusActive).Find(&enabled).Error; err != nil {
				return err
			}
			if len(enabled) <= 1 {
				return ErrLastSystemAdmin
			}
		}
		if err := tx.Model(&Session{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", at).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
}
