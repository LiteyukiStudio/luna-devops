package gitapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/LiteyukiStudio/devops/internal/security"
	"gorm.io/gorm"
)

// RegisterAITools registers all 12 Git domain AI facade tools (9 build_* + 3 admin_*).
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerGitReadAITools(reg)
	h.registerGitWriteAITools(reg)
	h.registerGitAdminAITools(reg)
}

type gitListSourcesInput struct {
	Project          *string `json:"project,omitempty" desc:"可选，按项目 ID 或 identifier 筛选可见范围"`
	GitAccount       *string `json:"gitAccount,omitempty" desc:"可选，按 Git 账号 ID（gita_xxx）或可见范围内用户名精确过滤"`
	Search           *string `json:"search,omitempty" desc:"可选，按名称、地址或用户名模糊搜索"`
	IncludeProviders *bool   `json:"includeProviders,omitempty" default:"true" desc:"是否返回可见 GitProvider 分页列表，默认 true"`
	IncludeAccounts  *bool   `json:"includeAccounts,omitempty" default:"true" desc:"是否返回可见 GitAccount 分页列表及实时连通状态，默认 true"`
	Page             *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize         *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

type gitSearchRepositoriesInput struct {
	GitAccount    string  `json:"gitAccount" desc:"必填，可见 Git 账号 ID（gita_xxx）、唯一用户名，或传 anonymous 搜索公开仓库"`
	Provider      *string `json:"provider,omitempty" desc:"可选，GitProvider ID 或名称；gitAccount=anonymous 时必填，或用于跨平台同名账号消歧"`
	Search        *string `json:"search,omitempty" desc:"可选（gitAccount=anonymous 时必填），仓库名称关键词"`
	IncludePublic *bool   `json:"includePublic,omitempty" default:"false" desc:"账号仓库无匹配时是否回退搜索公开仓库，默认 false"`
	Page          *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize      *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

type gitInspectRepositoryCodeInput struct {
	Project             *string `json:"project,omitempty" desc:"可选，使用 repositoryBinding 定位时必填（项目 ID 或 identifier）"`
	Application         *string `json:"application,omitempty" desc:"可选，同项目多应用绑定同一仓库时传入应用 ID 或 identifier 消歧"`
	RepositoryBinding   *string `json:"repositoryBinding,omitempty" desc:"可选，已绑定仓库 ID（rpb_xxx）或 owner/repo（与 gitAccount+owner+repo 二选一）"`
	GitAccount          *string `json:"gitAccount,omitempty" desc:"可选，未绑定仓库时传可见 Git 账号 ID/用户名或 anonymous"`
	Provider            *string `json:"provider,omitempty" desc:"可选，gitAccount=anonymous 或同名账号消歧时传入 GitProvider ID 或名称"`
	Owner               *string `json:"owner,omitempty" desc:"可选，传 gitAccount 时必填，仓库组织或用户名"`
	Repo                *string `json:"repo,omitempty" desc:"可选，传 gitAccount 时必填，仓库名称"`
	Ref                 *string `json:"ref,omitempty" desc:"可选，分支、Tag 或 Commit SHA（省略取仓库默认分支）"`
	IncludeBranches     *bool   `json:"includeBranches,omitempty" default:"true" desc:"是否返回分支列表（上限 50 条），默认 true"`
	BranchSearch        *string `json:"branchSearch,omitempty" desc:"可选，分支名过滤关键词"`
	IncludeBuildOptions *bool   `json:"includeBuildOptions,omitempty" default:"false" desc:"是否扫描仓库返回 Dockerfile、目录、暴露端口与推荐构建模板，默认 false"`
	Path                *string `json:"path,omitempty" desc:"可选，传 / 或 . 列出仓库根目录条目；传子目录路径返回目录条目；传文件路径返回截断文本内容"`
	MaxFileBytes        *int    `json:"maxFileBytes,omitempty" min:"256" max:"65536" default:"16384" desc:"读取文件时的最大字节数，默认 16384，最大 65536"`
}

type gitListRepositoryBindingsInput struct {
	Project     string  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application *string `json:"application,omitempty" desc:"可选，应用 ID、identifier 或唯一名称"`
	Binding     *string `json:"binding,omitempty" desc:"可选，按仓库绑定 ID（rpb_xxx）或 owner/repo 精确过滤"`
	Search      *string `json:"search,omitempty" desc:"可选，按 owner/repo、应用名或账号搜索"`
	SortBy      *string `json:"sortBy,omitempty" enum:"createdAt|repo|owner" default:"createdAt" desc:"排序字段"`
	SortOrder   *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page        *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize    *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

type adminListGitProvidersInput struct {
	Provider *string `json:"provider,omitempty" desc:"可选，按 GitProvider ID（gitp_xxx）或名称精确过滤"`
	Scope    *string `json:"scope,omitempty" enum:"global|project|user" desc:"可选，按作用域筛选"`
	Enabled  *bool   `json:"enabled,omitempty" desc:"可选，按启用状态筛选（省略时包含禁用项）"`
	Search   *string `json:"search,omitempty" desc:"可选，按名称或 baseUrl 搜索"`
	Page     *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码，默认 1"`
	PageSize *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数，默认 20，最大 50"`
}

func (h *Handler) registerGitReadAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[gitListSourcesInput, map[string]any]{
		Name:           "build_list_git_sources",
		Domain:         aitool.DomainBuild,
		Title:          "查看可用 Git 平台与账号凭据",
		Summary:        "分页查看当前可见的 Git 平台提供商（GitProvider）与已连接 Git 账号/凭据（GitAccount）及实时连通状态",
		Tags:           []string{"git", "provider", "account", "credential"},
		AliasesZH:      []string{"Git账号列表", "Git凭据", "代码源提供商"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionGitRead)},
		Execute:        h.aiListGitSources,
	})

	aitool.Register(reg, aitool.Spec[gitSearchRepositoriesInput, map[string]any]{
		Name:           "build_search_git_repositories",
		Domain:         aitool.DomainBuild,
		Title:          "搜索远端 Git 仓库",
		Summary:        "通过可见 Git 账号或匿名搜索远端 GitHub / Gitea 仓库列表",
		Tags:           []string{"git", "repository", "search"},
		AliasesZH:      []string{"搜索代码仓库", "查找Git仓库"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionGitRead)},
		Execute:        h.aiSearchGitRepositories,
	})

	aitool.Register(reg, aitool.Spec[gitInspectRepositoryCodeInput, map[string]any]{
		Name:           "build_inspect_repository_code",
		Domain:         aitool.DomainBuild,
		Title:          "探查远端代码仓库内容与构建建议",
		Summary:        "探查远端 GitHub / Gitea 仓库的分支、构建探测建议（Dockerfile/暴露端口/推荐模板）、目录结构或单个文本文件",
		Tags:           []string{"git", "repository", "branch", "dockerfile", "file"},
		AliasesZH:      []string{"查看仓库文件", "仓库分支", "构建探测"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionGitRead)},
		Execute:        h.aiInspectRepositoryCode,
	})

	aitool.Register(reg, aitool.Spec[gitListRepositoryBindingsInput, map[string]any]{
		Name:           "build_list_repository_bindings",
		Domain:         aitool.DomainBuild,
		Title:          "查看应用代码仓库绑定",
		Summary:        "分页列出项目下应用绑定的代码仓库及实时 Webhook 状态",
		Tags:           []string{"git", "repository", "binding", "webhook"},
		AliasesZH:      []string{"仓库绑定列表", "Webhook状态"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionGitRead)},
		Execute:        h.aiListRepositoryBindings,
	})
}

