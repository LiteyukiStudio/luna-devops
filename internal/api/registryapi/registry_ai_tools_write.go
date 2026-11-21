package registryapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type registrySaveInput struct {
	Registry       *string  `json:"registry,omitempty" desc:"可选，省略为新建；传入制品库 ID（reg_xxx）或可管理范围内唯一名称为增量更新"`
	Name           *string  `json:"name,omitempty" desc:"新建必填；制品库显示名称"`
	Provider       *string  `json:"provider,omitempty" enum:"harbor|dockerhub|gitea-registry|generic-oci" desc:"可选，制品库类型，新建默认 harbor"`
	Endpoint       *string  `json:"endpoint,omitempty" desc:"新建必填；制品库根地址如 https://registry.example.com（禁止内嵌用户名密码）"`
	Scope          *string  `json:"scope,omitempty" enum:"user|project|global" desc:"可选，新建传非空 addProjects 默认 project 否则默认 user；更新省略时严格保留原 scope"`
	AddProjects    []string `json:"addProjects,omitempty" desc:"可选，scope=project 时增量授权项目 ID 或 identifier"`
	RemoveProjects []string `json:"removeProjects,omitempty" desc:"可选，scope=project 时增量移除项目 ID 或 identifier"`
	IsDefault      *bool    `json:"isDefault,omitempty" desc:"可选，在对应 user/project/global 作用域内设为默认制品库"`
}

type registryDeleteInput struct {
	Registry string `json:"registry" desc:"必填，待删除的制品库 ID（reg_xxx）或可管理范围内唯一名称"`
}

type registrySaveCredentialInput struct {
	Registry                string   `json:"registry" desc:"必填，所属制品库 ID（reg_xxx）或唯一名称"`
	Credential              *string  `json:"credential,omitempty" desc:"可选，省略为新建；传入凭据 ID（regc_xxx）或唯一名称为增量更新"`
	Name                    *string  `json:"name,omitempty" desc:"新建必填；凭据名称"`
	Username                *string  `json:"username,omitempty" desc:"可选，登录用户名（清空传 clearUsername=true）"`
	ClearUsername           *bool    `json:"clearUsername,omitempty" desc:"可选，显式清空用户名"`
	Password                *string  `json:"password,omitempty" sensitive:"true" desc:"可选，登录密码（更新省略时保留原密码）"`
	Token                   *string  `json:"token,omitempty" sensitive:"true" desc:"可选，访问令牌（更新省略时保留原令牌）"`
	ClearPassword           *bool    `json:"clearPassword,omitempty" desc:"可选，显式清除密码（清除后密码与令牌至少须保留一项非空）"`
	ClearToken              *bool    `json:"clearToken,omitempty" desc:"可选，显式清除令牌"`
	Usage                   *string  `json:"usage,omitempty" enum:"push-pull|push|pull" desc:"可选，凭据用途，新建默认 push-pull"`
	Scope                   *string  `json:"scope,omitempty" enum:"user|project|global" desc:"可选，新建传非空 addProjects 默认 project 否则默认 user；更新省略时严格保留原 scope"`
	AddProjects             []string `json:"addProjects,omitempty" desc:"可选，scope=project 时增量授权项目"`
	RemoveProjects          []string `json:"removeProjects,omitempty" desc:"可选，scope=project 时增量移除项目"`
	RepositoryTemplate      *string  `json:"repositoryTemplate,omitempty" desc:"可选，自定义镜像仓库路径模板（重置默认传 resetRepositoryTemplate=true）"`
	ResetRepositoryTemplate *bool    `json:"resetRepositoryTemplate,omitempty" desc:"可选，重置镜像仓库路径模板为默认"`
	TagTemplate             *string  `json:"tagTemplate,omitempty" desc:"可选，自定义镜像 Tag 模板（重置默认传 resetTagTemplate=true）"`
	ResetTagTemplate        *bool    `json:"resetTagTemplate,omitempty" desc:"可选，重置镜像 Tag 模板为默认"`
}

type registryDeleteCredentialInput struct {
	Registry   string `json:"registry" desc:"必填，所属制品库 ID（reg_xxx）或唯一名称"`
	Credential string `json:"credential" desc:"必填，待删除的凭据 ID（regc_xxx）或唯一名称"`
}

type registryRegisterImageInput struct {
	Project      string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application  *string `json:"application,omitempty" desc:"可选，关联应用 ID、identifier 或唯一名称"`
	Registry     string  `json:"registry" desc:"必填，制品库 ID（reg_xxx）或唯一名称"`
	Repository   string  `json:"repository" desc:"必填，镜像仓库路径（如 team/api）"`
	Tag          *string `json:"tag,omitempty" default:"latest" desc:"可选，镜像 Tag，默认 latest"`
	Digest       *string `json:"digest,omitempty" desc:"可选，镜像摘要（如 sha256:...）"`
	SourceCommit *string `json:"sourceCommit,omitempty" desc:"可选，关联源码 Commit SHA"`
}

