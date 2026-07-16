package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) runtimeConfigRefsFromInput(ctx *gin.Context, projectID string, input deploymentTargetInput, existingRaw string) ([]model.DeploymentRuntimeConfigRef, bool) {
	refs := runtimeConfigRefInputs(input)
	if len(refs) == 0 {
		return nil, true
	}
	existingSnapshots := map[string]*model.DeploymentRuntimeConfigSnapshot{}
	for _, ref := range model.DecodeDeploymentRuntimeConfigRefs(existingRaw) {
		if ref.Mode == model.RuntimeConfigRefModeSnapshot && ref.Snapshot != nil {
			snapshot := *ref.Snapshot
			existingSnapshots[ref.SetID] = &snapshot
		}
	}
	setIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		mode := model.RuntimeConfigRefMode(ref.Mode)
		if mode == model.RuntimeConfigRefModeSnapshot && existingSnapshots[ref.SetID] != nil {
			continue
		}
		setIDs = append(setIDs, ref.SetID)
	}
	var sets []model.ProjectRuntimeConfigSet
	if len(setIDs) > 0 {
		if err := h.db.Where("project_id = ? and id in ?", projectID, setIDs).Find(&sets).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return nil, false
		}
	}
	setsByID := make(map[string]model.ProjectRuntimeConfigSet, len(sets))
	for _, set := range sets {
		setsByID[set.ID] = set
	}
	if len(setsByID) != len(setIDs) {
		writeError(ctx, http.StatusBadRequest, "运行配置集不存在或不属于当前项目空间")
		return nil, false
	}
	now := time.Now()
	output := make([]model.DeploymentRuntimeConfigRef, 0, len(refs))
	for _, ref := range refs {
		mode := model.RuntimeConfigRefMode(ref.Mode)
		next := model.DeploymentRuntimeConfigRef{SetID: ref.SetID, Mode: mode}
		if mode == model.RuntimeConfigRefModeSnapshot {
			if snapshot := existingSnapshots[ref.SetID]; snapshot != nil {
				next.Snapshot = snapshot
			} else {
				snapshot := model.ProjectRuntimeConfigSetSnapshot(setsByID[ref.SetID], now)
				next.Snapshot = &snapshot
			}
		}
		output = append(output, next)
	}
	return output, true
}

func runtimeConfigRefInputs(input deploymentTargetInput) []deploymentRuntimeConfigRefInput {
	refs := make([]deploymentRuntimeConfigRefInput, 0)
	if len(input.RuntimeConfigRefs) > 0 {
		seen := map[string]bool{}
		for _, ref := range input.RuntimeConfigRefs {
			setID := strings.TrimSpace(ref.SetID)
			if setID == "" || seen[setID] {
				continue
			}
			seen[setID] = true
			refs = append(refs, deploymentRuntimeConfigRefInput{
				SetID: setID,
				Mode:  model.RuntimeConfigRefMode(ref.Mode),
			})
		}
		return refs
	}
	for _, setID := range normalizeStringList(input.RuntimeConfigSetIDs) {
		refs = append(refs, deploymentRuntimeConfigRefInput{SetID: setID, Mode: model.RuntimeConfigRefModeLive})
	}
	return refs
}

func (h *Handlers) applyRegistryCredentialImageTemplate(ctx *gin.Context, user model.User, app model.Application, sourceType string, registryID string, repository string, tag string, target model.DeploymentTarget) (string, string, bool) {
	if sourceType != "repository" || strings.TrimSpace(registryID) == "" {
		return repository, tag, true
	}
	var project model.Project
	if err := h.db.First(&project, "id = ?", app.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "项目空间不存在")
		return repository, tag, false
	}
	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", registryID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "目标镜像站不存在")
		return repository, tag, false
	}
	credential, ok := h.registryPushCredentialForProject(user, registry, app.ProjectID)
	if !ok {
		return repository, tag, true
	}
	if strings.TrimSpace(repository) == "" || isDefaultImageRepository(registry, project, app, repository) {
		repository, _ = splitTargetImageRef(buildTargetImageRepositoryForCredential(registry, credential, project, app, target))
		repository = repositoryWithoutRegistryHost(registry, repository)
	}
	if strings.TrimSpace(tag) == "" || (strings.TrimSpace(tag) == "latest" && strings.TrimSpace(credential.TagTemplate) != "") {
		tag = buildStaticTargetImageTagForCredential(registry, credential, project, app, target)
	}
	return strings.Trim(strings.TrimSpace(repository), "/"), strings.TrimSpace(tag), true
}
