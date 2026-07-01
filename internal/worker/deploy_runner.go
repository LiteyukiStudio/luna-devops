package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

func (r *Runner) handleDeployRun(ctx context.Context, task *asynq.Task) error {
	var payload tasks.DeployRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

	var release model.Release
	if err := r.db.First(&release, "id = ? and project_id = ?", payload.ReleaseID, payload.ProjectID).Error; err != nil {
		return err
	}
	var project model.Project
	if err := r.db.First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return err
	}
	var application model.Application
	if err := r.db.First(&application, "id = ? and project_id = ?", release.ApplicationID, payload.ProjectID).Error; err != nil {
		return err
	}
	if !applicationRuntimeCanMutate(application) {
		message := "应用正在删除中，跳过部署"
		r.appendReleaseLog(release, message)
		return r.finishDeployRelease(release, "failed", message)
	}
	deploymentTarget, err := r.releaseDeploymentTarget(release)
	if err != nil {
		message := "部署配置不存在或已被删除，无法部署"
		r.appendReleaseLog(release, message)
		return r.finishDeployRelease(release, "failed", message)
	}
	environment := deploymentTargetEnvironment(deploymentTarget)

	now := time.Now()
	if release.StartedAt == nil {
		if err := r.db.Model(&release).Updates(map[string]any{"status": "running", "started_at": &now}).Error; err != nil {
			return err
		}
	}
	r.appendReleaseLog(release, fmt.Sprintf("开始部署 release=%s application=%s target=%s image=%s", release.ID, application.Slug, deploymentTarget.Name, release.ImageRef))

	namespace := deploymentNamespace(project, environment)
	r.appendReleaseLog(release, fmt.Sprintf("确保命名空间 %s 存在", namespace))
	if err := r.ensureProjectNamespace(ctx, namespace, project, environment); err != nil {
		_ = r.finishDeployRelease(release, "failed", err.Error())
		r.appendReleaseLog(release, "命名空间准备失败: "+err.Error())
		return err
	}
	r.appendReleaseLog(release, "下发 ConfigMap/Secret")
	if err := r.applyApplicationRuntimeConfig(ctx, release, project, application, environment, deploymentTarget, namespace); err != nil {
		_ = r.finishDeployRelease(release, "failed", err.Error())
		r.appendReleaseLog(release, "运行配置下发失败: "+err.Error())
		return err
	}
	if err := r.runDeploymentHooks(ctx, hookPhasePreDeployment, release, project, application, environment, deploymentTarget, namespace); err != nil {
		_ = r.finishDeployRelease(release, "failed", err.Error())
		r.appendReleaseLog(release, "preDeployment Hook 失败: "+err.Error())
		return err
	}
	r.appendReleaseLog(release, "下发 Deployment/Service/ConfigMap/Secret")
	if err := r.applyApplicationResources(ctx, release, project, application, environment, deploymentTarget, namespace); err != nil {
		_ = r.finishDeployRelease(release, "failed", err.Error())
		r.appendReleaseLog(release, "资源下发失败: "+err.Error())
		return err
	}
	if err := r.db.Model(&release).Updates(map[string]any{
		"status":  "running",
		"message": fmt.Sprintf("Deployment/Service/ConfigMap/Secret 已下发到命名空间 %s", namespace),
	}).Error; err != nil {
		return err
	}
	r.appendReleaseLog(release, "等待 Deployment rollout 完成")
	message, err := r.waitForDeploymentRollout(ctx, release, application, environment, deploymentTarget, namespace)
	if err != nil {
		_ = r.finishDeployRelease(release, "failed", err.Error())
		r.appendReleaseLog(release, "部署失败: "+err.Error())
		return err
	}
	r.appendReleaseLog(release, firstNonEmpty(message, "Deployment rollout completed"))
	if err := r.runDeploymentHooks(ctx, hookPhasePostDeployment, release, project, application, environment, deploymentTarget, namespace); err != nil {
		_ = r.finishDeployRelease(release, "failed", err.Error())
		r.appendReleaseLog(release, "postDeployment Hook 失败: "+err.Error())
		return err
	}
	return r.finishDeployRelease(release, "succeeded", firstNonEmpty(message, "Deployment rollout completed"))
}