func (h *Handler) aiResolveOptionalProject(tc *aitool.ToolContext, rawProject *string, action authz.Action) (string, error) {
	ref := trimPtr(rawProject)
	if ref == "" && strings.TrimSpace(tc.ConversationProjectID) != "" {
		ref = strings.TrimSpace(tc.ConversationProjectID)
	}
	if ref == "" {
		return "", nil
	}
	if _, project, ok := h.authorizeProjectRef(tc.Gin, ref, action); ok {
		return project.ID, nil
	}
	return "", tc.AbortedError("project.forbidden", "project is not accessible")
}

func (h *Handler) aiListGitSources(tc *aitool.ToolContext, in gitListSourcesInput) (map[string]any, error) {
	h.syncGinUser(tc)
	effectiveProjectID, err := h.aiResolveOptionalProject(tc, in.Project, authz.ActionGitRead)
	if err != nil {
		return nil, err
	}
	includeProviders := boolPtrDefault(in.IncludeProviders, true)
	includeAccounts := boolPtrDefault(in.IncludeAccounts, true)
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	search := trimPtr(in.Search)

	visibility := projectservice.ListVisibilityRelated
	if effectiveProjectID == "" && authz.IsPlatformAdmin(tc.User.Role) {
		visibility = projectservice.ListVisibilityAll
	}

	out := map[string]any{}
	if effectiveProjectID != "" {
		out["projectId"] = effectiveProjectID
	}

	if includeProviders {
		query := tc.DB.Model(&model.GitProvider{})
		if !authz.IsPlatformAdmin(tc.User.Role) {
			query = query.Where("enabled = ?", true)
		}
		var visible bool
		query, visible = h.applyScopedResourceListVisibility(tc.Gin, query, scopedResourceGitProvider, tc.User, effectiveProjectID, visibility)
		if !visible {
			return nil, tc.AbortedError("git.provider_forbidden", "cannot list git providers")
		}
		if search != "" {
			like := "%" + search + "%"
			query = query.Where("name ILIKE ? OR base_url ILIKE ?", like, like)
		}
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var providers []model.GitProvider
		if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&providers).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		responses, respErr := h.gitProviderResponsesForUser(tc.User, providers, tc.Context())
		if respErr != nil {
			return nil, aitool.NewError(http.StatusForbidden, "git.provider_forbidden", respErr.Error())
		}
		out["providers"] = aitool.PageEnvelope(responses, page, pageSize, "createdAt", "desc", total)
	}

	if includeAccounts {
		tc.SetNoStore()
		query := tc.DB.Model(&model.GitAccount{})
		var visible bool
		query, visible = h.applyScopedResourceListVisibility(tc.Gin, query, scopedResourceGitAccount, tc.User, effectiveProjectID, visibility)
		if !visible {
			return nil, tc.AbortedError("git.account_forbidden", "cannot list git accounts")
		}
		if accountFilter := trimPtr(in.GitAccount); accountFilter != "" {
			query = query.Where("id = ? OR lower(username) = lower(?)", accountFilter, accountFilter)
		}
		if search != "" {
			like := "%" + search + "%"
			query = query.Where("username ILIKE ? OR external_user_id ILIKE ?", like, like)
		}
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var accounts []model.GitAccount
		if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&accounts).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if accountFilter := trimPtr(in.GitAccount); accountFilter != "" && len(accounts) > 1 {
			for _, acc := range accounts {
				if acc.ID == accountFilter {
					accounts = []model.GitAccount{acc}
					total = 1
					break
				}
			}
		}
		h.attachGitAccountProjects(accounts, tc.Context())
		h.observeGitAccounts(tc.Context(), tc.User, accounts)
		out["accounts"] = aitool.PageEnvelope(gitAccountResponses(accounts), page, pageSize, "createdAt", "desc", total)
	}

	return out, nil
}

