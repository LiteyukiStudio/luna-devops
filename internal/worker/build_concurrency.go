package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

// applyBuildConcurrencyPolicy 在容量检查前执行部署配置的构建并发策略。
// queue 返回 errBuildCapacityUnavailable，让构建沿用容量等待的延迟重投路径。
func (r *Runner) applyBuildConcurrencyPolicy(ctx context.Context, run model.BuildRun, target model.DeploymentTarget) error {
	switch target.BuildConcurrencyPolicy {
	case model.BuildConcurrencyParallel:
		return nil
	case model.BuildConcurrencyCancelInProgress:
		return r.cancelSupersededBuildRuns(ctx, run)
	case model.BuildConcurrencyQueue:
		var blocking model.BuildRun
		// 排在前面的未结束构建，或已在运行的构建（例如策略切换前并行启动的）都要先结束。
		err := sameTargetActiveBuildRuns(r.db.WithContext(ctx), run).
			Where("(build_runs.status = ? or build_runs.created_at < ? or (build_runs.created_at = ? and build_runs.id < ?))", "running", run.CreatedAt, run.CreatedAt, run.ID).
			Order("build_runs.created_at asc, build_runs.id asc").
			Limit(1).
			Find(&blocking).Error
		if err != nil {
			return err
		}
		if blocking.ID != "" {
			return fmt.Errorf("%w: deployment target %s waits for build run %s", errBuildCapacityUnavailable, run.DeploymentTargetID, blocking.ID)
		}
		return nil
	default:
		return fmt.Errorf("unknown build concurrency policy %q", target.BuildConcurrencyPolicy)
	}
}

// sameTargetActiveBuildRuns 选出同一部署配置中除 run 以外未结束的构建。
func sameTargetActiveBuildRuns(db *gorm.DB, run model.BuildRun) *gorm.DB {
	return db.Model(&model.BuildRun{}).
		Where("build_runs.project_id = ? and build_runs.deployment_target_id = ? and build_runs.id <> ? and build_runs.status in ?", run.ProjectID, run.DeploymentTargetID, run.ID, model.BuildActiveStatuses)
}

// earlierActiveBuildRuns 按 (created_at, id) 的全序选出同一部署配置中排在 run 之前且未结束的构建。
func earlierActiveBuildRuns(db *gorm.DB, run model.BuildRun) *gorm.DB {
	return sameTargetActiveBuildRuns(db, run).
		Where("(build_runs.created_at < ? or (build_runs.created_at = ? and build_runs.id < ?))", run.CreatedAt, run.CreatedAt, run.ID)
}

// cancelSupersededBuildRuns 取消同一部署配置更早的排队中和运行中构建；运行中的构建 Job 由其跟踪循环在下一次轮询时删除。
func (r *Runner) cancelSupersededBuildRuns(ctx context.Context, run model.BuildRun) error {
	finishedAt := time.Now()
	var supersededIDs []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := earlierActiveBuildRuns(tx, run).Pluck("build_runs.id", &supersededIDs).Error; err != nil {
			return err
		}
		if len(supersededIDs) == 0 {
			return nil
		}
		updates := map[string]any{
			"status":       "canceled",
			"failure_code": model.BuildFailureSuperseded,
			"finished_at":  &finishedAt,
		}
		if err := tx.Model(&model.BuildJob{}).
			Where("project_id = ? and build_run_id in ? and status in ?", run.ProjectID, supersededIDs, model.BuildActiveStatuses).
			Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BuildRun{}).
			Where("project_id = ? and id in ? and status in ?", run.ProjectID, supersededIDs, model.BuildActiveStatuses).
			Updates(updates).Error; err != nil {
			return err
		}
		audits := make([]model.AuditLog, 0, len(supersededIDs))
		for _, supersededID := range supersededIDs {
			audits = append(audits, model.AuditLog{
				ID:       id.New("aud"),
				UserID:   strings.TrimSpace(run.CreatedBy),
				Action:   "build_run.supersede",
				Resource: supersededID,
				Success:  true,
				Message:  "superseded by " + run.ID,
			})
		}
		return tx.Create(&audits).Error
	})
	return err
}
