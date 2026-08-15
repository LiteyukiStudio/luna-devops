package volume

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	Transaction(context.Context, func(Repository) error) error
	ListProjectVolumes(context.Context, string, ProjectVolumeListOptions) (ProjectVolumeListResult, error)
	GetProjectVolume(context.Context, string, string) (model.ProjectVolume, error)
	GetProjectVolumeForMaintenance(context.Context, string) (model.ProjectVolume, error)
	LockProjectVolume(context.Context, string, string) (model.ProjectVolume, error)
	FindProjectVolumeByIdempotency(context.Context, string, string) (model.ProjectVolume, error)
	CreateProjectVolume(context.Context, *model.ProjectVolume) error
	UpdateProjectVolume(context.Context, *model.ProjectVolume, int64) (bool, error)
	TransitionProjectVolume(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	SoftDeleteProjectVolume(context.Context, string, string, int64) (bool, error)
	CountBlockingMounts(context.Context, string) (int64, error)
	CountActiveTransfers(context.Context, string) (int64, error)
	ListProjectVolumeMounts(context.Context, string, string, []string, int, int) ([]model.DeploymentVolumeMount, int64, error)
	ListBlockingVolumeTransfers(context.Context, string, string, int, int) ([]model.VolumeTransfer, int64, error)
	LockDeploymentTarget(context.Context, string, string) (model.DeploymentTarget, error)
	ListDeploymentTargetMounts(context.Context, string, string) ([]model.DeploymentVolumeMount, error)
	GetDeploymentVolumeMount(context.Context, string, string) (model.DeploymentVolumeMount, error)
	CreateDeploymentVolumeMount(context.Context, *model.DeploymentVolumeMount) error
	TransitionDeploymentVolumeMount(context.Context, string, string, []string, string, string, string) (model.DeploymentVolumeMount, error)
	DeleteDeploymentVolumeMount(context.Context, string, string, []string) (bool, error)
	ListVolumeTransfers(context.Context, string, VolumeTransferListOptions) (VolumeTransferListResult, error)
	GetVolumeTransfer(context.Context, string, string) (model.VolumeTransfer, error)
	GetVolumeTransferForMaintenance(context.Context, string) (model.VolumeTransfer, error)
	LockVolumeTransfer(context.Context, string, string) (model.VolumeTransfer, error)
	CreateVolumeTransfer(context.Context, *model.VolumeTransfer) error
	TransitionVolumeTransfer(context.Context, string, string, string, string, string, string) (model.VolumeTransfer, error)
	CompleteVolumeTransferUpload(context.Context, string, string, string, int64, string) (model.VolumeTransfer, error)
	ClaimVolumeTransferExecution(context.Context, string, string, string, string, time.Time, time.Time) (model.VolumeTransfer, error)
	RenewVolumeTransferExecutionLease(context.Context, string, string, string, int64, time.Time, time.Time) (model.VolumeTransfer, error)
	PrepareVolumeTransferExecution(context.Context, string, string, string, string, int64, string, time.Time) (model.VolumeTransfer, error)
	ConfirmVolumeTransferJobCreated(context.Context, string, string, int64) (model.VolumeTransfer, error)
	ReportVolumeTransferCompletion(context.Context, string, string, TransferCompletion) (model.VolumeTransfer, error)
	MarkVolumeTransferJobSucceeded(context.Context, string, string) (model.VolumeTransfer, error)
	FinalizeVolumeTransferExecution(context.Context, string, string) (model.VolumeTransfer, error)
	MarkVolumeTransferExecutionCleanupCompleted(context.Context, string, string) (model.VolumeTransfer, error)
	UpdateVolumeTransferProgress(context.Context, string, string, TransferProgress) (model.VolumeTransfer, error)
	CreateVolumeTransferPart(context.Context, *model.VolumeTransferPart) (bool, model.VolumeTransferPart, error)
	GetVolumeTransferPartByOffset(context.Context, string, int64) (model.VolumeTransferPart, error)
	TakeOverVolumeTransferPart(context.Context, string, int, string, string, time.Time) (bool, model.VolumeTransferPart, error)
	CompleteVolumeTransferPart(context.Context, string, int, string, string) (bool, model.VolumeTransferPart, error)
	ExpireVolumeTransferPartLease(context.Context, string, int, string, time.Time) (bool, error)
	ListVolumeTransferParts(context.Context, string, int, int) ([]model.VolumeTransferPart, int64, error)
	VolumeTransferUploadOffset(context.Context, string) (int64, error)
	NextVolumeTransferPartNumber(context.Context, string) (int, error)
	ListStaleProjectVolumes(context.Context, time.Time, int) ([]model.ProjectVolume, error)
	ListStaleVolumeTransfers(context.Context, time.Time, int) ([]model.VolumeTransfer, error)
	ListExpiredVolumeTransferObjects(context.Context, time.Time, int) ([]model.VolumeTransfer, error)
	TransferVolumeTransferObjectOwnership(context.Context, string, string, time.Time) (bool, error)
	ClaimVolumeTransferObjectCleanup(context.Context, string, string, string, time.Time, time.Time) (bool, model.VolumeTransfer, error)
	RenewVolumeTransferObjectCleanup(context.Context, string, string, string, time.Time, time.Time) (bool, model.VolumeTransfer, error)
	CompleteVolumeTransferObjectCleanup(context.Context, string, string, string, time.Time) (bool, model.VolumeTransfer, error)
	ReleaseVolumeTransferObjectCleanup(context.Context, string, string, string, time.Time) (bool, error)
	MarkVolumeTransferObjectDeleted(context.Context, string, string, time.Time) (bool, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (repository *GormRepository) Transaction(ctx context.Context, fn func(Repository) error) error {
	if repository == nil || repository.db == nil || fn == nil {
		return newDomainError(CodeInvalidInput, "volume repository is not configured")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormRepository{db: tx})
	})
}

