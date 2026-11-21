package registryapi

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"gorm.io/gorm"
)

// RegisterAITools registers all 7 container registry & image AI facade tools (build_*).
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerRegistryReadAITools(reg)
	h.registerRegistryWriteAITools(reg)
}

type registryListInput struct {
	Project            *string `json:"project,omitempty" desc:"可选，项目 ID 或 identifier；传入时返回 defaultRegistryId，配合 application 预览默认镜像命名模板"`
	Registry           *string `json:"registry,omitempty" desc:"可选，按制品库 ID（reg_xxx）或精确名称过滤（纯数据库过滤，不触发网络探测）"`
	Credential         *string `json:"credential,omitempty" desc:"可选，按凭据 ID（regc_xxx）或名称过滤嵌套凭据列表 items[].credentials"`
	Application        *string `json:"application,omitempty" desc:"可选，配合 project 预览默认目标镜像命名模板"`
	Stage              *string `json:"stage,omitempty" enum:"dev|test|staging|prod" desc:"可选，配合 project+application 渲染镜像模板阶段"`
	TestRegistry       *string `json:"testRegistry,omitempty" desc:"可选，传入可见制品库 ID 或名称主动执行连通性探测"`
	Search             *string `json:"search,omitempty" desc:"可选，按名称或 Endpoint 搜索"`
	IncludeCredentials *bool   `json:"includeCredentials,omitempty" default:"true" desc:"是否在每个制品库下返回可见凭据摘要 items[].credentials，默认 true"`
	Page               *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize           *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

type registrySearchImagesInput struct {
	Source      *string `json:"source,omitempty" enum:"recorded_images|remote_tags|remote_repositories" default:"recorded_images" desc:"查询源：默认 recorded_images 查询已登记镜像台账；remote_tags 查询远端 Tag；remote_repositories 搜索远端仓库"`
	Registry    *string `json:"registry,omitempty" desc:"remote_tags/remote_repositories 时必填；recorded_images 时可选按制品库 ID 或名称筛选"`
	Repository  *string `json:"repository,omitempty" desc:"remote_tags 时必填（如 library/nginx）；recorded_images 时可选按镜像仓库路径精确过滤"`
	Project     *string `json:"project,omitempty" desc:"可选，按项目 ID 或 identifier 限制可见范围与台账归属"`
	Application *string `json:"application,omitempty" desc:"可选，recorded_images 模式按应用 ID 或 identifier 筛选"`
	ImageID     *string `json:"imageId,omitempty" desc:"可选，recorded_images 模式按镜像台账 ID（img_xxx）精确单查"`
	Search      *string `json:"search,omitempty" desc:"可选，关键词过滤（remote_tags 模式支持按 Tag 名称子串过滤）"`
	Page        *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize    *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20（remote_repositories 硬上限 20，其余上限 50）"`
}

type registryWithCredentialsOutput struct {
	artifactRegistryOutput
	Credentials []registryCredentialOutput `json:"credentials,omitempty"`
}

func (h *Handler) registerRegistryReadAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[registryListInput, map[string]any]{
		Name:           "build_list_registries",
		Domain:         aitool.DomainBuild,
		Title:          "查看容器制品库与推拉凭据",
		Summary:        "分页列出可见容器制品库、连通状态、项目默认制品库及嵌套推拉凭据摘要（支持按需主动探测单个制品库）",
		Tags:           []string{"registry", "credential", "image", "harbor", "dockerhub"},
		AliasesZH:      []string{"镜像仓库列表", "制品库列表", "镜像凭据"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionRegistryRead)},
		Execute:        h.aiListRegistries,
	})

	aitool.Register(reg, aitool.Spec[registrySearchImagesInput, map[string]any]{
		Name:           "build_search_registry_images",
		Domain:         aitool.DomainBuild,
		Title:          "搜索远端镜像或查询项目镜像台账",
		Summary:        "搜索远端制品库中的仓库/Tag 或分页查询平台已登记的容器镜像台账",
		Tags:           []string{"registry", "image", "tag", "repository"},
		AliasesZH:      []string{"搜索容器镜像", "查询镜像Tag", "已登记镜像"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionRegistryRead), string(authz.ActionImageRead)},
		Execute:        h.aiSearchRegistryImages,
	})
}

func (h *Handler) syncGinUser(tc *aitool.ToolContext) {
	if tc == nil || tc.Gin == nil || tc.User.ID == "" {
		return
	}
	tc.Gin.Set("luna.devops.current_user", tc.User)
}

