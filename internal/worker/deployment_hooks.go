package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
)

func (r *Runner) runDeploymentHooks(ctx context.Context, phase string, release model.Release, project model.Project, application model.Application, environment model.Environment, deploymentTarget model.DeploymentTarget, namespace string) error {
	var bindings []model.DeploymentTargetHookBinding
	if err := r.db.WithContext(ctx).Where("project_id = ? and application_id = ? and target_id = ? and phase = ?", project.ID, application.ID, deploymentTarget.ID, phase).
		Order("run_order asc, created_at asc").
		Find(&bindings).Error; err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}
	hookIDs := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		hookIDs = append(hookIDs, binding.HookConfigID)
	}
	var configs []model.ProjectHookConfig
	if err := r.db.WithContext(ctx).Where("project_id = ? and id in ?", project.ID, hookIDs).Find(&configs).Error; err != nil {
		return err
	}
	configsByID := make(map[string]model.ProjectHookConfig, len(configs))
	for _, config := range configs {
		configsByID[config.ID] = config
	}
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return err
	}
	resourceName := applicationResourceName(deploymentTarget)
	buildContext := r.releaseBuildContext(ctx, release)
	sensitiveValues := r.deploymentHookSensitiveValues(ctx, project.ID, environment, deploymentTarget)
	for _, binding := range bindings {
		config, ok := configsByID[binding.HookConfigID]
		if !ok {
			continue
		}
		hookRun := model.HookRun{
			ID:                 id.New("hrun"),
			ProjectID:          project.ID,
			HookConfigID:       config.ID,
			ReleaseID:          release.ID,
			ApplicationID:      application.ID,
			EnvironmentID:      environment.ID,
			DeploymentTargetID: deploymentTarget.ID,
			Name:               config.Name,
			Phase:              binding.Phase,
			Status:             "running",
			ScriptSnapshot:     config.Script,
			Shell:              config.Shell,
			ImageRef:           release.ImageRef,
			TimeoutSeconds:     config.TimeoutSeconds,
			FailurePolicy:      config.FailurePolicy,
			StartedAt:          timePtr(time.Now()),
		}
		if err := r.db.WithContext(ctx).Create(&hookRun).Error; err != nil {
			return err
		}
		r.emitHookEvent(ctx, hookRun, "started", "Hook started")
		r.appendReleaseLog(ctx, release, fmt.Sprintf("执行 %s Hook: %s", phase, config.Name))
		result, err := manager.RunHookJob(ctx, kubeprovider.HookJobSpec{
			Name:               hookJobName(hookRun),
			Namespace:          namespace,
			ProjectID:          project.ID,
			ApplicationID:      application.ID,
			BuildRunID:         release.BuildRunID,
			EnvironmentID:      environment.ID,
			DeploymentTargetID: deploymentTarget.ID,
			ReleaseID:          release.ID,
			HookRunID:          hookRun.ID,
			Phase:              phase,
			Image:              release.ImageRef,
			GitBranch:          buildContext.GitBranch,
			GitTag:             buildContext.GitTag,
			GitRefName:         buildContext.GitRefName,
			GitRefType:         buildContext.GitRefType,
			GitRef:             buildContext.GitRef,
			GitSHA:             buildContext.GitSHA,
			GitShortSHA:        buildContext.GitShortSHA,
			Shell:              config.Shell,
			Script:             config.Script,
			TimeoutSeconds:     int32(normalizePositive(config.TimeoutSeconds, 300)),
			ConfigMapName:      resourceName + "-config",
			SecretName:         resourceName + "-secret",
		})
		if err != nil {
			result = kubeprovider.HookJobResult{Succeeded: false, ExitCode: 1, Message: err.Error()}
		}
		r.appendHookRunLog(ctx, hookRun, result.Logs, sensitiveValues)
		status := "succeeded"
		if !result.Succeeded {
			status = "failed"
		}
		finishedAt := time.Now()
		if updateErr := r.db.WithContext(ctx).Model(&model.HookRun{}).Where("id = ?", hookRun.ID).Updates(map[string]any{
			"status":      status,
			"exit_code":   result.ExitCode,
			"message":     result.Message,
			"finished_at": &finishedAt,
		}).Error; updateErr != nil {
			return updateErr
		}
		hookRun.Status = status
		hookRun.Message = result.Message
		hookRun.FinishedAt = &finishedAt
		r.emitHookEvent(ctx, hookRun, status, result.Message)
		if result.Logs != "" {
			r.appendReleaseLog(ctx, release, redactSensitiveLogContent(result.Logs, sensitiveValues))
		}
		if !result.Succeeded && config.FailurePolicy != "ignore" {
			return errors.New(firstNonEmpty(result.Message, phase+" hook failed"))
		}
	}
	return nil
}

