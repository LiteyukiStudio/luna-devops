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
	"gorm.io/gorm/clause"
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
		return ignoreRecordNotFound(err)
	}
	// Canceled, timed-out or already finished releases never re-run their side
	// effects, including on queue retries.
	if !releaseStatusActive(release.Status) {
		return nil
	}
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.failDeployReleaseWithLog(ctx, release, model.ReleaseFailureProjectUnavailable, "项目空间不存在，无法部署")
		}
		return err
	}
	if !model.DeleteStatusActive(project.DeleteStatus) {
		return r.failDeployReleaseWithLog(ctx, release, model.ReleaseFailureProjectUnavailable, "项目空间正在删除中，跳过部署")
	}
	var application model.Application
	if err := r.db.WithContext(ctx).First(&application, "id = ? and project_id = ?", release.ApplicationID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.failDeployReleaseWithLog(ctx, release, model.ReleaseFailureApplicationUnavailable, "应用不存在，无法部署")
		}
		return err
	}
	if !applicationRuntimeCanMutate(application) {
		return r.failDeployReleaseWithLog(ctx, release, model.ReleaseFailureApplicationUnavailable, "应用正在删除中，跳过部署")
	}
	deploymentTarget, err := r.releaseDeploymentTarget(ctx, release)
	if err != nil || !model.DeleteStatusActive(deploymentTarget.DeleteStatus) {
		return r.failDeployReleaseWithLog(ctx, release, model.ReleaseFailureTargetUnavailable, "部署配置不存在或正在删除，无法部署")
	}
	if release.StartedAt == nil {
		now := time.Now()
		result := r.db.WithContext(ctx).Model(&model.Release{}).
			Where("id = ? and status in ? and started_at is null", release.ID, model.ReleaseActiveStatuses).
			Updates(map[string]any{"status": "running", "started_at": &now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		release.Status = "running"
		release.StartedAt = &now
		r.emitReleaseEvent(ctx, release, "started", "Release started")
	}
	r.appendReleaseLog(ctx, release, fmt.Sprintf("开始部署 release=%s application=%s target=%s image=%s", release.ID, application.Identifier, deploymentTarget.Name, release.ImageRef))

	namespace := deploymentNamespace(project)
	r.appendReleaseLog(ctx, release, fmt.Sprintf("确保命名空间 %s 存在", namespace))
	if err := workerStage(ctx, "deploy.ensure_namespace", func(stageCtx context.Context) error {
		return r.ensureProjectNamespace(stageCtx, namespace, project, deploymentTarget)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailureNamespaceFailed, "命名空间准备失败: ", err, false)
	}
	r.appendReleaseLog(ctx, release, "下发 ConfigMap/Secret")
	serviceBindings, err := workerStageValue(ctx, "deploy.resolve_service_bindings", func(stageCtx context.Context) (resolvedServiceBindingConfig, error) {
		return r.resolveServiceBindingConfig(stageCtx, project, deploymentTarget)
	})
	if err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailureServiceBindingFailed, "服务引用解析失败: ", err, false)
	}
	if serviceBindings.Count > 0 {
		r.appendReleaseLog(ctx, release, fmt.Sprintf("已解析 %d 个服务引用", serviceBindings.Count))
	}
	if err := workerStage(ctx, "deploy.preflight_resources", func(stageCtx context.Context) error {
		return r.preflightApplicationResources(stageCtx, release, project, application, deploymentTarget, namespace, serviceBindings)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailurePreflightFailed, "资源归属预检失败: ", err, true)
	}
	if err := workerStage(ctx, "deploy.apply_runtime_config", func(stageCtx context.Context) error {
		return r.applyApplicationRuntimeConfig(stageCtx, release, project, application, deploymentTarget, namespace, serviceBindings)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailureRuntimeConfigFailed, "运行配置下发失败: ", err, false)
	}
	if err := workerStage(ctx, "deploy.run_pre_hook", func(stageCtx context.Context) error {
		return r.runDeploymentHooks(stageCtx, hookPhasePreDeployment, release, project, application, deploymentTarget, namespace)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailurePreHookFailed, "preDeployment Hook 失败: ", err, false)
	}
	r.appendReleaseLog(ctx, release, "下发 Deployment/Service/ConfigMap/Secret")
	if err := workerStage(ctx, "deploy.ensure_dependencies", func(stageCtx context.Context) error {
		return r.ensurePlatformApplicationDependencies(stageCtx, release, project, application, deploymentTarget, namespace)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailureDependenciesFailed, "平台组件依赖准备失败: ", err, true)
	}
	// A cancel or timeout recorded while preparing must stop the workload
	// apply, which is the first step that changes the running version.
	if active, err := r.releaseActive(ctx, release.ID); err != nil || !active {
		return err
	}
	if err := workerStage(ctx, "deploy.apply_resources", func(stageCtx context.Context) error {
		return r.applyApplicationResources(stageCtx, release, project, application, deploymentTarget, namespace, serviceBindings)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailureApplyFailed, "资源下发失败: ", err, true)
	}
	if err := r.db.WithContext(ctx).Model(&model.Release{}).Where("id = ? and status = ?", release.ID, "running").Updates(map[string]any{
		"message": fmt.Sprintf("Deployment/Service/ConfigMap/Secret 已下发到命名空间 %s", namespace),
	}).Error; err != nil {
		return err
	}
	r.appendReleaseLog(ctx, release, "等待 Deployment rollout 完成")
	message, err := workerStageValue(ctx, "deploy.wait_rollout", func(stageCtx context.Context) (string, error) {
		return r.waitForDeploymentRollout(stageCtx, release, application, deploymentTarget, namespace)
	})
	if err != nil {
		if errors.Is(err, errReleaseInactive) {
			r.appendReleaseLog(ctx, release, "发布已结束，停止等待 rollout")
			return nil
		}
		code := model.ReleaseFailureRolloutFailed
		if errors.Is(err, errReleaseRolloutTimedOut) {
			code = model.ReleaseFailureRolloutTimeout
		}
		return r.failDeployStage(ctx, release, code, "部署失败: ", err, true)
	}
	r.appendReleaseLog(ctx, release, firstNonEmpty(message, "Deployment rollout completed"))
	if err := workerStage(ctx, "deploy.reconcile_volume_mounts", func(stageCtx context.Context) error {
		return r.reconcileDeploymentVolumeMounts(stageCtx, deploymentTarget, namespace)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailureVolumeMountFailed, "数据卷绑定确认失败: ", err, false)
	}
	if err := workerStage(ctx, "deploy.run_post_hook", func(stageCtx context.Context) error {
		return r.runDeploymentHooks(stageCtx, hookPhasePostDeployment, release, project, application, deploymentTarget, namespace)
	}); err != nil {
		return r.failDeployStage(ctx, release, model.ReleaseFailurePostHookFailed, "postDeployment Hook 失败: ", err, true)
	}
	transitioned, err := r.finishDeployRelease(ctx, release, "succeeded", "", firstNonEmpty(message, "Deployment rollout completed"))
	if err != nil {
		return err
	}
	if transitioned {
		r.markSystemComponentDeployment(ctx, release, "deployed", "system component application deployed")
	}
	return nil
}

