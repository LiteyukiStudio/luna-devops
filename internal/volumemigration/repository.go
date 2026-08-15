package volumemigration

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (repository *GormRepository) ListProjects(ctx context.Context, page, pageSize int, projectID string) ([]model.Project, error) {
	query := repository.db.WithContext(ctx).Model(&model.Project{}).Order("id asc").Limit(pageSize).Offset((page - 1) * pageSize)
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		query = query.Where("id = ?", projectID)
	}
	var projects []model.Project
	err := query.Find(&projects).Error
	return projects, err
}

func (repository *GormRepository) ListRetainedVolumes(ctx context.Context, projectID string, page, pageSize int) ([]model.RetainedVolume, error) {
	var volumes []model.RetainedVolume
	err := repository.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&volumes).Error
	return volumes, err
}

func (repository *GormRepository) ListDeploymentTargets(ctx context.Context, projectID string, page, pageSize int) ([]model.DeploymentTarget, error) {
	var targets []model.DeploymentTarget
	err := repository.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("id asc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&targets).Error
	return targets, err
}

func (repository *GormRepository) ResolveRuntimeClusterID(ctx context.Context, clusterID string) (string, error) {
	if repository == nil || repository.db == nil {
		return "", ErrRuntimeCluster
	}
	var cluster model.RuntimeCluster
	db := repository.db.WithContext(ctx).Where("type IN ?", []string{"kubernetes", "k3s"})
	if clusterID = strings.TrimSpace(clusterID); clusterID != "" {
		if err := db.First(&cluster, "id = ?", clusterID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", ErrRuntimeCluster
			}
			return "", err
		}
		return cluster.ID, nil
	}
	err := db.Where("scope = ? AND is_default = ?", "global", true).Order("created_at asc, id asc").First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = db.Where("scope = ?", "global").Order("created_at asc, id asc").First(&cluster).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrRuntimeCluster
	}
	if err != nil {
		return "", err
	}
	return cluster.ID, nil
}

func (repository *GormRepository) GetApplication(ctx context.Context, projectID, applicationID string) (model.Application, error) {
	var application model.Application
	err := repository.db.WithContext(ctx).Unscoped().Where("project_id = ? AND id = ?", projectID, applicationID).First(&application).Error
	return application, err
}

func (repository *GormRepository) GetProjectVolume(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	var volume model.ProjectVolume
	err := repository.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, volumeID).First(&volume).Error
	return volume, err
}

func (repository *GormRepository) SyncProjectVolume(ctx context.Context, desired model.ProjectVolume, apply bool) (SyncResult, error) {
	if repository == nil || repository.db == nil {
		return SyncResult{}, ErrProjectVolumeConflict
	}
	if !apply {
		existing, err := findProjectVolumeForSync(repository.db.WithContext(ctx), desired, false)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SyncResult{Outcome: OutcomePlanned}, nil
		}
		if err != nil {
			return SyncResult{}, err
		}
		if !projectVolumesMatch(existing, desired) {
			return SyncResult{}, ErrProjectVolumeConflict
		}
		return SyncResult{Outcome: OutcomeUnchanged, CapacityBytes: existing.CapacityBytes}, nil
	}
	result := SyncResult{}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := findProjectVolumeForSync(tx, desired, true)
		if err == nil {
			if !projectVolumesMatch(existing, desired) {
				return ErrProjectVolumeConflict
			}
			result = SyncResult{Outcome: OutcomeUnchanged, CapacityBytes: existing.CapacityBytes}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&desired)
		if createResult.Error != nil {
			return createResult.Error
		}
		existing, err = findProjectVolumeForSync(tx, desired, true)
		if err != nil {
			if createResult.RowsAffected == 0 && errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrProjectVolumeConflict
			}
			return err
		}
		if !projectVolumesMatch(existing, desired) {
			return ErrProjectVolumeConflict
		}
		outcome := OutcomeApplied
		if createResult.RowsAffected == 0 {
			outcome = OutcomeUnchanged
		}
		result = SyncResult{Outcome: outcome, CapacityBytes: existing.CapacityBytes}
		return nil
	})
	return result, err
}

func findProjectVolumeForSync(tx *gorm.DB, desired model.ProjectVolume, lock bool) (model.ProjectVolume, error) {
	var existing model.ProjectVolume
	query := tx
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.
		Where("id = ? OR (cluster_id = ? AND namespace = ? AND claim_name = ? AND deleted_at IS NULL)", desired.ID, desired.ClusterID, desired.Namespace, desired.ClaimName).
		Order(clause.Expr{SQL: "CASE WHEN id = ? THEN 0 ELSE 1 END", Vars: []any{desired.ID}}).
		First(&existing).Error
	return existing, err
}