func (h *Handler) aiResolveOptionalProject(tc *aitool.ToolContext, rawProject *string, action authz.Action) (*model.Project, error) {
	ref := regTrimPtr(rawProject)
	if ref == "" && strings.TrimSpace(tc.ConversationProjectID) != "" {
		ref = strings.TrimSpace(tc.ConversationProjectID)
	}
	if ref == "" {
		return nil, nil
	}
	if _, project, ok := h.authorizeProjectRef(tc.Gin, ref, action); ok {
		return &project, nil
	}
	return nil, tc.AbortedError("project.forbidden", "project is not accessible")
}

func (h *Handler) aiListRegistries(tc *aitool.ToolContext, in registryListInput) (map[string]any, error) {
	h.syncGinUser(tc)
	resolvedProject, err := h.aiResolveOptionalProject(tc, in.Project, authz.ActionRegistryRead)
	if err != nil {
		return nil, err
	}
	effectiveProjectID := ""
	if resolvedProject != nil {
		effectiveProjectID = resolvedProject.ID
	}

	visibility := projectservice.ListVisibilityRelated
	if effectiveProjectID == "" && authz.IsPlatformAdmin(tc.User.Role) {
		visibility = projectservice.ListVisibilityAll
	}

	query := tc.DB.Model(&model.ArtifactRegistry{})
	var visible bool
	query, visible = h.applyScopedResourceListVisibility(tc.Gin, query, scopedResourceArtifactRegistry, tc.User, effectiveProjectID, visibility)
	if !visible {
		return nil, tc.AbortedError("registry.forbidden", "cannot list artifact registries")
	}

	regFilter := regTrimPtr(in.Registry)
	if regFilter != "" {
		query = query.Where("id = ? OR lower(name) = lower(?)", regFilter, regFilter)
	}
	if search := regTrimPtr(in.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where("name ILIKE ? OR endpoint ILIKE ?", like, like)
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var registries []model.ArtifactRegistry
	if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&registries).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if regFilter != "" && len(registries) > 1 {
		for _, r := range registries {
			if r.ID == regFilter {
				registries = []model.ArtifactRegistry{r}
				total = 1
				break
			}
		}
	}

	// Pure DB filter when `registry` is specified; live Ping otherwise.
	if regFilter == "" {
		tc.SetNoStore()
		h.observeArtifactRegistriesForProject(tc.Context(), tc.User, registries, effectiveProjectID)
	}

	baseOutputs, respErr := h.registryResponsesForUser(tc.User, registries, tc.Context())
	if respErr != nil {
		return nil, aitool.NewError(http.StatusForbidden, "registry.forbidden", respErr.Error())
	}

	includeCreds := regBoolPtrDefault(in.IncludeCredentials, true)
	credFilter := regTrimPtr(in.Credential)
	items := make([]registryWithCredentialsOutput, 0, len(baseOutputs))
	for _, base := range baseOutputs {
		item := registryWithCredentialsOutput{artifactRegistryOutput: base}
		if includeCreds {
			credQuery := tc.DB.Model(&model.RegistryCredential{}).Where("registry_id = ?", base.ID)
			if effectiveProjectID != "" {
				credQuery = h.applyScopedResourceVisibilityForProject(credQuery, scopedResourceRegistryCredential, tc.User, effectiveProjectID, tc.Context())
			} else {
				var ok bool
				credQuery, ok = h.applyScopedResourceListVisibility(tc.Gin, credQuery, scopedResourceRegistryCredential, tc.User, "", visibility)
				if !ok {
					return nil, tc.AbortedError("registry.credential_forbidden", "cannot list registry credentials")
				}
			}
			if credFilter != "" {
				credQuery = credQuery.Where("id = ? OR lower(name) = lower(?)", credFilter, credFilter)
			}
			var creds []model.RegistryCredential
			if err := credQuery.Order("created_at desc").Find(&creds).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if credFilter != "" && len(creds) > 1 {
				for _, c := range creds {
					if c.ID == credFilter {
						creds = []model.RegistryCredential{c}
						break
					}
				}
			}
			h.attachRegistryCredentialProjects(creds, tc.Context())
			credOutputs, credErr := h.credentialResponsesForUser(tc.User, creds, tc.Context())
			if credErr != nil {
				return nil, aitool.NewError(http.StatusServiceUnavailable, "project.authorization_unavailable", credErr.Error())
			}
			item.Credentials = credOutputs
		}
		items = append(items, item)
	}

	out := aitool.PageEnvelope(items, page, pageSize, "createdAt", "desc", total)
	if resolvedProject != nil {
		out["projectId"] = resolvedProject.ID
		if defReg, hasDef := h.defaultVisibleRegistryForProject(tc.Context(), tc.User, resolvedProject.ID); hasDef {
			out["defaultRegistryId"] = defReg.ID
		}
		if appRef := regTrimPtr(in.Application); appRef != "" {
			app, appErr := aitool.ResolveApplication(tc.DB, resolvedProject.ID, appRef)
			if appErr != nil {
				return nil, appErr
			}
			out["applicationId"] = app.ID
			var templateReg model.ArtifactRegistry
			var hasTemplateReg bool
			if len(registries) == 1 {
				templateReg = registries[0]
				hasTemplateReg = true
			} else if defReg, ok := h.defaultVisibleRegistryForProject(tc.Context(), tc.User, resolvedProject.ID); ok {
				templateReg = defReg
				hasTemplateReg = true
			}
			if hasTemplateReg {
				target := model.DeploymentTarget{Stage: normalizeStage(regTrimPtr(in.Stage))}
				repository := repositoryWithoutRegistryHost(templateReg, buildTargetImageRepository(templateReg, *resolvedProject, app))
				tag := "latest"
				if cred, ok := h.registryPushCredentialForProject(tc.User, templateReg, resolvedProject.ID, tc.Context()); ok {
					templatedRepo, _ := splitTargetImageRef(buildTargetImageRepositoryForCredential(templateReg, cred, *resolvedProject, app, target))
					repository = repositoryWithoutRegistryHost(templateReg, templatedRepo)
					tag = buildStaticTargetImageTagForCredential(templateReg, cred, *resolvedProject, app, target)
				}
				repository = strings.Trim(strings.TrimSpace(repository), "/")
				tag = fallback(strings.TrimSpace(tag), "latest")
				out["imageTemplateDefault"] = registryImageTemplateDefaultOutput{
					TargetImageRef:   repository + ":" + tag,
					TargetRepository: repository,
					TargetTag:        tag,
				}
			}
		}
	}

	if testRef := regTrimPtr(in.TestRegistry); testRef != "" {
		tc.SetNoStore()
		targetReg, testErr := h.aiResolveUsableRegistry(tc, testRef, effectiveProjectID)
		if testErr != nil {
			return nil, testErr
		}
		pingRes := h.pingRegistryForProject(tc.Context(), tc.User, targetReg, effectiveProjectID)
		canInspect, inspectErr := h.canInspectScopedResourceConfigByID(tc.User, targetReg.Scope, targetReg.OwnerRef, scopedResourceArtifactRegistry, targetReg.ID, tc.Context())
		if inspectErr != nil || !canInspect {
			pingRes.Endpoint = ""
		}
		out["testResult"] = map[string]any{
			"registryId": targetReg.ID,
			"success":    pingRes.Success,
			"statusCode": pingRes.StatusCode,
			"message":    pingRes.Message,
			"endpoint":   pingRes.Endpoint,
		}
	}

	return out, nil
}