func (r *Runner) applyApplicationResources(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string) error {
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	runtimeConfigSets, err := r.runtimeConfigSetsForTarget(project.ID, deploymentTarget)
	if err != nil {
		return err
	}
	deploymentTarget.SecretRefs = r.resolveRuntimeSecretRefsRaw(deploymentTarget.SecretRefs)
	deploymentTarget.SecretFiles = r.resolveRuntimeSecretFileRefsRaw(deploymentTarget.SecretFiles)
	spec, err := applicationResourcesSpec(release, project, application, environment, deploymentTarget, runtimeConfigSets, namespace, r.deployRolloutTimeoutSeconds)
	if err != nil {
		return err
	}
	spec.ForceImagePull = r.releaseShouldForceImagePull(release)
	return manager.ApplyApplicationResources(ctx, spec)
}

func (r *Runner) applyApplicationRuntimeConfig(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string) error {
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	runtimeConfigSets, err := r.runtimeConfigSetsForTarget(project.ID, deploymentTarget)
	if err != nil {
		return err
	}
	deploymentTarget.SecretRefs = r.resolveRuntimeSecretRefsRaw(deploymentTarget.SecretRefs)
	deploymentTarget.SecretFiles = r.resolveRuntimeSecretFileRefsRaw(deploymentTarget.SecretFiles)
	spec, err := applicationResourcesSpec(release, project, application, environment, deploymentTarget, runtimeConfigSets, namespace, r.deployRolloutTimeoutSeconds)
	if err != nil {
		return err
	}
	return manager.ApplyApplicationRuntimeConfig(ctx, spec)
}

