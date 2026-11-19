package gitapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type gitSaveAccountInput struct {
	GitAccount     *string  `json:"gitAccount,omitempty" desc:"可选，省略为新建；传入 Git 账号 ID（gita_xxx）或可管理范围内唯一用户名为增量更新"`
	Provider       *string  `json:"provider,omitempty" desc:"新建必填；GitProvider ID 或名称（更新时仅用于跨平台同名账号消歧，不可更改所属 Provider）"`
	Username       *string  `json:"username,omitempty" desc:"新建必填；Git 账号用户名"`
	AccessToken    *string  `json:"accessToken,omitempty" sensitive:"true" desc:"新建必填非空；访问令牌 PAT（更新省略时保留原令牌，与 refreshOAuth 互斥）"`
	RefreshOAuth   *bool    `json:"refreshOAuth,omitempty" desc:"仅更新已有 OAuth 账号时设为 true 主动刷新令牌（与 accessToken 互斥）"`
	Scope          *string  `json:"scope,omitempty" enum:"user|project|global" desc:"可选，新建传非空 addProjects 默认 project 否则默认 user；更新省略时严格保留原 scope"`
	AddProjects    []string `json:"addProjects,omitempty" desc:"可选，scope=project 时增量授权项目 ID 或 identifier"`
	RemoveProjects []string `json:"removeProjects,omitempty" desc:"可选，scope=project 时增量移除项目 ID 或 identifier"`
}

type gitDeleteAccountInput struct {
	GitAccount string  `json:"gitAccount" desc:"必填，待删除的 Git 账号 ID（gita_xxx）或可管理范围内唯一用户名"`
	Provider   *string `json:"provider,omitempty" desc:"可选，跨平台同名账号删除消歧（GitProvider ID 或名称）"`
}

type gitBindRepositoryInput struct {
	Project        string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application    string  `json:"application" desc:"必填，应用 ID、identifier 或唯一名称"`
	GitAccount     string  `json:"gitAccount" desc:"必填，可见 Git 账号 ID（gita_xxx）或唯一用户名"`
	Owner          string  `json:"owner" desc:"必填，仓库组织或用户名（禁止包含 / 或以 - 开头）"`
	Repo           string  `json:"repo" desc:"必填，仓库名称（禁止包含 / 或以 - 开头）"`
	CloneURL       *string `json:"cloneUrl,omitempty" desc:"可选，克隆地址（省略时自动推导；gitlab 必填；必须与所属 GitProvider BaseURL 同源且禁内嵌凭据）"`
	DefaultBranch  *string `json:"defaultBranch,omitempty" default:"main" desc:"可选，默认分支，默认 main（禁止以 - 开头）"`
	WebhookEnabled *bool   `json:"webhookEnabled,omitempty" desc:"可选，是否注册远端自动构建 Webhook（GitHub/Gitea 默认 true；gitlab 必须显式传 false）"`
}

type gitUpdateRepositoryBindingInput struct {
	Project            string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application        *string `json:"application,omitempty" desc:"可选，同项目多应用绑定同一仓库时传入应用消歧"`
	Binding            string  `json:"binding" desc:"必填，仓库绑定 ID（rpb_xxx）或唯一 owner/repo"`
	GitAccount         *string `json:"gitAccount,omitempty" desc:"可选，切换关联的可见 Git 账号 ID 或用户名"`
	Owner              *string `json:"owner,omitempty" desc:"可选，仓库迁移时更新组织或用户名"`
	Repo               *string `json:"repo,omitempty" desc:"可选，仓库改名时更新仓库名"`
	CloneURL           *string `json:"cloneUrl,omitempty" desc:"可选，更新同源克隆地址"`
	DefaultBranch      *string `json:"defaultBranch,omitempty" desc:"可选，更新默认分支（禁止以 - 开头）"`
	WebhookEnabled     *bool   `json:"webhookEnabled,omitempty" desc:"可选，启用或关闭远端 Webhook"`
	ReconfigureWebhook *bool   `json:"reconfigureWebhook,omitempty" desc:"可选，设为 true 时旋转密钥并重建远端 Webhook"`
}

type gitUnbindRepositoryInput struct {
	Project     string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application *string `json:"application,omitempty" desc:"可选，同项目多应用绑定同一仓库时传入应用消歧"`
	Binding     string  `json:"binding" desc:"必填，仓库绑定 ID（rpb_xxx）或唯一 owner/repo"`
}

type adminSaveGitProviderInput struct {
	Provider               *string  `json:"provider,omitempty" desc:"可选，省略为新建；传入 GitProvider ID（gitp_xxx）或唯一名称为增量更新"`
	Type                   *string  `json:"type,omitempty" enum:"github|gitea|gitlab" desc:"新建必填；Git 平台类型"`
	Name                   *string  `json:"name,omitempty" desc:"新建必填；提供商名称"`
	BaseURL                *string  `json:"baseUrl,omitempty" desc:"gitea/gitlab 新建必填；服务根地址（github 固定 https://github.com）"`
	Scope                  *string  `json:"scope,omitempty" enum:"global|project|user" desc:"可选，新建默认 global；更新省略时严格保留原 scope"`
	OwnerUser              *string  `json:"ownerUser,omitempty" desc:"可选，scope=user 时指定归属用户 ID 或邮箱（省略时新建默认当前管理员，更新保留原归属）"`
	AddProjects            []string `json:"addProjects,omitempty" desc:"可选，scope=project 时增量授权项目"`
	RemoveProjects         []string `json:"removeProjects,omitempty" desc:"可选，scope=project 时增量移除项目"`
	AuthType               *string  `json:"authType,omitempty" enum:"oauth|pat|github-app" desc:"可选，认证方式，新建默认 oauth（gitlab 仅支持 pat）"`
	ClientID               *string  `json:"clientId,omitempty" desc:"可选，OAuth Client ID（更新省略时保留原值，清空传 clearClientCredentials=true）"`
	ClientSecret           *string  `json:"clientSecret,omitempty" sensitive:"true" desc:"可选，OAuth Client Secret（更新省略时保留原密钥）"`
	ClearClientCredentials *bool    `json:"clearClientCredentials,omitempty" desc:"可选，显式清空 OAuth clientId 与 clientSecret"`
	Enabled                *bool    `json:"enabled,omitempty" desc:"可选，是否启用，新建默认 true"`
}

type adminDeleteGitProviderInput struct {
	Provider string `json:"provider" desc:"必填，待删除的 GitProvider ID（gitp_xxx）或唯一名称"`
}

func (h *Handler) registerGitWriteAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[gitSaveAccountInput, map[string]any]{
		Name:           "build_save_git_account",
		Domain:         aitool.DomainBuild,
		Title:          "创建或增量更新 Git 访问凭据",
		Summary:        "创建或增量修改个人/项目/全局 Git 访问凭据（PAT），或刷新 OAuth 访问令牌",
		Tags:           []string{"git", "account", "credential", "token"},
		AliasesZH:      []string{"保存Git账号", "配置Git令牌", "刷新Git授权"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_git_sources",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "gitAccount",
			Collection:     true,
			CollectionPath: "accounts.items",
		}},
		Preview: h.aiPreviewSaveGitAccount,
		Execute: h.aiExecuteSaveGitAccount,
	})

	aitool.Register(reg, aitool.Spec[gitDeleteAccountInput, map[string]any]{
		Name:           "build_delete_git_account",
		Domain:         aitool.DomainBuild,
		Title:          "删除 Git 访问凭据",
		Summary:        "删除未被活跃代码仓库绑定引用的 Git 访问凭据/账号",
		Tags:           []string{"git", "account", "delete"},
		AliasesZH:      []string{"删除Git账号", "移除Git凭据"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_git_sources",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "gitAccount",
			Collection:     true,
			CollectionPath: "accounts.items",
		}},
		Preview: h.aiPreviewDeleteGitAccount,
		Execute: h.aiExecuteDeleteGitAccount,
	})

	aitool.Register(reg, aitool.Spec[gitBindRepositoryInput, map[string]any]{
		Name:           "build_bind_repository",
		Domain:         aitool.DomainBuild,
		Title:          "绑定应用代码仓库",
		Summary:        "为应用绑定远端代码仓库并按需自动注册推送构建 Webhook",
		Tags:           []string{"git", "repository", "bind", "webhook"},
		AliasesZH:      []string{"绑定代码仓库", "关联Git仓库"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_repository_bindings",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "binding",
			Collection:     true,
		}},
		Preview: h.aiPreviewBindRepository,
		Execute: h.aiExecuteBindRepository,
	})

	aitool.Register(reg, aitool.Spec[gitUpdateRepositoryBindingInput, map[string]any]{
		Name:           "build_update_repository_binding",
		Domain:         aitool.DomainBuild,
		Title:          "增量修改应用代码仓库绑定",
		Summary:        "增量修改应用代码仓库绑定参数（账号、仓库、默认分支、Webhook 开关）或重建修复远端 Webhook",
		Tags:           []string{"git", "repository", "binding", "webhook", "update"},
		AliasesZH:      []string{"修改仓库绑定", "修复Webhook", "切换默认分支"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_repository_bindings",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "binding",
			Collection:     true,
		}},
		Preview: h.aiPreviewUpdateRepositoryBinding,
		Execute: h.aiExecuteUpdateRepositoryBinding,
	})

	aitool.Register(reg, aitool.Spec[gitUnbindRepositoryInput, map[string]any]{
		Name:           "build_unbind_repository",
		Domain:         aitool.DomainBuild,
		Title:          "解绑应用代码仓库",
		Summary:        "解绑未被部署目标引用的应用代码仓库并清理关联 Webhook",
		Tags:           []string{"git", "repository", "unbind", "delete"},
		AliasesZH:      []string{"解绑代码仓库", "移除仓库绑定"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_repository_bindings",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "binding",
			Collection:     true,
		}},
		Preview: h.aiPreviewUnbindRepository,
		Execute: h.aiExecuteUnbindRepository,
	})
}