func (h *Handler) aiSearchGitRepositories(tc *aitool.ToolContext, in gitSearchRepositoriesInput) (map[string]any, error) {
	h.syncGinUser(tc)
	tc.SetNoStore()
	effectiveProjectID := strings.TrimSpace(tc.ConversationProjectID)
	accountRef := strings.TrimSpace(in.GitAccount)
	if accountRef == "" {
		return nil, aitool.BadRequest("git.account_required", "gitAccount is required")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	search := trimPtr(in.Search)

	var (
		client   gitprovider.Client
		provider model.GitProvider
		err      error
	)
	if accountRef == anonymousGitAccountID {
		providerRef := trimPtr(in.Provider)
		if providerRef == "" {
			return nil, aitool.NewErrorWithHint(http.StatusBadRequest, "git.provider_required", "provider is required when gitAccount is anonymous", "Pass provider ID or name (e.g. GitHub) alongside gitAccount=\"anonymous\".")
		}
		if search == "" {
			return nil, aitool.NewErrorWithHint(http.StatusBadRequest, "git.search_required", "search keyword is required when gitAccount is anonymous", "Provide a non-empty search keyword to search public repositories.")
		}
		provider, err = h.aiResolveEnabledGitProvider(tc, providerRef, effectiveProjectID)
		if err != nil {
			return nil, err
		}
		if provider.Type == "gitlab" {
			return nil, aitool.NewErrorWithHint(http.StatusBadRequest, "git.provider_search_unsupported", "GitLab providers do not support remote repository search API", "Bind the GitLab repository directly with owner, repo, and cloneUrl.")
		}
		client = gitprovider.NewClientWithPolicy(provider, "", h.egressPolicyForUser(tc.User, tc.Context()))
		repos, searchErr := client.SearchPublicRepositories(tc.Context(), search, page, pageSize)
		if searchErr != nil {
			return nil, gitUpstreamToolError(searchErr)
		}
		out := aitool.PageEnvelope(repos, page, pageSize, "updatedAt", "desc", remoteTotalCount(page, pageSize, len(repos)))
		if effectiveProjectID != "" {
			out["projectId"] = effectiveProjectID
		}
		return out, nil
	}

	account, provider, err := h.aiResolveUsableGitAccountAndProvider(tc, accountRef, trimPtr(in.Provider), effectiveProjectID)
	if err != nil {
		return nil, err
	}
	if provider.Type == "gitlab" {
		return nil, aitool.NewErrorWithHint(http.StatusBadRequest, "git.provider_search_unsupported", "GitLab providers do not support remote repository search API", "Bind the GitLab repository directly with owner, repo, and cloneUrl.")
	}
	client, err = h.aiGitClientForAccount(tc, account, provider)
	if err != nil {
		return nil, err
	}
	repos, listErr := client.ListRepositories(tc.Context(), search, page, pageSize)
	if listErr != nil {
		return nil, gitUpstreamToolError(listErr)
	}
	if len(repos) == 0 && boolPtrDefault(in.IncludePublic, false) && search != "" {
		repos, listErr = client.SearchPublicRepositories(tc.Context(), search, page, pageSize)
		if listErr != nil {
			return nil, gitUpstreamToolError(listErr)
		}
	}
	out := aitool.PageEnvelope(repos, page, pageSize, "updatedAt", "desc", remoteTotalCount(page, pageSize, len(repos)))
	if effectiveProjectID != "" {
		out["projectId"] = effectiveProjectID
	}
	return out, nil
}

func (h *Handler) aiInspectRepositoryCode(tc *aitool.ToolContext, in gitInspectRepositoryCodeInput) (map[string]any, error) {
	h.syncGinUser(tc)
	tc.SetNoStore()

	bindingRef := trimPtr(in.RepositoryBinding)
	accountRef := trimPtr(in.GitAccount)
	if (bindingRef == "" && accountRef == "") || (bindingRef != "" && accountRef != "") {
		return nil, aitool.NewErrorWithHint(http.StatusBadRequest, "argument_invalid", "specify either repositoryBinding or gitAccount (+ owner + repo), not both", "Use repositoryBinding for bound repositories, or gitAccount + owner + repo for unbound repositories.")
	}

	var (
		effectiveProjectID string
		applicationID      string
		owner              string
		repo               string
		ref                = trimPtr(in.Ref)
		client             gitprovider.Client
		provider           model.GitProvider
	)

	if bindingRef != "" {
		projRef := trimPtr(in.Project)
		if projRef == "" {
			projRef = strings.TrimSpace(tc.ConversationProjectID)
		}
		if projRef == "" {
			return nil, aitool.BadRequest("project.required", "project is required when inspecting via repositoryBinding")
		}
		_, project, ok := h.authorizeProjectRef(tc.Gin, projRef, authz.ActionGitRead)
		if !ok {
			return nil, tc.AbortedError("project.forbidden", "project is not accessible")
		}
		effectiveProjectID = project.ID
		if appRef := trimPtr(in.Application); appRef != "" {
			app, appErr := aitool.ResolveApplication(tc.DB, project.ID, appRef)
			if appErr != nil {
				return nil, appErr
			}
			applicationID = app.ID
		}
		binding, bindErr := h.aiResolveRepositoryBinding(tc.DB, project.ID, applicationID, bindingRef)
		if bindErr != nil {
			return nil, bindErr
		}
		applicationID = binding.ApplicationID
		owner = binding.Owner
		repo = binding.Repo
		if ref == "" {
			ref = strings.TrimSpace(binding.DefaultBranch)
		}

		var account model.GitAccount
		if err := tc.DB.First(&account, "id = ?", binding.GitAccountID).Error; err != nil {
			return nil, aitool.NotFound("git.account_not_found", "git account bound to repository was not found")
		}
		if !h.gitAccountUsableInProject(tc.DB, tc.User, account, project.ID, tc.Context()) {
			return nil, aitool.Forbidden("git.credential_forbidden", "current user is not authorized to use the personal or cross-project Git credential bound to this repository")
		}
		if err := tc.DB.First(&provider, "id = ? AND enabled = ?", binding.GitProviderID, true).Error; err != nil {
			return nil, aitool.NotFound("git.provider_not_found", "git provider bound to repository is unavailable")
		}
		if !h.gitProviderUsableInProject(tc.DB, tc.User, provider, project.ID, tc.Context()) {
			return nil, aitool.Forbidden("git.provider_forbidden", "current user is not authorized to use the Git provider bound to this repository")
		}
		if provider.Type == "gitlab" {
			return nil, aitool.NewErrorWithHint(http.StatusBadRequest, "git.provider_inspect_unsupported", "GitLab providers do not support remote code inspection API", "Inspect GitHub or Gitea repositories, or configure Dockerfile path directly.")
		}
		var clientErr error
		client, clientErr = h.aiGitClientForAccount(tc, account, provider)
		if clientErr != nil {
			return nil, clientErr
		}
	} else {
		var projErr error
		effectiveProjectID, projErr = h.aiResolveOptionalProject(tc, in.Project, authz.ActionGitRead)
		if projErr != nil {
			return nil, projErr
		}
		owner = trimPtr(in.Owner)
		repo = trimPtr(in.Repo)
		if owner == "" || repo == "" {
			return nil, aitool.BadRequest("argument_invalid", "owner and repo are required when using gitAccount")
		}
		if accountRef == anonymousGitAccountID {
			providerRef := trimPtr(in.Provider)
			if providerRef == "" {
				return nil, aitool.BadRequest("git.provider_required", "provider is required when gitAccount is anonymous")
			}
			var provErr error
			provider, provErr = h.aiResolveEnabledGitProvider(tc, providerRef, effectiveProjectID)
			if provErr != nil {
				return nil, provErr
			}
			if provider.Type == "gitlab" {
				return nil, aitool.BadRequest("git.provider_inspect_unsupported", "GitLab providers do not support remote code inspection API")
			}
			client = gitprovider.NewClientWithPolicy(provider, "", h.egressPolicyForUser(tc.User, tc.Context()))
		} else {
			account, resolvedProv, accErr := h.aiResolveUsableGitAccountAndProvider(tc, accountRef, trimPtr(in.Provider), effectiveProjectID)
			if accErr != nil {
				return nil, accErr
			}
			provider = resolvedProv
			if provider.Type == "gitlab" {
				return nil, aitool.BadRequest("git.provider_inspect_unsupported", "GitLab providers do not support remote code inspection API")
			}
			var clientErr error
			client, clientErr = h.aiGitClientForAccount(tc, account, provider)
			if clientErr != nil {
				return nil, clientErr
			}
		}
	}

	out := map[string]any{
		"owner": owner,
		"repo":  repo,
		"ref":   ref,
	}
	if effectiveProjectID != "" {
		out["projectId"] = effectiveProjectID
	}
	if applicationID != "" {
		out["applicationId"] = applicationID
	}

	if boolPtrDefault(in.IncludeBranches, true) {
		branches, brErr := client.ListBranches(tc.Context(), owner, repo)
		if brErr != nil {
			return nil, gitUpstreamToolError(brErr)
		}
		filtered := filterGitBranches(branches, trimPtr(in.BranchSearch), 50)
		out["branches"] = map[string]any{
			"items":        filtered.items,
			"total":        len(branches),
			"matchedTotal": filtered.matchedTotal,
			"limited":      len(filtered.items) < filtered.matchedTotal,
		}
	}

	if boolPtrDefault(in.IncludeBuildOptions, false) {
		started := time.Now()
		options, optErr := client.DiscoverBuildOptions(tc.Context(), owner, repo, ref)
		if optErr != nil {
			return nil, gitUpstreamToolError(optErr)
		}
		out["buildOptions"] = map[string]any{
			"dockerfiles":            options.Dockerfiles,
			"directories":            options.Directories,
			"detectedFiles":          options.DetectedFiles,
			"recommendedTemplateIds": buildtemplate.Recommend(options.DetectedFiles),
			"exposedPorts":           options.ExposedPorts,
			"strategy":               options.Strategy,
			"truncated":              options.Truncated,
			"durationMs":             time.Since(started).Milliseconds(),
		}
	}

	if in.Path != nil && strings.TrimSpace(*in.Path) != "" {
		rawPath := strings.TrimSpace(*in.Path)
		maxBytes := 16384
		if in.MaxFileBytes != nil && *in.MaxFileBytes > 0 {
			maxBytes = *in.MaxFileBytes
		}
		if maxBytes < 256 {
			maxBytes = 256
		}
		if maxBytes > 65536 {
			maxBytes = 65536
		}

		if rawPath == "/" || rawPath == "." || strings.HasSuffix(rawPath, "/") {
			dirPath := strings.Trim(rawPath, "/")
			if dirPath == "." {
				dirPath = ""
			}
			entries, listErr := client.ListContents(tc.Context(), owner, repo, dirPath, ref)
			if listErr != nil {
				return nil, gitUpstreamToolError(listErr)
			}
			out["path"] = rawPath
			out["pathType"] = "directory"
			out["entries"] = entries
		} else {
			file, readErr := client.ReadFile(tc.Context(), owner, repo, rawPath, ref)
			if readErr != nil {
				var typeErr *json.UnmarshalTypeError
				if errors.As(readErr, &typeErr) {
					entries, listErr := client.ListContents(tc.Context(), owner, repo, rawPath, ref)
					if listErr != nil {
						return nil, gitUpstreamToolError(listErr)
					}
					out["path"] = rawPath
					out["pathType"] = "directory"
					out["entries"] = entries
					return out, nil
				}
				return nil, gitUpstreamToolError(readErr)
			}
			if isBinaryContent(file.Content) {
				return nil, aitool.BadRequest("git.binary_file_unsupported", "target path is a binary file and cannot be inspected as UTF-8 text")
			}
			truncatedContent, truncated := truncateUTF8Head(file.Content, maxBytes)
			out["path"] = file.Path
			out["pathType"] = "file"
			out["file"] = map[string]any{
				"path":      file.Path,
				"name":      file.Name,
				"sha":       file.SHA,
				"ref":       file.Ref,
				"encoding":  "utf-8",
				"sizeBytes": len(file.Content),
				"truncated": truncated,
				"content":   truncatedContent,
			}
		}
	}

	return out, nil
}

func (h *Handler) aiListRepositoryBindings(tc *aitool.ToolContext, in gitListRepositoryBindingsInput) (map[string]any, error) {
	h.syncGinUser(tc)
	tc.SetNoStore()
	user, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionGitRead)
	if !ok {
		return nil, tc.AbortedError("project.forbidden", "project is not accessible")
	}

	var applicationID string
	if appRef := trimPtr(in.Application); appRef != "" {
		app, err := aitool.ResolveApplication(tc.DB, project.ID, appRef)
		if err != nil {
			return nil, err
		}
		applicationID = app.ID
	}

	query := tc.DB.Table("repository_bindings").
		Select("repository_bindings.*, COALESCE(git_providers.name, '') as provider_name, COALESCE(git_providers.type, '') as provider_type, COALESCE(git_accounts.username, '') as account_username, COALESCE(users.email, '') as account_owner_email, COALESCE(users.name, '') as account_owner_name, COALESCE(applications.name, '') as application_name").
		Joins("LEFT JOIN git_providers ON git_providers.id = repository_bindings.git_provider_id AND git_providers.deleted_at IS NULL").
		Joins("LEFT JOIN git_accounts ON git_accounts.id = repository_bindings.git_account_id AND git_accounts.deleted_at IS NULL").
		Joins("LEFT JOIN users ON users.id = git_accounts.user_id AND users.deleted_at IS NULL").
		Joins("LEFT JOIN applications ON applications.id = repository_bindings.application_id AND applications.deleted_at IS NULL").
		Where("repository_bindings.project_id = ? AND repository_bindings.deleted_at IS NULL", project.ID)

	if applicationID != "" {
		query = query.Where("repository_bindings.application_id = ?", applicationID)
	}
	if bindingFilter := trimPtr(in.Binding); bindingFilter != "" {
		if strings.Contains(bindingFilter, "/") {
			parts := strings.SplitN(bindingFilter, "/", 2)
			query = query.Where("repository_bindings.id = ? OR (lower(repository_bindings.owner) = lower(?) AND lower(repository_bindings.repo) = lower(?))",
				bindingFilter, strings.TrimSpace(parts[0]), normalizeRepositoryBindingRepo(parts[1]))
		} else {
			query = query.Where("repository_bindings.id = ?", bindingFilter)
		}
	}
	if search := trimPtr(in.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where("repository_bindings.owner ILIKE ? OR repository_bindings.repo ILIKE ? OR applications.name ILIKE ? OR git_accounts.username ILIKE ?",
			like, like, like, like)
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	sortBy := fallback(trimPtr(in.SortBy), "createdAt")
	sortOrder := strings.ToLower(fallback(trimPtr(in.SortOrder), "desc"))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	sortCols := map[string]string{
		"createdAt": "repository_bindings.created_at",
		"repo":      "repository_bindings.repo",
		"owner":     "repository_bindings.owner",
	}
	sortCol, ok := sortCols[sortBy]
	if !ok {
		sortBy = "createdAt"
		sortCol = "repository_bindings.created_at"
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var bindings []repositoryBindingResponse
	if err := query.Order(sortCol + " " + sortOrder).Limit(pageSize).Offset((page - 1) * pageSize).Scan(&bindings).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	for i := range bindings {
		bindings[i].CredentialRef = ""
		bindings[i].WebhookCallbackURL = h.gitWebhookURL(tc.Gin, bindings[i].ID)
	}
	h.observeRepositoryBindings(tc.Context(), user, bindings)

	out := aitool.PageEnvelope(bindings, page, pageSize, sortBy, sortOrder, total)
	out["projectId"] = project.ID
	if applicationID != "" {
		out["applicationId"] = applicationID
	}
	return out, nil
}

func (h *Handler) aiListGitProvidersAdmin(tc *aitool.ToolContext, in adminListGitProvidersInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("auth.platform_admin_required", "platform administrator role is required")
	}
	query := tc.DB.Model(&model.GitProvider{})
	if provRef := trimPtr(in.Provider); provRef != "" {
		query = query.Where("id = ? OR lower(name) = lower(?)", provRef, provRef)
	}
	if scope := trimPtr(in.Scope); scope != "" {
		query = query.Where("scope = ?", strings.ToLower(scope))
	}
	if in.Enabled != nil {
		query = query.Where("enabled = ?", *in.Enabled)
	}
	if search := trimPtr(in.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where("name ILIKE ? OR base_url ILIKE ?", like, like)
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var providers []model.GitProvider
	if err := query.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&providers).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if provRef := trimPtr(in.Provider); provRef != "" && len(providers) > 1 {
		for _, p := range providers {
			if p.ID == provRef {
				providers = []model.GitProvider{p}
				total = 1
				break
			}
		}
	}

	items := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		p.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitProvider, p.ID, tc.Context())
		var accountCount int64
		_ = tc.DB.Model(&model.GitAccount{}).Where("provider_id = ?", p.ID).Count(&accountCount).Error
		var bindingCount int64
		_ = tc.DB.Model(&model.RepositoryBinding{}).
			Joins("JOIN projects ON projects.id = repository_bindings.project_id AND projects.deleted_at IS NULL").
			Where("repository_bindings.git_provider_id = ?", p.ID).
			Count(&bindingCount).Error

		entry := map[string]any(gitProviderResponse(p))
		entry["accountCount"] = accountCount
		entry["bindingCount"] = bindingCount
		items = append(items, entry)
	}
	return aitool.PageEnvelope(items, page, pageSize, "createdAt", "desc", total), nil
}

