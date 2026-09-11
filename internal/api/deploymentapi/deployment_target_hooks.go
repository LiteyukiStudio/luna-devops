package deploymentapi

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

type deploymentTargetAfterPersist func(current model.DeploymentTarget) error

func (h *Handlers) createDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.persistDeploymentTarget(target, dataVolumes, hookInputs, buildEnvironment, nil, true, ctx)
}

func (h *Handlers) saveDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.saveDeploymentTargetWithAfterPersist(target, dataVolumes, hookInputs, buildEnvironment, nil, ctx)
}

func (h *Handlers) persistDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, secretValues []model.SecretValue, create bool, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.persistDeploymentTargetWithAfterPersist(target, dataVolumes, hookInputs, buildEnvironment, secretValues, create, nil, ctx)
}

func (h *Handlers) saveDeploymentTargetWithAfterPersist(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, afterPersist deploymentTargetAfterPersist, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.persistDeploymentTargetWithAfterPersist(target, dataVolumes, hookInputs, buildEnvironment, nil, false, afterPersist, ctx)
}

func (h *Handlers) persistDeploymentTargetWithAfterPersist(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, secretValues []model.SecretValue, create bool, afterPersist deploymentTargetAfterPersist, ctx context.Context) (deploymentVolumeMountChanges, error) {
	changes := deploymentVolumeMountChanges{}
	err := h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.DeploymentTarget
		if create {
			requestedReplicas := target.Replicas
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
			// GORM substitutes the model's default for zero-valued integer fields on
			// Create. Persist an explicitly requested stop after the insert while the
			// same transaction still owns the new target.
			if requestedReplicas == 0 {
				if err := tx.Model(&model.DeploymentTarget{}).
					Where("id = ?", target.ID).
					UpdateColumn("replicas", requestedReplicas).Error; err != nil {
					return err
				}
				target.Replicas = requestedReplicas
			}
		} else {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "project_id", "application_id", "cluster_id", "kubernetes_name", "workload_type", "replicas", "secret_refs").
				First(&current, "id = ?", target.ID).Error; err != nil {
				return err
			}
			target.SecretRefs = current.SecretRefs
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
		if afterPersist != nil {
			if err := afterPersist(current); err != nil {
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