func (h *Handler) registerGitAdminAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[adminListGitProvidersInput, map[string]any]{
		Name:              "admin_list_git_providers",
		Domain:            aitool.DomainAdmin,
		Title:             "查看全平台 Git 代码提供商配置",
		Summary:           "全平台视角分页查看全部 Git 代码提供商配置（含所有作用域及禁用项，回显关联账号数与仓库绑定数）",
		Tags:              []string{"git", "provider", "admin"},
		AliasesZH:         []string{"管理Git提供商", "GitProvider列表"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL0,
		Effect:            aitool.EffectRead,
		RequiredScopes:    []string{string(authz.ActionGitRead)},
		Execute:           h.aiListGitProvidersAdmin,
	})

	aitool.Register(reg, aitool.Spec[adminSaveGitProviderInput, map[string]any]{
		Name:              "admin_save_git_provider",
		Domain:            aitool.DomainAdmin,
		Title:             "创建或增量修改 Git 代码提供商",
		Summary:           "创建或纯增量更新平台 Git 代码提供商（GitProvider）配置，省略密钥时完整保留原加密引用",
		Tags:              []string{"git", "provider", "admin", "oauth"},
		AliasesZH:         []string{"配置GitProvider", "新增Git平台"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL2,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "admin_list_git_providers",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "provider",
			Collection:     true,
		}},
		Preview: h.aiPreviewSaveGitProviderAdmin,
		Execute: h.aiExecuteSaveGitProviderAdmin,
	})

	aitool.Register(reg, aitool.Spec[adminDeleteGitProviderInput, map[string]any]{
		Name:              "admin_delete_git_provider",
		Domain:            aitool.DomainAdmin,
		Title:             "删除 Git 代码提供商",
		Summary:           "删除无任何关联用户账号或仓库绑定的 Git 代码提供商配置",
		Tags:              []string{"git", "provider", "admin", "delete"},
		AliasesZH:         []string{"删除GitProvider", "移除Git平台"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL2,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionGitWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "admin_list_git_providers",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "provider",
			Collection:     true,
		}},
		Preview: h.aiPreviewDeleteGitProviderAdmin,
		Execute: h.aiExecuteDeleteGitProviderAdmin,
	})
}

// --- 5. build_save_git_account ---

type preparedGitAccountSave struct {
	isCreate     bool
	existing     model.GitAccount
	provider     model.GitProvider
	target       model.GitAccount
	accessToken  string
	refreshOAuth bool
}

