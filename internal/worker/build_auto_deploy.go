package worker

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"gorm.io/gorm"
)

func (r *Runner) enqueueAutoDeploymentsForBuildRun(ctx context.Context, run model.BuildRun) {
	if run.Status != "succeeded" || strings.TrimSpace(run.ImageRef) == "" || strings.TrimSpace(run.DeploymentTargetID) == "" || r.taskClient == nil {
		return
	}
	var application model.Application
	if err := r.db.First(&application, "id = ? and project_id = ?", run.ApplicationID, run.ProjectID).Error; err != nil {
		return
	}
	if !applicationRuntimeCanMutate(application) {
		return
	}
	var project model.Project
	if err := r.db.First(&project, "id = ?", run.ProjectID).Error; err != nil || !model.DeleteStatusActive(project.DeleteStatus) {
		return
	}
	var target model.DeploymentTarget
	if err := r.db.First(
		&target,
		"id = ? and project_id = ? and application_id = ? and enabled = ? and auto_deploy = ? and require_approval = ? and delete_status in ?",
		run.DeploymentTargetID,
		run.ProjectID,
		run.ApplicationID,
		true,
		true,
		false,
		model.ActiveDeleteStatuses,
	).Error; err != nil {
		return
	}
	if !deploymentTargetMatchesBuildRun(target, run) {
		return
	}
	// 并行构建可能乱序完成：同一部署配置更新的构建已经产生发布时，旧构建不再自动部署，避免回退到旧镜像。
	if superseded, err := r.newerBuildReleased(run, target.ID); err != nil || superseded {
		return
	}
	release, ok := r.createAutoDeployRelease(run, target)
	if !ok {
		return
	}
	if _, err := r.taskClient.EnqueueDeployRun(ctx, tasks.DeployRunPayload{
		ReleaseID: release.ID,
		ProjectID: release.ProjectID,
		ActorID:   release.CreatedBy,
	}); err != nil {
		_, _ = r.finishDeployRelease(ctx, release, "failed", model.ReleaseFailureDispatchFailed, "deploy task dispatch failed: "+err.Error())
	}
}

func (r *Runner) newerBuildReleased(run model.BuildRun, targetID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.Release{}).
		Joins("join build_runs on build_runs.id = releases.build_run_id and build_runs.project_id = releases.project_id").
		Where("releases.project_id = ? and releases.application_id = ? and releases.deployment_target_id = ?", run.ProjectID, run.ApplicationID, targetID).
		Where("(build_runs.created_at > ? or (build_runs.created_at = ? and build_runs.id > ?))", run.CreatedAt, run.CreatedAt, run.ID).
		Count(&count).Error
	return count > 0, err
}

func (r *Runner) createAutoDeployRelease(run model.BuildRun, target model.DeploymentTarget) (model.Release, bool) {
	release := model.Release{}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var existing model.Release
		err := tx.First(&existing, "project_id = ? and application_id = ? and deployment_target_id = ? and build_run_id = ?", run.ProjectID, run.ApplicationID, target.ID, run.ID).Error
		if err == nil {
			release = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		revision, err := nextReleaseRevisionFor(tx, run.ProjectID, run.ApplicationID, target.ID)
		if err != nil {
			return err
		}
		release = model.Release{
			ID:                 id.New("rel"),
			ProjectID:          run.ProjectID,
			ApplicationID:      run.ApplicationID,
			DeploymentTargetID: target.ID,
			BuildRunID:         run.ID,
			ImageRef:           run.ImageRef,
			Type:               "deploy",
			Status:             "pending",
			Revision:           revision,
			Message:            "auto deploy from build",
			CreatedBy:          run.CreatedBy,
		}
		return tx.Create(&release).Error
	})
	return release, err == nil && release.ID != "" && release.Status == "pending"
}

func deploymentTargetMatchesBuildRun(target model.DeploymentTarget, run model.BuildRun) bool {
	return matchesDeploymentPattern(target.BranchPattern, run.SourceBranch) && matchesDeploymentPattern(target.TagPattern, run.SourceTag)
}

func matchesDeploymentPattern(patterns string, value string) bool {
	normalized := normalizeStringList(strings.Split(patterns, ","))
	if len(normalized) == 0 {
		return true
	}
	value = strings.TrimSpace(value)
	for _, patternValue := range normalized {
		if patternValue == "*" {
			return true
		}
		if value == "" {
			continue
		}
		if patternValue == value {
			return true
		}
		matched, err := path.Match(patternValue, value)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func nextReleaseRevisionFor(tx *gorm.DB, projectID string, applicationID string, deploymentTargetID string) (int, error) {
	var maxRevision int
	err := tx.Model(&model.Release{}).
		Where("project_id = ? and application_id = ? and deployment_target_id = ?", projectID, applicationID, deploymentTargetID).
		Select("coalesce(max(revision), 0)").
		Scan(&maxRevision).Error
	if err != nil {
		return 0, err
	}
	return maxRevision + 1, nil
}

func normalizeStringList(values []string) []string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			output = append(output, value)
		}
	}
	return output
}