func (h *Handler) aiSearchRegistryImages(tc *aitool.ToolContext, in registrySearchImagesInput) (map[string]any, error) {
	h.syncGinUser(tc)
	source := strings.ToLower(fallback(regTrimPtr(in.Source), "recorded_images"))
	resolvedProject, err := h.aiResolveOptionalProject(tc, in.Project, authz.ActionImageRead)
	if err != nil {
		return nil, err
	}
	effectiveProjectID := ""
	if resolvedProject != nil {
		effectiveProjectID = resolvedProject.ID
	}

	switch source {
	case "recorded_images":
		page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
		query := tc.DB.Model(&model.ContainerImage{})
		if effectiveProjectID != "" {
			query = query.Where("project_id = ?", effectiveProjectID)
		} else if authz.IsPlatformAdmin(tc.User.Role) {
			query = query.Where("created_by = ? OR project_id <> ''", tc.User.ID)
		} else if projectIDs := h.projectIDsForUser(tc.Context(), tc.User.ID); len(projectIDs) > 0 {
			query = query.Where("((project_id = '' AND created_by = ?) OR project_id IN ?)", tc.User.ID, projectIDs)
		} else {
			query = query.Where("project_id = '' AND created_by = ?", tc.User.ID)
		}

		if regRef := regTrimPtr(in.Registry); regRef != "" {
			reg, regErr := h.aiResolveUsableRegistry(tc, regRef, effectiveProjectID)
			if regErr != nil {
				return nil, regErr
			}
			query = query.Where("registry_id = ?", reg.ID)
		}
		if appRef := regTrimPtr(in.Application); appRef != "" {
			if effectiveProjectID == "" {
				return nil, aitool.BadRequest("project.required", "project is required when filtering recorded_images by application")
			}
			app, appErr := aitool.ResolveApplication(tc.DB, effectiveProjectID, appRef)
			if appErr != nil {
				return nil, appErr
			}
			query = query.Where("application_id = ?", app.ID)
		}
		if repoFilter := strings.Trim(regTrimPtr(in.Repository), "/"); repoFilter != "" {
			query = query.Where("repository = ?", repoFilter)
		}
		if imageID := regTrimPtr(in.ImageID); imageID != "" {
			query = query.Where("id = ?", imageID)
		}
		if search := regTrimPtr(in.Search); search != "" {
			like := "%" + search + "%"
			query = query.Where("image_ref ILIKE ? OR repository ILIKE ? OR tag ILIKE ? OR digest ILIKE ? OR source_commit ILIKE ?",
				like, like, like, like, like)
		}

		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var images []model.ContainerImage
		if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&images).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		out := aitool.PageEnvelope(images, page, pageSize, "createdAt", "desc", total)
		out["source"] = "recorded_images"
		if effectiveProjectID != "" {
			out["projectId"] = effectiveProjectID
		}
		return out, nil

	case "remote_repositories":
		tc.SetNoStore()
		regRef := regTrimPtr(in.Registry)
		if regRef == "" {
			return nil, aitool.BadRequest("argument_invalid", "registry is required when source=remote_repositories")
		}
		reg, regErr := h.aiResolveUsableRegistry(tc, regRef, effectiveProjectID)
		if regErr != nil {
			return nil, regErr
		}
		if !h.allowRegistrySearch(tc.Gin, tc.User.ID) {
			return nil, tc.AbortedError("registry.rate_limited", "registry search rate limit exceeded")
		}
		page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 20)
		cred := h.registryCredentialInputForProject(tc.Context(), tc.User, reg, effectiveProjectID)
		res, searchErr := registryprovider.SearchRepositories(
			tc.Context(), reg.Provider, reg.Endpoint, "", regTrimPtr(in.Search), page, pageSize, h.egressPolicyForRegistry(tc.User, reg, tc.Context()), cred,
		)
		if searchErr != nil {
			return nil, registryUpstreamToolError(searchErr)
		}
		out := map[string]any{
			"source":     "remote_repositories",
			"registryId": reg.ID,
			"items":      res.Items,
			"page":       page,
			"pageSize":   pageSize,
			"total":      res.Total,
			"limited":    res.Limited,
		}
		if effectiveProjectID != "" {
			out["projectId"] = effectiveProjectID
		}
		return out, nil

	case "remote_tags":
		tc.SetNoStore()
		regRef := regTrimPtr(in.Registry)
		repo := strings.Trim(regTrimPtr(in.Repository), "/")
		if regRef == "" || repo == "" {
			return nil, aitool.BadRequest("argument_invalid", "registry and repository are required when source=remote_tags")
		}
		reg, regErr := h.aiResolveUsableRegistry(tc, regRef, effectiveProjectID)
		if regErr != nil {
			return nil, regErr
		}
		if !h.allowRegistrySearch(tc.Gin, tc.User.ID) {
			return nil, tc.AbortedError("registry.rate_limited", "registry search rate limit exceeded")
		}
		_, limit := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
		search := strings.ToLower(regTrimPtr(in.Search))
		fetchLimit := limit
		if search != "" {
			fetchLimit = 50
		}
		cred := h.registryCredentialInputForProject(tc.Context(), tc.User, reg, effectiveProjectID)
		res, tagErr := registryprovider.ListTags(
			tc.Context(), reg.Provider, reg.Endpoint, repo, fetchLimit, h.egressPolicyForRegistry(tc.User, reg, tc.Context()), cred,
		)
		if tagErr != nil {
			return nil, registryUpstreamToolError(tagErr)
		}
		filtered := res.Items
		limited := res.Limited
		total := res.Total
		if search != "" {
			filtered = make([]registryprovider.TagItem, 0, len(res.Items))
			for _, item := range res.Items {
				if strings.Contains(strings.ToLower(item.Name), search) {
					filtered = append(filtered, item)
				}
			}
			total = len(filtered)
			if len(filtered) > limit {
				filtered = filtered[:limit]
				limited = true
			}
		}
		if filtered == nil {
			filtered = []registryprovider.TagItem{}
		}
		out := map[string]any{
			"source":     "remote_tags",
			"registryId": reg.ID,
			"repository": repo,
			"items":      filtered,
			"total":      total,
			"limited":    limited,
		}
		if effectiveProjectID != "" {
			out["projectId"] = effectiveProjectID
		}
		return out, nil

	default:
		return nil, aitool.BadRequest("argument_invalid", "invalid source; expected recorded_images, remote_tags, or remote_repositories")
	}
}