func (r *Runner) appendReleaseLog(ctx context.Context, release model.Release, content string) {
	if r.db == nil {
		return
	}
	content = trimReleaseLogContent(content)
	if content == "" {
		return
	}
	var existing model.ReleaseLog
	err := r.db.WithContext(ctx).First(&existing, "release_id = ? and project_id = ?", release.ID, release.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = r.db.WithContext(ctx).Create(&model.ReleaseLog{
			ID:        id.New("rlog"),
			ReleaseID: release.ID,
			ProjectID: release.ProjectID,
			Content:   content,
		}).Error
		return
	}
	if err != nil {
		return
	}
	existing.Content = trimReleaseLogContent(existing.Content + "\n" + content)
	_ = r.db.WithContext(ctx).Save(&existing).Error
}

func (r *Runner) appendHookRunLog(ctx context.Context, run model.HookRun, content string, sensitiveValues []string) {
	if r.db == nil {
		return
	}
	content = trimReleaseLogContent(redactSensitiveLogContent(content, sensitiveValues))
	if content == "" {
		return
	}
	var existing model.HookRunLog
	err := r.db.WithContext(ctx).First(&existing, "hook_run_id = ? and project_id = ?", run.ID, run.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = r.db.WithContext(ctx).Create(&model.HookRunLog{
			ID:        id.New("hlog"),
			HookRunID: run.ID,
			ProjectID: run.ProjectID,
			Content:   content,
		}).Error
		return
	}
	if err != nil {
		return
	}
	existing.Content = trimReleaseLogContent(existing.Content + "\n" + content)
	_ = r.db.WithContext(ctx).Save(&existing).Error
}

func (r *Runner) deploymentHookSensitiveValues(ctx context.Context, projectID string, environment model.Environment, target model.DeploymentTarget) []string {
	values := make([]string, 0, 8)
	values = append(values, r.resolveSecretValues(ctx, environment.SecretRefs, target.SecretRefs, target.SecretFiles)...)
	sets, err := r.runtimeConfigSetsForTarget(ctx, projectID, target)
	if err != nil {
		return values
	}
	for _, set := range sets {
		values = append(values, decodedMapValues(set.SecretRefs)...)
		values = append(values, decodedFileContents(set.SecretFiles)...)
	}
	return values
}

func (r *Runner) resolveSecretValues(ctx context.Context, rawValues ...string) []string {
	values := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		refs := map[string]string{}
		if json.Unmarshal([]byte(strings.TrimSpace(raw)), &refs) != nil {
			continue
		}
		for _, ref := range refs {
			if value := r.secrets.ResolveContext(ctx, ref); strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
		}
	}
	return values
}

func decodedMapValues(raw string) []string {
	valuesByKey := map[string]string{}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &valuesByKey) != nil {
		return nil
	}
	values := make([]string, 0, len(valuesByKey))
	for _, value := range valuesByKey {
		if strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	return values
}

func decodedFileContents(raw string) []string {
	var files []runtimeConfigFileInput
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &files) != nil {
		return nil
	}
	values := make([]string, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Content) != "" {
			values = append(values, file.Content)
		}
	}
	return values
}

func (r *Runner) releaseBuildContext(ctx context.Context, release model.Release) deploymentHookBuildContext {
	var run model.BuildRun
	if strings.TrimSpace(release.BuildRunID) == "" || r.db.WithContext(ctx).First(&run, "id = ? and project_id = ?", release.BuildRunID, release.ProjectID).Error != nil {
		return deploymentHookBuildContext{}
	}
	refName := firstNonEmpty(run.SourceTag, run.SourceBranch)
	refType := "branch"
	refValue := ""
	if strings.TrimSpace(run.SourceTag) != "" {
		refType = "tag"
		refValue = "refs/tags/" + strings.TrimSpace(run.SourceTag)
	} else if strings.TrimSpace(run.SourceBranch) != "" {
		refValue = "refs/heads/" + strings.TrimSpace(run.SourceBranch)
	}
	return deploymentHookBuildContext{
		GitBranch:   run.SourceBranch,
		GitTag:      run.SourceTag,
		GitRefName:  refName,
		GitRefType:  refType,
		GitRef:      refValue,
		GitSHA:      run.SourceCommit,
		GitShortSHA: shortCommit(run.SourceCommit),
	}
}

type deploymentHookBuildContext struct {
	GitBranch   string
	GitTag      string
	GitRefName  string
	GitRefType  string
	GitRef      string
	GitSHA      string
	GitShortSHA string
}

func trimReleaseLogContent(content string) string {
	content = strings.TrimSpace(content)
	const maxLogBytes = 1024 * 1024
	if len(content) <= maxLogBytes {
		return content
	}
	return content[len(content)-maxLogBytes:]
}