func (h *Handler) registerRegistryWriteAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[registrySaveInput, map[string]any]{
		Name:           "build_save_registry",
		Domain:         aitool.DomainBuild,
		Title:          "创建或增量修改容器制品库",
		Summary:        "创建或纯增量修改容器制品库配置，保留未传作用域与项目默认标记",
		Tags:           []string{"registry", "harbor", "dockerhub", "oci"},
		AliasesZH:      []string{"新增镜像仓库", "修改制品库", "配置Harbor"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionRegistryWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_registries",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "registry",
			Collection:     true,
		}},
		Preview: h.aiPreviewSaveRegistry,
		Execute: h.aiExecuteSaveRegistry,
	})

	aitool.Register(reg, aitool.Spec[registryDeleteInput, map[string]any]{
		Name:           "build_delete_registry",
		Domain:         aitool.DomainBuild,
		Title:          "删除容器制品库",
		Summary:        "删除未被部署目标引用的容器制品库及其下属凭据",
		Tags:           []string{"registry", "delete"},
		AliasesZH:      []string{"删除镜像仓库", "移除制品库"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionRegistryWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_registries",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "registry",
			Collection:     true,
		}},
		Preview: h.aiPreviewDeleteRegistry,
		Execute: h.aiExecuteDeleteRegistry,
	})

	aitool.Register(reg, aitool.Spec[registrySaveCredentialInput, map[string]any]{
		Name:           "build_save_registry_credential",
		Domain:         aitool.DomainBuild,
		Title:          "创建或增量修改制品库推拉凭据",
		Summary:        "创建或纯增量修改容器制品库推拉凭据，省略密码/Token 时完整保留原密文引用",
		Tags:           []string{"registry", "credential", "secret", "push", "pull"},
		AliasesZH:      []string{"配置镜像凭据", "保存Registry密码"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionRegistryWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_registries",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "credential",
			Collection:     true,
			CollectionPath: "items[].credentials",
		}},
		Preview: h.aiPreviewSaveRegistryCredential,
		Execute: h.aiExecuteSaveRegistryCredential,
	})

	aitool.Register(reg, aitool.Spec[registryDeleteCredentialInput, map[string]any]{
		Name:           "build_delete_registry_credential",
		Domain:         aitool.DomainBuild,
		Title:          "删除制品库推拉凭据",
		Summary:        "删除未被代码源部署目标依赖的容器制品库推拉凭据",
		Tags:           []string{"registry", "credential", "delete"},
		AliasesZH:      []string{"删除镜像凭据", "移除Registry密码"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionRegistryWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_registries",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "credential",
			Collection:     true,
			CollectionPath: "items[].credentials",
		}},
		Preview: h.aiPreviewDeleteRegistryCredential,
		Execute: h.aiExecuteDeleteRegistryCredential,
	})

	aitool.Register(reg, aitool.Spec[registryRegisterImageInput, model.ContainerImage]{
		Name:           "build_register_container_image",
		Domain:         aitool.DomainBuild,
		Title:          "登记外部容器镜像到项目台账",
		Summary:        "手动登记外部已构建容器镜像到项目镜像台账（仅写静态台账，不改动线上工作负载）",
		Tags:           []string{"registry", "image", "register"},
		AliasesZH:      []string{"登记镜像", "录入外部镜像"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionImageWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_search_registry_images",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "imageId",
			Collection:     true,
		}},
		Execute: h.aiExecuteRegisterContainerImage,
	})
}

func (h *Handler) canManageRegistrySilent(tc *aitool.ToolContext, reg model.ArtifactRegistry) bool {
	child := aitool.NewToolContext(tc.ParentGin, tc.DB, tc.User, tc.SessionID, tc.ConversationID, tc.ConversationProjectID, tc.RunID, tc.ToolCallID, tc.IdempotencyKey, tc.InputMode, nil)
	h.syncGinUser(child)
	return h.canManageScopedResourceByID(child.Gin, tc.User, reg.Scope, reg.OwnerRef, scopedResourceArtifactRegistry, reg.ID, "forbidden")
}

func (h *Handler) canManageCredentialSilent(tc *aitool.ToolContext, cred model.RegistryCredential) bool {
	child := aitool.NewToolContext(tc.ParentGin, tc.DB, tc.User, tc.SessionID, tc.ConversationID, tc.ConversationProjectID, tc.RunID, tc.ToolCallID, tc.IdempotencyKey, tc.InputMode, nil)
	h.syncGinUser(child)
	return h.canManageScopedResourceByID(child.Gin, tc.User, cred.Scope, cred.OwnerRef, scopedResourceRegistryCredential, cred.ID, "forbidden")
}