func (repository *GormRepository) ListProjectVolumes(ctx context.Context, projectID string, options ProjectVolumeListOptions) (ProjectVolumeListResult, error) {
	result := ProjectVolumeListResult{
		Items:     make([]model.ProjectVolume, 0),
		Page:      options.Page,
		PageSize:  options.PageSize,
		SortBy:    options.SortBy,
		SortOrder: options.SortOrder,
	}
	column, ok := projectVolumeSortColumns[options.SortBy]
	if !ok {
		return ProjectVolumeListResult{}, newDomainError(CodePaginationSortByInvalid, "unsupported project volume sort field")
	}
	query := repository.db.WithContext(ctx).Model(&model.ProjectVolume{}).Where("project_id = ?", projectID)
	query = applyProjectVolumeFilters(query, options)
	if err := query.Session(&gorm.Session{}).Count(&result.Total).Error; err != nil {
		return ProjectVolumeListResult{}, err
	}
	if options.PageSize > 0 {
		result.TotalPages = int(math.Ceil(float64(result.Total) / float64(options.PageSize)))
	}
	if result.Total == 0 {
		return result, nil
	}
	if err := query.Order(column + " " + options.SortOrder + ", id " + options.SortOrder).
		Limit(options.PageSize).
		Offset((options.Page - 1) * options.PageSize).
		Find(&result.Items).Error; err != nil {
		return ProjectVolumeListResult{}, err
	}
	if err := repository.populateVolumeBindingSummaries(ctx, result.Items); err != nil {
		return ProjectVolumeListResult{}, err
	}
	return result, nil
}

func applyProjectVolumeFilters(query *gorm.DB, options ProjectVolumeListOptions) *gorm.DB {
	if options.Search != "" {
		term := "%" + escapeLike(options.Search) + "%"
		query = query.Where("(display_name ILIKE ? ESCAPE '!' OR claim_name ILIKE ? ESCAPE '!' OR source_application_name ILIKE ? ESCAPE '!')", term, term, term)
	}
	if options.LifecycleState != "" {
		query = query.Where("lifecycle_state = ?", options.LifecycleState)
	}
	if options.ClusterID != "" {
		query = query.Where("cluster_id = ?", options.ClusterID)
	}
	if options.SourceKind != "" {
		query = query.Where("source_kind = ?", options.SourceKind)
	}
	if options.OwnershipMode != "" {
		query = query.Where("ownership_mode = ?", options.OwnershipMode)
	}
	if options.VolumeMode != "" {
		query = query.Where("volume_mode = ?", options.VolumeMode)
	}
	switch options.Availability {
	case model.ProjectVolumeAvailabilityAvailable:
		query = query.Where("lifecycle_state = ?", model.ProjectVolumeLifecycleReady).
			Where("NOT EXISTS (?)", activeMountSubquery(query, []string{
				model.DeploymentVolumeActivationReserved,
				model.DeploymentVolumeActivationActive,
				model.DeploymentVolumeActivationReleasePending,
				model.DeploymentVolumeActivationError,
			}))
	case model.ProjectVolumeAvailabilityReserved:
		query = query.Where("lifecycle_state = ?", model.ProjectVolumeLifecycleReady).
			Where("EXISTS (?)", activeMountSubquery(query, []string{
				model.DeploymentVolumeActivationReserved,
				model.DeploymentVolumeActivationError,
			})).
			Where("NOT EXISTS (?)", activeMountSubquery(query, []string{
				model.DeploymentVolumeActivationActive,
				model.DeploymentVolumeActivationReleasePending,
			}))
	case model.ProjectVolumeAvailabilityInUse:
		query = query.Where("lifecycle_state = ?", model.ProjectVolumeLifecycleReady).
			Where("EXISTS (?)", activeMountSubquery(query, []string{
				model.DeploymentVolumeActivationActive,
				model.DeploymentVolumeActivationReleasePending,
			}))
	case model.ProjectVolumeAvailabilityUnavailable:
		query = query.Where("lifecycle_state <> ?", model.ProjectVolumeLifecycleReady)
	}
	return query
}

func activeMountSubquery(query *gorm.DB, states []string) *gorm.DB {
	return query.Session(&gorm.Session{NewDB: true}).
		Model(&model.DeploymentVolumeMount{}).
		Select("1").
		Where("deployment_volume_mounts.project_volume_id = project_volumes.id").
		Where("deployment_volume_mounts.activation_state IN ?", states)
}

