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
	"gorm.io/gorm"
)

const (
	trustedGatewayTrafficProbeComponentID    = "gateway-traffic-probe"
	trustedGatewayTrafficProbeServiceAccount = "luna-gateway-traffic-probe"
)

func (r *Runner) handleDeployRun(ctx context.Context, task *asynq.Task) error {
	var payload tasks.DeployRunPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}

	var release model.Release
	if err := r.db.WithContext(ctx).First(&release, "id = ? and project_id = ?", payload.ReleaseID, payload.ProjectID).Error; err != nil {
		return err
	}
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return err
	}
	var application model.Application
	if err := r.db.WithContext(ctx).First(&application, "id = ? and project_id = ?", release.ApplicationID, payload.ProjectID).Error; err != nil {
		return err
	}
	if !applicationRuntimeCanMutate(application) {
		message := "应用正在删除中，跳过部署"
		r.appendReleaseLog(ctx, release, message)
		return r.finishDeployRelease(ctx, release, "failed", message)
	}
	deploymentTarget, err := r.releaseDeploymentTarget(ctx, release)
	if err != nil {
		message := "部署配置不存在或已被删除，无法部署"
		r.appendReleaseLog(ctx, release, message)
		return r.finishDeployRelease(ctx, release, "failed", message)
	}
	environment := deploymentTargetEnvironment(deploymentTarget)

	now := time.Now()
	if release.StartedAt == nil {
		if err := r.db.WithContext(ctx).Model(&release).Updates(map[string]any{"status": "running", "started_at": &now}).Error; err != nil {
			return err
		}
		release.Status = "running"
		release.StartedAt = &now
		r.emitReleaseEvent(ctx, release, "started", "Release started")
	}
	r.appendReleaseLog(ctx, release, fmt.Sprintf("开始部署 release=%s application=%s target=%s image=%s", release.ID, application.Identifier, deploymentTarget.Name, release.ImageRef))

	namespace := deploymentNamespace(project, environment)
	r.appendReleaseLog(ctx, release, fmt.Sprintf("确保命名空间 %s 存在", namespace))
	if err := workerStage(ctx, "deploy.ensure_namespace", func(stageCtx context.Context) error {
		return r.ensureProjectNamespace(stageCtx, namespace, project, environment)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "命名空间准备失败: "+err.Error())
		return err
	}
	r.appendReleaseLog(ctx, release, "下发 ConfigMap/Secret")
	serviceBindings, err := workerStageValue(ctx, "deploy.resolve_service_bindings", func(stageCtx context.Context) (resolvedServiceBindingConfig, error) {
		return r.resolveServiceBindingConfig(stageCtx, project, deploymentTarget)
	})
	if err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "服务引用解析失败: "+err.Error())
		return err
	}
	if serviceBindings.Count > 0 {
		r.appendReleaseLog(ctx, release, fmt.Sprintf("已解析 %d 个服务引用", serviceBindings.Count))
	}
	if err := workerStage(ctx, "deploy.preflight_resources", func(stageCtx context.Context) error {
		return r.preflightApplicationResources(stageCtx, release, project, application, environment, deploymentTarget, namespace, serviceBindings)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "资源归属预检失败: "+err.Error())
		r.markSystemComponentDeployment(ctx, release, "failed", err.Error())
		return err
	}
	if err := workerStage(ctx, "deploy.apply_runtime_config", func(stageCtx context.Context) error {
		return r.applyApplicationRuntimeConfig(stageCtx, release, project, application, environment, deploymentTarget, namespace, serviceBindings)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "运行配置下发失败: "+err.Error())
		return err
	}
	if err := workerStage(ctx, "deploy.run_pre_hook", func(stageCtx context.Context) error {
		return r.runDeploymentHooks(stageCtx, hookPhasePreDeployment, release, project, application, environment, deploymentTarget, namespace)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "preDeployment Hook 失败: "+err.Error())
		return err
	}
	r.appendReleaseLog(ctx, release, "下发 Deployment/Service/ConfigMap/Secret")
	if err := workerStage(ctx, "deploy.ensure_dependencies", func(stageCtx context.Context) error {
		return r.ensurePlatformApplicationDependencies(stageCtx, release, project, application, deploymentTarget, namespace)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "平台组件依赖准备失败: "+err.Error())
		r.markSystemComponentDeployment(ctx, release, "failed", err.Error())
		return err
	}
	if err := workerStage(ctx, "deploy.apply_resources", func(stageCtx context.Context) error {
		return r.applyApplicationResources(stageCtx, release, project, application, environment, deploymentTarget, namespace, serviceBindings)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "资源下发失败: "+err.Error())
		r.markSystemComponentDeployment(ctx, release, "failed", err.Error())
		return err
	}
	if err := r.db.WithContext(ctx).Model(&release).Updates(map[string]any{
		"status":  "running",
		"message": fmt.Sprintf("Deployment/Service/ConfigMap/Secret 已下发到命名空间 %s", namespace),
	}).Error; err != nil {
		return err
	}
	r.appendReleaseLog(ctx, release, "等待 Deployment rollout 完成")
	message, err := workerStageValue(ctx, "deploy.wait_rollout", func(stageCtx context.Context) (string, error) {
		return r.waitForDeploymentRollout(stageCtx, release, application, environment, deploymentTarget, namespace)
	})
	if err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "部署失败: "+err.Error())
		r.markSystemComponentDeployment(ctx, release, "failed", err.Error())
		return err
	}
	r.appendReleaseLog(ctx, release, firstNonEmpty(message, "Deployment rollout completed"))
	if err := workerStage(ctx, "deploy.reconcile_volume_mounts", func(stageCtx context.Context) error {
		return r.reconcileDeploymentVolumeMounts(stageCtx, deploymentTarget, environment, namespace)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "数据卷绑定确认失败: "+err.Error())
		return err
	}
	if err := workerStage(ctx, "deploy.run_post_hook", func(stageCtx context.Context) error {
		return r.runDeploymentHooks(stageCtx, hookPhasePostDeployment, release, project, application, environment, deploymentTarget, namespace)
	}); err != nil {
		_ = r.finishDeployRelease(ctx, release, "failed", err.Error())
		r.appendReleaseLog(ctx, release, "postDeployment Hook 失败: "+err.Error())
		r.markSystemComponentDeployment(ctx, release, "failed", err.Error())
		return err
	}
	if err := r.finishDeployRelease(ctx, release, "succeeded", firstNonEmpty(message, "Deployment rollout completed")); err != nil {
		return err
	}
	r.markSystemComponentDeployment(ctx, release, "deployed", "system component application deployed")
	return nil
}

