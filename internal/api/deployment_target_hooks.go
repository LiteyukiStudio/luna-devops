package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errDeploymentStageExists = errors.New("deployment stage already exists")

func (h *Handlers) createDeploymentTarget(target model.DeploymentTarget, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) error {
	return h.persistDeploymentTarget(target, hookInputs, buildEnvironment, true, ctx)
}

func (h *Handlers) saveDeploymentTarget(target model.DeploymentTarget, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) error {
	return h.persistDeploymentTarget(target, hookInputs, buildEnvironment, false, ctx)
}

func (h *Handlers) persistDeploymentTarget(target model.DeploymentTarget, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, create bool, ctx context.Context) error {
	return h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		} else if err := tx.Save(&target).Error; err != nil {
			return err
		}
		var dataVolumes []deploymentTargetDataVolumeInput
		if strings.TrimSpace(target.DataVolumes) != "" {
			if err := json.Unmarshal([]byte(target.DataVolumes), &dataVolumes); err != nil {
				return err
			}
		}
		if err := reserveRetainedVolumes(tx, target.ProjectID, target.ApplicationID, target.ID, target.ClusterID, dataVolumes); err != nil {
			return err
		}
		if err := h.replaceDeploymentTargetHookBindings(tx, target, hookInputs); err != nil {
			return err
		}
		if buildEnvironment != nil {
			return tx.Save(buildEnvironment).Error
		}
		return nil
	})
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

func (h *Handlers) replaceDeploymentTargetHookBindings(tx *gorm.DB, target model.DeploymentTarget, inputs []deploymentTargetHookBindingInput) error {
	if err := tx.Where("target_id = ?", target.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
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
		return nil
	}
	var hooks []model.ProjectHookConfig
	if err := tx.Where("project_id = ? and id in ?", target.ProjectID, hookIDs).Find(&hooks).Error; err != nil {
		return err
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
			return errors.New("构建钩子不存在")
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
	return tx.Create(&bindings).Error
}