func (repository *GormRepository) populateVolumeBindingSummaries(ctx context.Context, volumes []model.ProjectVolume) error {
	if len(volumes) == 0 {
		return nil
	}
	ids := make([]string, 0, len(volumes))
	index := make(map[string]int, len(volumes))
	for position := range volumes {
		ids = append(ids, volumes[position].ID)
		index[volumes[position].ID] = position
		volumes[position].Availability = availabilityForVolume(volumes[position], model.ProjectVolumeBindingSummary{})
	}
	type countRow struct {
		ProjectVolumeID string
		ActivationState string
		Count           int64
	}
	var rows []countRow
	if err := repository.db.WithContext(ctx).Model(&model.DeploymentVolumeMount{}).
		Select("project_volume_id, activation_state, count(*) AS count").
		Where("project_volume_id IN ?", ids).
		Group("project_volume_id, activation_state").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		position, exists := index[row.ProjectVolumeID]
		if !exists {
			continue
		}
		summary := volumes[position].BindingSummary
		switch row.ActivationState {
		case model.DeploymentVolumeActivationActive, model.DeploymentVolumeActivationReleasePending:
			summary.Active += row.Count
		case model.DeploymentVolumeActivationReserved, model.DeploymentVolumeActivationError:
			summary.Reserved += row.Count
		}
		volumes[position].BindingSummary = summary
		volumes[position].Availability = availabilityForVolume(volumes[position], summary)
	}
	return nil
}

func availabilityForVolume(volume model.ProjectVolume, summary model.ProjectVolumeBindingSummary) string {
	if volume.LifecycleState != model.ProjectVolumeLifecycleReady {
		return model.ProjectVolumeAvailabilityUnavailable
	}
	if summary.Active > 0 {
		return model.ProjectVolumeAvailabilityInUse
	}
	if summary.Reserved > 0 {
		return model.ProjectVolumeAvailabilityReserved
	}
	return model.ProjectVolumeAvailabilityAvailable
}

func (repository *GormRepository) GetProjectVolume(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	var volume model.ProjectVolume
	err := repository.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, volumeID).First(&volume).Error
	if err == nil {
		items := []model.ProjectVolume{volume}
		err = repository.populateVolumeBindingSummaries(ctx, items)
		volume = items[0]
	}
	return volume, err
}

func (repository *GormRepository) GetProjectVolumeForMaintenance(ctx context.Context, volumeID string) (model.ProjectVolume, error) {
	var projectVolume model.ProjectVolume
	err := repository.db.WithContext(ctx).Where("id = ?", volumeID).First(&projectVolume).Error
	return projectVolume, err
}

func (repository *GormRepository) LockProjectVolume(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	var volume model.ProjectVolume
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND id = ?", projectID, volumeID).First(&volume).Error
	return volume, err
}

func (repository *GormRepository) FindProjectVolumeByIdempotency(ctx context.Context, projectID, keyHash string) (model.ProjectVolume, error) {
	var volume model.ProjectVolume
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND idempotency_key_hash = ?", projectID, keyHash).First(&volume).Error
	return volume, err
}

func (repository *GormRepository) CreateProjectVolume(ctx context.Context, volume *model.ProjectVolume) error {
	return repository.db.WithContext(ctx).Create(volume).Error
}

func (repository *GormRepository) UpdateProjectVolume(ctx context.Context, volume *model.ProjectVolume, expectedRevision int64) (bool, error) {
	result := repository.db.WithContext(ctx).Model(&model.ProjectVolume{}).
		Where("project_id = ? AND id = ? AND revision = ?", volume.ProjectID, volume.ID, expectedRevision).
		Updates(map[string]any{
			"display_name":       volume.DisplayName,
			"capacity_request":   volume.CapacityRequest,
			"capacity_bytes":     volume.CapacityBytes,
			"lifecycle_state":    volume.LifecycleState,
			"pending_operation":  volume.PendingOperation,
			"last_error_code":    volume.LastErrorCode,
			"last_error_message": volume.LastErrorMessage,
			"revision":           gorm.Expr("revision + 1"),
			"updated_at":         time.Now().UTC(),
		})
	return result.RowsAffected == 1, result.Error
}

func (repository *GormRepository) TransitionProjectVolume(ctx context.Context, projectID, volumeID string, from []string, to, errorCode, errorMessage string) (model.ProjectVolume, error) {
	updates := projectVolumeTransitionUpdates(to, errorCode, errorMessage, time.Now().UTC())
	result := repository.db.WithContext(ctx).Model(&model.ProjectVolume{}).
		Where("project_id = ? AND id = ? AND lifecycle_state IN ?", projectID, volumeID, from).
		Updates(updates)
	if result.Error != nil {
		return model.ProjectVolume{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.ProjectVolume{}, newDomainError(CodeStateConflict, "project volume lifecycle state changed")
	}
	return repository.GetProjectVolume(ctx, projectID, volumeID)
}

func (repository *GormRepository) SoftDeleteProjectVolume(ctx context.Context, projectID, volumeID string, expectedRevision int64) (bool, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.ProjectVolume{}).
		Where("project_id = ? AND id = ? AND revision = ?", projectID, volumeID, expectedRevision).
		Updates(map[string]any{"deleted_at": now, "updated_at": now, "revision": gorm.Expr("revision + 1")})
	return result.RowsAffected == 1, result.Error
}

func (repository *GormRepository) CountBlockingMounts(ctx context.Context, volumeID string) (int64, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&model.DeploymentVolumeMount{}).
		Where("project_volume_id = ? AND activation_state IN ?", volumeID, []string{
			model.DeploymentVolumeActivationReserved,
			model.DeploymentVolumeActivationActive,
			model.DeploymentVolumeActivationReleasePending,
			model.DeploymentVolumeActivationError,
		}).Count(&count).Error
	return count, err
}