func (repository *GormRepository) SyncDeploymentVolumeMount(ctx context.Context, desired model.DeploymentVolumeMount, apply bool) (SyncResult, error) {
	if repository == nil || repository.db == nil {
		return SyncResult{}, ErrDeploymentMountConflict
	}
	if !apply {
		existing, err := findDeploymentMountForSync(repository.db.WithContext(ctx), desired, false)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SyncResult{Outcome: OutcomePlanned}, nil
		}
		if err != nil {
			return SyncResult{}, err
		}
		if !deploymentMountStaticFieldsMatch(existing, desired) || !deploymentMountActivationCompatible(existing.ActivationState, desired.ActivationState) {
			return SyncResult{}, ErrDeploymentMountConflict
		}
		if existing.ActivationState == model.DeploymentVolumeActivationReserved && desired.ActivationState == model.DeploymentVolumeActivationActive {
			return SyncResult{Outcome: OutcomePlanned}, nil
		}
		return SyncResult{Outcome: OutcomeUnchanged}, nil
	}
	result := SyncResult{}
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := findDeploymentMountForSync(tx, desired, true)
		if err == nil {
			if !deploymentMountStaticFieldsMatch(existing, desired) || !deploymentMountActivationCompatible(existing.ActivationState, desired.ActivationState) {
				return ErrDeploymentMountConflict
			}
			if existing.ActivationState == model.DeploymentVolumeActivationReserved && desired.ActivationState == model.DeploymentVolumeActivationActive {
				if err := tx.Model(&model.DeploymentVolumeMount{}).Where("id = ? AND activation_state = ?", existing.ID, model.DeploymentVolumeActivationReserved).
					Updates(map[string]any{"activation_state": model.DeploymentVolumeActivationActive, "updated_at": desired.UpdatedAt}).Error; err != nil {
					return err
				}
				result = SyncResult{Outcome: OutcomeApplied}
				return nil
			}
			result = SyncResult{Outcome: OutcomeUnchanged}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&desired)
		if createResult.Error != nil {
			return createResult.Error
		}
		existing, err = findDeploymentMountForSync(tx, desired, true)
		if err != nil {
			if createResult.RowsAffected == 0 && errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDeploymentMountConflict
			}
			return err
		}
		if !deploymentMountsMatch(existing, desired) {
			return ErrDeploymentMountConflict
		}
		outcome := OutcomeApplied
		if createResult.RowsAffected == 0 {
			outcome = OutcomeUnchanged
		}
		result = SyncResult{Outcome: outcome}
		return nil
	})
	return result, err
}

func findDeploymentMountForSync(tx *gorm.DB, desired model.DeploymentVolumeMount, lock bool) (model.DeploymentVolumeMount, error) {
	var existing model.DeploymentVolumeMount
	query := tx
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.
		Where("id = ? OR (deployment_target_id = ? AND logical_name = ? AND deleted_at IS NULL)", desired.ID, desired.DeploymentTargetID, desired.LogicalName).
		Order(clause.Expr{SQL: "CASE WHEN id = ? THEN 0 ELSE 1 END", Vars: []any{desired.ID}}).
		First(&existing).Error
	return existing, err
}

func projectVolumesMatch(existing, desired model.ProjectVolume) bool {
	return existing.ID == desired.ID &&
		existing.ProjectID == desired.ProjectID &&
		existing.ClusterID == desired.ClusterID &&
		existing.Namespace == desired.Namespace &&
		existing.ClaimName == desired.ClaimName &&
		existing.OwnershipMode == desired.OwnershipMode &&
		existing.SourceKind == desired.SourceKind &&
		existing.LifecycleState == desired.LifecycleState &&
		existing.PendingOperation == desired.PendingOperation &&
		existing.CapacityBytes == desired.CapacityBytes &&
		existing.StorageClassName == desired.StorageClassName &&
		existing.AccessMode == desired.AccessMode &&
		existing.VolumeMode == desired.VolumeMode &&
		optionalStringsEqual(existing.SourceApplicationID, desired.SourceApplicationID) &&
		optionalStringsEqual(existing.SourceDeploymentTargetID, desired.SourceDeploymentTargetID)
}

func deploymentMountsMatch(existing, desired model.DeploymentVolumeMount) bool {
	return deploymentMountStaticFieldsMatch(existing, desired) && deploymentMountActivationCompatible(existing.ActivationState, desired.ActivationState)
}

func deploymentMountStaticFieldsMatch(existing, desired model.DeploymentVolumeMount) bool {
	return existing.ID == desired.ID &&
		existing.ProjectID == desired.ProjectID &&
		existing.ApplicationID == desired.ApplicationID &&
		existing.DeploymentTargetID == desired.DeploymentTargetID &&
		existing.SourceType == desired.SourceType &&
		optionalStringsEqual(existing.ProjectVolumeID, desired.ProjectVolumeID) &&
		existing.LogicalName == desired.LogicalName &&
		optionalStringsEqual(existing.MountPath, desired.MountPath) &&
		optionalStringsEqual(existing.DevicePath, desired.DevicePath) &&
		existing.ReadOnly == desired.ReadOnly &&
		existing.Exclusive == desired.Exclusive &&
		existing.EmptyDirMedium == desired.EmptyDirMedium &&
		existing.EmptyDirSizeLimit == desired.EmptyDirSizeLimit
}

func deploymentMountActivationCompatible(existing, desired string) bool {
	return existing == desired ||
		existing == model.DeploymentVolumeActivationActive && desired == model.DeploymentVolumeActivationReserved ||
		existing == model.DeploymentVolumeActivationReserved && desired == model.DeploymentVolumeActivationActive
}

func optionalStringsEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