func (h *Handler) aiResolveManageableGitAccount(tc *aitool.ToolContext, accountRef, providerRef string) (model.GitAccount, error) {
	var filterProviderID string
	if providerRef != "" {
		var providers []model.GitProvider
		if err := tc.DB.Order("created_at asc").Find(&providers).Error; err != nil {
			return model.GitAccount{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		prov, err := aitool.ResolveUniqueByIDOrName(
			providers,
			"git_provider",
			"git.provider_not_found",
			providerRef,
			func(p model.GitProvider) string { return p.ID },
			func(p model.GitProvider) string { return p.Name },
		)
		if err != nil {
			return model.GitAccount{}, err
		}
		filterProviderID = prov.ID
	}

	var all []model.GitAccount
	if err := tc.DB.Order("created_at asc").Find(&all).Error; err != nil {
		return model.GitAccount{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	// Exact ID match first so provider mismatch yields 400 argument_invalid per spec line 499.
	for _, acc := range all {
		if acc.ID == strings.TrimSpace(accountRef) {
			if !h.canManageGitAccountSilent(tc, acc) {
				return model.GitAccount{}, aitool.Forbidden("git.account_forbidden", "not authorized to manage this Git account")
			}
			if filterProviderID != "" && acc.ProviderID != filterProviderID {
				return model.GitAccount{}, aitool.BadRequest("argument_invalid", "cannot change provider of an existing GitAccount")
			}
			acc.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitAccount, acc.ID, tc.Context())
			return acc, nil
		}
	}

	manageable := make([]model.GitAccount, 0, len(all))
	for _, acc := range all {
		if filterProviderID != "" && acc.ProviderID != filterProviderID {
			continue
		}
		if h.canManageGitAccountSilent(tc, acc) {
			manageable = append(manageable, acc)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		manageable,
		"git_account",
		"git.account_not_found",
		accountRef,
		func(a model.GitAccount) string { return a.ID },
		func(a model.GitAccount) string { return a.Username },
	)
	if err != nil {
		return model.GitAccount{}, err
	}
	resolved.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitAccount, resolved.ID, tc.Context())
	return resolved, nil
}

func (h *Handler) canManageGitAccountSilent(tc *aitool.ToolContext, account model.GitAccount) bool {
	child, _ := aitool.NewToolContext(tc.ParentGin, tc.DB, tc.User, tc.SessionID, tc.ConversationID, tc.ConversationProjectID, tc.RunID, tc.ToolCallID, tc.IdempotencyKey, tc.InputMode, nil), false
	h.syncGinUser(child)
	return h.canManageScopedResourceByID(child.Gin, tc.User, account.Scope, account.OwnerRef, scopedResourceGitAccount, account.ID, "forbidden")
}

func (h *Handler) prepareSaveGitAccount(tc *aitool.ToolContext, in gitSaveAccountInput) (preparedGitAccountSave, error) {
	h.syncGinUser(tc)
	var prep preparedGitAccountSave

	accessToken := trimPtr(in.AccessToken)
	refreshOAuth := boolPtrDefault(in.RefreshOAuth, false)
	if accessToken != "" && refreshOAuth {
		return prep, aitool.BadRequest("argument_invalid", "accessToken and refreshOAuth cannot both be specified")
	}
	addIDs, err := resolveProjectRefs(tc.DB, in.AddProjects)
	if err != nil {
		return prep, err
	}
	removeIDs, err := resolveProjectRefs(tc.DB, in.RemoveProjects)
	if err != nil {
		return prep, err
	}

	accountRef := trimPtr(in.GitAccount)
	if accountRef == "" {
		// Create mode
		prep.isCreate = true
		if refreshOAuth {
			return prep, aitool.BadRequest("argument_invalid", "refreshOAuth can only be used when updating an existing OAuth GitAccount")
		}
		providerRef := trimPtr(in.Provider)
		if providerRef == "" {
			return prep, aitool.BadRequest("argument_invalid", "provider is required when creating a GitAccount")
		}
		username := trimPtr(in.Username)
		if username == "" {
			return prep, aitool.BadRequest("argument_invalid", "username is required when creating a GitAccount")
		}
		if accessToken == "" {
			return prep, aitool.BadRequest("argument_invalid", "accessToken is required and must be non-empty when creating a GitAccount")
		}
		provider, provErr := h.aiResolveEnabledGitProvider(tc, providerRef, strings.TrimSpace(tc.ConversationProjectID))
		if provErr != nil {
			return prep, provErr
		}
		rawScope := trimPtr(in.Scope)
		if rawScope == "" {
			if len(addIDs) > 0 {
				rawScope = "project"
			} else {
				rawScope = "user"
			}
		}
		projectIDs := mergeIncrementalProjectIDs(nil, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeCredentialScopeWithinParent(
			tc.Gin, tc.User, normalizeGitScope(rawScope), projectIDs, provider.Scope, provider.ProjectIDs, "platform administrator role is required for global Git credentials",
		)
		if !ok {
			return prep, tc.AbortedError("git.account_scope_invalid", "invalid Git credential scope")
		}
		if err := enforceScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, scope, ownerRef, normalizedProjects); err != nil {
			return prep, err
		}
		candidate := model.GitAccount{
			ID:         id.New("gita"),
			UserID:     tc.User.ID,
			ProviderID: provider.ID,
			Scope:      scope,
			OwnerRef:   ownerRef,
			ProjectIDs: normalizedProjects,
			Username:   username,
		}
		if err := ensureGitAccountFitsProviderScopeTx(tc.DB, provider, candidate); err != nil {
			return prep, aitool.Conflict("git.credential_scope_conflict", "Git credential scope exceeds parent provider scope")
		}
		prep.provider = provider
		prep.target = candidate
		prep.accessToken = accessToken
		return prep, nil
	}

	// Update mode
	existing, err := h.aiResolveManageableGitAccount(tc, accountRef, trimPtr(in.Provider))
	if err != nil {
		return prep, err
	}
	if err := enforceScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, existing.Scope, existing.OwnerRef, existing.ProjectIDs); err != nil {
		return prep, err
	}
	var provider model.GitProvider
	if err := tc.DB.First(&provider, "id = ? AND enabled = ?", existing.ProviderID, true).Error; err != nil {
		return prep, aitool.NotFound("git.provider_not_found", "parent GitProvider is disabled or missing")
	}
	provider.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitProvider, provider.ID, tc.Context())

	if refreshOAuth {
		if strings.TrimSpace(existing.RefreshTokenRef) == "" {
			return prep, aitool.BadRequest("git.refresh_token_missing", "Git account has no stored OAuth refresh token")
		}
	}

	next := existing
	if in.Username != nil {
		u := strings.TrimSpace(*in.Username)
		if u == "" {
			return prep, aitool.BadRequest("argument_invalid", "username cannot be empty")
		}
		next.Username = u
	}
	scopeChanged := in.Scope != nil || len(addIDs) > 0 || len(removeIDs) > 0
	if scopeChanged {
		targetScope := existing.Scope
		if in.Scope != nil && strings.TrimSpace(*in.Scope) != "" {
			targetScope = normalizeGitScope(*in.Scope)
		}
		mergedProjects := mergeIncrementalProjectIDs(existing.ProjectIDs, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeCredentialScopeWithinParent(
			tc.Gin, tc.User, targetScope, mergedProjects, provider.Scope, provider.ProjectIDs, "platform administrator role is required for global Git credentials",
		)
		if !ok {
			return prep, tc.AbortedError("git.account_scope_invalid", "invalid Git credential scope")
		}
		next.Scope = scope
		next.OwnerRef = ownerRef
		next.ProjectIDs = normalizedProjects
	}
	if err := enforceScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, next.Scope, next.OwnerRef, next.ProjectIDs); err != nil {
		return prep, err
	}
	if err := ensureGitAccountFitsProviderScopeTx(tc.DB, provider, next); err != nil {
		return prep, aitool.Conflict("git.credential_scope_conflict", "Git credential scope exceeds parent provider scope")
	}
	if err := ensureGitAccountScopeCompatibleTx(tc.DB, next); err != nil {
		return prep, aitool.Conflict("git.account_in_use", "Git credential is referenced by repository bindings outside the target scope")
	}

	prep.existing = existing
	prep.provider = provider
	prep.target = next
	prep.accessToken = accessToken
	prep.refreshOAuth = refreshOAuth
	return prep, nil
}

func (h *Handler) aiPreviewSaveGitAccount(tc *aitool.ToolContext, in gitSaveAccountInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareSaveGitAccount(tc, in)
	if err != nil {
		return nil, err
	}
	var sensitiveKeys []string
	if prep.accessToken != "" {
		sensitiveKeys = append(sensitiveKeys, "accessToken")
	}
	var changes []aitool.PreviewChange
	summary := fmt.Sprintf("Create %s Git account %q on provider %s", prep.target.Scope, prep.target.Username, prep.provider.Name)
	targetID := ""
	if !prep.isCreate {
		targetID = prep.existing.ID
		summary = fmt.Sprintf("Update Git account %q (%s)", prep.target.Username, prep.existing.ID)
		if prep.existing.Username != prep.target.Username {
			changes = append(changes, aitool.PreviewChange{Field: "username", Before: prep.existing.Username, After: prep.target.Username, Action: "update"})
		}
		if prep.existing.Scope != prep.target.Scope {
			changes = append(changes, aitool.PreviewChange{Field: "scope", Before: prep.existing.Scope, After: prep.target.Scope, Action: "update"})
		}
		if prep.refreshOAuth {
			changes = append(changes, aitool.PreviewChange{Field: "oauthToken", Action: "refresh"})
		}
	} else {
		changes = append(changes,
			aitool.PreviewChange{Field: "username", After: prep.target.Username, Action: "create"},
			aitool.PreviewChange{Field: "scope", After: prep.target.Scope, Action: "create"},
		)
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "git_account",
			TargetID:       targetID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
	}, nil
}

func (h *Handler) aiExecuteSaveGitAccount(tc *aitool.ToolContext, in gitSaveAccountInput) (map[string]any, error) {
	prep, err := h.prepareSaveGitAccount(tc, in)
	if err != nil {
		return nil, err
	}
	if prep.isCreate {
		account := prep.target
		txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
			var lockedProvider model.GitProvider
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProvider, "id = ? AND enabled = ?", account.ProviderID, true).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errGitProviderUnavailable
				}
				return err
			}
			if err := ensureGitAccountFitsProviderScopeTx(tx, lockedProvider, account); err != nil {
				return err
			}
			if h.secrets.Available() {
				ref, storeErr := h.secrets.StoreContextWithDB(tc.Context(), tx, prep.accessToken, tc.User.ID, "git_account:"+account.ID+":access")
				if storeErr != nil {
					return storeErr
				}
				account.AccessTokenRef = ref
			}
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			return h.replaceScopedResourceProjectBindings(tx, scopedResourceGitAccount, account.ID, sortedProjectIDs(account.ProjectIDs), nil)
		})
		if errors.Is(txErr, errGitProviderUnavailable) {
			return nil, aitool.NotFound("git.provider_not_found", "git provider not found")
		}
		if errors.Is(txErr, errGitCredentialScopeConflict) {
			return nil, aitool.Conflict("git.credential_scope_conflict", "Git credential scope exceeds parent provider scope")
		}
		if txErr != nil {
			return nil, aitool.BadRequest("git.account_save_failed", txErr.Error())
		}
		h.auditWithContext(tc.User.ID, "git_account.create", account.ID, true, account.Scope, tc.Context())
		h.observeGitAccount(tc.Context(), tc.User, &account)
		resp := map[string]any(gitAccountResponse(account))
		if strings.TrimSpace(tc.ConversationProjectID) != "" {
			resp["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
		}
		return resp, nil
	}

	current := prep.existing
	if prep.refreshOAuth {
		refreshed, ok := h.refreshGitAccountForUser(tc.Gin, tc.User, current, prep.provider)
		if !ok {
			return nil, tc.AbortedError("git.token_refresh_failed", "git token refresh failed")
		}
		current = refreshed
	}

	var updated model.GitAccount
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var lockedProvider model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProvider, "id = ? AND enabled = ?", prep.provider.ID, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errGitProviderUnavailable
			}
			return err
		}
		var locked model.GitAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", current.ID).Error; err != nil {
			return err
		}
		// Strictly preserve ExternalUserID, AvatarURL, Scopes; apply incremental patches.
		locked.Username = prep.target.Username
		locked.Scope = prep.target.Scope
		locked.OwnerRef = prep.target.OwnerRef
		locked.ProjectIDs = prep.target.ProjectIDs
		if err := ensureGitAccountFitsProviderScopeTx(tx, lockedProvider, locked); err != nil {
			return err
		}
		if err := ensureGitAccountScopeCompatibleTx(tx, locked); err != nil {
			return err
		}
		if prep.accessToken != "" {
			// Manual static PAT replacement: clear ExpiresAt and RefreshTokenRef, atomically store new AccessTokenRef and delete old refs.
			locked.ExpiresAt = nil
			if h.secrets.Available() {
				accessRes := "git_account:" + locked.ID + ":access"
				newRef, storeErr := h.secrets.StoreContextWithDB(tc.Context(), tx, prep.accessToken, tc.User.ID, accessRes)
				if storeErr != nil {
					return storeErr
				}
				if strings.TrimSpace(locked.AccessTokenRef) != "" && locked.AccessTokenRef != newRef {
					if delErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.AccessTokenRef, accessRes); delErr != nil {
						return delErr
					}
				}
				locked.AccessTokenRef = newRef
			}
			if strings.TrimSpace(locked.RefreshTokenRef) != "" {
				if h.secrets.Available() {
					if delErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.RefreshTokenRef, "git_account:"+locked.ID+":refresh"); delErr != nil {
						return delErr
					}
				}
				locked.RefreshTokenRef = ""
			}
		}
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceGitAccount, locked.ID, sortedProjectIDs(locked.ProjectIDs), nil); err != nil {
			return err
		}
		updated = locked
		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("git.account_not_found", "git account not found")
	}
	if errors.Is(txErr, errGitProviderUnavailable) {
		return nil, aitool.NotFound("git.provider_not_found", "git provider not found")
	}
	if errors.Is(txErr, errGitCredentialScopeConflict) {
		return nil, aitool.Conflict("git.credential_scope_conflict", "Git credential scope exceeds parent provider scope")
	}
	if errors.Is(txErr, errGitAccountInUse) {
		return nil, aitool.Conflict("git.account_in_use", "Git credential is referenced by repository bindings outside the target scope")
	}
	if txErr != nil {
		return nil, aitool.BadRequest("git.account_save_failed", txErr.Error())
	}

	h.auditWithContext(tc.User.ID, "git_account.update", updated.ID, true, updated.Scope, tc.Context())
	h.observeGitAccount(tc.Context(), tc.User, &updated)
	resp := map[string]any(gitAccountResponse(updated))
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		resp["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
	}
	return resp, nil
}