func (repository *GormRepository) CountActiveTransfers(ctx context.Context, volumeID string) (int64, error) {
	var count int64
	err := blockingVolumeTransferQuery(repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_volume_id = ?", volumeID)).Count(&count).Error
	return count, err
}

func (repository *GormRepository) ListProjectVolumeMounts(ctx context.Context, projectID, volumeID string, states []string, page, pageSize int) ([]model.DeploymentVolumeMount, int64, error) {
	page, pageSize = normalizeRepositoryPage(page, pageSize)
	query := repository.db.WithContext(ctx).Model(&model.DeploymentVolumeMount{}).
		Where("project_id = ? AND project_volume_id = ?", projectID, volumeID)
	if len(states) > 0 {
		query = query.Where("activation_state IN ?", states)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.DeploymentVolumeMount, 0)
	err := query.Order("created_at DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error
	return items, total, err
}

func (repository *GormRepository) ListBlockingVolumeTransfers(ctx context.Context, projectID, volumeID string, page, pageSize int) ([]model.VolumeTransfer, int64, error) {
	page, pageSize = normalizeRepositoryPage(page, pageSize)
	query := blockingVolumeTransferQuery(repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND project_volume_id = ?", projectID, volumeID))
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]model.VolumeTransfer, 0)
	err := query.Order("created_at DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error
	return items, total, err
}

func (repository *GormRepository) LockDeploymentTarget(ctx context.Context, projectID, targetID string) (model.DeploymentTarget, error) {
	var target model.DeploymentTarget
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND id = ?", projectID, targetID).First(&target).Error
	return target, err
}

func (repository *GormRepository) ListDeploymentTargetMounts(ctx context.Context, projectID, targetID string) ([]model.DeploymentVolumeMount, error) {
	var mounts []model.DeploymentVolumeMount
	err := repository.db.WithContext(ctx).
		Where("project_id = ? AND deployment_target_id = ?", projectID, targetID).
		Order("created_at ASC, id ASC").Find(&mounts).Error
	return mounts, err
}

func (repository *GormRepository) GetDeploymentVolumeMount(ctx context.Context, projectID, mountID string) (model.DeploymentVolumeMount, error) {
	var mount model.DeploymentVolumeMount
	err := repository.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, mountID).First(&mount).Error
	return mount, err
}

func (repository *GormRepository) CreateDeploymentVolumeMount(ctx context.Context, mount *model.DeploymentVolumeMount) error {
	return repository.db.WithContext(ctx).Create(mount).Error
}

func (repository *GormRepository) TransitionDeploymentVolumeMount(ctx context.Context, projectID, mountID string, from []string, to, errorCode, errorMessage string) (model.DeploymentVolumeMount, error) {
	result := repository.db.WithContext(ctx).Model(&model.DeploymentVolumeMount{}).
		Where("project_id = ? AND id = ? AND activation_state IN ?", projectID, mountID, from).
		Updates(map[string]any{
			"activation_state":   to,
			"last_error_code":    errorCode,
			"last_error_message": errorMessage,
			"updated_at":         time.Now().UTC(),
		})
	if result.Error != nil {
		return model.DeploymentVolumeMount{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.DeploymentVolumeMount{}, newDomainError(CodeStateConflict, "deployment volume mount state changed")
	}
	return repository.GetDeploymentVolumeMount(ctx, projectID, mountID)
}

func (repository *GormRepository) DeleteDeploymentVolumeMount(ctx context.Context, projectID, mountID string, from []string) (bool, error) {
	result := repository.db.WithContext(ctx).
		Where("project_id = ? AND id = ? AND activation_state IN ?", projectID, mountID, from).
		Delete(&model.DeploymentVolumeMount{})
	return result.RowsAffected == 1, result.Error
}

func (repository *GormRepository) ListVolumeTransfers(ctx context.Context, projectID string, options VolumeTransferListOptions) (VolumeTransferListResult, error) {
	result := VolumeTransferListResult{
		Items:     make([]model.VolumeTransfer, 0),
		Page:      options.Page,
		PageSize:  options.PageSize,
		SortBy:    options.SortBy,
		SortOrder: options.SortOrder,
	}
	column, ok := volumeTransferSortColumns[options.SortBy]
	if !ok {
		return VolumeTransferListResult{}, newDomainError(CodePaginationSortByInvalid, "unsupported volume transfer sort field")
	}
	query := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).Where("project_id = ?", projectID)
	if options.Direction != "" {
		query = query.Where("direction = ?", options.Direction)
	}
	if options.State != "" {
		query = query.Where("state = ?", options.State)
	}
	if options.VolumeID != "" {
		query = query.Where("project_volume_id = ?", options.VolumeID)
	}
	if options.CreatedBy != "" {
		query = query.Where("actor_id = ?", options.CreatedBy)
	}
	if err := query.Session(&gorm.Session{}).Count(&result.Total).Error; err != nil {
		return VolumeTransferListResult{}, err
	}
	if options.PageSize > 0 {
		result.TotalPages = int(math.Ceil(float64(result.Total) / float64(options.PageSize)))
	}
	if result.Total == 0 {
		return result, nil
	}
	err := query.Order(column + " " + options.SortOrder + ", id " + options.SortOrder).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).
		Find(&result.Items).Error
	return result, err
}

func (repository *GormRepository) GetVolumeTransfer(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	var transfer model.VolumeTransfer
	err := repository.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, transferID).First(&transfer).Error
	return transfer, err
}

func (repository *GormRepository) GetVolumeTransferForMaintenance(ctx context.Context, transferID string) (model.VolumeTransfer, error) {
	var transfer model.VolumeTransfer
	err := repository.db.WithContext(ctx).Where("id = ?", transferID).First(&transfer).Error
	return transfer, err
}

func (repository *GormRepository) LockVolumeTransfer(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	var transfer model.VolumeTransfer
	err := repository.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND id = ?", projectID, transferID).First(&transfer).Error
	return transfer, err
}