// ensurePlatformApplicationDependencies provisions ServiceAccount/RBAC only for
// a release linked to a trusted system-component installation plan.
func (r *Runner) ensurePlatformApplicationDependencies(ctx context.Context, release model.Release, project model.Project, application model.Application, target model.DeploymentTarget, namespace string) error {
	serviceAccountName, err := r.trustedPlatformApplicationServiceAccount(ctx, release, target, namespace)
	if err != nil {
		return err
	}
	if serviceAccountName == "" {
		return nil
	}
	manager, err := r.kubernetesManager(ctx, deploymentTargetEnvironment(target))
	if err != nil {
		return err
	}
	return manager.EnsureGatewayTrafficProbeAccess(ctx, kubeprovider.GatewayTrafficProbeSpec{
		Name:             serviceAccountName,
		Namespace:        namespace,
		RuntimeClusterID: strings.TrimSpace(target.ClusterID),
	})
}

func (r *Runner) trustedPlatformApplicationServiceAccount(ctx context.Context, release model.Release, target model.DeploymentTarget, namespace string) (string, error) {
	requested := strings.TrimSpace(target.ServiceAccountName)
	if requested == "" {
		return "", nil
	}
	var installation model.SystemComponentInstallation
	err := r.db.WithContext(ctx).First(&installation,
		"release_id = ? and project_id = ? and application_id = ? and deployment_target_id = ? and runtime_cluster_id = ?",
		release.ID, release.ProjectID, release.ApplicationID, target.ID, target.ClusterID,
	).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("deployment service account is not authorized by a trusted system component plan")
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(installation.ComponentID) != trustedGatewayTrafficProbeComponentID ||
		requested != trustedGatewayTrafficProbeServiceAccount ||
		strings.TrimSpace(installation.Namespace) != strings.TrimSpace(namespace) {
		return "", fmt.Errorf("deployment service account is not authorized by a trusted system component plan")
	}
	return requested, nil
}

func (r *Runner) markSystemComponentDeployment(ctx context.Context, release model.Release, status string, message string) {
	if strings.TrimSpace(release.ID) == "" {
		return
	}
	_ = r.db.WithContext(ctx).Model(&model.SystemComponentInstallation{}).
		Where("release_id = ?", release.ID).
		Updates(map[string]any{
			"status":     status,
			"message":    strings.TrimSpace(message),
			"last_error": systemComponentLastError(status, message),
			"updated_at": time.Now(),
		}).Error
}

func systemComponentLastError(status string, message string) string {
	if status == "failed" {
		return strings.TrimSpace(message)
	}
	return ""
}

func (r *Runner) applyApplicationResources(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) error {
	manager, spec, err := r.applicationResourcesManagerAndSpec(ctx, release, project, application, environment, deploymentTarget, namespace, serviceBindings)
	if err != nil {
		return err
	}
	spec.ForceImagePull = r.releaseShouldForceImagePull(ctx, release)
	if err := manager.ApplyApplicationResources(ctx, spec); err != nil {
		return err
	}
	return nil
}

func (r *Runner) applyApplicationRuntimeConfig(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) error {
	manager, spec, err := r.applicationResourcesManagerAndSpec(ctx, release, project, application, environment, deploymentTarget, namespace, serviceBindings)
	if err != nil {
		return err
	}
	return manager.ApplyApplicationRuntimeConfig(ctx, spec)
}

func (r *Runner) preflightApplicationResources(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) error {
	manager, spec, err := r.applicationResourcesManagerAndSpec(ctx, release, project, application, environment, deploymentTarget, namespace, serviceBindings)
	if err != nil {
		return err
	}
	return manager.PreflightApplicationResources(ctx, spec)
}