// --- 6. build_delete_git_account ---

func (h *Handler) checkDeleteGitAccount(tc *aitool.ToolContext, in gitDeleteAccountInput) (model.GitAccount, error) {
	h.syncGinUser(tc)
	account, err := h.aiResolveManageableGitAccount(tc, in.GitAccount, trimPtr(in.Provider))
	if err != nil {
		return model.GitAccount{}, err
	}
	if err := enforceScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, account.Scope, account.OwnerRef, account.ProjectIDs); err != nil {
		return model.GitAccount{}, err
	}
	var bindingCount int64
	if err := tc.DB.Model(&model.RepositoryBinding{}).
		Joins("JOIN projects ON projects.id = repository_bindings.project_id AND projects.deleted_at IS NULL").
		Where("repository_bindings.git_account_id = ? AND repository_bindings.deleted_at IS NULL", account.ID).
		Count(&bindingCount).Error; err != nil {
		return model.GitAccount{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if bindingCount > 0 {
		return model.GitAccount{}, aitool.Conflict("git.account_in_use", fmt.Sprintf("Git account %s is referenced by %d active repository binding(s)", account.ID, bindingCount))
	}
	return account, nil
}

func (h *Handler) aiPreviewDeleteGitAccount(tc *aitool.ToolContext, in gitDeleteAccountInput) (*aitool.PreviewResult, error) {
	account, err := h.checkDeleteGitAccount(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete Git account %q (%s)", account.Username, account.ID),
			TargetResource: "git_account",
			TargetID:       account.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "gitAccount", Before: account.Username, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteDeleteGitAccount(tc *aitool.ToolContext, in gitDeleteAccountInput) (map[string]any, error) {
	account, err := h.checkDeleteGitAccount(tc, in)
	if err != nil {
		return nil, err
	}
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.GitAccount
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", account.ID).Error; err != nil {
			return err
		}
		var bindingCount int64
		if err := tx.Model(&model.RepositoryBinding{}).
			Joins("JOIN projects ON projects.id = repository_bindings.project_id AND projects.deleted_at IS NULL").
			Where("repository_bindings.git_account_id = ? AND repository_bindings.deleted_at IS NULL", locked.ID).
			Count(&bindingCount).Error; err != nil {
			return err
		}
		if bindingCount > 0 {
			return errGitAccountInUse
		}
		if h.secrets.Available() {
			if err := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.AccessTokenRef, "git_account:"+locked.ID+":access"); err != nil {
				return err
			}
			if err := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.RefreshTokenRef, "git_account:"+locked.ID+":refresh"); err != nil {
				return err
			}
		}
		if err := tx.Where("resource_type = ? AND resource_id = ?", scopedResourceGitAccount, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("git.account_not_found", "git account not found")
	}
	if errors.Is(txErr, errGitAccountInUse) {
		return nil, aitool.Conflict("git.account_in_use", "Git credential is referenced by active repository bindings")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "git.account_delete_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "git_account.delete", account.ID, true, account.Username, tc.Context())
	out := map[string]any{
		"id":      account.ID,
		"deleted": true,
	}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		out["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
	}
	return out, nil
}

// --- 7. build_bind_repository ---

func (h *Handler) prepareBindRepository(tc *aitool.ToolContext, in gitBindRepositoryInput) (model.Project, model.Application, model.GitProvider, model.RepositoryBinding, error) {
	h.syncGinUser(tc)
	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionGitWrite)
	if !ok {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	app, err := aitool.ResolveApplication(tc.DB, project.ID, in.Application)
	if err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}
	account, provider, err := h.aiResolveUsableGitAccountAndProvider(tc, in.GitAccount, "", project.ID)
	if err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}
	owner, err := validateSafeGitIdentifierSegment(in.Owner, "owner")
	if err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}
	repo, err := validateSafeGitIdentifierSegment(in.Repo, "repo")
	if err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}
	defaultBranch, err := validateSafeGitBranch(trimPtr(in.DefaultBranch))
	if err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}

	webhookEnabled := true
	if provider.Type == "gitlab" {
		if in.WebhookEnabled == nil || *in.WebhookEnabled {
			return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"git.webhook_unsupported_for_gitlab",
				"GitLab providers currently only support PAT clone builds and do not support automatic webhook registration",
				"Pass webhookEnabled: false explicitly when binding a GitLab repository.",
			)
		}
		webhookEnabled = false
		if trimPtr(in.CloneURL) == "" {
			return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"git.clone_url_required",
				"cloneUrl is required when binding a GitLab repository",
				"Pass the exact http(s) cloneUrl matching the GitLab provider baseUrl.",
			)
		}
	} else if in.WebhookEnabled != nil {
		webhookEnabled = *in.WebhookEnabled
	}

	rawCloneURL := trimPtr(in.CloneURL)
	if rawCloneURL == "" {
		rawCloneURL = defaultCloneURL(provider, owner, repo)
	}
	safeCloneURL, err := validateSafeSameOriginCloneURL(rawCloneURL, provider, h.egressPolicyForUser(tc.User, tc.Context()))
	if err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}

	if err := h.ensureRepoBindingUniqueErr(tc.DB, project.ID, app.ID, provider.ID, owner, repo, ""); err != nil {
		return model.Project{}, model.Application{}, model.GitProvider{}, model.RepositoryBinding{}, err
	}

	binding := model.RepositoryBinding{
		ID:             id.New("rpb"),
		ProjectID:      project.ID,
		ApplicationID:  app.ID,
		GitProviderID:  provider.ID,
		GitAccountID:   account.ID,
		Owner:          owner,
		Repo:           repo,
		CloneURL:       safeCloneURL,
		DefaultBranch:  defaultBranch,
		WebhookEnabled: webhookEnabled,
	}
	return project, app, provider, binding, nil
}