// --- Shared Git AI Helpers ---

func (h *Handler) syncGinUser(tc *aitool.ToolContext) {
	if tc == nil || tc.Gin == nil || tc.User.ID == "" {
		return
	}
	tc.Gin.Set("luna.devops.current_user", tc.User)
}

func (h *Handler) gitProviderUsableInProject(db *gorm.DB, user model.User, provider model.GitProvider, projectID string, ctx context.Context) bool {
	if projectID == "" {
		return h.canUseScopedResourceByID(user, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID, ctx)
	}
	switch normalizeGitScope(provider.Scope) {
	case "global":
		return true
	case "user":
		return strings.TrimSpace(provider.OwnerRef) == user.ID
	case "project":
		var count int64
		_ = db.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? AND resource_id = ? AND project_id = ?", scopedResourceGitProvider, provider.ID, projectID).
			Count(&count).Error
		return count > 0
	default:
		return false
	}
}

func (h *Handler) gitAccountUsableInProject(db *gorm.DB, user model.User, account model.GitAccount, projectID string, ctx context.Context) bool {
	if projectID == "" {
		return h.canUseScopedResourceByID(user, account.Scope, account.OwnerRef, scopedResourceGitAccount, account.ID, ctx)
	}
	switch normalizeGitScope(account.Scope) {
	case "global":
		return true
	case "user":
		return strings.TrimSpace(account.OwnerRef) == user.ID
	case "project":
		var count int64
		_ = db.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? AND resource_id = ? AND project_id = ?", scopedResourceGitAccount, account.ID, projectID).
			Count(&count).Error
		return count > 0
	default:
		return false
	}
}

