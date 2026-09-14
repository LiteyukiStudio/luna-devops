package runtimecluster

import (
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

// ActiveProjectReferenceCounts counts explicit cluster references owned by
// projects that have not completed deletion.
func ActiveProjectReferenceCounts(db *gorm.DB, clusterID string) (int64, int64, error) {
	var targetCount int64
	if err := db.Model(&model.DeploymentTarget{}).
		Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
		Where("cluster_id = ?", clusterID).
		Count(&targetCount).Error; err != nil {
		return 0, 0, err
	}
	var volumeCount int64
	if err := db.Model(&model.ProjectVolume{}).
		Joins("join projects on projects.id = project_volumes.project_id and projects.deleted_at is null").
		Where("cluster_id = ?", clusterID).
		Count(&volumeCount).Error; err != nil {
		return 0, 0, err
	}
	return targetCount, volumeCount, nil
}