// --- Shared Registry AI Helpers ---

func (h *Handler) observeArtifactRegistriesForProject(parent context.Context, user model.User, registries []model.ArtifactRegistry, projectID string) {
	var wg sync.WaitGroup
	limit := make(chan struct{}, 6)
	for i := range registries {
		wg.Add(1)
		go func(reg *model.ArtifactRegistry) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			h.observeArtifactRegistryForProject(parent, user, reg, projectID)
		}(&registries[i])
	}
	wg.Wait()
}

func (h *Handler) observeArtifactRegistryForProject(parent context.Context, user model.User, registry *model.ArtifactRegistry, projectID string) {
	observedAt := time.Now().UTC()
	registry.LastCheckedAt = &observedAt
	if registry.Endpoint == "" {
		registry.Status = observation.StatusNotConfigured
		registry.ObservationCode = "registry.endpoint_missing"
		return
	}
	ctx, cancel := context.WithTimeout(parent, registryObservationTimeout)
	defer cancel()
	result := h.pingRegistryForProject(ctx, user, *registry, projectID)
	if result.Success {
		registry.Status = observation.StatusReady
		registry.ObservationCode = ""
		return
	}
	if ctx.Err() != nil || result.StatusCode == 0 {
		registry.Status = observation.StatusUnavailable
		registry.ObservationCode = "registry.upstream_unavailable"
		return
	}
	registry.Status = observation.StatusDegraded
	registry.ObservationCode = "registry.health_check_failed"
}