func (repository *GormRepository) CreateVolumeTransfer(ctx context.Context, transfer *model.VolumeTransfer) error {
	return repository.db.WithContext(ctx).Create(transfer).Error
}

func (repository *GormRepository) TransitionVolumeTransfer(ctx context.Context, projectID, transferID, from, to, errorCode, errorMessage string) (model.VolumeTransfer, error) {
	updates := map[string]any{
		"state":              to,
		"last_error_code":    errorCode,
		"last_error_message": errorMessage,
		"updated_at":         time.Now().UTC(),
	}
	if to == model.VolumeTransferStateRunning {
		updates["started_at"] = time.Now().UTC()
	}
	if IsVolumeTransferTerminal(to) {
		updates["finished_at"] = time.Now().UTC()
	}
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state = ?", projectID, transferID, from).
		Updates(updates)
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) CompleteVolumeTransferUpload(ctx context.Context, projectID, transferID, from string, expectedBytes int64, sha256 string) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state = ? AND expected_bytes = ? AND (sha256 = '' OR sha256 = ?)", projectID, transferID, from, expectedBytes, sha256).
		Updates(map[string]any{
			"state":               model.VolumeTransferStateQueued,
			"transferred_bytes":   expectedBytes,
			"sha256":              sha256,
			"multipart_upload_id": "",
			"updated_at":          now,
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer upload state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) ClaimVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, claimedAt, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND state = ?
			AND (creation_lease_expires_at IS NULL OR creation_lease_expires_at <= ?)`,
			projectID, transferID, expectedState, claimedAt.UTC()).
		Updates(map[string]any{
			"execution_generation":      gorm.Expr("execution_generation + 1"),
			"creation_lease_owner":      leaseOwner,
			"creation_lease_expires_at": leaseExpiresAt.UTC(),
			"job_created_at":            nil,
			"updated_at":                claimedAt.UTC(),
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer execution lease is held")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) RenewVolumeTransferExecutionLease(ctx context.Context, projectID, transferID, leaseOwner string, generation int64, renewedAt, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND state IN ? AND execution_generation = ?
			AND creation_lease_owner = ? AND creation_lease_expires_at > ?`,
			projectID, transferID, []string{model.VolumeTransferStateQueued, model.VolumeTransferStateRunning}, generation, leaseOwner, renewedAt.UTC()).
		Updates(map[string]any{
			"creation_lease_expires_at": leaseExpiresAt.UTC(),
			"updated_at":                renewedAt.UTC(),
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer execution lease changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) PrepareVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, generation int64, tokenHash string, expiresAt time.Time) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	updates := map[string]any{
		"state":                     model.VolumeTransferStateRunning,
		"callback_token_hash":       tokenHash,
		"callback_token_expires_at": expiresAt.UTC(),
		"completion_reported_at":    nil,
		"job_succeeded_at":          nil,
		"updated_at":                now,
	}
	if expectedState == model.VolumeTransferStateQueued {
		updates["started_at"] = now
	}
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND state = ? AND execution_generation = ?
			AND creation_lease_owner = ? AND creation_lease_expires_at > ?`,
			projectID, transferID, expectedState, generation, leaseOwner, now).
		Updates(updates)
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer execution state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) ConfirmVolumeTransferJobCreated(ctx context.Context, projectID, transferID string, generation int64) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state = ? AND execution_generation = ? AND job_created_at IS NULL",
			projectID, transferID, model.VolumeTransferStateRunning, generation).
		Updates(map[string]any{
			"job_created_at":            now,
			"creation_lease_owner":      "",
			"creation_lease_expires_at": nil,
			"updated_at":                now,
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer Job creation state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) ReportVolumeTransferCompletion(ctx context.Context, projectID, transferID string, completion TransferCompletion) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state = ? AND completion_reported_at IS NULL", projectID, transferID, completion.ExpectedState).
		Updates(map[string]any{
			"expected_bytes":         completion.TransferredBytes,
			"transferred_bytes":      completion.TransferredBytes,
			"sha256":                 completion.SHA256,
			"logical_bytes":          completion.LogicalBytes,
			"data_sha256":            completion.DataSHA256,
			"multipart_upload_id":    "",
			"completion_reported_at": now,
			"last_error_code":        "",
			"last_error_message":     "",
			"updated_at":             now,
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer completion report state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) MarkVolumeTransferJobSucceeded(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state = ? AND job_succeeded_at IS NULL", projectID, transferID, model.VolumeTransferStateRunning).
		Updates(map[string]any{"job_succeeded_at": now, "updated_at": now})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer job success state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) FinalizeVolumeTransferExecution(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state = ? AND completion_reported_at IS NOT NULL AND job_succeeded_at IS NOT NULL",
			projectID, transferID, model.VolumeTransferStateRunning).
		Updates(map[string]any{
			"state": model.VolumeTransferStateSucceeded, "finished_at": now,
			"last_error_code": "", "last_error_message": "", "updated_at": now,
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer finalization state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) MarkVolumeTransferExecutionCleanupCompleted(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	now := time.Now().UTC()
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state IN ? AND execution_cleanup_completed_at IS NULL", projectID, transferID, []string{
			model.VolumeTransferStateSucceeded,
			model.VolumeTransferStateFailed,
			model.VolumeTransferStateCancelled,
			model.VolumeTransferStateExpired,
		}).Updates(map[string]any{"execution_cleanup_completed_at": now, "updated_at": now})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "volume transfer execution cleanup state changed")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) UpdateVolumeTransferProgress(ctx context.Context, projectID, transferID string, progress TransferProgress) (model.VolumeTransfer, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND state IN ? AND transferred_bytes <= ? AND processed_files <= ?", projectID, transferID,
			[]string{model.VolumeTransferStateUploading, model.VolumeTransferStateRunning}, progress.TransferredBytes, progress.ProcessedFiles).
		Updates(map[string]any{
			"transferred_bytes": progress.TransferredBytes,
			"processed_files":   progress.ProcessedFiles,
			"phase":             progress.Phase,
			"updated_at":        time.Now().UTC(),
		})
	if result.Error != nil {
		return model.VolumeTransfer{}, result.Error
	}
	if result.RowsAffected != 1 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferProgressInvalid, "volume transfer progress cannot move backwards")
	}
	return repository.GetVolumeTransfer(ctx, projectID, transferID)
}

func (repository *GormRepository) CreateVolumeTransferPart(ctx context.Context, part *model.VolumeTransferPart) (bool, model.VolumeTransferPart, error) {
	result := repository.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(part)
	if result.Error != nil {
		return false, model.VolumeTransferPart{}, result.Error
	}
	if result.RowsAffected == 1 {
		return true, *part, nil
	}
	var existing model.VolumeTransferPart
	err := repository.db.WithContext(ctx).
		Where("transfer_id = ? AND (part_number = ? OR byte_offset = ?)", part.TransferID, part.PartNumber, part.Offset).
		First(&existing).Error
	return false, existing, err
}

func (repository *GormRepository) GetVolumeTransferPartByOffset(ctx context.Context, transferID string, offset int64) (model.VolumeTransferPart, error) {
	var part model.VolumeTransferPart
	err := repository.db.WithContext(ctx).
		Where("transfer_id = ? AND byte_offset = ?", transferID, offset).
		First(&part).Error
	return part, err
}

func (repository *GormRepository) TakeOverVolumeTransferPart(ctx context.Context, transferID string, partNumber int, expectedLeaseToken, leaseToken string, leaseExpiresAt time.Time) (bool, model.VolumeTransferPart, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransferPart{}).
		Where("transfer_id = ? AND part_number = ? AND state = ? AND lease_token = ?", transferID, partNumber, model.VolumeTransferPartStateReserved, expectedLeaseToken).
		Updates(map[string]any{
			"lease_token":      leaseToken,
			"lease_expires_at": leaseExpiresAt.UTC(),
			"updated_at":       time.Now().UTC(),
		})
	if result.Error != nil {
		return false, model.VolumeTransferPart{}, result.Error
	}
	if result.RowsAffected != 1 {
		return false, model.VolumeTransferPart{}, nil
	}
	var stored model.VolumeTransferPart
	err := repository.db.WithContext(ctx).Where("transfer_id = ? AND part_number = ?", transferID, partNumber).First(&stored).Error
	return true, stored, err
}

func (repository *GormRepository) CompleteVolumeTransferPart(ctx context.Context, transferID string, partNumber int, leaseToken, etag string) (bool, model.VolumeTransferPart, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransferPart{}).
		Where("transfer_id = ? AND part_number = ? AND state = ? AND lease_token = ?", transferID, partNumber, model.VolumeTransferPartStateReserved, leaseToken).
		Updates(map[string]any{
			"etag":             etag,
			"state":            model.VolumeTransferPartStateCompleted,
			"lease_token":      "",
			"lease_expires_at": nil,
			"updated_at":       time.Now().UTC(),
		})
	if result.Error != nil {
		return false, model.VolumeTransferPart{}, result.Error
	}
	var stored model.VolumeTransferPart
	err := repository.db.WithContext(ctx).Where("transfer_id = ? AND part_number = ?", transferID, partNumber).First(&stored).Error
	return result.RowsAffected == 1, stored, err
}

func (repository *GormRepository) ExpireVolumeTransferPartLease(ctx context.Context, transferID string, partNumber int, leaseToken string, expiredAt time.Time) (bool, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransferPart{}).
		Where("transfer_id = ? AND part_number = ? AND state = ? AND lease_token = ?", transferID, partNumber, model.VolumeTransferPartStateReserved, leaseToken).
		Updates(map[string]any{"lease_expires_at": expiredAt.UTC(), "updated_at": time.Now().UTC()})
	return result.RowsAffected == 1, result.Error
}

func (repository *GormRepository) ListVolumeTransferParts(ctx context.Context, transferID string, page, pageSize int) ([]model.VolumeTransferPart, int64, error) {
	page, pageSize = normalizeRepositoryPage(page, pageSize)
	query := repository.db.WithContext(ctx).Model(&model.VolumeTransferPart{}).
		Where("transfer_id = ? AND state = ?", transferID, model.VolumeTransferPartStateCompleted)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	parts := make([]model.VolumeTransferPart, 0)
	err := query.Order("part_number ASC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&parts).Error
	return parts, total, err
}

func (repository *GormRepository) VolumeTransferUploadOffset(ctx context.Context, transferID string) (int64, error) {
	var result struct{ Offset int64 }
	err := repository.db.WithContext(ctx).Model(&model.VolumeTransferPart{}).
		Select(`COALESCE(MAX(byte_offset + size), 0) AS "offset"`).
		Where("transfer_id = ? AND state = ?", transferID, model.VolumeTransferPartStateCompleted).Scan(&result).Error
	return result.Offset, err
}

func (repository *GormRepository) NextVolumeTransferPartNumber(ctx context.Context, transferID string) (int, error) {
	var result struct{ PartNumber int }
	err := repository.db.WithContext(ctx).Model(&model.VolumeTransferPart{}).
		Select("COALESCE(MAX(part_number), 0) + 1 AS part_number").
		Where("transfer_id = ?", transferID).Scan(&result).Error
	return result.PartNumber, err
}

func (repository *GormRepository) ListStaleProjectVolumes(ctx context.Context, cutoff time.Time, limit int) ([]model.ProjectVolume, error) {
	limit = normalizeMaintenanceLimit(limit)
	items := make([]model.ProjectVolume, 0)
	err := repository.db.WithContext(ctx).Where("lifecycle_state IN ? AND updated_at < ?", []string{
		model.ProjectVolumeLifecycleProvisioning,
		model.ProjectVolumeLifecycleDeleting,
	}, cutoff.UTC()).Order("updated_at ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (repository *GormRepository) ListStaleVolumeTransfers(ctx context.Context, cutoff time.Time, limit int) ([]model.VolumeTransfer, error) {
	limit = normalizeMaintenanceLimit(limit)
	items := make([]model.VolumeTransfer, 0)
	// A cancellation is durable before its cleanup task is enqueued. Include
	// cancelled transfers whose backing object has not been removed so the
	// periodic reconciler repairs a transient queue failure without requiring
	// another user request. An import remains eligible until its provisional
	// ProjectVolume is also soft-deleted; this closes the narrow crash window
	// between object cleanup and domain-asset cleanup. Completed cleanup is
	// excluded to avoid requeueing a terminal transfer forever.
	err := repository.db.WithContext(ctx).
		Where(`updated_at < ? AND (state IN ? OR (state IN ? AND execution_cleanup_completed_at IS NULL) OR (state = ? AND (
			(object_owned = true AND object_deleted_at IS NULL) OR (direction = ? AND EXISTS (
				SELECT 1 FROM project_volumes
				WHERE project_volumes.id = volume_transfers.project_volume_id
				  AND project_volumes.deleted_at IS NULL
			))
		)))`, cutoff.UTC(), []string{
			model.VolumeTransferStateQueued,
			model.VolumeTransferStateRunning,
		}, []string{
			model.VolumeTransferStateSucceeded,
			model.VolumeTransferStateFailed,
			model.VolumeTransferStateCancelled,
			model.VolumeTransferStateExpired,
		}, model.VolumeTransferStateCancelled, model.VolumeTransferDirectionImport).
		Order("updated_at ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (repository *GormRepository) ListExpiredVolumeTransferObjects(ctx context.Context, now time.Time, limit int) ([]model.VolumeTransfer, error) {
	limit = normalizeMaintenanceLimit(limit)
	items := make([]model.VolumeTransfer, 0)
	err := repository.db.WithContext(ctx).
		Where(`expires_at <= ? AND object_owned = true AND object_deleted_at IS NULL AND (
			state IN ? OR (state IN ? AND execution_cleanup_completed_at IS NOT NULL)
		)`, now.UTC(), []string{
			model.VolumeTransferStateCreated,
			model.VolumeTransferStateUploading,
		}, []string{
			model.VolumeTransferStateSucceeded,
			model.VolumeTransferStateFailed,
			model.VolumeTransferStateCancelled,
			model.VolumeTransferStateExpired,
		}).Order("expires_at ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (repository *GormRepository) TransferVolumeTransferObjectOwnership(ctx context.Context, projectID, transferID string, transferredAt time.Time) (bool, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND object_owned = true AND object_deleted_at IS NULL
			AND object_cleanup_started_at IS NULL`, projectID, transferID).
		Updates(map[string]any{
			"object_owned":                    false,
			"object_cleanup_lease_token":      "",
			"object_cleanup_lease_expires_at": nil,
			"updated_at":                      transferredAt.UTC(),
		})
	return result.RowsAffected == 1, result.Error
}