func (h *Handler) aiResolveManageableRegistry(tc *aitool.ToolContext, ref string) (model.ArtifactRegistry, error) {
	var all []model.ArtifactRegistry
	if err := tc.DB.Order("created_at asc").Find(&all).Error; err != nil {
		return model.ArtifactRegistry{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	manageable := make([]model.ArtifactRegistry, 0, len(all))
	for _, r := range all {
		if h.canManageRegistrySilent(tc, r) {
			manageable = append(manageable, r)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		manageable,
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

func validateCleanRegistryEndpoint(raw string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(raw), "/")
	if endpoint == "" {
		return "", aitool.BadRequest("argument_invalid", "endpoint is required")
	}
	parsed, parseErr := url.Parse(endpoint)
	if parseErr == nil && parsed.User != nil {
		return "", aitool.BadRequest("registry.endpoint_credentials_forbidden", "endpoint must not embed username or password credentials")
	}
	if _, err := registryprovider.ParseEndpoint(endpoint); err != nil {
		return "", aitool.BadRequest("registry.endpoint_invalid", "endpoint must be a valid http(s) registry URL")
	}
	// Re-parse with scheme if ParseEndpoint allows bare host or requires http(s).
	if strings.Contains(endpoint, "@") {
		return "", aitool.BadRequest("registry.endpoint_credentials_forbidden", "endpoint must not embed credentials")
	}
	return endpoint, nil
}

// --- 15. build_save_registry ---

type preparedRegistrySave struct {
	isCreate          bool
	existing          model.ArtifactRegistry
	target            model.ArtifactRegistry
	explicitIsDefault *bool
	defaultProjectIDs []string
}

func (h *Handler) prepareSaveRegistry(tc *aitool.ToolContext, in registrySaveInput) (preparedRegistrySave, error) {
	h.syncGinUser(tc)
	var prep preparedRegistrySave

	addIDs, err := regResolveProjectRefs(tc.DB, in.AddProjects)
	if err != nil {
		return prep, err
	}
	removeIDs, err := regResolveProjectRefs(tc.DB, in.RemoveProjects)
	if err != nil {
		return prep, err
	}

	regRef := regTrimPtr(in.Registry)
	if regRef == "" {
		prep.isCreate = true
		name := regTrimPtr(in.Name)
		if name == "" {
			return prep, aitool.BadRequest("argument_invalid", "name is required when creating an ArtifactRegistry")
		}
		endpoint, endErr := validateCleanRegistryEndpoint(regTrimPtr(in.Endpoint))
		if endErr != nil {
			return prep, endErr
		}
		rawScope := regTrimPtr(in.Scope)
		if rawScope == "" {
			if len(addIDs) > 0 {
				rawScope = "project"
			} else {
				rawScope = "user"
			}
		}
		projectIDs := regMergeProjectIDs(nil, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeScopedOwnerWithProjects(
			tc.Gin, tc.User, rawScope, "", projectIDs, "platform administrator role is required for global artifact registries",
		)
		if !ok {
			return prep, tc.AbortedError("registry.scope_invalid", "invalid registry scope")
		}
		if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, scope, ownerRef, normalizedProjects); err != nil {
			return prep, err
		}
		isDef := regBoolPtrDefault(in.IsDefault, false)
		var defProjIDs []string
		if scope == "project" && isDef {
			defProjIDs = sortedProjectIDs(normalizedProjects)
		}
		prep.target = model.ArtifactRegistry{
			ID:                id.New("reg"),
			Name:              name,
			Provider:          normalizeRegistryProvider(regTrimPtr(in.Provider)),
			Endpoint:          endpoint,
			Scope:             scope,
			OwnerRef:          ownerRef,
			ProjectIDs:        normalizedProjects,
			DefaultProjectIDs: defProjIDs,
			IsDefault:         isDef && scope != "project",
			CreatedBy:         tc.User.ID,
		}
		prep.explicitIsDefault = in.IsDefault
		prep.defaultProjectIDs = defProjIDs
		return prep, nil
	}

	existing, err := h.aiResolveManageableRegistry(tc, regRef)
	if err != nil {
		return prep, err
	}
	if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, existing.Scope, existing.OwnerRef, existing.ProjectIDs); err != nil {
		return prep, err
	}

	next := existing
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return prep, aitool.BadRequest("argument_invalid", "name cannot be empty")
		}
		next.Name = n
	}
	if in.Provider != nil && strings.TrimSpace(*in.Provider) != "" {
		next.Provider = normalizeRegistryProvider(*in.Provider)
	}
	if in.Endpoint != nil {
		ep, epErr := validateCleanRegistryEndpoint(*in.Endpoint)
		if epErr != nil {
			return prep, epErr
		}
		next.Endpoint = ep
	}

	scopeChanged := in.Scope != nil || len(addIDs) > 0 || len(removeIDs) > 0
	if scopeChanged {
		targetScope := existing.Scope
		if in.Scope != nil && strings.TrimSpace(*in.Scope) != "" {
			targetScope = strings.ToLower(strings.TrimSpace(*in.Scope))
		}
		mergedProjects := regMergeProjectIDs(existing.ProjectIDs, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeScopedOwnerWithProjects(
			tc.Gin, tc.User, targetScope, existing.OwnerRef, mergedProjects, "platform administrator role is required for global artifact registries",
		)
		if !ok {
			return prep, tc.AbortedError("registry.scope_invalid", "invalid registry scope")
		}
		next.Scope = scope
		next.OwnerRef = ownerRef
		next.ProjectIDs = normalizedProjects
	}
	if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, next.Scope, next.OwnerRef, next.ProjectIDs); err != nil {
		return prep, err
	}

	// Preserve DefaultProjectIDs on project-scoped updates when isDefault is omitted (spec line 557).
	var defProjIDs []string
	if next.Scope == "project" {
		next.IsDefault = false
		if in.IsDefault != nil {
			if *in.IsDefault {
				defProjIDs = sortedProjectIDs(next.ProjectIDs)
			} else {
				defProjIDs = []string{}
			}
		} else {
			allowedSet := make(map[string]bool, len(next.ProjectIDs))
			for _, pid := range next.ProjectIDs {
				allowedSet[pid] = true
			}
			for _, pid := range existing.DefaultProjectIDs {
				if allowedSet[pid] && !slices.Contains(defProjIDs, pid) {
					defProjIDs = append(defProjIDs, pid)
				}
			}
			defProjIDs = sortedProjectIDs(defProjIDs)
		}
	} else {
		if in.IsDefault != nil {
			next.IsDefault = *in.IsDefault
		}
	}
	next.DefaultProjectIDs = defProjIDs

	if compatErr := ensureRegistryScopeCompatibleTx(tc.DB, next); compatErr != nil {
		if errors.Is(compatErr, errRegistryCredentialScopeConflict) {
			return prep, aitool.Conflict("registry.credential_scope_conflict", "child registry credentials exceed the target registry scope")
		}
		return prep, aitool.Conflict("registry.in_use", "artifact registry is referenced by deployment targets outside the target scope")
	}

	prep.existing = existing
	prep.target = next
	prep.explicitIsDefault = in.IsDefault
	prep.defaultProjectIDs = defProjIDs
	return prep, nil
}

func (h *Handler) aiPreviewSaveRegistry(tc *aitool.ToolContext, in registrySaveInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareSaveRegistry(tc, in)
	if err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("Create %s registry %q (%s)", prep.target.Scope, prep.target.Name, prep.target.Provider)
	targetID := ""
	var changes []aitool.PreviewChange
	if !prep.isCreate {
		targetID = prep.existing.ID
		summary = fmt.Sprintf("Update registry %q (%s)", prep.target.Name, prep.existing.ID)
		if prep.existing.Name != prep.target.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: prep.existing.Name, After: prep.target.Name, Action: "update"})
		}
		if prep.existing.Endpoint != prep.target.Endpoint {
			changes = append(changes, aitool.PreviewChange{Field: "endpoint", Before: prep.existing.Endpoint, After: prep.target.Endpoint, Action: "update"})
		}
		if prep.existing.Scope != prep.target.Scope {
			changes = append(changes, aitool.PreviewChange{Field: "scope", Before: prep.existing.Scope, After: prep.target.Scope, Action: "update"})
		}
	} else {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: prep.target.Name, Action: "create"},
			aitool.PreviewChange{Field: "provider", After: prep.target.Provider, Action: "create"},
			aitool.PreviewChange{Field: "endpoint", After: prep.target.Endpoint, Action: "create"},
			aitool.PreviewChange{Field: "scope", After: prep.target.Scope, Action: "create"},
		)
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "artifact_registry",
			TargetID:       targetID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes:        changes,
		},
	}, nil
}