var (
	errReleaseInactive        = errors.New("release is no longer active")
	errReleaseRolloutTimedOut = errors.New("deployment rollout timed out")
)

func releaseStatusActive(status string) bool {
	return status == "pending" || status == "running"
}

// releaseActive re-reads the authoritative release state so long-running
// stages stop once the release was canceled or timed out elsewhere.
func (r *Runner) releaseActive(ctx context.Context, releaseID string) (bool, error) {
	var current model.Release
	if err := r.db.WithContext(ctx).Select("id", "status").First(&current, "id = ?", releaseID).Error; err != nil {
		return false, ignoreRecordNotFound(err)
	}
	return releaseStatusActive(current.Status), nil
}

func (r *Runner) failDeployReleaseWithLog(ctx context.Context, release model.Release, code string, message string) error {
	r.appendReleaseLog(ctx, release, message)
	_, err := r.finishDeployRelease(ctx, release, "failed", code, message)
	return err
}

// failDeployStage records a stage failure with its stable code; the raw error
// is kept in the release log and message for diagnostics.
func (r *Runner) failDeployStage(ctx context.Context, release model.Release, code string, logPrefix string, stageErr error, systemComponent bool) error {
	transitioned, finishErr := r.finishDeployRelease(ctx, release, "failed", code, stageErr.Error())
	r.appendReleaseLog(ctx, release, logPrefix+stageErr.Error())
	if systemComponent && transitioned {
		r.markSystemComponentDeployment(ctx, release, "failed", stageErr.Error())
	}
	return errors.Join(stageErr, finishErr)
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
	manager, err := r.kubernetesManager(ctx, target)
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

func (r *Runner) applyApplicationResources(ctx context.Context, release model.Release, project model.Project, application model.Application, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) error {
	latestTarget, err := r.releaseDeploymentTarget(ctx, release)
	if err != nil {
		return err
	}
	deploymentTarget.Replicas = latestTarget.Replicas
	manager, spec, err := r.applicationResourcesManagerAndSpec(ctx, release, project, application, deploymentTarget, namespace, serviceBindings)
	if err != nil {
		return err
	}
	spec.ForceImagePull = r.releaseShouldForceImagePull(ctx, release)
	if err := manager.ApplyApplicationResources(ctx, spec); err != nil {
		return err
	}
	return r.reconcileApplicationResourceReplicas(ctx, release, manager, spec)
}

func scaleApplicationResourcesToLatestTarget(ctx context.Context, manager kubeprovider.NamespaceManager, spec kubeprovider.ApplicationResourcesSpec, target model.DeploymentTarget) error {
	if target.Replicas < 0 {
		return errors.New("deployment replicas cannot be negative")
	}
	desired := int32(target.Replicas)
	if desired == spec.Replicas {
		return nil
	}
	return manager.ScaleApplicationWorkload(ctx, spec, desired)
}

func (r *Runner) reconcileApplicationResourceReplicas(ctx context.Context, release model.Release, manager kubeprovider.NamespaceManager, spec kubeprovider.ApplicationResourcesSpec) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target model.DeploymentTarget
		// Use the same target row lock as the update endpoint so the last
		// committed replica value also wins over an in-flight Release apply.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "replicas").
			First(&target, "id = ? and project_id = ? and application_id = ?", release.DeploymentTargetID, release.ProjectID, release.ApplicationID).Error; err != nil {
			return err
		}
		return scaleApplicationResourcesToLatestTarget(ctx, manager, spec, target)
	})
}