func (h *Handler) pingRegistryForProject(parent context.Context, user model.User, registry model.ArtifactRegistry, projectID string) registryTestResult {
	cred := h.registryCredentialInputForProject(parent, user, registry, projectID)
	res := registryprovider.Ping(parent, registry.Endpoint, h.egressPolicyForRegistry(user, registry, parent), cred)
	return registryTestResult{
		Success:    res.Success,
		StatusCode: res.StatusCode,
		Message:    res.Message,
		Endpoint:   res.Endpoint,
	}
}

func (h *Handler) registryCredentialInputForProject(ctx context.Context, user model.User, registry model.ArtifactRegistry, projectID string) registryprovider.Credential {
	if strings.TrimSpace(projectID) == "" {
		return h.registryCredentialInput(ctx, user, registry)
	}
	visibleCreds := func() *gorm.DB {
		return h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.RegistryCredential{}), scopedResourceRegistryCredential, user, projectID, ctx)
	}
	var cred model.RegistryCredential
	found := false
	if registry.CredentialRef != "" {
		if err := visibleCreds().First(&cred, "id = ? AND registry_id = ?", registry.CredentialRef, registry.ID).Error; err == nil {
			found = true
		}
	}
	if !found {
		if err := visibleCreds().Where("registry_id = ?", registry.ID).
			Order("CASE scope WHEN 'user' THEN 0 WHEN 'project' THEN 1 ELSE 2 END, created_at desc").
			First(&cred).Error; err == nil {
			found = true
		}
	}
	if !found {
		return registryprovider.Credential{}
	}
	sec := h.resolveSecret(ctx, cred.TokenRef)
	if sec == "" {
		sec = h.resolveSecret(ctx, cred.PasswordRef)
	}
	return registryprovider.Credential{Username: cred.Username, Secret: sec}
}