func (repository *GormRepository) ClaimVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, claimedAt, leaseExpiresAt time.Time) (bool, model.VolumeTransfer, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND object_owned = true AND object_deleted_at IS NULL
			AND execution_cleanup_completed_at IS NOT NULL AND state IN ?
			AND (object_cleanup_lease_expires_at IS NULL OR object_cleanup_lease_expires_at <= ?)`,
			projectID, transferID, []string{
				model.VolumeTransferStateSucceeded,
				model.VolumeTransferStateFailed,
				model.VolumeTransferStateCancelled,
				model.VolumeTransferStateExpired,
			}, claimedAt.UTC()).
		Updates(map[string]any{
			"object_cleanup_started_at":       gorm.Expr("COALESCE(object_cleanup_started_at, ?)", claimedAt.UTC()),
			"object_cleanup_lease_token":      leaseToken,
			"object_cleanup_lease_expires_at": leaseExpiresAt.UTC(),
			"updated_at":                      claimedAt.UTC(),
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return false, model.VolumeTransfer{}, result.Error
	}
	transfer, err := repository.GetVolumeTransfer(ctx, projectID, transferID)
	return err == nil, transfer, err
}

func (repository *GormRepository) RenewVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, renewedAt, leaseExpiresAt time.Time) (bool, model.VolumeTransfer, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND object_owned = true AND object_deleted_at IS NULL
			AND object_cleanup_lease_token = ? AND object_cleanup_lease_expires_at > ?`,
			projectID, transferID, leaseToken, renewedAt.UTC()).
		Updates(map[string]any{
			"object_cleanup_lease_expires_at": leaseExpiresAt.UTC(),
			"updated_at":                      renewedAt.UTC(),
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return false, model.VolumeTransfer{}, result.Error
	}
	transfer, err := repository.GetVolumeTransfer(ctx, projectID, transferID)
	return err == nil, transfer, err
}

