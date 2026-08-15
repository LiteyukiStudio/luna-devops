package volume

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	ProjectManagedCapacityLimitConfigKey = "storage.projectManagedCapacityLimitGiB"
	bytesPerGiB                          = int64(1024 * 1024 * 1024)
)

type ProjectVolumeQuotaSnapshot struct {
	ProjectID     string `json:"projectId"`
	LimitBytes    int64  `json:"limitBytes"`
	ReservedBytes int64  `json:"reservedBytes"`
}

// QuotaRepository exposes the durable aggregate for diagnostics and tests.
// Reservation mutation is owned by the PostgreSQL trigger so every writer,
// including migration and Worker paths, shares the same transaction boundary.
type QuotaRepository struct {
	db *gorm.DB
}

func NewQuotaRepository(db *gorm.DB) *QuotaRepository {
	return &QuotaRepository{db: db}
}

func (repository *QuotaRepository) Get(ctx context.Context, projectID string) (ProjectVolumeQuotaSnapshot, error) {
	projectID = strings.TrimSpace(projectID)
	if repository == nil || repository.db == nil || ctx == nil || projectID == "" {
		return ProjectVolumeQuotaSnapshot{}, newDomainError(CodeInvalidInput, "project volume quota repository input is invalid")
	}
	var usage model.ProjectVolumeQuotaUsage
	err := repository.db.WithContext(ctx).First(&usage, "project_id = ?", projectID).Error
	if err != nil && !isRecordNotFound(err) {
		return ProjectVolumeQuotaSnapshot{}, normalizeQuotaPersistenceError(err)
	}
	var limitBytes int64
	if err = repository.db.WithContext(ctx).Raw("SELECT luna_project_volume_quota_limit_bytes()").Scan(&limitBytes).Error; err != nil {
		return ProjectVolumeQuotaSnapshot{}, normalizeQuotaPersistenceError(err)
	}
	return ProjectVolumeQuotaSnapshot{ProjectID: projectID, LimitBytes: limitBytes, ReservedBytes: usage.ReservedBytes}, nil
}

func isRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func normalizeQuotaPersistenceError(err error) error {
	if err == nil || ErrorCode(err) != "" {
		return err
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "sqlstate pvr01") || strings.Contains(message, "project_volume_quota_exceeded"):
		return newDomainError(CodeQuotaExceeded, "managed project volume capacity quota is exceeded", err)
	case strings.Contains(message, "sqlstate pvr02") || strings.Contains(message, "project_volume_quota_config_invalid"):
		return newDomainError(CodeQuotaUnavailable, "managed project volume capacity quota is unavailable", err)
	case strings.Contains(message, "sqlstate pvr03") || strings.Contains(message, "project_volume_quota_invariant_failed") || strings.Contains(message, "project_volume_quota_project_missing") || strings.Contains(message, "project_volume_quota_project_immutable"):
		return newDomainError(CodeQuotaUnavailable, "managed project volume capacity quota is unavailable", err)
	default:
		return err
	}
}