func (h *Handler) ensureRepoBindingUniqueErr(db *gorm.DB, projectID, appID, providerID, owner, repo, excludeID string) error {
	query := db.Model(&model.RepositoryBinding{}).
		Where("project_id = ? AND application_id = ? AND git_provider_id = ? AND lower(owner) = ? AND lower(repo) = ? AND deleted_at IS NULL",
			projectID, appID, providerID, normalizeRepositoryBindingOwner(owner), normalizeRepositoryBindingRepo(repo))
	if strings.TrimSpace(excludeID) != "" {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if count > 0 {
		return aitool.Conflict("git.repository_binding_exists", fmt.Sprintf("application already binds repository %s/%s", owner, repo))
	}
	return nil
}

func (h *Handler) aiPreviewBindRepository(tc *aitool.ToolContext, in gitBindRepositoryInput) (*aitool.PreviewResult, error) {
	project, app, provider, binding, err := h.prepareBindRepository(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Bind repository %s/%s (%s) to application %s", binding.Owner, binding.Repo, provider.Name, app.Name),
			TargetResource: "repository_binding",
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "repository", After: binding.Owner + "/" + binding.Repo, Action: "create"},
				{Field: "cloneUrl", After: binding.CloneURL, Action: "create"},
				{Field: "defaultBranch", After: binding.DefaultBranch, Action: "create"},
				{Field: "webhookEnabled", After: binding.WebhookEnabled, Action: "create"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteBindRepository(tc *aitool.ToolContext, in gitBindRepositoryInput) (map[string]any, error) {
	project, app, _, binding, err := h.prepareBindRepository(tc, in)
	if err != nil {
		return nil, err
	}
	if binding.WebhookEnabled {
		if hookErr := h.configureRepositoryWebhook(tc.Gin, tc.User, &binding, false); hookErr != nil {
			return nil, repositoryWebhookToolError(hookErr)
		}
	} else if createErr := tc.DB.Create(&binding).Error; createErr != nil {
		return nil, aitool.BadRequest("git.repository_bind_failed", createErr.Error())
	}
	h.auditWithContext(tc.User.ID, "repository_binding.create", binding.ID, true, binding.Owner+"/"+binding.Repo, tc.Context())
	binding.CredentialRef = ""
	resp := repositoryBindingResponse{
		RepositoryBinding:  binding,
		WebhookCallbackURL: h.gitWebhookURL(tc.Gin, binding.ID),
		ApplicationName:    app.Name,
	}
	h.observeRepositoryBinding(tc.Context(), tc.User, &resp)
	return repositoryBindingToMap(resp, project.ID), nil
}

// --- 8. build_update_repository_binding ---

type preparedRepoBindingUpdate struct {
	project            model.Project
	existing           model.RepositoryBinding
	next               model.RepositoryBinding
	targetChanged      bool
	reconfigureWebhook bool
}

func (h *Handler) prepareUpdateRepositoryBinding(tc *aitool.ToolContext, in gitUpdateRepositoryBindingInput) (preparedRepoBindingUpdate, error) {
	h.syncGinUser(tc)
	var prep preparedRepoBindingUpdate

	if in.WebhookEnabled != nil && !*in.WebhookEnabled && boolPtrDefault(in.ReconfigureWebhook, false) {
		return prep, aitool.BadRequest("argument_invalid", "cannot set webhookEnabled=false and reconfigureWebhook=true simultaneously")
	}

	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionGitWrite)
	if !ok {
		return prep, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	var appFilterID string
	if appRef := trimPtr(in.Application); appRef != "" {
		app, err := aitool.ResolveApplication(tc.DB, project.ID, appRef)
		if err != nil {
			return prep, err
		}
		appFilterID = app.ID
	}
	existing, err := h.aiResolveRepositoryBinding(tc.DB, project.ID, appFilterID, in.Binding)
	if err != nil {
		return prep, err
	}

	next := existing
	var nextProvider model.GitProvider
	if accRef := trimPtr(in.GitAccount); accRef != "" {
		account, provider, accErr := h.aiResolveUsableGitAccountAndProvider(tc, accRef, "", project.ID)
		if accErr != nil {
			return prep, accErr
		}
		next.GitAccountID = account.ID
		next.GitProviderID = provider.ID
		nextProvider = provider
	} else {
		var account model.GitAccount
		if err := tc.DB.First(&account, "id = ?", existing.GitAccountID).Error; err != nil {
			return prep, aitool.NotFound("git.account_not_found", "bound Git account was not found; pass gitAccount to switch to an active account")
		}
		if !h.gitAccountUsableInProject(tc.DB, tc.User, account, project.ID, tc.Context()) {
			return prep, aitool.Forbidden("git.credential_forbidden", "bound Git account is not accessible in this project")
		}
		if err := tc.DB.First(&nextProvider, "id = ? AND enabled = ?", existing.GitProviderID, true).Error; err != nil {
			return prep, aitool.NotFound("git.provider_not_found", "bound Git provider is unavailable")
		}
		if !h.gitProviderUsableInProject(tc.DB, tc.User, nextProvider, project.ID, tc.Context()) {
			return prep, aitool.Forbidden("git.provider_forbidden", "bound Git provider is not accessible in this project")
		}
	}

	if in.Owner != nil {
		o, oErr := validateSafeGitIdentifierSegment(*in.Owner, "owner")
		if oErr != nil {
			return prep, oErr
		}
		next.Owner = o
	}
	if in.Repo != nil {
		r, rErr := validateSafeGitIdentifierSegment(*in.Repo, "repo")
		if rErr != nil {
			return prep, rErr
		}
		next.Repo = r
	}
	if in.DefaultBranch != nil {
		b, bErr := validateSafeGitBranch(*in.DefaultBranch)
		if bErr != nil {
			return prep, bErr
		}
		next.DefaultBranch = b
	}
	if in.WebhookEnabled != nil {
		next.WebhookEnabled = *in.WebhookEnabled
	}

	targetChanged := repositoryBindingWebhookTargetChanged(existing, next)
	if in.CloneURL != nil {
		safeURL, urlErr := validateSafeSameOriginCloneURL(*in.CloneURL, nextProvider, h.egressPolicyForUser(tc.User, tc.Context()))
		if urlErr != nil {
			return prep, urlErr
		}
		next.CloneURL = safeURL
	} else if targetChanged {
		if nextProvider.Type == "gitlab" {
			return prep, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"git.clone_url_required",
				"cloneUrl must be explicitly provided when switching repository or account to a GitLab provider",
				"Pass the exact http(s) cloneUrl matching the GitLab provider baseUrl.",
			)
		}
		recomputed := defaultCloneURL(nextProvider, next.Owner, next.Repo)
		safeURL, urlErr := validateSafeSameOriginCloneURL(recomputed, nextProvider, h.egressPolicyForUser(tc.User, tc.Context()))
		if urlErr != nil {
			return prep, urlErr
		}
		next.CloneURL = safeURL
	} else {
		// Validate existing cloneURL remains safe & same-origin.
		if _, urlErr := validateSafeSameOriginCloneURL(next.CloneURL, nextProvider, h.egressPolicyForUser(tc.User, tc.Context())); urlErr != nil {
			return prep, urlErr
		}
	}

	if nextProvider.Type == "gitlab" && (next.WebhookEnabled || boolPtrDefault(in.ReconfigureWebhook, false)) {
		return prep, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"git.webhook_unsupported_for_gitlab",
			"GitLab providers do not support automatic webhook registration",
			"Set webhookEnabled: false for GitLab repository bindings.",
		)
	}

	if err := h.ensureRepoBindingUniqueErr(tc.DB, project.ID, next.ApplicationID, next.GitProviderID, next.Owner, next.Repo, existing.ID); err != nil {
		return prep, err
	}

	prep.project = project
	prep.existing = existing
	prep.next = next
	prep.targetChanged = targetChanged
	prep.reconfigureWebhook = boolPtrDefault(in.ReconfigureWebhook, false)
	return prep, nil
}

func (h *Handler) aiPreviewUpdateRepositoryBinding(tc *aitool.ToolContext, in gitUpdateRepositoryBindingInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareUpdateRepositoryBinding(tc, in)
	if err != nil {
		return nil, err
	}
	var changes []aitool.PreviewChange
	if prep.existing.Owner != prep.next.Owner || prep.existing.Repo != prep.next.Repo {
		changes = append(changes, aitool.PreviewChange{
			Field:  "repository",
			Before: prep.existing.Owner + "/" + prep.existing.Repo,
			After:  prep.next.Owner + "/" + prep.next.Repo,
			Action: "update",
		})
	}
	if prep.existing.CloneURL != prep.next.CloneURL {
		changes = append(changes, aitool.PreviewChange{Field: "cloneUrl", Before: prep.existing.CloneURL, After: prep.next.CloneURL, Action: "update"})
	}
	if prep.existing.DefaultBranch != prep.next.DefaultBranch {
		changes = append(changes, aitool.PreviewChange{Field: "defaultBranch", Before: prep.existing.DefaultBranch, After: prep.next.DefaultBranch, Action: "update"})
	}
	if prep.existing.WebhookEnabled != prep.next.WebhookEnabled {
		changes = append(changes, aitool.PreviewChange{Field: "webhookEnabled", Before: prep.existing.WebhookEnabled, After: prep.next.WebhookEnabled, Action: "update"})
	}
	if prep.reconfigureWebhook {
		changes = append(changes, aitool.PreviewChange{Field: "webhook", Action: "reconfigure"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update repository binding %s/%s (%s)", prep.next.Owner, prep.next.Repo, prep.existing.ID),
			TargetResource: "repository_binding",
			TargetID:       prep.existing.ID,
			ProjectID:      prep.project.ID,
			Changes:        changes,
		},
	}, nil
}

func (h *Handler) bestEffortDeleteRemoteWebhook(tc *aitool.ToolContext, binding model.RepositoryBinding) {
	if strings.TrimSpace(binding.WebhookID) == "" {
		return
	}
	client, ok := h.gitClientForObservation(tc.User, binding.GitProviderID, binding.GitAccountID, tc.Context())
	if !ok {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(tc.Context()), repositoryWebhookCompensationTimeout)
	defer cancel()
	_ = client.DeleteWebhook(cleanupCtx, binding.Owner, binding.Repo, binding.WebhookID)
}

func (h *Handler) aiExecuteUpdateRepositoryBinding(tc *aitool.ToolContext, in gitUpdateRepositoryBindingInput) (map[string]any, error) {
	prep, err := h.prepareUpdateRepositoryBinding(tc, in)
	if err != nil {
		return nil, err
	}

	cleanupOldWebhook := !prep.next.WebhookEnabled || prep.targetChanged || prep.reconfigureWebhook
	if cleanupOldWebhook {
		h.bestEffortDeleteRemoteWebhook(tc, prep.existing)
		prep.next.WebhookID = ""
		prep.next.WebhookSecret = ""
		if prep.targetChanged {
			prep.next.LastEvent = ""
			prep.next.LastCommitSHA = ""
			prep.next.LastWebhookAt = nil
		}
	}
	prep.next.CredentialRef = ""

	if prep.next.WebhookEnabled && (prep.targetChanged || prep.reconfigureWebhook || strings.TrimSpace(prep.existing.WebhookID) == "") {
		if hookErr := h.configureRepositoryWebhookWithGuard(tc.Gin, tc.User, &prep.next, false, true); hookErr != nil {
			if errors.Is(hookErr, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("git.repository_binding_not_found", "repository binding not found")
			}
			if errors.Is(hookErr, errRepositoryBindingInUse) {
				return nil, aitool.Conflict("git.repository_binding_in_use", "repository binding is referenced by deployment targets")
			}
			return nil, repositoryWebhookToolError(hookErr)
		}
	} else {
		txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
			var locked model.RepositoryBinding
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ? AND project_id = ?", prep.existing.ID, prep.project.ID).Error; err != nil {
				return err
			}
			if strings.TrimSpace(locked.WebhookSecret) != "" && strings.TrimSpace(prep.next.WebhookSecret) == "" && h.secrets.Available() {
				if err := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.WebhookSecret, "repository_binding:"+locked.ID+":webhook"); err != nil {
					return err
				}
			}
			return tx.Save(&prep.next).Error
		})
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("git.repository_binding_not_found", "repository binding not found")
		}
		if txErr != nil {
			return nil, aitool.BadRequest("git.repository_binding_update_failed", txErr.Error())
		}
	}

	h.auditWithContext(tc.User.ID, "repository_binding.update", prep.next.ID, true, prep.next.Owner+"/"+prep.next.Repo, tc.Context())
	resp := repositoryBindingResponse{
		RepositoryBinding:  prep.next,
		WebhookCallbackURL: h.gitWebhookURL(tc.Gin, prep.next.ID),
	}
	h.observeRepositoryBinding(tc.Context(), tc.User, &resp)
	return repositoryBindingToMap(resp, prep.project.ID), nil
}

