package dbrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	UninstallTaskPending   = "Pending"
	UninstallTaskReceived  = "Received"
	UninstallTaskRunning   = "Running"
	UninstallTaskSucceeded = "Succeeded"
	UninstallTaskFailed    = "Failed"
)

var ErrActiveUninstallTask = errors.New("active uninstall task already exists")

type UninstallTaskRepo struct{ db *gorm.DB }

func NewUninstallTaskRepo(db *gorm.DB) *UninstallTaskRepo { return &UninstallTaskRepo{db: db} }

func (r *UninstallTaskRepo) Create(ctx context.Context, task *UninstallTask) error {
	if task.RequestedAt.IsZero() {
		task.RequestedAt = time.Now().UTC()
	}
	if task.Status == "" {
		task.Status = UninstallTaskPending
	}
	if task.Attempts == 0 {
		task.Attempts = 1
	}
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *UninstallTaskRepo) GetByID(ctx context.Context, id int64) (*UninstallTask, error) {
	var task UninstallTask
	if err := r.db.WithContext(ctx).First(&task, id).Error; err != nil {
		return nil, fmt.Errorf("get uninstall task %d: %w", id, err)
	}
	return &task, nil
}

func (r *UninstallTaskRepo) ListByClusterID(ctx context.Context, clusterID int64) ([]UninstallTask, error) {
	var tasks []UninstallTask
	if err := r.db.WithContext(ctx).Where("cluster_id = ?", clusterID).Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list uninstall tasks for cluster %d: %w", clusterID, err)
	}
	return tasks, nil
}

func (r *UninstallTaskRepo) HasActive(ctx context.Context, clusterID int64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&UninstallTask{}).
		Where("cluster_id = ? AND status IN ?", clusterID, []string{UninstallTaskPending, UninstallTaskReceived, UninstallTaskRunning}).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count active uninstall tasks: %w", err)
	}
	return count > 0, nil
}

func (r *UninstallTaskRepo) ListActiveByClusterID(ctx context.Context, clusterID int64) ([]UninstallTask, error) {
	var tasks []UninstallTask
	if err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND status IN ?", clusterID, []string{UninstallTaskPending, UninstallTaskReceived, UninstallTaskRunning}).
		Order("created_at ASC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list active uninstall tasks for cluster %d: %w", clusterID, err)
	}
	return tasks, nil
}

// Transition atomically moves a task from one expected status to the next.
// It deliberately updates only task-owned fields so task reports cannot
// overwrite the cluster heartbeat writer's columns.
func (r *UninstallTaskRepo) Transition(ctx context.Context, id int64, expected, next, message string, at time.Time) error {
	fields := map[string]any{"status": next, "error": message}
	if next == UninstallTaskReceived {
		fields["received_at"] = at
	}
	if next == UninstallTaskSucceeded || next == UninstallTaskFailed {
		fields["completed_at"] = at
	}
	result := r.db.WithContext(ctx).Model(&UninstallTask{}).Where("id = ? AND status = ?", id, expected).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

// Retry requeues a failed task while preserving its history and incrementing
// the delivery attempt. Only failed tasks are eligible for retry.
func (r *UninstallTaskRepo) Retry(ctx context.Context, id int64, at time.Time) error {
	result := r.db.WithContext(ctx).Model(&UninstallTask{}).
		Where("id = ? AND status = ?", id, UninstallTaskFailed).
		Updates(map[string]any{
			"status":       UninstallTaskPending,
			"error":        "",
			"requested_at": at,
			"received_at":  nil,
			"completed_at": nil,
			"attempts":     gorm.Expr("attempts + ?", 1),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}