func (repository *GormRepository) CompleteVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, deletedAt time.Time) (bool, model.VolumeTransfer, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND object_owned = true AND object_deleted_at IS NULL
			AND object_cleanup_lease_token = ?`, projectID, transferID, leaseToken).
		Updates(map[string]any{
			"object_owned":                    false,
			"object_deleted_at":               deletedAt.UTC(),
			"object_cleanup_lease_token":      "",
			"object_cleanup_lease_expires_at": nil,
			"updated_at":                      deletedAt.UTC(),
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return false, model.VolumeTransfer{}, result.Error
	}
	transfer, err := repository.GetVolumeTransfer(ctx, projectID, transferID)
	return err == nil, transfer, err
}

func (repository *GormRepository) ReleaseVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, releasedAt time.Time) (bool, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where(`project_id = ? AND id = ? AND object_owned = true AND object_deleted_at IS NULL
			AND object_cleanup_lease_token = ?`, projectID, transferID, leaseToken).
		Updates(map[string]any{
			"object_cleanup_lease_token":      "",
			"object_cleanup_lease_expires_at": nil,
			"updated_at":                      releasedAt.UTC(),
		})
	return result.RowsAffected == 1, result.Error
}

func (repository *GormRepository) MarkVolumeTransferObjectDeleted(ctx context.Context, projectID, transferID string, deletedAt time.Time) (bool, error) {
	result := repository.db.WithContext(ctx).Model(&model.VolumeTransfer{}).
		Where("project_id = ? AND id = ? AND object_owned = true AND object_deleted_at IS NULL AND execution_cleanup_completed_at IS NOT NULL AND state IN ?", projectID, transferID, []string{
			model.VolumeTransferStateSucceeded,
			model.VolumeTransferStateFailed,
			model.VolumeTransferStateCancelled,
			model.VolumeTransferStateExpired,
		}).Updates(map[string]any{
		"object_owned":                    false,
		"object_deleted_at":               deletedAt.UTC(),
		"object_cleanup_lease_token":      "",
		"object_cleanup_lease_expires_at": nil,
		"updated_at":                      deletedAt.UTC(),
	})
	return result.RowsAffected == 1, result.Error
}

var projectVolumeSortColumns = map[string]string{
	"createdAt":   "created_at",
	"updatedAt":   "updated_at",
	"displayName": "lower(display_name)",
	"capacity":    "capacity_bytes",
}

var volumeTransferSortColumns = map[string]string{
	"createdAt":        "created_at",
	"updatedAt":        "updated_at",
	"state":            "state",
	"transferredBytes": "transferred_bytes",
	"direction":        "direction",
	"size":             "expected_bytes",
}

func blockingVolumeTransferQuery(query *gorm.DB) *gorm.DB {
	return query.Where("(state IN ? OR (state IN ? AND execution_cleanup_completed_at IS NULL) OR (state = ? AND object_owned = true AND object_deleted_at IS NULL))", []string{
		model.VolumeTransferStateCreated,
		model.VolumeTransferStateUploading,
		model.VolumeTransferStateQueued,
		model.VolumeTransferStateRunning,
	}, []string{
		model.VolumeTransferStateSucceeded,
		model.VolumeTransferStateFailed,
		model.VolumeTransferStateCancelled,
		model.VolumeTransferStateExpired,
	}, model.VolumeTransferStateCancelled)
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)
	return replacer.Replace(strings.TrimSpace(value))
}

func normalizeRepositoryPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	return page, pageSize
}

func normalizeMaintenanceLimit(limit int) int {
	if limit < 1 {
		return MaxPageSize
	}
	if limit > MaxPageSize {
		return MaxPageSize
	}
	return limit
}

func projectVolumeTransitionUpdates(to, errorCode, errorMessage string, now time.Time) map[string]any {
	updates := map[string]any{
		"lifecycle_state":    to,
		"last_error_code":    errorCode,
		"last_error_message": errorMessage,
		"revision":           gorm.Expr("revision + 1"),
		"updated_at":         now,
	}
	if to == model.ProjectVolumeLifecycleReady {
		updates["pending_operation"] = ""
	}
	return updates
}

func normalizeRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if normalized := normalizeQuotaPersistenceError(err); ErrorCode(normalized) != "" {
		return normalized
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return newDomainError(CodeNotFound, "project volume resource was not found")
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "idx_project_volumes_display_name_active"):
		return newDomainError(CodeNameConflict, "a project volume with this display name already exists", err)
	case strings.Contains(message, "idx_project_volumes_claim_active"):
		return newDomainError(CodeClaimConflict, "this persistent volume claim is already registered", err)
	case strings.Contains(message, "idx_project_volumes_idempotency_active"):
		return newDomainError(CodeIdempotencyConflict, "idempotency key is already in use", err)
	case strings.Contains(message, "idx_deployment_volume_mounts_"):
		return newDomainError(CodeBindingConflict, "deployment volume mount conflicts with an existing binding", err)
	case strings.Contains(message, "idx_volume_transfers_active_"):
		return newDomainError(CodeInUse, "the project volume already has an active transfer", err)
	default:
		return fmt.Errorf("volume persistence operation: %w", err)
	}
}