// --- 9. build_unbind_repository ---

func (h *Handler) checkUnbindRepository(tc *aitool.ToolContext, in gitUnbindRepositoryInput) (model.Project, model.RepositoryBinding, error) {
	h.syncGinUser(tc)
	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionGitWrite)
	if !ok {
		return model.Project{}, model.RepositoryBinding{}, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	var appFilterID string
	if appRef := trimPtr(in.Application); appRef != "" {
		app, err := aitool.ResolveApplication(tc.DB, project.ID, appRef)
		if err != nil {
			return model.Project{}, model.RepositoryBinding{}, err
		}
		appFilterID = app.ID
	}
	binding, err := h.aiResolveRepositoryBinding(tc.DB, project.ID, appFilterID, in.Binding)
	if err != nil {
		return model.Project{}, model.RepositoryBinding{}, err
	}
	var targets []model.DeploymentTarget
	if err := tc.DB.Select("id", "name", "stage").
		Where("project_id = ? AND repository_binding_id = ? AND deleted_at IS NULL", project.ID, binding.ID).
		Find(&targets).Error; err != nil {
		return model.Project{}, model.RepositoryBinding{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if len(targets) > 0 {
		targetRefs := make([]string, len(targets))
		for i, t := range targets {
			targetRefs[i] = fmt.Sprintf("%s (%s)", t.Name, t.Stage)
		}
		te := aitool.Conflict("git.repository_binding_in_use", fmt.Sprintf("repository binding is referenced by deployment target(s): %s", strings.Join(targetRefs, ", ")))
		te.Details = map[string]any{"targets": targetRefs}
		return model.Project{}, model.RepositoryBinding{}, te
	}
	return project, binding, nil
}

func (h *Handler) aiPreviewUnbindRepository(tc *aitool.ToolContext, in gitUnbindRepositoryInput) (*aitool.PreviewResult, error) {
	project, binding, err := h.checkUnbindRepository(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Unbind repository %s/%s (%s)", binding.Owner, binding.Repo, binding.ID),
			TargetResource: "repository_binding",
			TargetID:       binding.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "repositoryBinding", Before: binding.Owner + "/" + binding.Repo, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteUnbindRepository(tc *aitool.ToolContext, in gitUnbindRepositoryInput) (map[string]any, error) {
	project, binding, err := h.checkUnbindRepository(tc, in)
	if err != nil {
		return nil, err
	}
	h.bestEffortDeleteRemoteWebhook(tc, binding)
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.RepositoryBinding
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ? AND project_id = ?", binding.ID, project.ID).Error; err != nil {
			return err
		}
		var targetCount int64
		if err := tx.Model(&model.DeploymentTarget{}).
			Where("project_id = ? AND repository_binding_id = ? AND deleted_at IS NULL", locked.ProjectID, locked.ID).
			Count(&targetCount).Error; err != nil {
			return err
		}
		if targetCount > 0 {
			return errRepositoryBindingInUse
		}
		if h.secrets.Available() {
			if err := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.WebhookSecret, "repository_binding:"+locked.ID+":webhook"); err != nil {
				return err
			}
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("git.repository_binding_not_found", "repository binding not found")
	}
	if errors.Is(txErr, errRepositoryBindingInUse) {
		return nil, aitool.Conflict("git.repository_binding_in_use", "repository binding is referenced by deployment targets")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "git.repository_unbind_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "repository_binding.delete", binding.ID, true, binding.Owner+"/"+binding.Repo, tc.Context())
	return map[string]any{
		"id":            binding.ID,
		"projectId":     project.ID,
		"applicationId": binding.ApplicationID,
		"unbound":       true,
		"deleted":       true,
	}, nil
}

// --- 11 & 12. admin_save_git_provider & admin_delete_git_provider ---

type preparedAdminGitProviderSave struct {
	isCreate               bool
	existing               model.GitProvider
	target                 model.GitProvider
	clientSecret           string
	clearClientCredentials bool
}

func (h *Handler) aiResolveGitProviderAdmin(db *gorm.DB, ref string) (model.GitProvider, error) {
	var all []model.GitProvider
	if err := db.Order("created_at asc").Find(&all).Error; err != nil {
		return model.GitProvider{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		all,
		"git_provider",
		"git.provider_not_found",
		ref,
		func(p model.GitProvider) string { return p.ID },
		func(p model.GitProvider) string { return p.Name },
	)
}

func resolveAdminOwnerUser(db *gorm.DB, ref string) (model.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.User{}, aitool.BadRequest("user.required", "ownerUser is required")
	}
	var u model.User
	if err := db.Where("(id = ? OR lower(email) = lower(?)) AND deleted_at IS NULL", ref, ref).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, aitool.NotFound("user.not_found", "ownerUser was not found")
		}
		return model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return u, nil
}

func validateProviderBaseURL(providerType, rawBaseURL string) (string, error) {
	normalized := normalizeGitBaseURL(providerType, rawBaseURL)
	if normalized == "" {
		return "", aitool.BadRequest("git.provider_base_url_required", "baseUrl is required for gitea and gitlab providers")
	}
	parsed, err := url.Parse(normalized)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", aitool.BadRequest("git.provider_base_url_invalid", "baseUrl must be a valid http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", aitool.BadRequest("git.provider_base_url_invalid", "baseUrl must not contain embedded credentials, query parameters, or fragments")
	}
	return normalized, nil
}

func (h *Handler) prepareSaveGitProviderAdmin(tc *aitool.ToolContext, in adminSaveGitProviderInput) (preparedAdminGitProviderSave, error) {
	var prep preparedAdminGitProviderSave
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return prep, aitool.Forbidden("auth.platform_admin_required", "platform administrator role is required")
	}
	clearCreds := boolPtrDefault(in.ClearClientCredentials, false)
	clientSecret := trimPtr(in.ClientSecret)
	if clearCreds && (trimPtr(in.ClientID) != "" || clientSecret != "") {
		return prep, aitool.BadRequest("argument_invalid", "cannot pass clientId or clientSecret when clearClientCredentials is true")
	}
	addIDs, err := resolveProjectRefs(tc.DB, in.AddProjects)
	if err != nil {
		return prep, err
	}
	removeIDs, err := resolveProjectRefs(tc.DB, in.RemoveProjects)
	if err != nil {
		return prep, err
	}

	provRef := trimPtr(in.Provider)
	if provRef == "" {
		// Create
		prep.isCreate = true
		rawType := strings.ToLower(trimPtr(in.Type))
		if rawType == "" {
			return prep, aitool.BadRequest("argument_invalid", "type (github|gitea|gitlab) is required when creating a GitProvider")
		}
		name := trimPtr(in.Name)
		if name == "" {
			return prep, aitool.BadRequest("argument_invalid", "name is required when creating a GitProvider")
		}
		providerType := normalizeGitProviderType(rawType)
		scope := strings.ToLower(fallback(trimPtr(in.Scope), "global"))
		var ownerRef string
		var projectIDs []string

		if providerType == "github" {
			if (in.Scope != nil && scope != "global") || trimPtr(in.OwnerUser) != "" || len(addIDs) > 0 {
				return prep, aitool.NewErrorWithHint(
					http.StatusBadRequest,
					"argument_invalid",
					"GitHub provider is a single global instance and cannot specify non-global scope, ownerUser, or addProjects",
					"Omit scope/ownerUser/addProjects or pass scope=\"global\" for type=\"github\".",
				)
			}
			var existingCount int64
			if err := tc.DB.Model(&model.GitProvider{}).Where("type = ?", "github").Count(&existingCount).Error; err != nil {
				return prep, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if existingCount > 0 {
				return prep, aitool.Conflict("git.github_provider_exists", "a GitHub provider already exists; update the existing provider instead")
			}
			scope = "global"
		} else {
			switch scope {
			case "global":
				if trimPtr(in.OwnerUser) != "" || len(addIDs) > 0 {
					return prep, aitool.BadRequest("argument_invalid", "ownerUser and addProjects are not allowed when scope=global")
				}
			case "project":
				projectIDs = mergeIncrementalProjectIDs(nil, addIDs, removeIDs)
				if len(projectIDs) == 0 {
					return prep, aitool.BadRequest("argument_invalid", "addProjects must be non-empty when scope=project")
				}
			case "user":
				if ownerRaw := trimPtr(in.OwnerUser); ownerRaw != "" {
					u, uErr := resolveAdminOwnerUser(tc.DB, ownerRaw)
					if uErr != nil {
						return prep, uErr
					}
					ownerRef = u.ID
				} else {
					ownerRef = tc.User.ID
				}
			default:
				return prep, aitool.BadRequest("argument_invalid", "invalid scope")
			}
		}

		baseURL, urlErr := validateProviderBaseURL(providerType, trimPtr(in.BaseURL))
		if urlErr != nil {
			return prep, urlErr
		}

		authType := "oauth"
		if providerType == "gitlab" {
			if in.AuthType != nil && strings.ToLower(strings.TrimSpace(*in.AuthType)) != "pat" {
				return prep, aitool.BadRequest("argument_invalid", "GitLab providers currently only support authType=\"pat\"")
			}
			authType = "pat"
		} else if in.AuthType != nil {
			authType = normalizeGitAuthType(*in.AuthType)
		}

		clientID := trimPtr(in.ClientID)
		if clearCreds {
			clientID = ""
		}
		prep.target = model.GitProvider{
			ID:         id.New("gitp"),
			Type:       providerType,
			Name:       name,
			BaseURL:    baseURL,
			Scope:      scope,
			OwnerRef:   ownerRef,
			ProjectIDs: projectIDs,
			AuthType:   authType,
			ClientID:   clientID,
			Enabled:    boolPtrDefault(in.Enabled, true),
		}
		prep.clientSecret = clientSecret
		prep.clearClientCredentials = clearCreds
		return prep, nil
	}

	// Update
	existing, err := h.aiResolveGitProviderAdmin(tc.DB, provRef)
	if err != nil {
		return prep, err
	}
	existing.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitProvider, existing.ID, tc.Context())
	next := existing

	if in.Type != nil {
		t := normalizeGitProviderType(*in.Type)
		if t != existing.Type {
			return prep, aitool.BadRequest("argument_invalid", "cannot change type of an existing GitProvider")
		}
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return prep, aitool.BadRequest("argument_invalid", "name cannot be empty")
		}
		next.Name = n
	}
	if in.BaseURL != nil || next.Type == "github" {
		rawURL := next.BaseURL
		if in.BaseURL != nil {
			rawURL = *in.BaseURL
		}
		baseURL, urlErr := validateProviderBaseURL(next.Type, rawURL)
		if urlErr != nil {
			return prep, urlErr
		}
		next.BaseURL = baseURL
	}

	if next.Type == "github" {
		if (in.Scope != nil && strings.ToLower(strings.TrimSpace(*in.Scope)) != "global") || trimPtr(in.OwnerUser) != "" || len(addIDs) > 0 {
			return prep, aitool.BadRequest("argument_invalid", "GitHub provider must remain scope=global without ownerUser or addProjects")
		}
		next.Scope = "global"
		next.OwnerRef = ""
		next.ProjectIDs = nil
	} else {
		if in.Scope != nil && strings.TrimSpace(*in.Scope) != "" {
			next.Scope = strings.ToLower(strings.TrimSpace(*in.Scope))
		}
		switch next.Scope {
		case "global":
			next.OwnerRef = ""
			next.ProjectIDs = nil
		case "project":
			next.OwnerRef = ""
			next.ProjectIDs = mergeIncrementalProjectIDs(existing.ProjectIDs, addIDs, removeIDs)
			if len(next.ProjectIDs) == 0 {
				return prep, aitool.BadRequest("argument_invalid", "scope=project requires at least one bound project")
			}
		case "user":
			next.ProjectIDs = nil
			if ownerRaw := trimPtr(in.OwnerUser); ownerRaw != "" {
				u, uErr := resolveAdminOwnerUser(tc.DB, ownerRaw)
				if uErr != nil {
					return prep, uErr
				}
				next.OwnerRef = u.ID
			} else if existing.Scope == "user" && strings.TrimSpace(existing.OwnerRef) != "" {
				next.OwnerRef = existing.OwnerRef
			} else {
				next.OwnerRef = tc.User.ID
			}
		default:
			return prep, aitool.BadRequest("argument_invalid", "invalid scope")
		}
	}

	if in.AuthType != nil {
		at := strings.ToLower(strings.TrimSpace(*in.AuthType))
		if next.Type == "gitlab" && at != "pat" {
			return prep, aitool.BadRequest("argument_invalid", "GitLab providers only support authType=\"pat\"")
		}
		next.AuthType = normalizeGitAuthType(at)
	}
	if clearCreds {
		next.ClientID = ""
	} else if in.ClientID != nil {
		next.ClientID = strings.TrimSpace(*in.ClientID)
	}
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}

	if compatErr := ensureGitProviderScopeCompatibleTx(tc.DB, next); compatErr != nil {
		if errors.Is(compatErr, errGitCredentialScopeConflict) {
			return prep, aitool.Conflict("git.credential_scope_conflict", "child Git accounts exceed the target provider scope")
		}
		return prep, aitool.Conflict("git.provider_in_use", "Git provider is referenced by repository bindings outside the target scope")
	}

	prep.existing = existing
	prep.target = next
	prep.clientSecret = clientSecret
	prep.clearClientCredentials = clearCreds
	return prep, nil
}

