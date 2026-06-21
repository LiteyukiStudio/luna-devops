package api

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func (h *Handlers) enqueueAutoDeploymentsForBuildRun(ctx context.Context, run model.BuildRun) {
	if run.Status != "succeeded" || strings.TrimSpace(run.ImageRef) == "" || strings.TrimSpace(run.DeploymentTargetID) == "" {
		return
	}
	var application model.Application
	if err := h.db.First(&application, "id = ? and project_id = ?", run.ApplicationID, run.ProjectID).Error; err != nil {
		return
	}
	if !applicationCanMutate(application) {
		return
	}
	var target model.DeploymentTarget
	if err := h.db.First(
		&target,
		"id = ? and project_id = ? and application_id = ? and enabled = ? and auto_deploy = ? and require_approval = ?",
		run.DeploymentTargetID,
		run.ProjectID,
		run.ApplicationID,
		true,
		true,
		false,
	).Error; err != nil {
		return
	}
	if !deploymentTargetMatchesBuildRun(target, run) {
		return
	}
	release, ok := h.createAutoDeployRelease(ctx, run, target)
	if !ok {
		return
	}
	if !h.enqueueDeployRun(ctx, release) {
		release.Status = "failed"
		release.Message = "部署任务投递失败，请稍后重试"
		_ = h.db.Save(&release).Error
	}
}

func (h *Handlers) createAutoDeployRelease(ctx context.Context, run model.BuildRun, target model.DeploymentTarget) (model.Release, bool) {
	release := model.Release{}
	err := h.db.Transaction(func(tx *gorm.DB) error {
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
			EnvironmentID:      target.EnvironmentID,
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