func (h *Handler) saveRegistryPreservingDefaultTx(tx *gorm.DB, registry model.ArtifactRegistry, defaultProjectIDs []string) error {
	projectIDs := sortedProjectIDs(registry.ProjectIDs)
	defIDs := sortedProjectIDs(defaultProjectIDs)
	if registry.Scope == "project" {
		if len(defIDs) > 0 {
			if err := tx.Model(&model.ScopedResourceProjectBinding{}).
				Where("resource_type = ? AND project_id IN ? AND resource_id <> ?", scopedResourceArtifactRegistry, defIDs, registry.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		registry.IsDefault = false
	} else {
		defIDs = []string{}
		if registry.IsDefault {
			if err := tx.Model(&model.ArtifactRegistry{}).
				Where("scope = ? AND owner_ref = ? AND id <> ?", registry.Scope, registry.OwnerRef, registry.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Save(&registry).Error; err != nil {
		return err
	}
	return h.replaceScopedResourceProjectBindings(tx, scopedResourceArtifactRegistry, registry.ID, projectIDs, defIDs)
}

func (h *Handler) aiExecuteSaveRegistry(tc *aitool.ToolContext, in registrySaveInput) (map[string]any, error) {
	prep, err := h.prepareSaveRegistry(tc, in)
	if err != nil {
		return nil, err
	}
	if prep.isCreate {
		reg := prep.target
		txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
			return h.saveRegistryPreservingDefaultTx(tx, reg, prep.defaultProjectIDs)
		})
		if txErr != nil {
			return nil, aitool.BadRequest("registry.save_failed", txErr.Error())
		}
		h.auditWithContext(tc.User.ID, "registry.create", reg.ID, true, reg.Scope, tc.Context())
		h.observeArtifactRegistryForProject(tc.Context(), tc.User, &reg, strings.TrimSpace(tc.ConversationProjectID))
		return registryOutputMap(registryResponseWithManage(reg, true), tc.ConversationProjectID), nil
	}

	var updated model.ArtifactRegistry
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", prep.existing.ID).Error; err != nil {
			return err
		}
		locked.Name = prep.target.Name
		locked.Provider = prep.target.Provider
		locked.Endpoint = prep.target.Endpoint
		locked.Scope = prep.target.Scope
		locked.OwnerRef = prep.target.OwnerRef
		locked.ProjectIDs = prep.target.ProjectIDs
		locked.IsDefault = prep.target.IsDefault
		locked.DefaultProjectIDs = prep.defaultProjectIDs
		if compatErr := ensureRegistryScopeCompatibleTx(tx, locked); compatErr != nil {
			return compatErr
		}
		if saveErr := h.saveRegistryPreservingDefaultTx(tx, locked, prep.defaultProjectIDs); saveErr != nil {
			return saveErr
		}
		updated = locked
		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("registry.not_found", "artifact registry not found")
	}
	if errors.Is(txErr, errRegistryCredentialScopeConflict) {
		return nil, aitool.Conflict("registry.credential_scope_conflict", "child registry credentials exceed the target registry scope")
	}
	if errors.Is(txErr, errArtifactRegistryInUse) {
		return nil, aitool.Conflict("registry.in_use", "artifact registry is referenced by deployment targets outside the target scope")
	}
	if txErr != nil {
		return nil, aitool.BadRequest("registry.save_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "registry.update", updated.ID, true, updated.Scope, tc.Context())
	h.observeArtifactRegistryForProject(tc.Context(), tc.User, &updated, strings.TrimSpace(tc.ConversationProjectID))
	return registryOutputMap(registryResponseWithManage(updated, true), tc.ConversationProjectID), nil
}

// --- 16. build_delete_registry ---

func (h *Handler) checkDeleteRegistry(tc *aitool.ToolContext, in registryDeleteInput) (model.ArtifactRegistry, error) {
	h.syncGinUser(tc)
	reg, err := h.aiResolveManageableRegistry(tc, in.Registry)
	if err != nil {
		return model.ArtifactRegistry{}, err
	}
	if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, reg.Scope, reg.OwnerRef, reg.ProjectIDs); err != nil {
		return model.ArtifactRegistry{}, err
	}
	var targetCount int64
	if err := tc.DB.Model(&model.DeploymentTarget{}).
		Joins("JOIN projects ON projects.id = deployment_targets.project_id AND projects.deleted_at IS NULL").
		Where("deployment_targets.target_registry_id = ? AND deployment_targets.deleted_at IS NULL", reg.ID).
		Count(&targetCount).Error; err != nil {
		return model.ArtifactRegistry{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if targetCount > 0 {
		return model.ArtifactRegistry{}, aitool.Conflict("registry.in_use", fmt.Sprintf("artifact registry %s is referenced by %d active deployment target(s)", reg.ID, targetCount))
	}
	return reg, nil
}

func (h *Handler) aiPreviewDeleteRegistry(tc *aitool.ToolContext, in registryDeleteInput) (*aitool.PreviewResult, error) {
	reg, err := h.checkDeleteRegistry(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete artifact registry %q (%s) and its credentials", reg.Name, reg.ID),
			TargetResource: "artifact_registry",
			TargetID:       reg.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "registry", Before: reg.Name, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteDeleteRegistry(tc *aitool.ToolContext, in registryDeleteInput) (map[string]any, error) {
	reg, err := h.checkDeleteRegistry(tc, in)
	if err != nil {
		return nil, err
	}
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", reg.ID).Error; err != nil {
			return err
		}
		var targetCount int64
		if err := tx.Model(&model.DeploymentTarget{}).
			Joins("JOIN projects ON projects.id = deployment_targets.project_id AND projects.deleted_at IS NULL").
			Where("deployment_targets.target_registry_id = ? AND deployment_targets.deleted_at IS NULL", locked.ID).
			Count(&targetCount).Error; err != nil {
			return err
		}
		if targetCount > 0 {
			return errArtifactRegistryInUse
		}
		var credentials []model.RegistryCredential
		if err := tx.Where("registry_id = ?", locked.ID).Find(&credentials).Error; err != nil {
			return err
		}
		credIDs := make([]string, 0, len(credentials))
		for _, cred := range credentials {
			credIDs = append(credIDs, cred.ID)
			if h.secretAvailable() {
				if err := h.deleteSecretRefWithDB(tc.Context(), tx, cred.PasswordRef, "registry_credential:"+cred.RegistryID+":password"); err != nil {
					return err
				}
				if err := h.deleteSecretRefWithDB(tc.Context(), tx, cred.TokenRef, "registry_credential:"+cred.RegistryID+":token"); err != nil {
					return err
				}
			}
		}
		if len(credIDs) > 0 {
			if err := tx.Where("resource_type = ? AND resource_id IN ?", scopedResourceRegistryCredential, credIDs).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("registry_id = ?", locked.ID).Delete(&model.RegistryCredential{}).Error; err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? AND resource_id = ?", scopedResourceArtifactRegistry, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("registry.not_found", "artifact registry not found")
	}
	if errors.Is(txErr, errArtifactRegistryInUse) {
		return nil, aitool.Conflict("registry.in_use", "artifact registry is referenced by active deployment targets")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "registry.delete_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "registry.delete", reg.ID, true, reg.Scope, tc.Context())
	out := map[string]any{
		"id":      reg.ID,
		"deleted": true,
	}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		out["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
	}
	return out, nil
}

// --- 17. build_save_registry_credential ---

type preparedCredentialSave struct {
	isCreate      bool
	registry      model.ArtifactRegistry
	existing      model.RegistryCredential
	target        model.RegistryCredential
	newPassword   string
	newToken      string
	clearPassword bool
	clearToken    bool
}

func (h *Handler) aiResolveManageableCredential(tc *aitool.ToolContext, registryID, credRef string) (model.RegistryCredential, error) {
	var all []model.RegistryCredential
	if err := tc.DB.Where("registry_id = ?", registryID).Order("created_at asc").Find(&all).Error; err != nil {
		return model.RegistryCredential{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	manageable := make([]model.RegistryCredential, 0, len(all))
	for _, c := range all {
		if h.canManageCredentialSilent(tc, c) {
			manageable = append(manageable, c)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		manageable,
		"registry_credential",
		"registry.credential_not_found",
		credRef,
		func(c model.RegistryCredential) string { return c.ID },
		func(c model.RegistryCredential) string { return c.Name },
	)
	if err != nil {
		return model.RegistryCredential{}, err
	}
	resolved.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceRegistryCredential, resolved.ID, tc.Context())
	return resolved, nil
}

func (h *Handler) prepareSaveRegistryCredential(tc *aitool.ToolContext, in registrySaveCredentialInput) (preparedCredentialSave, error) {
	h.syncGinUser(tc)
	var prep preparedCredentialSave

	clearUser := regBoolPtrDefault(in.ClearUsername, false)
	clearPass := regBoolPtrDefault(in.ClearPassword, false)
	clearTok := regBoolPtrDefault(in.ClearToken, false)
	resetRepoTpl := regBoolPtrDefault(in.ResetRepositoryTemplate, false)
	resetTagTpl := regBoolPtrDefault(in.ResetTagTemplate, false)

	newPassword := regTrimPtr(in.Password)
	newToken := regTrimPtr(in.Token)

	if clearUser && regTrimPtr(in.Username) != "" {
		return prep, aitool.BadRequest("argument_invalid", "cannot pass username and clearUsername=true simultaneously")
	}
	if clearPass && newPassword != "" {
		return prep, aitool.BadRequest("argument_invalid", "cannot pass password and clearPassword=true simultaneously")
	}
	if clearTok && newToken != "" {
		return prep, aitool.BadRequest("argument_invalid", "cannot pass token and clearToken=true simultaneously")
	}
	if resetRepoTpl && regTrimPtr(in.RepositoryTemplate) != "" {
		return prep, aitool.BadRequest("argument_invalid", "cannot pass repositoryTemplate and resetRepositoryTemplate=true simultaneously")
	}
	if resetTagTpl && regTrimPtr(in.TagTemplate) != "" {
		return prep, aitool.BadRequest("argument_invalid", "cannot pass tagTemplate and resetTagTemplate=true simultaneously")
	}
	if (newPassword != "" || newToken != "") && !h.secretAvailable() {
		return prep, aitool.NewError(http.StatusInternalServerError, "secret.store_unavailable", "SECRET_ENCRYPTION_KEY is required to store registry credentials")
	}

	reg, err := h.aiResolveUsableRegistry(tc, in.Registry, strings.TrimSpace(tc.ConversationProjectID))
	if err != nil {
		return prep, err
	}
	addIDs, err := regResolveProjectRefs(tc.DB, in.AddProjects)
	if err != nil {
		return prep, err
	}
	removeIDs, err := regResolveProjectRefs(tc.DB, in.RemoveProjects)
	if err != nil {
		return prep, err
	}

	credRef := regTrimPtr(in.Credential)
	if credRef == "" {
		prep.isCreate = true
		name := regTrimPtr(in.Name)
		if name == "" {
			return prep, aitool.BadRequest("argument_invalid", "name is required when creating a RegistryCredential")
		}
		if newPassword == "" && newToken == "" {
			return prep, aitool.BadRequest("argument_invalid", "at least one of password or token must be provided when creating a RegistryCredential")
		}
		rawScope := regTrimPtr(in.Scope)
		if rawScope == "" {
			if len(addIDs) > 0 {
				rawScope = "project"
			} else {
				rawScope = "user"
			}
		}
		projectIDs := regMergeProjectIDs(nil, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeCredentialScopeWithinParent(
			tc.Gin, tc.User, rawScope, projectIDs, reg.Scope, reg.ProjectIDs, "platform administrator role is required for global registry credentials",
		)
		if !ok {
			return prep, tc.AbortedError("registry.credential_scope_invalid", "invalid registry credential scope")
		}
		if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, scope, ownerRef, normalizedProjects); err != nil {
			return prep, err
		}
		username := ""
		if !clearUser {
			username = regTrimPtr(in.Username)
		}
		repoTpl := ""
		if !resetRepoTpl {
			repoTpl = normalizeImageRepositoryTemplate(regTrimPtr(in.RepositoryTemplate))
		}
		tagTpl := ""
		if !resetTagTpl {
			tagTpl = normalizeImageTagTemplate(regTrimPtr(in.TagTemplate))
		}
		candidate := model.RegistryCredential{
			ID:                 id.New("regc"),
			RegistryID:         reg.ID,
			Name:               name,
			Username:           username,
			Usage:              normalizeCredentialUsage(regTrimPtr(in.Usage)),
			Scope:              scope,
			OwnerRef:           ownerRef,
			ProjectIDs:         normalizedProjects,
			RepositoryTemplate: repoTpl,
			TagTemplate:        tagTpl,
			CreatedBy:          tc.User.ID,
		}
		if err := ensureCredentialFitsRegistryScopeTx(tc.DB, reg, candidate); err != nil {
			return prep, aitool.Conflict("registry.credential_scope_conflict", "registry credential scope exceeds parent registry scope")
		}
		prep.registry = reg
		prep.target = candidate
		prep.newPassword = newPassword
		prep.newToken = newToken
		return prep, nil
	}

	existing, err := h.aiResolveManageableCredential(tc, reg.ID, credRef)
	if err != nil {
		return prep, err
	}
	if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, existing.Scope, existing.OwnerRef, existing.ProjectIDs); err != nil {
		return prep, err
	}

	next := existing
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return prep, aitool.BadRequest("argument_invalid", "name cannot be empty")
		}
		next.Name = n
	}
	if clearUser {
		next.Username = ""
	} else if in.Username != nil {
		next.Username = strings.TrimSpace(*in.Username)
	}
	if in.Usage != nil && strings.TrimSpace(*in.Usage) != "" {
		next.Usage = normalizeCredentialUsage(*in.Usage)
	}
	if resetRepoTpl {
		next.RepositoryTemplate = ""
	} else if in.RepositoryTemplate != nil {
		next.RepositoryTemplate = normalizeImageRepositoryTemplate(*in.RepositoryTemplate)
	}
	if resetTagTpl {
		next.TagTemplate = ""
	} else if in.TagTemplate != nil {
		next.TagTemplate = normalizeImageTagTemplate(*in.TagTemplate)
	}

	willHavePassword := (secret.HasValue(existing.PasswordRef) && !clearPass) || newPassword != ""
	willHaveToken := (secret.HasValue(existing.TokenRef) && !clearTok) || newToken != ""
	if !willHavePassword && !willHaveToken {
		return prep, aitool.BadRequest("argument_invalid", "at least one of password or token must remain configured on the credential")
	}

	scopeChanged := in.Scope != nil || len(addIDs) > 0 || len(removeIDs) > 0
	if scopeChanged {
		targetScope := existing.Scope
		if in.Scope != nil && strings.TrimSpace(*in.Scope) != "" {
			targetScope = strings.ToLower(strings.TrimSpace(*in.Scope))
		}
		mergedProjects := regMergeProjectIDs(existing.ProjectIDs, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeCredentialScopeWithinParent(
			tc.Gin, tc.User, targetScope, mergedProjects, reg.Scope, reg.ProjectIDs, "platform administrator role is required for global registry credentials",
		)
		if !ok {
			return prep, tc.AbortedError("registry.credential_scope_invalid", "invalid registry credential scope")
		}
		next.Scope = scope
		next.OwnerRef = ownerRef
		next.ProjectIDs = normalizedProjects
	}
	if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, next.Scope, next.OwnerRef, next.ProjectIDs); err != nil {
		return prep, err
	}
	if err := ensureCredentialFitsRegistryScopeTx(tc.DB, reg, next); err != nil {
		return prep, aitool.Conflict("registry.credential_scope_conflict", "registry credential scope exceeds parent registry scope")
	}

	// Dry-run push credential coverage check using rollback transaction so Preview rejects 409 immediately.
	if dryErr := h.dryRunCredentialReconcile(tc.DB, reg, &next, false); dryErr != nil {
		return prep, dryErr
	}

	prep.registry = reg
	prep.existing = existing
	prep.target = next
	prep.newPassword = newPassword
	prep.newToken = newToken
	prep.clearPassword = clearPass
	prep.clearToken = clearTok
	return prep, nil
}

func (h *Handler) dryRunCredentialReconcile(db *gorm.DB, reg model.ArtifactRegistry, candidate *model.RegistryCredential, isDelete bool) error {
	sentinelRollback := errors.New("dry_run_rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		if isDelete {
			if err := tx.Where("resource_type = ? AND resource_id = ?", scopedResourceRegistryCredential, candidate.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&model.RegistryCredential{}, "id = ?", candidate.ID).Error; err != nil {
				return err
			}
			if err := reconcileRegistryPushCredentialsTx(tx, reg, nil); err != nil {
				return err
			}
		} else {
			if err := tx.Save(candidate).Error; err != nil {
				return err
			}
			if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, candidate.ID, sortedProjectIDs(candidate.ProjectIDs), nil); err != nil {
				return err
			}
			if err := reconcileRegistryPushCredentialsTx(tx, reg, candidate); err != nil {
				return err
			}
		}
		return sentinelRollback
	})
	if errors.Is(err, sentinelRollback) {
		return nil
	}
	if errors.Is(err, errRegistryCredentialInUse) {
		return aitool.Conflict("registry.credential_in_use", "registry credential is required as a push credential by active repository deployment targets")
	}
	return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
}

func (h *Handler) aiPreviewSaveRegistryCredential(tc *aitool.ToolContext, in registrySaveCredentialInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareSaveRegistryCredential(tc, in)
	if err != nil {
		return nil, err
	}
	var sensitiveKeys []string
	if prep.newPassword != "" || prep.clearPassword {
		sensitiveKeys = append(sensitiveKeys, "password")
	}
	if prep.newToken != "" || prep.clearToken {
		sensitiveKeys = append(sensitiveKeys, "token")
	}
	summary := fmt.Sprintf("Create %s credential %q on registry %s", prep.target.Scope, prep.target.Name, prep.registry.Name)
	targetID := ""
	var changes []aitool.PreviewChange
	if !prep.isCreate {
		targetID = prep.existing.ID
		summary = fmt.Sprintf("Update credential %q (%s) on registry %s", prep.target.Name, prep.existing.ID, prep.registry.Name)
		if prep.existing.Name != prep.target.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: prep.existing.Name, After: prep.target.Name, Action: "update"})
		}
		if prep.existing.Username != prep.target.Username {
			changes = append(changes, aitool.PreviewChange{Field: "username", Before: prep.existing.Username, After: prep.target.Username, Action: "update"})
		}
		if prep.existing.Usage != prep.target.Usage {
			changes = append(changes, aitool.PreviewChange{Field: "usage", Before: prep.existing.Usage, After: prep.target.Usage, Action: "update"})
		}
	} else {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: prep.target.Name, Action: "create"},
			aitool.PreviewChange{Field: "usage", After: prep.target.Usage, Action: "create"},
			aitool.PreviewChange{Field: "scope", After: prep.target.Scope, Action: "create"},
		)
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "registry_credential",
			TargetID:       targetID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
	}, nil
}