func (h *Handler) aiPreviewSaveGitProviderAdmin(tc *aitool.ToolContext, in adminSaveGitProviderInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareSaveGitProviderAdmin(tc, in)
	if err != nil {
		return nil, err
	}
	var sensitiveKeys []string
	if prep.clientSecret != "" || prep.clearClientCredentials {
		sensitiveKeys = append(sensitiveKeys, "clientSecret")
	}
	summary := fmt.Sprintf("Create %s GitProvider %q (%s)", prep.target.Type, prep.target.Name, prep.target.Scope)
	targetID := ""
	var changes []aitool.PreviewChange
	if !prep.isCreate {
		targetID = prep.existing.ID
		summary = fmt.Sprintf("Update GitProvider %q (%s)", prep.target.Name, prep.existing.ID)
		if prep.existing.Name != prep.target.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: prep.existing.Name, After: prep.target.Name, Action: "update"})
		}
		if prep.existing.BaseURL != prep.target.BaseURL {
			changes = append(changes, aitool.PreviewChange{Field: "baseUrl", Before: prep.existing.BaseURL, After: prep.target.BaseURL, Action: "update"})
		}
		if prep.existing.Scope != prep.target.Scope {
			changes = append(changes, aitool.PreviewChange{Field: "scope", Before: prep.existing.Scope, After: prep.target.Scope, Action: "update"})
		}
		if prep.existing.Enabled != prep.target.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: prep.existing.Enabled, After: prep.target.Enabled, Action: "update"})
		}
	} else {
		changes = append(changes,
			aitool.PreviewChange{Field: "type", After: prep.target.Type, Action: "create"},
			aitool.PreviewChange{Field: "name", After: prep.target.Name, Action: "create"},
			aitool.PreviewChange{Field: "baseUrl", After: prep.target.BaseURL, Action: "create"},
		)
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "git_provider",
			TargetID:       targetID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
	}, nil
}

