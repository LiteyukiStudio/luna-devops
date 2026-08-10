package worker

import (
	"context"
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
	if err := r.db.Where("project_id = ? and application_id = ? and target_id = ? and phase = ?", project.ID, application.ID, deploymentTarget.ID, phase).
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
	if err := r.db.Where("project_id = ? and id in ?", project.ID, hookIDs).Find(&configs).Error; err != nil {
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
	buildContext := r.releaseBuildContext(release)
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
		if err := r.db.Create(&hookRun).Error; err != nil {
			return err
		}
		r.emitHookEvent(ctx, hookRun, "started", "Hook started")
		r.appendReleaseLog(release, fmt.Sprintf("执行 %s Hook: %s", phase, config.Name))
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
		r.appendHookRunLog(hookRun, result.Logs)
		status := "succeeded"
		if !result.Succeeded {
			status = "failed"
		}
		finishedAt := time.Now()
		if updateErr := r.db.Model(&model.HookRun{}).Where("id = ?", hookRun.ID).Updates(map[string]any{
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
			r.appendReleaseLog(release, result.Logs)
		}
		if !result.Succeeded && config.FailurePolicy != "ignore" {
			return errors.New(firstNonEmpty(result.Message, phase+" hook failed"))
		}
	}
	return nil
}

func (r *Runner) appendReleaseLog(release model.Release, content string) {
	if r.db == nil {
		return
	}
	content = trimReleaseLogContent(content)
	if content == "" {
		return
	}
	var existing model.ReleaseLog
	err := r.db.First(&existing, "release_id = ? and project_id = ?", release.ID, release.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = r.db.Create(&model.ReleaseLog{
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
	_ = r.db.Save(&existing).Error
}

func (r *Runner) appendHookRunLog(run model.HookRun, content string) {
	if r.db == nil {
		return
	}
	content = trimReleaseLogContent(content)
	if content == "" {
		return
	}
	var existing model.HookRunLog
	err := r.db.First(&existing, "hook_run_id = ? and project_id = ?", run.ID, run.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = r.db.Create(&model.HookRunLog{
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
	_ = r.db.Save(&existing).Error
}

func (r *Runner) releaseBuildContext(release model.Release) deploymentHookBuildContext {
	var run model.BuildRun
	if strings.TrimSpace(release.BuildRunID) == "" || r.db.First(&run, "id = ? and project_id = ?", release.BuildRunID, release.ProjectID).Error != nil {
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
