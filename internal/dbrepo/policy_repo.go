package dbrepo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type PolicyRepo struct{ db *gorm.DB }

type PolicyPageOptions struct {
	PageOptions
	Sources []string
	Phases  []string
	Modes   []string
	Keyword string
}

func NewPolicyRepo(db *gorm.DB) *PolicyRepo { return &PolicyRepo{db: db} }

// WithTx runs fn against a PolicyRepo bound to a single DB transaction, so a
// Policy write and its matching PolicyVersion insert commit or roll back
// together instead of leaving an orphaned Policy row behind on failure.
func (r *PolicyRepo) WithTx(ctx context.Context, fn func(txRepo *PolicyRepo) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PolicyRepo{db: tx})
	})
}

func (r *PolicyRepo) Create(ctx context.Context, p *Policy) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *PolicyRepo) GetByName(ctx context.Context, name string) (*Policy, error) {
	var p Policy
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&p).Error; err != nil {
		return nil, fmt.Errorf("get policy %q: %w", name, err)
	}
	return &p, nil
}

func (r *PolicyRepo) List(ctx context.Context) ([]Policy, error) {
	var out []Policy
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	return out, nil
}

func (r *PolicyRepo) ListPage(ctx context.Context, options PolicyPageOptions) ([]Policy, int64, error) {
	query := r.db.WithContext(ctx).Model(&Policy{})
	query = applyPolicySourceFilter(query, options.Sources)
	query = applyPolicyPhaseFilter(query, options.Phases)
	if len(options.Modes) > 0 {
		query = query.Where("enforcement_mode IN ?", options.Modes)
	}
	if options.Keyword != "" {
		pattern := "%" + escapeLike(strings.ToLower(options.Keyword)) + "%"
		query = query.Where(
			"(LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\')",
			pattern, pattern,
		)
	}

	items, total, err := queryPage[Policy](
		query,
		options.PageOptions,
		map[string]string{
			"name":            "name",
			"source":          "(CASE WHEN labels ->> 'kubesentry.io/builtin' = 'true' THEN 'builtin' ELSE 'custom' END)",
			"enforcementMode": "enforcement_mode",
			"phase":           "(CASE WHEN compile_ok THEN 'Ready' ELSE 'Invalid' END)",
			"createdAt":       "created_at",
			"updatedAt":       "updated_at",
		},
		"name ASC",
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list policy page: %w", err)
	}
	return items, total, nil
}

func applyPolicySourceFilter(query *gorm.DB, sources []string) *gorm.DB {
	if len(sources) == 0 {
		return query
	}
	builtin, custom := contains(sources, "builtin"), contains(sources, "custom")
	switch {
	case builtin && custom:
		return query
	case builtin:
		return query.Where("labels ->> 'kubesentry.io/builtin' = 'true'")
	case custom:
		return query.Where("COALESCE(labels ->> 'kubesentry.io/builtin', '') <> 'true'")
	default:
		return query.Where("1 = 0")
	}
}

func applyPolicyPhaseFilter(query *gorm.DB, phases []string) *gorm.DB {
	if len(phases) == 0 {
		return query
	}
	ready, invalid := contains(phases, "Ready"), contains(phases, "Invalid")
	switch {
	case ready && invalid:
		return query
	case ready:
		return query.Where("compile_ok = ?", true)
	case invalid:
		return query.Where("compile_ok = ?", false)
	default:
		return query.Where("1 = 0")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func (r *PolicyRepo) Update(ctx context.Context, p *Policy) error {
	return r.db.WithContext(ctx).Save(p).Error
}

// UpdateCAS applies fields to the Policy row id, but only if its updated_at
// still equals expectedUpdatedAt (the value the caller read before building
// fields) — a real compare-and-swap instead of a check-then-act race.
// Returns ErrConflict if another writer changed the row in between.
func (r *PolicyRepo) UpdateCAS(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any) error {
	result := r.db.WithContext(ctx).Model(&Policy{}).
		Where("id = ? AND updated_at = ?", id, expectedUpdatedAt).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (r *PolicyRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Policy{}, id).Error
}

// CreateVersion inserts an immutable policy_versions snapshot in the same
// transaction as updating policies (spec 二: PolicyVersion 语义).
func (r *PolicyRepo) CreateVersion(ctx context.Context, v *PolicyVersion) error {
	return r.db.WithContext(ctx).Create(v).Error
}

// ListVersions returns all versions for a policy, newest first.
func (r *PolicyRepo) ListVersions(ctx context.Context, policyID int64) ([]PolicyVersion, error) {
	var out []PolicyVersion
	err := r.db.WithContext(ctx).Where("policy_id = ?", policyID).Order("version DESC").Find(&out).Error
	return out, err
}

// PruneVersions deletes versions beyond keepLast (oldest first), matching
// the existing PolicyReconciler history-limit behavior.
func (r *PolicyRepo) PruneVersions(ctx context.Context, policyID int64, keepLast int) error {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&PolicyVersion{}).
		Where("policy_id = ?", policyID).
		Order("version DESC").
		Offset(keepLast).
		Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return err
	}
	return r.db.WithContext(ctx).Delete(&PolicyVersion{}, ids).Error
}