func (h *Handler) aiExecuteSaveGitProviderAdmin(tc *aitool.ToolContext, in adminSaveGitProviderInput) (map[string]any, error) {
	prep, err := h.prepareSaveGitProviderAdmin(tc, in)
	if err != nil {
		return nil, err
	}
	if prep.isCreate {
		provider := prep.target
		txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
			if prep.clientSecret != "" && h.secrets.Available() {
				ref, storeErr := h.secrets.StoreContextWithDB(tc.Context(), tx, prep.clientSecret, tc.User.ID, "git_provider:"+provider.ID)
				if storeErr != nil {
					return storeErr
				}
				provider.ClientSecretRef = ref
			}
			if err := tx.Create(&provider).Error; err != nil {
				return err
			}
			return h.replaceScopedResourceProjectBindings(tx, scopedResourceGitProvider, provider.ID, sortedProjectIDs(provider.ProjectIDs), nil)
		})
		if txErr != nil {
			return nil, aitool.BadRequest("git.provider_save_failed", txErr.Error())
		}
		h.auditWithContext(tc.User.ID, "git_provider.create", provider.ID, true, provider.Type, tc.Context())
		return map[string]any(gitProviderResponse(provider)), nil
	}

	var updated model.GitProvider
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", prep.existing.ID).Error; err != nil {
			return err
		}
		locked.Name = prep.target.Name
		locked.BaseURL = prep.target.BaseURL
		locked.Scope = prep.target.Scope
		locked.OwnerRef = prep.target.OwnerRef
		locked.ProjectIDs = prep.target.ProjectIDs
		locked.AuthType = prep.target.AuthType
		locked.ClientID = prep.target.ClientID
		locked.Enabled = prep.target.Enabled
		if compatErr := ensureGitProviderScopeCompatibleTx(tx, locked); compatErr != nil {
			return compatErr
		}
		resource := "git_provider:" + locked.ID
		if prep.clearClientCredentials {
			if strings.TrimSpace(locked.ClientSecretRef) != "" && h.secrets.Available() {
				if delErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.ClientSecretRef, resource); delErr != nil {
					return delErr
				}
			}
			locked.ClientSecretRef = ""
		} else if prep.clientSecret != "" && h.secrets.Available() {
			newRef, storeErr := h.secrets.StoreContextWithDB(tc.Context(), tx, prep.clientSecret, tc.User.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(locked.ClientSecretRef) != "" && locked.ClientSecretRef != newRef {
				if delErr := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.ClientSecretRef, resource); delErr != nil {
					return delErr
				}
			}
			locked.ClientSecretRef = newRef
		}
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceGitProvider, locked.ID, sortedProjectIDs(locked.ProjectIDs), nil); err != nil {
			return err
		}
		updated = locked
		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("git.provider_not_found", "git provider not found")
	}
	if errors.Is(txErr, errGitCredentialScopeConflict) {
		return nil, aitool.Conflict("git.credential_scope_conflict", "child Git accounts exceed the target provider scope")
	}
	if errors.Is(txErr, errGitProviderInUse) {
		return nil, aitool.Conflict("git.provider_in_use", "Git provider is referenced by repository bindings outside the target scope")
	}
	if txErr != nil {
		return nil, aitool.BadRequest("git.provider_save_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "git_provider.update", updated.ID, true, updated.Type, tc.Context())
	return map[string]any(gitProviderResponse(updated)), nil
}

func (h *Handler) checkDeleteGitProviderAdmin(tc *aitool.ToolContext, in adminDeleteGitProviderInput) (model.GitProvider, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return model.GitProvider{}, aitool.Forbidden("auth.platform_admin_required", "platform administrator role is required")
	}
	provider, err := h.aiResolveGitProviderAdmin(tc.DB, in.Provider)
	if err != nil {
		return model.GitProvider{}, err
	}
	accountCount, bindingCount, countErr := countGitProviderUsage(tc.DB, provider.ID)
	if countErr != nil {
		return model.GitProvider{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", countErr.Error())
	}
	if accountCount > 0 || bindingCount > 0 {
		te := aitool.Conflict("git.provider_in_use", fmt.Sprintf("GitProvider %s cannot be deleted while referenced by %d GitAccount(s) and %d RepositoryBinding(s)", provider.ID, accountCount, bindingCount))
		te.Details = map[string]any{
			"accountCount": accountCount,
			"bindingCount": bindingCount,
		}
		return model.GitProvider{}, te
	}
	return provider, nil
}

func countGitProviderUsage(db *gorm.DB, providerID string) (int64, int64, error) {
	var accounts []model.GitAccount
	if err := db.Where("provider_id = ?", providerID).Find(&accounts).Error; err != nil {
		return 0, 0, err
	}
	accountIDs := gitAccountIDs(accounts)
	bindingQuery := db.Model(&model.RepositoryBinding{}).
		Joins("JOIN projects ON projects.id = repository_bindings.project_id AND projects.deleted_at IS NULL")
	if len(accountIDs) > 0 {
		bindingQuery = bindingQuery.Where("repository_bindings.git_provider_id = ? OR repository_bindings.git_account_id IN ?", providerID, accountIDs)
	} else {
		bindingQuery = bindingQuery.Where("repository_bindings.git_provider_id = ?", providerID)
	}
	var bindingCount int64
	if err := bindingQuery.Count(&bindingCount).Error; err != nil {
		return 0, 0, err
	}
	return int64(len(accounts)), bindingCount, nil
}

func (h *Handler) aiPreviewDeleteGitProviderAdmin(tc *aitool.ToolContext, in adminDeleteGitProviderInput) (*aitool.PreviewResult, error) {
	provider, err := h.checkDeleteGitProviderAdmin(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete GitProvider %q (%s)", provider.Name, provider.ID),
			TargetResource: "git_provider",
			TargetID:       provider.ID,
			Changes: []aitool.PreviewChange{
				{Field: "gitProvider", Before: provider.Name, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteDeleteGitProviderAdmin(tc *aitool.ToolContext, in adminDeleteGitProviderInput) (map[string]any, error) {
	provider, err := h.checkDeleteGitProviderAdmin(tc, in)
	if err != nil {
		return nil, err
	}
	var accountCount, bindingCount int64
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.GitProvider
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", provider.ID).Error; err != nil {
			return err
		}
		var countErr error
		accountCount, bindingCount, countErr = countGitProviderUsage(tx, locked.ID)
		if countErr != nil {
			return countErr
		}
		if accountCount > 0 || bindingCount > 0 {
			return errGitProviderInUse
		}
		if h.secrets.Available() {
			if err := h.secrets.DeleteRefContextWithDB(tc.Context(), tx, locked.ClientSecretRef, "git_provider:"+locked.ID); err != nil {
				return err
			}
		}
		if err := tx.Where("resource_type = ? AND resource_id = ?", scopedResourceGitProvider, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("git.provider_not_found", "git provider not found")
	}
	if errors.Is(txErr, errGitProviderInUse) {
		te := aitool.Conflict("git.provider_in_use", fmt.Sprintf("GitProvider %s cannot be deleted while referenced by %d GitAccount(s) and %d RepositoryBinding(s)", provider.ID, accountCount, bindingCount))
		te.Details = map[string]any{"accountCount": accountCount, "bindingCount": bindingCount}
		return nil, te
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "git.provider_delete_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "git_provider.delete", provider.ID, true, provider.Type, tc.Context())
	return map[string]any{
		"id":      provider.ID,
		"deleted": true,
	}, nil
}

func repositoryWebhookToolError(err error) *aitool.ToolError {
	switch {
	case errors.Is(err, errRepositoryWebhookCompensation):
		return aitool.NewError(http.StatusBadGateway, "git.webhook_compensation_failed", err.Error())
	case errors.Is(err, errRepositoryWebhookPersistence):
		return aitool.NewError(http.StatusInternalServerError, "git.webhook_persistence_failed", err.Error())
	default:
		return gitUpstreamToolError(err)
	}
}

func repositoryBindingToMap(resp repositoryBindingResponse, projectID string) map[string]any {
	return map[string]any{
		"id":                     resp.ID,
		"projectId":              projectID,
		"applicationId":          resp.ApplicationID,
		"applicationName":        resp.ApplicationName,
		"gitProviderId":          resp.GitProviderID,
		"providerName":           resp.ProviderName,
		"providerType":           resp.ProviderType,
		"gitAccountId":           resp.GitAccountID,
		"accountUsername":        resp.AccountUsername,
		"owner":                  resp.Owner,
		"repo":                   resp.Repo,
		"cloneUrl":               resp.CloneURL,
		"defaultBranch":          resp.DefaultBranch,
		"webhookEnabled":         resp.WebhookEnabled,
		"webhookId":              resp.WebhookID,
		"webhookCallbackUrl":     resp.WebhookCallbackURL,
		"webhookStatus":          resp.WebhookStatus,
		"webhookObservationCode": resp.WebhookObservationCode,
		"webhookObservedAt":      resp.WebhookObservedAt,
		"lastEvent":              resp.LastEvent,
		"lastCommitSha":          resp.LastCommitSHA,
		"lastWebhookAt":          resp.LastWebhookAt,
		"createdAt":              resp.CreatedAt,
	}
}

var _ = time.Second
