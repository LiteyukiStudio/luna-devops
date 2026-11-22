package deploymentapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type aiHookBindingUpsertInput struct {
	Hook     string `json:"hook" required:"true" desc:"项目生命周期 Hook ID（hook_xxx）或唯一名称"`
	Phase    string `json:"phase" required:"true" enum:"prePull|postPull|preBuild|postBuild|prePush|postPush|preDeployment|postDeployment" desc:"生命周期执行阶段"`
	RunOrder *int   `json:"runOrder,omitempty" min:"0" desc:"阶段内执行顺序（显式传入时严格保留；省略默认取当前阶段 max(runOrder)+1）"`
}

type aiHookBindingRemoveInput struct {
	Hook  string `json:"hook" required:"true" desc:"项目生命周期 Hook ID（hook_xxx）或唯一名称"`
	Phase string `json:"phase" required:"true" enum:"prePull|postPull|preBuild|postBuild|prePush|postPush|preDeployment|postDeployment" desc:"生命周期执行阶段"`
}

type buildConfigureTargetPipelineInput struct {
	Project                 string                     `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application             string                     `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target                  string                     `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	SourceType              string                     `json:"sourceType,omitempty" enum:"repository|image" desc:"切换制品来源模式：repository 或 image"`
	ImageRef                string                     `json:"imageRef,omitempty" desc:"当最终 sourceType=image 时设置期望镜像地址"`
	RepositoryBinding       string                     `json:"repositoryBinding,omitempty" desc:"绑定代码仓库 ID（rpb_xxx）或 owner/repo"`
	BuildDefinitionMode     string                     `json:"buildDefinitionMode,omitempty" enum:"repository_dockerfile|template" desc:"构建定义模式：仓库 Dockerfile 或内置构建模板"`
	DockerfilePath          string                     `json:"dockerfilePath,omitempty" desc:"仓库内干净相对 Dockerfile 路径（如 Dockerfile 或 services/api/Dockerfile）"`
	BuildContext            string                     `json:"buildContext,omitempty" desc:"仓库内干净相对构建上下文路径（如 . 或 services/api）"`
	BuildDirectory          string                     `json:"buildDirectory,omitempty" desc:"仓库内干净相对工作子目录（禁止 / 或 ..）"`
	ClearBuildDirectory     *bool                      `json:"clearBuildDirectory,omitempty" desc:"清空自定义 buildDirectory 恢复仓库根目录"`
	BuildTemplate           string                     `json:"buildTemplate,omitempty" desc:"内置构建模板 ID（传入且未指定 buildDefinitionMode 时自动切为 template）"`
	BuildTemplateVersion    string                     `json:"buildTemplateVersion,omitempty" desc:"可选构建模板版本"`
	UpsertTemplateValues    map[string]string          `json:"upsertTemplateValues,omitempty" desc:"增量设置构建模板参数"`
	RemoveTemplateValueKeys []string                   `json:"removeTemplateValueKeys,omitempty" desc:"移除指定构建模板参数键"`
	UpsertBuildArgs         []aiEnvKeyValueInput       `json:"upsertBuildArgs,omitempty" desc:"增量设置 Docker --build-arg（禁止 LITEYUKI_* 保留前缀）"`
	RemoveBuildArgKeys      []string                   `json:"removeBuildArgKeys,omitempty" desc:"删除指定 --build-arg"`
	BuildCPURequest         string                     `json:"buildCpuRequest,omitempty" desc:"构建任务 CPU 请求量（如 2）"`
	BuildMemoryRequest      string                     `json:"buildMemoryRequest,omitempty" desc:"构建任务内存请求量（如 4Gi）"`
	BuildTimeoutSeconds     *int                       `json:"buildTimeoutSeconds,omitempty" min:"60" max:"86400" desc:"构建超时时间秒数（60..86400）"`
	TargetRegistry          string                     `json:"targetRegistry,omitempty" desc:"目标制品库 ID 或名称（省略时若尚未绑定自动解析项目默认制品库）"`
	TargetRepository        string                     `json:"targetRepository,omitempty" desc:"自定义目标镜像仓库路径"`
	TargetTag               string                     `json:"targetTag,omitempty" desc:"自定义目标镜像默认 Tag"`
	ResetTargetImage        *bool                      `json:"resetTargetImage,omitempty" desc:"清空自定义 targetRepository/targetTag，恢复按制品库凭据模板自动推导"`
	AttachVariableSets      []string                   `json:"attachVariableSets,omitempty" desc:"增量绑定构建变量集（ID 或名称）"`
	DetachVariableSets      []string                   `json:"detachVariableSets,omitempty" desc:"解绑构建变量集（ID 或名称）"`
	AutoDeploy              *bool                      `json:"autoDeploy,omitempty" desc:"构建成功后是否自动创建 Release（需 requireApproval=false 才自动发版）"`
	RequireApproval         *bool                      `json:"requireApproval,omitempty" desc:"自动构建产物的发版门禁开关"`
	BuildConcurrencyPolicy  string                     `json:"buildConcurrencyPolicy,omitempty" enum:"queue|cancel_in_progress|parallel" desc:"同目标并发构建策略"`
	BranchPattern           string                     `json:"branchPattern,omitempty" desc:"自动触发构建/发版的分支匹配规则"`
	ClearBranchPattern      *bool                      `json:"clearBranchPattern,omitempty" desc:"清空分支匹配规则"`
	TagPattern              string                     `json:"tagPattern,omitempty" desc:"自动触发构建/发版的 Git Tag 匹配规则"`
	ClearTagPattern         *bool                      `json:"clearTagPattern,omitempty" desc:"清空 Tag 匹配规则"`
	BuildHooksEnabled       *bool                      `json:"buildHooksEnabled,omitempty" desc:"是否启用生命周期构建/发布脚本 Hook"`
	UpsertHookBindings      []aiHookBindingUpsertInput `json:"upsertHookBindings,omitempty" desc:"按 (hook, phase) 新增或更新生命周期 Hook 顺序"`
	RemoveHookBindings      []aiHookBindingRemoveInput `json:"removeHookBindings,omitempty" desc:"解绑指定阶段生命周期 Hook"`
}

type buildConfigureTargetPipelineOutput struct {
	ID                  string                              `json:"id"`
	DeploymentTargetID  string                              `json:"deploymentTargetId"`
	ProjectID           string                              `json:"projectId"`
	ApplicationID       string                              `json:"applicationId"`
	Stage               string                              `json:"stage"`
	SourceType          string                              `json:"sourceType"`
	ImageRef            string                              `json:"imageRef,omitempty"`
	RepositoryBindingID string                              `json:"repositoryBindingId,omitempty"`
	BuildDefinitionMode string                              `json:"buildDefinitionMode"`
	BuildTemplateID     string                              `json:"buildTemplateId,omitempty"`
	DockerfilePath      string                              `json:"dockerfilePath,omitempty"`
	BuildContext        string                              `json:"buildContext,omitempty"`
	BuildDirectory      string                              `json:"buildDirectory,omitempty"`
	TargetRegistryID    string                              `json:"targetRegistryId,omitempty"`
	TargetRepository    string                              `json:"targetRepository,omitempty"`
	TargetTag           string                              `json:"targetTag,omitempty"`
	BuildVariableSetIDs []string                            `json:"buildVariableSetIds"`
	AutoDeploy          bool                                `json:"autoDeploy"`
	RequireApproval     bool                                `json:"requireApproval"`
	BuildHooksEnabled   bool                                `json:"buildHooksEnabled"`
	BuildHookBindings   []model.DeploymentTargetHookBinding `json:"buildHookBindings"`
	RequiresRelease     bool                                `json:"requiresRelease"`
}

func (h *Handler) registerBuildTargetPipelineAITool(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[buildConfigureTargetPipelineInput, buildConfigureTargetPipelineOutput]{
		Name:           "build_configure_target_pipeline",
		Domain:         aitool.DomainBuild,
		Title:          "增量配置部署目标构建流水线与生命周期 Hook",
		Summary:        "纯增量配置部署目标的代码源/镜像源模式、Dockerfile/构建模板、构建规格、变量集、自动发版策略与生命周期 Hook 绑定，100% 保护存量运行时配置与持久化卷。",
		AliasesZH:      []string{"配置构建流水线", "修改Dockerfile路径", "绑定构建Hook", "自动发版规则"},
		AliasesEN:      []string{"configure target pipeline", "update build config", "bind deployment hooks"},
		Purpose:        aitool.Text("纯增量修改部署目标上的 CI/CD 构建流水线配置、变量集引用、自动触发规则与 Hook 阶段绑定。", "Incrementally patch CI/CD build pipeline fields, variable sets, auto-deploy triggers, and lifecycle hook bindings of a deployment target."),
		ApprovalTier:   aitool.ApprovalTierL2,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate)},
		Effect:         aitool.EffectWrite,
		Verification:   targetVerificationRule(),
		Preview:        h.previewBuildConfigureTargetPipeline,
		Execute:        h.executeBuildConfigureTargetPipeline,
	})
}

type preparedPipelinePatchPlan struct {
	user            model.User
	project         model.Project
	app             model.Application
	existing        model.DeploymentTarget
	next            model.DeploymentTarget
	hooksChanged    bool
	nextHookInputs  []deploymentTargetHookBindingInput
	requiresRelease bool
	changes         []aitool.PreviewChange
	warnings        []string
}

func (h *Handler) preparePipelinePatchPlan(tc *aitool.ToolContext, in buildConfigureTargetPipelineInput) (preparedPipelinePatchPlan, error) {
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentUpdate)
	if err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	if err := ensureAIApplicationCanMutate(app); err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	existingRaw, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, existingRaw); err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	if err := h.ensureAIBillingAllowsDeployChange(tc, project.ID); err != nil {
		return preparedPipelinePatchPlan{}, err
	}
	existing, err := h.deploymentTargetWithHookBindings(existingRaw, tc.Context())
	if err != nil {
		return preparedPipelinePatchPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	next := existing
	var (
		changes  []aitool.PreviewChange
		warnings []string
	)
	addChange := func(field string, before, after any) {
		changes = append(changes, aitool.PreviewChange{Field: field, Before: before, After: after, Action: "update"})
	}

	curSourceType := normalizeDeploymentSourceType(existing.SourceType)
	nextSourceType := curSourceType
	if st := strings.TrimSpace(in.SourceType); st != "" {
		if st != "repository" && st != "image" {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "sourceType must be repository or image")
		}
		nextSourceType = st
		if nextSourceType != curSourceType {
			next.SourceType = nextSourceType
			addChange("sourceType", curSourceType, nextSourceType)
		}
	} else {
		next.SourceType = curSourceType
	}

	if img := strings.TrimSpace(in.ImageRef); img != "" && img != strings.TrimSpace(existing.ImageRef) {
		next.ImageRef = img
		addChange("imageRef", existing.ImageRef, img)
	}
	if nextSourceType == "image" && strings.TrimSpace(next.ImageRef) == "" {
		return preparedPipelinePatchPlan{}, aitool.BadRequest("deployment_target.image_ref_required", "imageRef is required when sourceType=image")
	}

	if strings.TrimSpace(in.RepositoryBinding) != "" {
		binding, bErr := resolveRepositoryBindingForApp(db, project.ID, app.ID, in.RepositoryBinding)
		if bErr != nil {
			return preparedPipelinePatchPlan{}, bErr
		}
		if binding.ID != existing.RepositoryBindingID {
			beforeRepo := existing.RepositoryBindingID
			if strings.TrimSpace(existing.RepositoryBindingID) != "" {
				var prevBinding model.RepositoryBinding
				if db.First(&prevBinding, "id = ? AND project_id = ? AND deleted_at IS NULL", existing.RepositoryBindingID, project.ID).Error == nil {
					beforeRepo = fmt.Sprintf("%s/%s", prevBinding.Owner, prevBinding.Repo)
				}
			}
			next.RepositoryBindingID = binding.ID
			addChange("repositoryBinding", beforeRepo, fmt.Sprintf("%s/%s", binding.Owner, binding.Repo))
		}
	}
	if nextSourceType == "repository" && strings.TrimSpace(next.RepositoryBindingID) == "" {
		return preparedPipelinePatchPlan{}, aitool.BadRequest("deployment_target.repository_binding_required", "repositoryBinding is required when sourceType=repository")
	}

	// Dockerfile, context, buildDirectory (with relative path traversal defense)
	if strings.TrimSpace(in.DockerfilePath) != "" {
		cleaned, pErr := validateRelativeRepoPath(in.DockerfilePath, "dockerfilePath", false)
		if pErr != nil {
			return preparedPipelinePatchPlan{}, pErr
		}
		if cleaned != existing.DockerfilePath {
			next.DockerfilePath = cleaned
			addChange("dockerfilePath", existing.DockerfilePath, cleaned)
		}
	} else if strings.TrimSpace(next.DockerfilePath) == "" {
		next.DockerfilePath = "Dockerfile"
	}

	if strings.TrimSpace(in.BuildContext) != "" {
		cleaned, pErr := validateRelativeRepoPath(in.BuildContext, "buildContext", true)
		if pErr != nil {
			return preparedPipelinePatchPlan{}, pErr
		}
		if cleaned != existing.BuildContext {
			next.BuildContext = cleaned
			addChange("buildContext", existing.BuildContext, cleaned)
		}
	} else if strings.TrimSpace(next.BuildContext) == "" {
		next.BuildContext = "."
	}

	if boolVal(in.ClearBuildDirectory, false) {
		if strings.TrimSpace(in.BuildDirectory) != "" {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "buildDirectory and clearBuildDirectory=true are mutually exclusive")
		}
		if existing.BuildDirectory != "" {
			next.BuildDirectory = ""
			addChange("buildDirectory", existing.BuildDirectory, "")
		}
	} else if strings.TrimSpace(in.BuildDirectory) != "" {
		cleaned, pErr := validateRelativeRepoPath(in.BuildDirectory, "buildDirectory", true)
		if pErr != nil {
			return preparedPipelinePatchPlan{}, pErr
		}
		if cleaned == "." {
			cleaned = ""
		}
		if cleaned != existing.BuildDirectory {
			next.BuildDirectory = cleaned
			addChange("buildDirectory", existing.BuildDirectory, cleaned)
		}
	}

	// BuildDefinitionMode & BuildTemplate
	curMode := fallback(strings.TrimSpace(existing.BuildDefinitionMode), buildtemplate.DefinitionModeRepository)
	nextMode := curMode
	if m := strings.TrimSpace(in.BuildDefinitionMode); m != "" {
		if m != buildtemplate.DefinitionModeRepository && m != buildtemplate.DefinitionModeTemplate {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "buildDefinitionMode must be repository_dockerfile or template")
		}
		nextMode = m
	} else if strings.TrimSpace(in.BuildTemplate) != "" {
		nextMode = buildtemplate.DefinitionModeTemplate
	}
	next.BuildDefinitionMode = nextMode
	if nextMode != curMode {
		addChange("buildDefinitionMode", curMode, nextMode)
	}

	if strings.TrimSpace(in.BuildTemplate) != "" {
		next.BuildTemplateID = strings.TrimSpace(in.BuildTemplate)
	}
	if strings.TrimSpace(in.BuildTemplateVersion) != "" {
		next.BuildTemplateVersion = strings.TrimSpace(in.BuildTemplateVersion)
	}

	if nextSourceType == "repository" && nextMode == buildtemplate.DefinitionModeTemplate {
		if strings.TrimSpace(next.BuildTemplateID) == "" {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("build_template.required", "buildTemplate is required when buildDefinitionMode=template")
		}
		def, found := buildtemplate.Find(next.BuildTemplateID, next.BuildTemplateVersion)
		if !found {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("build_template.not_found", fmt.Sprintf("build template %q was not found", next.BuildTemplateID))
		}
		tplValues := map[string]string{}
		if strings.TrimSpace(existing.BuildTemplateValues) != "" {
			_ = json.Unmarshal([]byte(existing.BuildTemplateValues), &tplValues)
		}
		// Strip stale keys that don't belong to the newly selected template's Parameters!
		if next.BuildTemplateID != existing.BuildTemplateID {
			allowedParamKeys := make(map[string]bool, len(def.Parameters))
			for _, p := range def.Parameters {
				allowedParamKeys[p.Key] = true
			}
			for k := range tplValues {
				if !allowedParamKeys[k] {
					delete(tplValues, k)
				}
			}
		}
		for _, rk := range in.RemoveTemplateValueKeys {
			delete(tplValues, strings.TrimSpace(rk))
		}
		for k, v := range in.UpsertTemplateValues {
			tplValues[strings.TrimSpace(k)] = v
		}
		rawValJSON, _ := json.Marshal(tplValues)
		normVals, tErr := buildtemplate.NormalizeValues(def, string(rawValJSON))
		if tErr != nil {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("build_template.invalid", tErr.Error())
		}
		next.BuildTemplateVersion = def.Version
		next.BuildTemplateValues = buildtemplate.EncodeValues(normVals)
		if next.BuildTemplateID != existing.BuildTemplateID || next.BuildTemplateValues != existing.BuildTemplateValues {
			addChange("buildTemplate", existing.BuildTemplateID, next.BuildTemplateID)
		}
	}

	// BuildArgs patch (forbid LITEYUKI_* reserved prefix)
	if len(in.UpsertBuildArgs) > 0 || len(in.RemoveBuildArgKeys) > 0 {
		argsMap := model.BuildArgs(existing.BuildArgs)
		if argsMap == nil {
			argsMap = map[string]string{}
		}
		removeArgSet := map[string]bool{}
		for _, rk := range in.RemoveBuildArgKeys {
			k := strings.TrimSpace(rk)
			if k != "" {
				removeArgSet[k] = true
				delete(argsMap, k)
			}
		}
		for _, item := range in.UpsertBuildArgs {
			k := strings.TrimSpace(item.Key)
			if !isBuildEnvKey(k) {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("build.variable_key_invalid", fmt.Sprintf("invalid buildArg key %q", k))
			}
			if strings.HasPrefix(strings.ToUpper(k), "LITEYUKI_") {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("build.reserved_variable_key", fmt.Sprintf("buildArg key %q cannot use reserved prefix LITEYUKI_", k))
			}
			if removeArgSet[k] {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("buildArg %q cannot be both upserted and removed", k))
			}
			argsMap[k] = item.Value
		}
		keys := make([]string, 0, len(argsMap))
		for k := range argsMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, k := range keys {
			lines = append(lines, k+"="+argsMap[k])
		}
		scratchCtx, rec := newScratchGin(tc)
		normArgs, ok := normalizeBuildArgsInput(scratchCtx, strings.Join(lines, "\n"))
		if !ok {
			return preparedPipelinePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "build.args_invalid", "buildArgs are invalid")
		}
		if normArgs != existing.BuildArgs {
			next.BuildArgs = normArgs
			addChange("buildArgs", model.BuildArgs(existing.BuildArgs), argsMap)
		}
	}

	if strings.TrimSpace(in.BuildCPURequest) != "" {
		cpuQ, qErr := normalizeBuildResourceQuantityValue(in.BuildCPURequest, "", "buildCpuRequest")
		if qErr != nil {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("deployment.resource_quantity_invalid", "buildCpuRequest must be a positive Kubernetes quantity")
		}
		if cpuQ != existing.BuildCPURequest {
			next.BuildCPURequest = cpuQ
			addChange("buildCpuRequest", existing.BuildCPURequest, cpuQ)
		}
	}
	if strings.TrimSpace(in.BuildMemoryRequest) != "" {
		memQ, qErr := normalizeBuildResourceQuantityValue(in.BuildMemoryRequest, "", "buildMemoryRequest")
		if qErr != nil {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("deployment.resource_quantity_invalid", "buildMemoryRequest must be a positive Kubernetes quantity")
		}
		if memQ != existing.BuildMemoryRequest {
			next.BuildMemoryRequest = memQ
			addChange("buildMemoryRequest", existing.BuildMemoryRequest, memQ)
		}
	}
	if in.BuildTimeoutSeconds != nil {
		if *in.BuildTimeoutSeconds < minBuildTimeoutSeconds || *in.BuildTimeoutSeconds > maxBuildTimeoutSeconds {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "buildTimeoutSeconds must be between 60 and 86400")
		}
		if *in.BuildTimeoutSeconds != existing.BuildTimeoutSeconds {
			next.BuildTimeoutSeconds = *in.BuildTimeoutSeconds
			addChange("buildTimeoutSeconds", existing.BuildTimeoutSeconds, next.BuildTimeoutSeconds)
		}
	}

	// TargetRegistry & TargetRepository/TargetTag
	resolveRegistryDisplayName := func(registryID string) string {
		trimmed := strings.TrimSpace(registryID)
		if trimmed == "" {
			return ""
		}
		var prevReg model.ArtifactRegistry
		if db.First(&prevReg, "id = ? AND deleted_at IS NULL", trimmed).Error == nil && strings.TrimSpace(prevReg.Name) != "" {
			if authz.IsPlatformAdmin(user.Role) || canUseScopedResourceForTargetProject(db, user, prevReg.Scope, prevReg.OwnerRef, scopedResourceArtifactRegistry, prevReg.ID, project.ID) {
				return prevReg.Name
			}
		}
		return trimmed
	}
	registryChanged := false
	if strings.TrimSpace(in.TargetRegistry) != "" {
		reg, rErr := h.resolveArtifactRegistryForProject(tc, user, project.ID, in.TargetRegistry)
		if rErr != nil {
			return preparedPipelinePatchPlan{}, rErr
		}
		if reg.ID != existing.TargetRegistryID {
			next.TargetRegistryID = reg.ID
			registryChanged = true
			addChange("targetRegistry", resolveRegistryDisplayName(existing.TargetRegistryID), fallback(strings.TrimSpace(reg.Name), reg.ID))
		}
	} else if nextSourceType == "repository" && strings.TrimSpace(next.TargetRegistryID) == "" {
		defReg, hasDef := h.resolveDefaultArtifactRegistryForProject(tc, user, project.ID)
		if !hasDef {
			return preparedPipelinePatchPlan{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"deployment_target.target_registry_required",
				"targetRegistry is required when sourceType=repository and no default artifact registry is configured for this project",
				"Pass `targetRegistry` explicitly (inspect available registries via `build_list_registries`).",
			)
		}
		next.TargetRegistryID = defReg.ID
		registryChanged = true
		addChange("targetRegistry", resolveRegistryDisplayName(existing.TargetRegistryID), fallback(strings.TrimSpace(defReg.Name), defReg.ID))
	}

	imageFieldsTouched := registryChanged
	if boolVal(in.ResetTargetImage, false) {
		if strings.TrimSpace(in.TargetRepository) != "" || strings.TrimSpace(in.TargetTag) != "" {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "resetTargetImage=true cannot be combined with targetRepository or targetTag")
		}
		next.TargetRepository = ""
		next.TargetTag = ""
		imageFieldsTouched = true
	} else {
		if strings.TrimSpace(in.TargetRepository) != "" {
			next.TargetRepository = strings.Trim(strings.TrimSpace(in.TargetRepository), "/")
			imageFieldsTouched = true
		}
		if strings.TrimSpace(in.TargetTag) != "" {
			next.TargetTag = strings.TrimSpace(in.TargetTag)
			imageFieldsTouched = true
		}
	}

	if nextSourceType == "repository" && (imageFieldsTouched || strings.TrimSpace(next.TargetRepository) == "" || strings.TrimSpace(next.TargetTag) == "") {
		scratchCtx, rec := newScratchGin(tc)
		repoOut, tagOut, ok := h.applyRegistryCredentialImageTemplate(scratchCtx, user, app, nextSourceType, next.TargetRegistryID, next.TargetRepository, next.TargetTag, next)
		if !ok {
			return preparedPipelinePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment_target.target_image_invalid", "target registry image template failed")
		}
		next.TargetRepository = repoOut
		next.TargetTag = fallback(tagOut, "latest")

		scratchCheck, checkRec := newScratchGin(tc)
		if !h.ensureTargetImageRepositoryAvailable(scratchCheck, project.ID, next.TargetRegistryID, next.TargetRepository) {
			return preparedPipelinePatchPlan{}, toolErrorFromRecorder(checkRec, http.StatusConflict, "deployment_target.image_repository_claimed", "target image repository is already claimed by another project")
		}
		if next.TargetRepository != existing.TargetRepository {
			addChange("targetRepository", existing.TargetRepository, next.TargetRepository)
		}
		if next.TargetTag != existing.TargetTag {
			addChange("targetTag", existing.TargetTag, next.TargetTag)
		}
	}

	// BuildVariableSetIDs patch
	if len(in.AttachVariableSets) > 0 || len(in.DetachVariableSets) > 0 {
		curSetIDs := buildVariableSetIDs(existing.BuildVariableSetIDs)
		detachMap := map[string]bool{}
		for _, rawDetach := range in.DetachVariableSets {
			resolved, sErr := h.resolveBuildVariableSetForProject(tc, user, project.ID, rawDetach)
			if sErr != nil {
				if slices.Contains(curSetIDs, strings.TrimSpace(rawDetach)) {
					detachMap[strings.TrimSpace(rawDetach)] = true
					continue
				}
				return preparedPipelinePatchPlan{}, sErr
			}
			detachMap[resolved.ID] = true
		}
		mergedIDs := make([]string, 0, len(curSetIDs)+len(in.AttachVariableSets))
		seenID := map[string]bool{}
		for _, idVal := range curSetIDs {
			if detachMap[idVal] || seenID[idVal] {
				continue
			}
			seenID[idVal] = true
			mergedIDs = append(mergedIDs, idVal)
		}
		for _, rawAttach := range in.AttachVariableSets {
			resolved, sErr := h.resolveBuildVariableSetForProject(tc, user, project.ID, rawAttach)
			if sErr != nil {
				return preparedPipelinePatchPlan{}, sErr
			}
			if detachMap[resolved.ID] {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("variable set %q cannot be both attached and detached", resolved.Name))
			}
			if !seenID[resolved.ID] {
				seenID[resolved.ID] = true
				mergedIDs = append(mergedIDs, resolved.ID)
			}
		}
		encodedSets := encodeBuildVariableSetIDs(mergedIDs)
		if encodedSets != existing.BuildVariableSetIDs {
			next.BuildVariableSetIDs = encodedSets
			resolveVarSetNames := func(ids []string) []string {
				if len(ids) == 0 {
					return []string{}
				}
				var sets []model.BuildVariableSet
				_ = db.Where("id IN ? AND deleted_at IS NULL", ids).Find(&sets).Error
				nameByID := make(map[string]string, len(sets))
				for _, s := range sets {
					if strings.TrimSpace(s.Name) != "" && (authz.IsPlatformAdmin(user.Role) || canUseScopedResourceForTargetProject(db, user, s.Scope, s.OwnerRef, scopedResourceBuildVariableSet, s.ID, project.ID)) {
						nameByID[s.ID] = s.Name
					}
				}
				out := make([]string, 0, len(ids))
				for _, idItem := range ids {
					if name, ok := nameByID[idItem]; ok {
						out = append(out, name)
					} else {
						out = append(out, idItem)
					}
				}
				return out
			}
			addChange("buildVariableSets", resolveVarSetNames(curSetIDs), resolveVarSetNames(buildVariableSetIDs(encodedSets)))
		}
	}

	if in.AutoDeploy != nil && *in.AutoDeploy != existing.AutoDeploy {
		next.AutoDeploy = *in.AutoDeploy
		addChange("autoDeploy", existing.AutoDeploy, next.AutoDeploy)
	}
	if in.RequireApproval != nil && *in.RequireApproval != existing.RequireApproval {
		next.RequireApproval = *in.RequireApproval
		addChange("requireApproval", existing.RequireApproval, next.RequireApproval)
	}
	if next.AutoDeploy && next.RequireApproval {
		warnings = append(warnings, "auto_deploy_gated_by_approval")
	}

	if pol := strings.TrimSpace(in.BuildConcurrencyPolicy); pol != "" {
		scratchCtx, rec := newScratchGin(tc)
		normPol, ok := normalizeBuildConcurrencyPolicy(scratchCtx, pol)
		if !ok {
			return preparedPipelinePatchPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "deployment_target.build_concurrency_policy_invalid", "invalid buildConcurrencyPolicy")
		}
		if normPol != existing.BuildConcurrencyPolicy {
			next.BuildConcurrencyPolicy = normPol
			addChange("buildConcurrencyPolicy", existing.BuildConcurrencyPolicy, normPol)
		}
	}

	if boolVal(in.ClearBranchPattern, false) {
		if strings.TrimSpace(in.BranchPattern) != "" {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "branchPattern and clearBranchPattern=true are mutually exclusive")
		}
		if existing.BranchPattern != "" {
			next.BranchPattern = ""
			addChange("branchPattern", existing.BranchPattern, "")
		}
	} else if strings.TrimSpace(in.BranchPattern) != "" && strings.TrimSpace(in.BranchPattern) != existing.BranchPattern {
		next.BranchPattern = strings.TrimSpace(in.BranchPattern)
		addChange("branchPattern", existing.BranchPattern, next.BranchPattern)
	}

	if boolVal(in.ClearTagPattern, false) {
		if strings.TrimSpace(in.TagPattern) != "" {
			return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", "tagPattern and clearTagPattern=true are mutually exclusive")
		}
		if existing.TagPattern != "" {
			next.TagPattern = ""
			addChange("tagPattern", existing.TagPattern, "")
		}
	} else if strings.TrimSpace(in.TagPattern) != "" && strings.TrimSpace(in.TagPattern) != existing.TagPattern {
		next.TagPattern = strings.TrimSpace(in.TagPattern)
		addChange("tagPattern", existing.TagPattern, next.TagPattern)
	}

	if in.BuildHooksEnabled != nil && *in.BuildHooksEnabled != existing.BuildHooksEnabled {
		next.BuildHooksEnabled = *in.BuildHooksEnabled
		addChange("buildHooksEnabled", existing.BuildHooksEnabled, next.BuildHooksEnabled)
	}

	// Hook bindings patch (only updated when upsertHookBindings or removeHookBindings is passed!)
	hooksChanged := len(in.UpsertHookBindings) > 0 || len(in.RemoveHookBindings) > 0
	var nextHookInputs []deploymentTargetHookBindingInput
	if hooksChanged {
		removePairs := map[string]bool{}
		for _, rem := range in.RemoveHookBindings {
			phase := normalizeHookPhase(rem.Phase)
			if phase == "" {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid hook phase %q", rem.Phase))
			}
			hc, hErr := resolveProjectHookConfig(db, project.ID, rem.Hook)
			if hErr != nil {
				return preparedPipelinePatchPlan{}, hErr
			}
			removePairs[phase+"\x00"+hc.ID] = true
			changes = append(changes, aitool.PreviewChange{Field: fmt.Sprintf("hookBinding:%s:%s", phase, hc.Name), Action: "remove"})
		}

		// Collect surviving existing bindings and compute max RunOrder per phase
		maxOrderByPhase := map[string]int{}
		surviving := make([]deploymentTargetHookBindingInput, 0, len(existing.BuildHookBindings))
		existingIndexByKey := map[string]int{}
		for _, hb := range existing.BuildHookBindings {
			key := hb.Phase + "\x00" + hb.HookConfigID
			if removePairs[key] {
				continue
			}
			if hb.RunOrder > maxOrderByPhase[hb.Phase] {
				maxOrderByPhase[hb.Phase] = hb.RunOrder
			}
			existingIndexByKey[key] = len(surviving)
			surviving = append(surviving, deploymentTargetHookBindingInput{
				HookConfigID: hb.HookConfigID,
				Phase:        hb.Phase,
				RunOrder:     hb.RunOrder,
			})
		}

		seenUpsertKey := map[string]bool{}
		for _, up := range in.UpsertHookBindings {
			phase := normalizeHookPhase(up.Phase)
			if phase == "" {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid hook phase %q", up.Phase))
			}
			hc, hErr := resolveProjectHookConfig(db, project.ID, up.Hook)
			if hErr != nil {
				return preparedPipelinePatchPlan{}, hErr
			}
			key := phase + "\x00" + hc.ID
			if removePairs[key] {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("hook %q in phase %s cannot be both upserted and removed", hc.Name, phase))
			}
			if seenUpsertKey[key] {
				return preparedPipelinePatchPlan{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("duplicate hook %q for phase %s in upsertHookBindings", hc.Name, phase))
			}
			seenUpsertKey[key] = true

			if idx, existed := existingIndexByKey[key]; existed {
				order := surviving[idx].RunOrder
				if up.RunOrder != nil {
					order = *up.RunOrder
				}
				surviving[idx].RunOrder = order
				if order > maxOrderByPhase[phase] {
					maxOrderByPhase[phase] = order
				}
				changes = append(changes, aitool.PreviewChange{Field: fmt.Sprintf("hookBinding:%s:%s", phase, hc.Name), After: order, Action: "update"})
			} else {
				var order int
				if up.RunOrder != nil {
					order = *up.RunOrder
				} else {
					order = maxOrderByPhase[phase] + 1
				}
				if order > maxOrderByPhase[phase] {
					maxOrderByPhase[phase] = order
				}
				surviving = append(surviving, deploymentTargetHookBindingInput{
					HookConfigID: hc.ID,
					Phase:        phase,
					RunOrder:     order,
				})
				changes = append(changes, aitool.PreviewChange{Field: fmt.Sprintf("hookBinding:%s:%s", phase, hc.Name), After: order, Action: "bind"})
			}
		}
		nextHookInputs = surviving
	}

	requiresRelease := strings.TrimSpace(next.ImageRef) != strings.TrimSpace(existing.ImageRef)
	if !requiresRelease && nextSourceType == "repository" {
		warnings = append(warnings, "pipeline_applies_to_next_build")
	}

	return preparedPipelinePatchPlan{
		user:            user,
		project:         project,
		app:             app,
		existing:        existing,
		next:            next,
		hooksChanged:    hooksChanged,
		nextHookInputs:  nextHookInputs,
		requiresRelease: requiresRelease,
		changes:         changes,
		warnings:        warnings,
	}, nil
}

// replaceDeploymentTargetHookBindingsPreserveOrder replaces DeploymentTargetHookBinding rows while
// strictly preserving each input's explicit RunOrder integer instead of rewriting to index+1.
func (h *Handler) replaceDeploymentTargetHookBindingsPreserveOrder(
	tx *gorm.DB,
	target model.DeploymentTarget,
	inputs []deploymentTargetHookBindingInput,
) ([]model.DeploymentTargetHookBinding, error) {
	if err := tx.Where("target_id = ?", target.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return []model.DeploymentTargetHookBinding{}, nil
	}
	hookIDs := make([]string, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	for _, in := range inputs {
		hid := strings.TrimSpace(in.HookConfigID)
		ph := normalizeHookPhase(in.Phase)
		if hid == "" || ph == "" {
			continue
		}
		k := ph + "\x00" + hid
		if seen[k] {
			continue
		}
		seen[k] = true
		hookIDs = append(hookIDs, hid)
	}
	if len(hookIDs) == 0 {
		return []model.DeploymentTargetHookBinding{}, nil
	}
	var hooks []model.ProjectHookConfig
	if err := tx.Where("project_id = ? AND id IN ? AND deleted_at IS NULL", target.ProjectID, hookIDs).Find(&hooks).Error; err != nil {
		return nil, err
	}
	validHookIDs := make(map[string]bool, len(hooks))
	for _, hc := range hooks {
		validHookIDs[hc.ID] = true
	}

	bindings := make([]model.DeploymentTargetHookBinding, 0, len(seen))
	created := make(map[string]bool, len(seen))
	for idx, in := range inputs {
		hid := strings.TrimSpace(in.HookConfigID)
		ph := normalizeHookPhase(in.Phase)
		if hid == "" || ph == "" {
			continue
		}
		k := ph + "\x00" + hid
		if created[k] {
			continue
		}
		created[k] = true
		if !validHookIDs[hid] {
			return nil, aitool.NotFound("hook.not_found", fmt.Sprintf("hook config %s was not found in this project", hid))
		}
		order := in.RunOrder
		if order < 0 {
			order = idx + 1
		}
		bindings = append(bindings, model.DeploymentTargetHookBinding{
			ID:            id.New("dtmhb"),
			ProjectID:     target.ProjectID,
			ApplicationID: target.ApplicationID,
			TargetID:      target.ID,
			HookConfigID:  hid,
			Phase:         ph,
			RunOrder:      order,
		})
	}
	if err := tx.Create(&bindings).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].RunOrder != bindings[j].RunOrder {
			return bindings[i].RunOrder < bindings[j].RunOrder
		}
		return bindings[i].ID < bindings[j].ID
	})
	return bindings, nil
}

func (h *Handler) previewBuildConfigureTargetPipeline(tc *aitool.ToolContext, in buildConfigureTargetPipelineInput) (*aitool.PreviewResult, error) {
	plan, err := h.preparePipelinePatchPlan(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			TargetResource: fmt.Sprintf("%s/%s/%s", plan.project.Identifier, plan.app.Identifier, plan.existing.Stage),
			TargetID:       plan.existing.ID,
			ProjectID:      plan.project.ID,
			Changes:        plan.changes,
			Warnings:       plan.warnings,
		},
	}, nil
}

func (h *Handler) executeBuildConfigureTargetPipeline(tc *aitool.ToolContext, in buildConfigureTargetPipelineInput) (buildConfigureTargetPipelineOutput, error) {
	plan, err := h.preparePipelinePatchPlan(tc, in)
	if err != nil {
		return buildConfigureTargetPipelineOutput{}, err
	}
	ctx := tc.Context()
	db := h.aiDB(tc)
	next := plan.next
	finalHooks := plan.existing.BuildHookBindings

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var locked model.DeploymentTarget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&locked, "id = ? AND project_id = ?", next.ID, plan.project.ID).Error; err != nil {
			return err
		}
		if !h.host.ResourceCanMutateDuringDelete(locked.DeleteStatus) {
			return aitool.Conflict("deployment_target.delete_in_progress", "deployment target deletion is in progress")
		}
		// Strictly preserve 100% of runtime spec, K8s advanced columns, env/secrets/files/configSets, and BuildEnvironmentID!
		// NEVER call syncDeploymentTargetVolumeMounts here!
		locked.SourceType = next.SourceType
		locked.ImageRef = next.ImageRef
		locked.RepositoryBindingID = next.RepositoryBindingID
		locked.BuildDefinitionMode = next.BuildDefinitionMode
		locked.BuildTemplateID = next.BuildTemplateID
		locked.BuildTemplateVersion = next.BuildTemplateVersion
		locked.BuildTemplateValues = next.BuildTemplateValues
		locked.DockerfilePath = next.DockerfilePath
		locked.BuildContext = next.BuildContext
		locked.BuildDirectory = next.BuildDirectory
		locked.BuildArgs = next.BuildArgs
		locked.BuildCPURequest = next.BuildCPURequest
		locked.BuildMemoryRequest = next.BuildMemoryRequest
		locked.BuildTimeoutSeconds = next.BuildTimeoutSeconds
		locked.TargetRegistryID = next.TargetRegistryID
		locked.TargetRepository = next.TargetRepository
		locked.TargetTag = next.TargetTag
		locked.BuildVariableSetIDs = next.BuildVariableSetIDs
		locked.AutoDeploy = next.AutoDeploy
		locked.RequireApproval = next.RequireApproval
		locked.BuildConcurrencyPolicy = next.BuildConcurrencyPolicy
		locked.BranchPattern = next.BranchPattern
		locked.TagPattern = next.TagPattern
		locked.BuildHooksEnabled = next.BuildHooksEnabled

		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if plan.hooksChanged {
			replaced, hErr := h.replaceDeploymentTargetHookBindingsPreserveOrder(tx, locked, plan.nextHookInputs)
			if hErr != nil {
				return hErr
			}
			finalHooks = replaced
		}
		next = locked
		return nil
	})
	if txErr != nil {
		if te, ok := aitool.AsToolError(txErr); ok {
			return buildConfigureTargetPipelineOutput{}, te
		}
		return buildConfigureTargetPipelineOutput{}, aitool.NewError(http.StatusBadRequest, "build.pipeline_update_failed", txErr.Error())
	}

	h.auditWithContext(plan.user.ID, "deployment_target.pipeline_update", next.ID, true, next.Stage, ctx)
	if finalHooks == nil {
		finalHooks = []model.DeploymentTargetHookBinding{}
	}
	varSetIDs := buildVariableSetIDs(next.BuildVariableSetIDs)
	if varSetIDs == nil {
		varSetIDs = []string{}
	}

	return buildConfigureTargetPipelineOutput{
		ID:                  next.ID,
		DeploymentTargetID:  next.ID,
		ProjectID:           plan.project.ID,
		ApplicationID:       plan.app.ID,
		Stage:               next.Stage,
		SourceType:          normalizeDeploymentSourceType(next.SourceType),
		ImageRef:            next.ImageRef,
		RepositoryBindingID: next.RepositoryBindingID,
		BuildDefinitionMode: fallback(strings.TrimSpace(next.BuildDefinitionMode), buildtemplate.DefinitionModeRepository),
		BuildTemplateID:     next.BuildTemplateID,
		DockerfilePath:      next.DockerfilePath,
		BuildContext:        next.BuildContext,
		BuildDirectory:      next.BuildDirectory,
		TargetRegistryID:    next.TargetRegistryID,
		TargetRepository:    next.TargetRepository,
		TargetTag:           next.TargetTag,
		BuildVariableSetIDs: varSetIDs,
		AutoDeploy:          next.AutoDeploy,
		RequireApproval:     next.RequireApproval,
		BuildHooksEnabled:   next.BuildHooksEnabled,
		BuildHookBindings:   finalHooks,
		RequiresRelease:     plan.requiresRelease,
	}, nil
}