func (h *Handler) aiResolveEnabledGitProvider(tc *aitool.ToolContext, ref string, projectID string) (model.GitProvider, error) {
	var all []model.GitProvider
	if err := tc.DB.Where("enabled = ?", true).Order("created_at asc").Find(&all).Error; err != nil {
		return model.GitProvider{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	visible := make([]model.GitProvider, 0, len(all))
	for _, p := range all {
		if h.gitProviderUsableInProject(tc.DB, tc.User, p, projectID, tc.Context()) {
			visible = append(visible, p)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		visible,
		"git_provider",
		"git.provider_not_found",
		ref,
		func(p model.GitProvider) string { return p.ID },
		func(p model.GitProvider) string { return p.Name },
	)
	if err != nil {
		return model.GitProvider{}, err
	}
	resolved.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitProvider, resolved.ID, tc.Context())
	return resolved, nil
}

func (h *Handler) aiResolveUsableGitAccountAndProvider(tc *aitool.ToolContext, accountRef, providerRef, projectID string) (model.GitAccount, model.GitProvider, error) {
	var filterProviderID string
	if providerRef != "" {
		prov, err := h.aiResolveEnabledGitProvider(tc, providerRef, projectID)
		if err != nil {
			return model.GitAccount{}, model.GitProvider{}, err
		}
		filterProviderID = prov.ID
	}

	var allAccounts []model.GitAccount
	q := tc.DB.Order("created_at asc")
	if filterProviderID != "" {
		q = q.Where("provider_id = ?", filterProviderID)
	}
	if err := q.Find(&allAccounts).Error; err != nil {
		return model.GitAccount{}, model.GitProvider{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	usable := make([]model.GitAccount, 0, len(allAccounts))
	for _, acc := range allAccounts {
		if h.gitAccountUsableInProject(tc.DB, tc.User, acc, projectID, tc.Context()) {
			usable = append(usable, acc)
		}
	}
	account, err := aitool.ResolveUniqueByIDOrName(
		usable,
		"git_account",
		"git.account_not_found",
		accountRef,
		func(a model.GitAccount) string { return a.ID },
		func(a model.GitAccount) string { return a.Username },
	)
	if err != nil {
		return model.GitAccount{}, model.GitProvider{}, err
	}

	var provider model.GitProvider
	if err := tc.DB.First(&provider, "id = ? AND enabled = ?", account.ProviderID, true).Error; err != nil {
		return model.GitAccount{}, model.GitProvider{}, aitool.NotFound("git.provider_not_found", "git provider is disabled or missing")
	}
	if !h.gitProviderUsableInProject(tc.DB, tc.User, provider, projectID, tc.Context()) {
		return model.GitAccount{}, model.GitProvider{}, aitool.Forbidden("git.provider_forbidden", "git provider is not accessible in this project scope")
	}
	account.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitAccount, account.ID, tc.Context())
	provider.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceGitProvider, provider.ID, tc.Context())
	return account, provider, nil
}

func (h *Handler) aiGitClientForAccount(tc *aitool.ToolContext, account model.GitAccount, provider model.GitProvider) (gitprovider.Client, error) {
	if gitAccountNeedsRefresh(account) {
		refreshed, ok := h.refreshGitAccountForUser(tc.Gin, tc.User, account, provider)
		if !ok {
			return gitprovider.Client{}, tc.AbortedError("git.token_refresh_failed", "failed to refresh git OAuth token")
		}
		account = refreshed
	}
	token := strings.TrimSpace(h.secrets.ResolveContext(tc.Context(), account.AccessTokenRef))
	if token == "" {
		return gitprovider.Client{}, aitool.BadRequest("git.access_token_missing", "git account has no configured access token")
	}
	return gitprovider.NewClientWithPolicy(provider, token, h.egressPolicyForUser(tc.User, tc.Context())), nil
}

func (h *Handler) aiResolveRepositoryBinding(db *gorm.DB, projectID, applicationID, ref string) (model.RepositoryBinding, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.RepositoryBinding{}, aitool.BadRequest("repository_binding.required", "repository binding is required")
	}
	q := db.Where("project_id = ? AND deleted_at IS NULL", projectID)
	if applicationID != "" {
		q = q.Where("application_id = ?", applicationID)
	}
	var all []model.RepositoryBinding
	if err := q.Order("created_at asc").Find(&all).Error; err != nil {
		return model.RepositoryBinding{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	for _, b := range all {
		if b.ID == ref {
			return b, nil
		}
	}
	normalizedTarget := strings.TrimSuffix(strings.ToLower(ref), ".git")
	var matches []model.RepositoryBinding
	for _, b := range all {
		fullName := strings.ToLower(b.Owner + "/" + normalizeRepositoryBindingRepo(b.Repo))
		if fullName == normalizedTarget || strings.EqualFold(b.Repo, ref) {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		candidates := make([]aitool.AmbiguousCandidate, len(matches))
		for i, m := range matches {
			candidates[i] = aitool.AmbiguousCandidate{
				ID:         m.ID,
				Name:       m.Owner + "/" + m.Repo,
				Identifier: m.ApplicationID,
			}
		}
		return model.RepositoryBinding{}, aitool.AmbiguousResource("repository_binding", ref, candidates)
	}
	return model.RepositoryBinding{}, aitool.NotFound("git.repository_binding_not_found", "repository binding was not found")
}

func validateSafeGitIdentifierSegment(value, fieldName string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", aitool.BadRequest("argument_invalid", fieldName+" is required")
	}
	if strings.HasPrefix(trimmed, "-") || strings.Contains(trimmed, "/") || strings.ContainsAny(trimmed, " \t\r\n") {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("%s must not start with '-' or contain '/' or whitespace", fieldName))
	}
	return trimmed, nil
}

func validateSafeGitBranch(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "main", nil
	}
	if strings.HasPrefix(trimmed, "-") || strings.ContainsAny(trimmed, " \t\r\n") {
		return "", aitool.BadRequest("argument_invalid", "defaultBranch must not start with '-' or contain whitespace")
	}
	return trimmed, nil
}

func validateSafeSameOriginCloneURL(rawCloneURL string, provider model.GitProvider, policy security.EgressPolicy) (string, error) {
	trimmed := strings.TrimSpace(rawCloneURL)
	if trimmed == "" {
		return "", aitool.BadRequest("git.clone_url_required", "cloneUrl is required")
	}
	if strings.HasPrefix(trimmed, "-") {
		return "", aitool.BadRequest("git.clone_url_invalid", "cloneUrl must not start with '-'")
	}
	if strings.Contains(trimmed, "?") || strings.Contains(trimmed, "#") {
		return "", aitool.BadRequest("git.clone_url_invalid", "cloneUrl must not contain query parameters or fragments")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", aitool.BadRequest("git.clone_url_invalid", "cloneUrl must be a valid http(s) URL")
	}
	if parsed.User != nil {
		return "", aitool.BadRequest("git.clone_url_credentials_forbidden", "cloneUrl must not embed username or password credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", aitool.BadRequest("git.clone_url_invalid", "cloneUrl must not contain query parameters or fragments")
	}

	expectedBase := strings.TrimSpace(provider.BaseURL)
	if normalizeGitProviderType(provider.Type) == "github" {
		expectedBase = "https://github.com"
	}
	if expectedBase == "" {
		return "", aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"git.provider_base_url_missing",
			"parent GitProvider has no configured baseUrl",
			"Ask a platform administrator to configure baseUrl on this GitProvider before binding repositories.",
		)
	}
	baseParsed, baseErr := url.Parse(expectedBase)
	if baseErr != nil || baseParsed.Host == "" {
		return "", aitool.BadRequest("git.provider_base_url_invalid", "parent GitProvider baseUrl is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, baseParsed.Scheme) || !strings.EqualFold(parsed.Host, baseParsed.Host) {
		return "", aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"git.clone_url_origin_mismatch",
			fmt.Sprintf("cloneUrl origin (%s://%s) must match GitProvider baseUrl origin (%s://%s)", parsed.Scheme, parsed.Host, baseParsed.Scheme, baseParsed.Host),
			"Use a cloneUrl hosted on the exact same scheme and host as the selected GitProvider.",
		)
	}
	if _, egressErr := policy.ValidateURL(trimmed); egressErr != nil {
		return "", aitool.Forbidden("git.clone_url_egress_blocked", "cloneUrl is blocked by platform network egress policy")
	}
	return trimmed, nil
}

func enforceScopedConversationSandbox(conversationProjectID, currentUserID, scope, ownerRef string, projectIDs []string) error {
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

func resolveProjectRefs(db *gorm.DB, refs []string) ([]string, error) {
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

func mergeIncrementalProjectIDs(existing, addIDs, removeIDs []string) []string {
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

func gitUpstreamToolError(err error) *aitool.ToolError {
	status, code := gitUpstreamErrorStatusAndCode(err)
	return aitool.NewError(status, code, code)
}

func remoteTotalCount(page, pageSize, itemCount int) int64 {
	total := int64((page-1)*pageSize + itemCount)
	if itemCount == pageSize {
		total++
	}
	return total
}

func isBinaryContent(s string) bool {
	if strings.IndexByte(s, 0) >= 0 {
		return true
	}
	return !utf8.ValidString(s)
}

func truncateUTF8Head(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func trimPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func boolPtrDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}