func (h *Handler) aiExecuteSaveRegistryCredential(tc *aitool.ToolContext, in registrySaveCredentialInput) (map[string]any, error) {
	prep, err := h.prepareSaveRegistryCredential(tc, in)
	if err != nil {
		return nil, err
	}
	if prep.isCreate {
		cred := prep.target
		txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
			var lockedReg model.ArtifactRegistry
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedReg, "id = ?", prep.registry.ID).Error; err != nil {
				return err
			}
			if err := ensureCredentialFitsRegistryScopeTx(tx, lockedReg, cred); err != nil {
				return err
			}
			if prep.newPassword != "" {
				ref, storeErr := h.storeSecretWithDB(tc.Context(), tx, prep.newPassword, tc.User.ID, "registry_credential:"+lockedReg.ID+":password")
				if storeErr != nil {
					return storeErr
				}
				cred.PasswordRef = ref
			}
			if prep.newToken != "" {
				ref, storeErr := h.storeSecretWithDB(tc.Context(), tx, prep.newToken, tc.User.ID, "registry_credential:"+lockedReg.ID+":token")
				if storeErr != nil {
					return storeErr
				}
				cred.TokenRef = ref
			}
			if err := tx.Create(&cred).Error; err != nil {
				return err
			}
			if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, cred.ID, sortedProjectIDs(cred.ProjectIDs), nil); err != nil {
				return err
			}
			if lockedReg.CredentialRef == "" && credentialAllowsPush(cred.Usage) {
				return tx.Model(&model.ArtifactRegistry{}).Where("id = ?", lockedReg.ID).Update("credential_ref", cred.ID).Error
			}
			return nil
		})
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("registry.not_found", "artifact registry not found")
		}
		if errors.Is(txErr, errRegistryCredentialScopeConflict) {
			return nil, aitool.Conflict("registry.credential_scope_conflict", "registry credential scope exceeds parent registry scope")
		}
		if txErr != nil {
			return nil, aitool.BadRequest("registry.credential_save_failed", txErr.Error())
		}
		h.auditWithContext(tc.User.ID, "registry_credential.create", cred.ID, true, cred.Scope, tc.Context())
		return credentialOutputMap(credentialResponseWithManage(cred, true), tc.ConversationProjectID), nil
	}

	var updated model.RegistryCredential
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var lockedReg model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedReg, "id = ?", prep.registry.ID).Error; err != nil {
			return err
		}
		var lockedCred model.RegistryCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedCred, "id = ? AND registry_id = ?", prep.existing.ID, lockedReg.ID).Error; err != nil {
			return err
		}
		passRes := "registry_credential:" + lockedReg.ID + ":password"
		passwordRef := lockedCred.PasswordRef
		if prep.clearPassword {
			if strings.TrimSpace(lockedCred.PasswordRef) != "" && h.secretAvailable() {
				if err := h.deleteSecretRefWithDB(tc.Context(), tx, lockedCred.PasswordRef, passRes); err != nil {
					return err
				}
			}
			passwordRef = ""
		} else if prep.newPassword != "" {
			newRef, storeErr := h.storeSecretWithDB(tc.Context(), tx, prep.newPassword, tc.User.ID, passRes)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(lockedCred.PasswordRef) != "" && lockedCred.PasswordRef != newRef {
				if delErr := h.deleteSecretRefWithDB(tc.Context(), tx, lockedCred.PasswordRef, passRes); delErr != nil {
					return delErr
				}
			}
			passwordRef = newRef
		}

		tokRes := "registry_credential:" + lockedReg.ID + ":token"
		tokenRef := lockedCred.TokenRef
		if prep.clearToken {
			if strings.TrimSpace(lockedCred.TokenRef) != "" && h.secretAvailable() {
				if err := h.deleteSecretRefWithDB(tc.Context(), tx, lockedCred.TokenRef, tokRes); err != nil {
					return err
				}
			}
			tokenRef = ""
		} else if prep.newToken != "" {
			newRef, storeErr := h.storeSecretWithDB(tc.Context(), tx, prep.newToken, tc.User.ID, tokRes)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(lockedCred.TokenRef) != "" && lockedCred.TokenRef != newRef {
				if delErr := h.deleteSecretRefWithDB(tc.Context(), tx, lockedCred.TokenRef, tokRes); delErr != nil {
					return delErr
				}
			}
			tokenRef = newRef
		}
		if passwordRef == "" && tokenRef == "" {
			return errRegistryCredentialSecretMissing
		}

		lockedCred.Name = prep.target.Name
		lockedCred.Username = prep.target.Username
		lockedCred.PasswordRef = passwordRef
		lockedCred.TokenRef = tokenRef
		lockedCred.Usage = prep.target.Usage
		lockedCred.Scope = prep.target.Scope
		lockedCred.OwnerRef = prep.target.OwnerRef
		lockedCred.ProjectIDs = prep.target.ProjectIDs
		lockedCred.RepositoryTemplate = prep.target.RepositoryTemplate
		lockedCred.TagTemplate = prep.target.TagTemplate

		if err := ensureCredentialFitsRegistryScopeTx(tx, lockedReg, lockedCred); err != nil {
			return err
		}
		if err := tx.Save(&lockedCred).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, lockedCred.ID, sortedProjectIDs(lockedCred.ProjectIDs), nil); err != nil {
			return err
		}
		if err := reconcileRegistryPushCredentialsTx(tx, lockedReg, &lockedCred); err != nil {
			return err
		}
		updated = lockedCred
		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("registry.credential_not_found", "registry credential not found")
	}
	if errors.Is(txErr, errRegistryCredentialSecretMissing) {
		return nil, aitool.BadRequest("argument_invalid", "at least one of password or token must remain configured")
	}
	if errors.Is(txErr, errRegistryCredentialScopeConflict) {
		return nil, aitool.Conflict("registry.credential_scope_conflict", "registry credential scope exceeds parent registry scope")
	}
	if errors.Is(txErr, errRegistryCredentialInUse) {
		return nil, aitool.Conflict("registry.credential_in_use", "registry credential is required by active repository deployment targets")
	}
	if txErr != nil {
		return nil, aitool.BadRequest("registry.credential_save_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "registry_credential.update", updated.ID, true, updated.Scope, tc.Context())
	return credentialOutputMap(credentialResponseWithManage(updated, true), tc.ConversationProjectID), nil
}

// --- 18. build_delete_registry_credential ---

func (h *Handler) checkDeleteRegistryCredential(tc *aitool.ToolContext, in registryDeleteCredentialInput) (model.ArtifactRegistry, model.RegistryCredential, error) {
	h.syncGinUser(tc)
	reg, err := h.aiResolveUsableRegistry(tc, in.Registry, strings.TrimSpace(tc.ConversationProjectID))
	if err != nil {
		return model.ArtifactRegistry{}, model.RegistryCredential{}, err
	}
	cred, err := h.aiResolveManageableCredential(tc, reg.ID, in.Credential)
	if err != nil {
		return model.ArtifactRegistry{}, model.RegistryCredential{}, err
	}
	if err := enforceRegistryConversationSandbox(tc.ConversationProjectID, tc.User.ID, cred.Scope, cred.OwnerRef, cred.ProjectIDs); err != nil {
		return model.ArtifactRegistry{}, model.RegistryCredential{}, err
	}
	if dryErr := h.dryRunCredentialReconcile(tc.DB, reg, &cred, true); dryErr != nil {
		return model.ArtifactRegistry{}, model.RegistryCredential{}, dryErr
	}
	return reg, cred, nil
}

func (h *Handler) aiPreviewDeleteRegistryCredential(tc *aitool.ToolContext, in registryDeleteCredentialInput) (*aitool.PreviewResult, error) {
	reg, cred, err := h.checkDeleteRegistryCredential(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete credential %q (%s) from registry %s", cred.Name, cred.ID, reg.Name),
			TargetResource: "registry_credential",
			TargetID:       cred.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "credential", Before: cred.Name, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteDeleteRegistryCredential(tc *aitool.ToolContext, in registryDeleteCredentialInput) (map[string]any, error) {
	reg, cred, err := h.checkDeleteRegistryCredential(tc, in)
	if err != nil {
		return nil, err
	}
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var lockedReg model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedReg, "id = ?", reg.ID).Error; err != nil {
			return err
		}
		var lockedCred model.RegistryCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedCred, "id = ? AND registry_id = ?", cred.ID, lockedReg.ID).Error; err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? AND resource_id = ?", scopedResourceRegistryCredential, lockedCred.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		if h.secretAvailable() {
			if err := h.deleteSecretRefWithDB(tc.Context(), tx, lockedCred.PasswordRef, "registry_credential:"+lockedCred.RegistryID+":password"); err != nil {
				return err
			}
			if err := h.deleteSecretRefWithDB(tc.Context(), tx, lockedCred.TokenRef, "registry_credential:"+lockedCred.RegistryID+":token"); err != nil {
				return err
			}
		}
		if err := tx.Delete(&lockedCred).Error; err != nil {
			return err
		}
		if lockedReg.CredentialRef == lockedCred.ID {
			if err := tx.Model(&model.ArtifactRegistry{}).Where("id = ?", lockedReg.ID).Update("credential_ref", "").Error; err != nil {
				return err
			}
			lockedReg.CredentialRef = ""
		}
		return reconcileRegistryPushCredentialsTx(tx, lockedReg, nil)
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("registry.credential_not_found", "registry credential not found")
	}
	if errors.Is(txErr, errRegistryCredentialInUse) {
		return nil, aitool.Conflict("registry.credential_in_use", "registry credential is required by active repository deployment targets")
	}
	if txErr != nil {
		return nil, aitool.BadRequest("registry.credential_delete_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "registry_credential.delete", cred.ID, true, cred.Scope, tc.Context())
	out := map[string]any{
		"id":         cred.ID,
		"registryId": reg.ID,
		"deleted":    true,
	}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		out["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
	}
	return out, nil
}

// --- 19. build_register_container_image ---

func (h *Handler) aiExecuteRegisterContainerImage(tc *aitool.ToolContext, in registryRegisterImageInput) (model.ContainerImage, error) {
	h.syncGinUser(tc)
	user, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionImageWrite)
	if !ok {
		return model.ContainerImage{}, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	var applicationID string
	if appRef := regTrimPtr(in.Application); appRef != "" {
		app, appErr := aitool.ResolveApplication(tc.DB, project.ID, appRef)
		if appErr != nil {
			return model.ContainerImage{}, appErr
		}
		applicationID = app.ID
	}
	reg, regErr := h.aiResolveUsableRegistry(tc, in.Registry, project.ID)
	if regErr != nil {
		return model.ContainerImage{}, regErr
	}
	repo := strings.Trim(strings.TrimSpace(in.Repository), "/")
	if repo == "" {
		return model.ContainerImage{}, aitool.BadRequest("argument_invalid", "repository is required")
	}
	tag := fallback(regTrimPtr(in.Tag), "latest")
	digest := regTrimPtr(in.Digest)

	image := model.ContainerImage{
		ID:            id.New("img"),
		ProjectID:     project.ID,
		ApplicationID: applicationID,
		RegistryID:    reg.ID,
		Repository:    repo,
		Tag:           tag,
		Digest:        digest,
		ImageRef:      imageReference(reg, repo, tag, digest),
		SourceCommit:  regTrimPtr(in.SourceCommit),
		SourceType:    "manual-image",
		ScanStatus:    "unknown",
		CreatedBy:     user.ID,
	}
	if err := tc.DB.Create(&image).Error; err != nil {
		return model.ContainerImage{}, aitool.BadRequest("image.register_failed", err.Error())
	}
	h.auditWithContext(user.ID, "container_image.create", image.ID, true, image.ImageRef, tc.Context())
	return image, nil
}

func registryOutputMap(out artifactRegistryOutput, conversationProjectID string) map[string]any {
	m := map[string]any{
		"id":                out.ID,
		"name":              out.Name,
		"provider":          out.Provider,
		"endpoint":          out.Endpoint,
		"namespace":         out.Namespace,
		"scope":             out.Scope,
		"ownerRef":          out.OwnerRef,
		"projectIds":        out.ProjectIDs,
		"defaultProjectIds": out.DefaultProjectIDs,
		"credentialSet":     out.CredentialSet,
		"isDefault":         out.IsDefault,
		"capabilities":      out.Capabilities,
		"status":            out.Status,
		"observationCode":   out.ObservationCode,
		"lastCheckedAt":     out.LastCheckedAt,
		"canManage":         out.CanManage,
		"createdBy":         out.CreatedBy,
		"createdAt":         out.CreatedAt,
	}
	if strings.TrimSpace(conversationProjectID) != "" {
		m["projectId"] = strings.TrimSpace(conversationProjectID)
	}
	return m
}

func credentialOutputMap(out registryCredentialOutput, conversationProjectID string) map[string]any {
	m := map[string]any{
		"id":                 out.ID,
		"registryId":         out.RegistryID,
		"name":               out.Name,
		"username":           out.Username,
		"usage":              out.Usage,
		"scope":              out.Scope,
		"ownerRef":           out.OwnerRef,
		"projectIds":         out.ProjectIDs,
		"repositoryTemplate": out.RepositoryTemplate,
		"tagTemplate":        out.TagTemplate,
		"passwordSet":        out.PasswordSet,
		"tokenSet":           out.TokenSet,
		"canManage":          out.CanManage,
		"createdAt":          out.CreatedAt,
	}
	if strings.TrimSpace(conversationProjectID) != "" {
		m["projectId"] = strings.TrimSpace(conversationProjectID)
	}
	return m
}