func (r *Runner) applicationResourcesManagerAndSpec(ctx context.Context, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) (kubeprovider.NamespaceManager, kubeprovider.ApplicationResourcesSpec, error) {
	cluster, err := r.runtimeClusterForEnvironment(ctx, environment)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	runtimeConfigSets, err := r.runtimeConfigSetsForTarget(ctx, project.ID, deploymentTarget)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	deploymentTarget.SecretRefs = r.resolveRuntimeSecretRefsRaw(ctx, deploymentTarget.SecretRefs)
	deploymentTarget.SecretFiles = r.resolveRuntimeSecretFileRefsRaw(ctx, deploymentTarget.SecretFiles)
	dataVolumes, err := r.deploymentTargetDataVolumes(ctx, deploymentTarget, namespace)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	spec, err := applicationResourcesSpec(release, project, application, environment, deploymentTarget, runtimeConfigSets, dataVolumes, namespace, r.deployRolloutTimeoutSeconds)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	trustedServiceAccount, err := r.trustedPlatformApplicationServiceAccount(ctx, release, deploymentTarget, namespace)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	if trustedServiceAccount != "" {
		spec.ServiceAccountName = trustedServiceAccount
		spec.AutomountServiceAccountToken = strings.TrimSpace(deploymentTarget.AutomountServiceAccountToken)
		spec.TrustedServiceAccounts = []string{trustedServiceAccount}
	}
	if err := applyRuntimeClusterResourcePolicy(&spec, cluster); err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	if err := applyServiceBindingConfig(&spec, serviceBindings); err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	expandEnvRefsCrossBoundary(spec.ConfigData, spec.SecretData)
	return manager, spec, nil
}

func (r *Runner) runtimeConfigSetsForTarget(ctx context.Context, projectID string, deploymentTarget model.DeploymentTarget) ([]model.ProjectRuntimeConfigSet, error) {
	refs := model.DecodeDeploymentRuntimeConfigRefs(deploymentTarget.RuntimeConfigRefs)
	if len(refs) == 0 {
		return nil, nil
	}
	liveIDs := model.DeploymentRuntimeConfigLiveSetIDs(refs)
	var sets []model.ProjectRuntimeConfigSet
	if len(liveIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("project_id = ? and enabled = ? and id in ?", projectID, true, liveIDs).Find(&sets).Error; err != nil {
			return nil, err
		}
	}
	byID := make(map[string]model.ProjectRuntimeConfigSet, len(sets))
	for _, set := range sets {
		set.SecretRefs = r.resolveRuntimeSecretRefsRaw(ctx, set.SecretRefs)
		set.SecretFiles = r.resolveRuntimeSecretFileRefsRaw(ctx, set.SecretFiles)
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
				SecretRefs:  r.resolveRuntimeSecretRefsRaw(ctx, ref.Snapshot.SecretRefs),
				SecretFiles: r.resolveRuntimeSecretFileRefsRaw(ctx, ref.Snapshot.SecretFiles),
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

func (r *Runner) releaseShouldForceImagePull(ctx context.Context, release model.Release) bool {
	if release.ForceImagePull {
		return true
	}
	if strings.TrimSpace(release.BuildRunID) == "" || strings.TrimSpace(release.ImageRef) == "" {
		return false
	}
	var previous model.Release
	err := r.db.WithContext(ctx).Where(
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

func (r *Runner) resolveRuntimeSecretRefsRaw(ctx context.Context, raw string) string {
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
		value := r.secrets.ResolveContext(ctx, ref)
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

func (r *Runner) resolveRuntimeSecretFileRefsRaw(ctx context.Context, raw string) string {
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
		value := r.secrets.ResolveContext(ctx, ref)
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
	manager, err := r.kubernetesManager(ctx, environment)
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
			_ = r.db.WithContext(rolloutCtx).Model(&model.Release{}).Where("id = ?", release.ID).Update("message", snapshot.Message).Error
			r.appendReleaseLog(rolloutCtx, release, snapshot.Message)
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

func (r *Runner) finishDeployRelease(ctx context.Context, release model.Release, status string, message string) error {
	finishedAt := time.Now()
	err := r.db.WithContext(ctx).Model(&model.Release{}).Where("id = ?", release.ID).Updates(releaseFinishUpdates(status, message, finishedAt)).Error
	if err == nil {
		release.Status = status
		release.Message = firstNonEmpty(message, "Deployment "+status)
		release.FinishedAt = &finishedAt
		r.recordReleaseMetrics(ctx, release)
		r.emitReleaseEvent(ctx, release, status, message)
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

func (r *Runner) releaseDeploymentTarget(ctx context.Context, release model.Release) (model.DeploymentTarget, error) {
	var target model.DeploymentTarget
	if strings.TrimSpace(release.DeploymentTargetID) == "" {
		return target, fmt.Errorf("release %s has no deployment target", release.ID)
	}
	if err := r.db.WithContext(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", release.DeploymentTargetID, release.ProjectID, release.ApplicationID).Error; err != nil {
		return target, err
	}
	return target, nil
}
