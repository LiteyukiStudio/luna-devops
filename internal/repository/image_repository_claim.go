package repository

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/imageref"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

// ImageRepositoryClaimedByOtherProject 报告同一镜像站的目标仓库是否已被其它项目空间的部署配置或构建使用。
// 目标仓库按项目空间独占：共享推送凭据下，跨项目写入会覆盖对方镜像或向对方的 Registry 构建缓存投毒。
func ImageRepositoryClaimedByOtherProject(ctx context.Context, db *gorm.DB, projectID string, registry model.ArtifactRegistry, targetRepository string) (bool, error) {
	references := imageref.RepositoryReferences(registry, targetRepository)
	if registry.ID == "" || len(references) == 0 {
		return false, nil
	}
	for _, claimant := range []any{&model.DeploymentTarget{}, &model.BuildRun{}} {
		var ids []string
		if err := db.WithContext(ctx).Model(claimant).
			Where("target_registry_id = ? and target_repository in ? and project_id <> ?", registry.ID, references, projectID).
			Limit(1).
			Pluck("id", &ids).Error; err != nil {
			return false, err
		}
		if len(ids) > 0 {
			return true, nil
		}
	}
	return false, nil
}
