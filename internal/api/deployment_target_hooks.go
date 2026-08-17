package api

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errDeploymentStageExists = errors.New("deployment stage already exists")
var errRuntimeEnvironmentValueModeConflict = errors.New("runtime environment value mode conflict")

func (h *Handlers) createDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.persistDeploymentTarget(target, dataVolumes, hookInputs, buildEnvironment, nil, true, ctx)
}

func (h *Handlers) saveDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.persistDeploymentTarget(target, dataVolumes, hookInputs, buildEnvironment, nil, false, ctx)
}

func (h *Handlers) persistDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, secretValues []model.SecretValue, create bool, ctx context.Context) (deploymentVolumeMountChanges, error) {
	changes := deploymentVolumeMountChanges{}
	err := h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if create {
			result := tx.Clauses(clause.OnConflict{
				Columns:     []clause.Column{{Name: "application_id"}, {Name: "stage"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
				DoNothing:   true,
			}).Create(&target)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errDeploymentStageExists
			}
		} else {
			var current model.DeploymentTarget
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "secret_refs").First(&current, "id = ?", target.ID).Error; err != nil {
				return err
			}
			target.SecretRefs = current.SecretRefs
			if publicEnvironmentConflictsWithSecretRefs(target.EnvVars, target.SecretRefs) {
				return errRuntimeEnvironmentValueModeConflict
			}
			if err := tx.Save(&target).Error; err != nil {
				return err
			}
		}
		var syncErr error
		changes, syncErr = syncDeploymentTargetVolumeMounts(ctx, tx, target, dataVolumes)
		if syncErr != nil {
			return syncErr
		}
		changes.HookBindings, syncErr = h.replaceDeploymentTargetHookBindings(tx, target, hookInputs)
		if syncErr != nil {
			return syncErr
		}
		if buildEnvironment != nil {
			if err := tx.Save(buildEnvironment).Error; err != nil {
				return err
			}
		}
		if len(secretValues) > 0 {
			if err := tx.Create(&secretValues).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return changes, err
	}
	return changes, nil
}

func (h *Handlers) attachDeploymentTargetHookBindings(targets []model.DeploymentTarget, ctx context.Context) error {
	if len(targets) == 0 {
		return nil
	}
	targetIDs := make([]string, 0, len(targets))
	targetIndex := make(map[string]int, len(targets))
	for index := range targets {
		targetIDs = append(targetIDs, targets[index].ID)
		targetIndex[targets[index].ID] = index
	}
	var bindings []model.DeploymentTargetHookBinding
	if err := h.dbWithContext(ctx).Where("target_id in ?", targetIDs).Order("run_order asc, created_at asc").Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		index, ok := targetIndex[binding.TargetID]
		if !ok {
			continue
		}
		targets[index].BuildHookBindings = append(targets[index].BuildHookBindings, binding)
	}
	return nil
}

func (h *Handlers) deploymentTargetWithHookBindings(target model.DeploymentTarget, ctx context.Context) (model.DeploymentTarget, error) {
	targets := []model.DeploymentTarget{target}
	if err := h.attachDeploymentTargetHookBindings(targets, ctx); err != nil {
		return target, err
	}
	return targets[0], nil
}

func (h *Handlers) replaceDeploymentTargetHookBindings(tx *gorm.DB, target model.DeploymentTarget, inputs []deploymentTargetHookBindingInput) ([]model.DeploymentTargetHookBinding, error) {
	if err := tx.Where("target_id = ?", target.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	hookIDs := make([]string, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		hookID := strings.TrimSpace(input.HookConfigID)
		phase := normalizeHookPhase(input.Phase)
		if hookID == "" || phase == "" {
			continue
		}
		key := phase + "\x00" + hookID
		if seen[key] {
			continue
		}
		seen[key] = true
		hookIDs = append(hookIDs, hookID)
	}
	if len(hookIDs) == 0 {
		return nil, nil
	}
	var hooks []model.ProjectHookConfig
	if err := tx.Where("project_id = ? and id in ?", target.ProjectID, hookIDs).Find(&hooks).Error; err != nil {
		return nil, err
	}
	validHookIDs := make(map[string]bool, len(hooks))
	for _, hook := range hooks {
		validHookIDs[hook.ID] = true
	}
	bindings := make([]model.DeploymentTargetHookBinding, 0, len(seen))
	created := make(map[string]bool, len(seen))
	for index, input := range inputs {
		hookID := strings.TrimSpace(input.HookConfigID)
		phase := normalizeHookPhase(input.Phase)
		if hookID == "" || phase == "" {
			continue
		}
		key := phase + "\x00" + hookID
		if created[key] {
			continue
		}
		created[key] = true
		if !validHookIDs[hookID] {
			return nil, errors.New("构建钩子不存在")
		}
		bindings = append(bindings, model.DeploymentTargetHookBinding{
			ID:            id.New("dtmhb"),
			ProjectID:     target.ProjectID,
			ApplicationID: target.ApplicationID,
			TargetID:      target.ID,
			HookConfigID:  hookID,
			Phase:         phase,
			RunOrder:      index + 1,
		})
	}
	if err := tx.Create(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}