func (r *Runner) runtimeConfigSetsForTarget(projectID string, deploymentTarget model.DeploymentTarget) ([]model.ProjectRuntimeConfigSet, error) {
	refs := model.DecodeDeploymentRuntimeConfigRefs(deploymentTarget.RuntimeConfigRefs)
	if len(refs) == 0 {
		for _, setID := range runtimeConfigSetIDs(deploymentTarget.RuntimeConfigSetIDs) {
			refs = append(refs, model.DeploymentRuntimeConfigRef{SetID: setID, Mode: model.RuntimeConfigRefModeLive})
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}
	liveIDs := model.DeploymentRuntimeConfigLiveSetIDs(refs)
	var sets []model.ProjectRuntimeConfigSet
	if len(liveIDs) > 0 {
		if err := r.db.Where("project_id = ? and enabled = ? and id in ?", projectID, true, liveIDs).Find(&sets).Error; err != nil {
			return nil, err
		}
	}
	byID := make(map[string]model.ProjectRuntimeConfigSet, len(sets))
	for _, set := range sets {
		set.SecretRefs = r.resolveRuntimeSecretRefsRaw(set.SecretRefs)
		set.SecretFiles = r.resolveRuntimeSecretFileRefsRaw(set.SecretFiles)
		byID[set.ID] = set
	}
	ordered := make([]model.ProjectRuntimeConfigSet, 0, len(refs))
	for _, ref := range refs {
		if ref.Mode == model.RuntimeConfigRefModeSnapshot {
			if ref.Snapshot == nil || !ref.Snapshot.Enabled {
				continue
			}
			set := model.ProjectRuntimeConfigSet{
				ID:          ref.SetID,
				ProjectID:   projectID,
				Name:        ref.Snapshot.Name,
				EnvVars:     ref.Snapshot.EnvVars,
				ConfigFiles: ref.Snapshot.ConfigFiles,
				SecretRefs:  r.resolveRuntimeSecretRefsRaw(ref.Snapshot.SecretRefs),
				SecretFiles: r.resolveRuntimeSecretFileRefsRaw(ref.Snapshot.SecretFiles),
				Enabled:     ref.Snapshot.Enabled,
			}
			ordered = append(ordered, set)
			continue
		}
		if set, ok := byID[ref.SetID]; ok {
			ordered = append(ordered, set)
		}
	}
	return ordered, nil
}

func (r *Runner) releaseShouldForceImagePull(release model.Release) bool {
	if release.ForceImagePull {
		return true
	}
	if strings.TrimSpace(release.BuildRunID) == "" || strings.TrimSpace(release.ImageRef) == "" {
		return false
	}
	var previous model.Release
	err := r.db.Where(
		"project_id = ? and application_id = ? and deployment_target_id = ? and status = ? and revision < ?",
		release.ProjectID,
		release.ApplicationID,
		release.DeploymentTargetID,
		"succeeded",
		release.Revision,
	).Order("revision desc, created_at desc").First(&previous).Error
	if err != nil {
		return false
	}
	return strings.TrimSpace(previous.BuildRunID) != strings.TrimSpace(release.BuildRunID) &&
		strings.TrimSpace(previous.ImageRef) == strings.TrimSpace(release.ImageRef)
}

func (r *Runner) resolveRuntimeSecretRefsRaw(raw string) string {
	refs := map[string]string{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = "{}"
	}
	if err := json.Unmarshal([]byte(trimmed), &refs); err != nil {
		return raw
	}
	resolved := make(map[string]string, len(refs))
	for key, ref := range refs {
		value := r.secrets.Resolve(ref)
		if strings.TrimSpace(value) == "" {
			continue
		}
		resolved[key] = value
	}
	content, err := json.Marshal(resolved)
	if err != nil {
		return ""
	}
	return string(content)
}

func (r *Runner) resolveRuntimeSecretFileRefsRaw(raw string) string {
	refs := map[string]string{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if err := json.Unmarshal([]byte(trimmed), &refs); err != nil {
		return raw
	}
	files := make([]runtimeConfigFileInput, 0, len(refs))
	for filePath, ref := range refs {
		value := r.secrets.Resolve(ref)
		if strings.TrimSpace(filePath) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		files = append(files, runtimeConfigFileInput{Path: strings.TrimSpace(filePath), Content: value})
	}
	content, err := json.Marshal(files)
	if err != nil {
		return ""
	}
	return string(content)
}

func (r *Runner) waitForDeploymentRollout(ctx context.Context, release model.Release, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string) (string, error) {
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return "", err
	}
	resourceName := applicationResourceName(deploymentTarget)
	timeout := time.Duration(r.deployRolloutTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	rolloutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		snapshot, err := manager.GetDeploymentSnapshot(rolloutCtx, namespace, resourceName)
		if err != nil {
			return "", err
		}
		if snapshot.Message != "" {
			_ = r.db.Model(&model.Release{}).Where("id = ?", release.ID).Update("message", snapshot.Message).Error
			r.appendReleaseLog(release, snapshot.Message)
		}

		switch snapshot.Phase {
		case kubeprovider.DeploymentSucceeded:
			return snapshot.Message, nil
		case kubeprovider.DeploymentFailed:
			return "", errors.New(firstNonEmpty(snapshot.Message, "Deployment rollout failed"))
		}

		select {
		case <-rolloutCtx.Done():
			return "", fmt.Errorf("Deployment rollout timed out after %s", timeout)
		case <-ticker.C:
		}
	}
}

func (r *Runner) finishDeployRelease(release model.Release, status string, message string) error {
	finishedAt := time.Now()
	err := r.db.Model(&model.Release{}).Where("id = ?", release.ID).Updates(releaseFinishUpdates(status, message, finishedAt)).Error
	if err == nil {
		release.Status = status
		release.Message = firstNonEmpty(message, "Deployment "+status)
		release.FinishedAt = &finishedAt
		r.recordReleaseMetrics(release)
	}
	return err
}

func releaseFinishUpdates(status string, message string, finishedAt time.Time) map[string]any {
	return map[string]any{
		"status":      status,
		"message":     firstNonEmpty(message, "Deployment "+status),
		"finished_at": &finishedAt,
	}
}

func (r *Runner) releaseDeploymentTarget(release model.Release) (model.DeploymentTarget, error) {
	var target model.DeploymentTarget
	if strings.TrimSpace(release.DeploymentTargetID) == "" {
		return target, fmt.Errorf("release %s has no deployment target", release.ID)
	}
	if err := r.db.First(&target, "id = ? and project_id = ? and application_id = ?", release.DeploymentTargetID, release.ProjectID, release.ApplicationID).Error; err != nil {
		return target, err
	}
	return target, nil
}