func (h *Handler) registryUsableInProject(db *gorm.DB, user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) bool {
	if projectID == "" {
		return h.canUseScopedResourceByID(user, registry.Scope, registry.OwnerRef, scopedResourceArtifactRegistry, registry.ID, ctx)
	}
	switch strings.ToLower(strings.TrimSpace(registry.Scope)) {
	case "", "global":
		return true
	case "user":
		return strings.TrimSpace(registry.OwnerRef) == user.ID
	case "project":
		var count int64
		_ = db.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? AND resource_id = ? AND project_id = ?", scopedResourceArtifactRegistry, registry.ID, projectID).
			Count(&count).Error
		return count > 0
	default:
		return false
	}
}

func (h *Handler) aiResolveUsableRegistry(tc *aitool.ToolContext, ref string, projectID string) (model.ArtifactRegistry, error) {
	var all []model.ArtifactRegistry
	if err := tc.DB.Order("created_at asc").Find(&all).Error; err != nil {
		return model.ArtifactRegistry{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	usable := make([]model.ArtifactRegistry, 0, len(all))
	for _, r := range all {
		if h.registryUsableInProject(tc.DB, tc.User, r, projectID, tc.Context()) {
			usable = append(usable, r)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		usable,
		"artifact_registry",
		"registry.not_found",
		ref,
		func(r model.ArtifactRegistry) string { return r.ID },
		func(r model.ArtifactRegistry) string { return r.Name },
	)
	if err != nil {
		return model.ArtifactRegistry{}, err
	}
	resolved.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceArtifactRegistry, resolved.ID, tc.Context())
	resolved.DefaultProjectIDs = h.scopedResourceDefaultProjectIDMap(scopedResourceArtifactRegistry, []string{resolved.ID}, tc.Context())[resolved.ID]
	return resolved, nil
}

func (h *Handler) defaultVisibleRegistryForProject(ctx context.Context, user model.User, projectID string) (model.ArtifactRegistry, bool) {
	reg, ok := h.defaultRegistryFor(user.ID, projectID, ctx)
	if !ok {
		return model.ArtifactRegistry{}, false
	}
	if !h.registryUsableInProject(h.dbWithContext(ctx), user, reg, projectID, ctx) {
		return model.ArtifactRegistry{}, false
	}
	return reg, true
}

func registryUpstreamToolError(err error) *aitool.ToolError {
	if up, ok := registryprovider.AsUpstreamError(err); ok && (up.StatusCode == http.StatusUnauthorized || up.StatusCode == http.StatusForbidden) {
		return aitool.Forbidden("registry.authentication_required", "registry authentication is required or credential was rejected")
	}
	return aitool.NewError(http.StatusBadGateway, "registry.upstream_failed", "registry upstream request failed")
}

func enforceRegistryConversationSandbox(conversationProjectID, currentUserID, scope, ownerRef string, projectIDs []string) error {
	convProj := strings.TrimSpace(conversationProjectID)
	if convProj == "" {
		return nil
	}
	normScope := strings.ToLower(strings.TrimSpace(scope))
	if normScope == "user" && strings.TrimSpace(ownerRef) == currentUserID {
		return nil
	}
	cleaned := sortedProjectIDs(projectIDs)
	if normScope == "project" && len(cleaned) == 1 && cleaned[0] == convProj {
		return nil
	}
	err := aitool.Forbidden("ai.project_scope_mismatch", "single-project conversation may only mutate personal (scope=user) or single-project resources bound to the current project")
	err.Hint = "Switch to an unbound global conversation to manage global or multi-project shared resources."
	return err
}

func regResolveProjectRefs(db *gorm.DB, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(refs))
	for _, ref := range refs {
		proj, err := aitool.ResolveProject(db, ref)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(resolved, proj.ID) {
			resolved = append(resolved, proj.ID)
		}
	}
	return sortedProjectIDs(resolved), nil
}

func regMergeProjectIDs(existing, addIDs, removeIDs []string) []string {
	removeSet := make(map[string]bool, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[strings.TrimSpace(id)] = true
	}
	merged := make([]string, 0, len(existing)+len(addIDs))
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !removeSet[trimmed] && !slices.Contains(merged, trimmed) {
			merged = append(merged, trimmed)
		}
	}
	for _, id := range addIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !removeSet[trimmed] && !slices.Contains(merged, trimmed) {
			merged = append(merged, trimmed)
		}
	}
	return sortedProjectIDs(merged)
}

func regTrimPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func regBoolPtrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}