func (r *Runner) applyApplicationRuntimeConfig(ctx context.Context, release model.Release, project model.Project, application model.Application, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) error {
	manager, spec, err := r.applicationResourcesManagerAndSpec(ctx, release, project, application, deploymentTarget, namespace, serviceBindings)
	if err != nil {
		return err
	}
	return manager.ApplyApplicationRuntimeConfig(ctx, spec)
}

func (r *Runner) preflightApplicationResources(ctx context.Context, release model.Release, project model.Project, application model.Application, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) error {
	manager, spec, err := r.applicationResourcesManagerAndSpec(ctx, release, project, application, deploymentTarget, namespace, serviceBindings)
	if err != nil {
		return err
	}
	return manager.PreflightApplicationResources(ctx, spec)
}

func (r *Runner) applicationResourcesManagerAndSpec(ctx context.Context, release model.Release, project model.Project, application model.Application, deploymentTarget model.DeploymentTarget, namespace string, serviceBindings resolvedServiceBindingConfig) (kubeprovider.NamespaceManager, kubeprovider.ApplicationResourcesSpec, error) {
	cluster, err := r.runtimeClusterForDeploymentTarget(ctx, deploymentTarget)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	manager, err := r.kubernetesManager(ctx, deploymentTarget)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	runtimeConfigSets, err := r.runtimeConfigSetsForTarget(ctx, project.ID, deploymentTarget)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	deploymentTarget.SecretRefs = r.resolveRuntimeSecretRefsRaw(ctx, deploymentTarget.SecretRefs)
	deploymentTarget.SecretFiles = r.resolveRuntimeSecretFileRefsRaw(ctx, deploymentTarget.SecretFiles)
	volumeProvider, _ := manager.(kubeprovider.ProjectVolumeProvider)
	dataVolumes, err := r.deploymentTargetDataVolumes(ctx, deploymentTarget, namespace, volumeProvider)
	if err != nil {
		return nil, kubeprovider.ApplicationResourcesSpec{}, err
	}
	spec, err := applicationResourcesSpec(release, project, application, deploymentTarget, runtimeConfigSets, dataVolumes, namespace, r.deployRolloutTimeoutSeconds)
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

func (r *Runner) waitForDeploymentRollout(ctx context.Context, release model.Release, application model.Application, deploymentTarget model.DeploymentTarget, namespace string) (string, error) {
	manager, err := r.kubernetesManager(ctx, deploymentTarget)
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
			result := r.db.WithContext(rolloutCtx).Model(&model.Release{}).Where("id = ? and status = ?", release.ID, "running").Update("message", snapshot.Message)
			if result.Error == nil && result.RowsAffected == 0 {
				return "", errReleaseInactive
			}
			r.appendReleaseLog(rolloutCtx, release, snapshot.Message)
		} else if active, err := r.releaseActive(rolloutCtx, release.ID); err == nil && !active {
			return "", errReleaseInactive
		}

		switch snapshot.Phase {
		case kubeprovider.DeploymentSucceeded:
			return snapshot.Message, nil
		case kubeprovider.DeploymentFailed:
			return "", errors.New(firstNonEmpty(snapshot.Message, "Deployment rollout failed"))
		}

		select {
		case <-rolloutCtx.Done():
			return "", fmt.Errorf("%w after %s", errReleaseRolloutTimedOut, timeout)
		case <-ticker.C:
		}
	}
}

// finishDeployRelease moves an active release into a terminal state. It reports
// whether this call performed the transition; only that writer records metrics
// and emits the notification event, so late writers cannot flip the result.
func (r *Runner) finishDeployRelease(ctx context.Context, release model.Release, status string, code string, message string) (bool, error) {
	finishedAt := time.Now()
	result := r.db.WithContext(ctx).Model(&model.Release{}).
		Where("id = ? and status in ?", release.ID, model.ReleaseActiveStatuses).
		Updates(releaseFinishUpdates(status, code, message, finishedAt))
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected != 1 {
		return false, nil
	}
	release.Status = status
	release.FailureCode = code
	release.Message = firstNonEmpty(message, "Deployment "+status)
	release.FinishedAt = &finishedAt
	r.recordReleaseMetrics(ctx, release)
	r.emitReleaseEvent(ctx, release, status, message)
	return true, nil
}

func releaseFinishUpdates(status string, code string, message string, finishedAt time.Time) map[string]any {
	return map[string]any{
		"status":       status,
		"failure_code": code,
		"message":      firstNonEmpty(message, "Deployment "+status),
		"finished_at":  &finishedAt,
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
