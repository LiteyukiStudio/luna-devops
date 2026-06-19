package api

import (
	"errors"
	"fmt"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"io"
	"net/http"
	"strings"
	"time"
)

var errGitClientResponseWritten = errors.New("git client response written")

func (h *Handlers) ListGitRepositories(ctx *gin.Context) {
	client, ok := h.gitClientForCurrentUserAccount(ctx, ctx.Param("accountId"))
	if !ok {
		return
	}
	page := positiveInt(ctx.DefaultQuery("page", "1"), 1)
	pageSize := positiveInt(ctx.DefaultQuery("pageSize", "50"), 50)
	if pageSize > 100 {
		pageSize = 100
	}
	repos, err := client.ListRepositories(ctx.Request.Context(), ctx.Query("search"), page, pageSize)
	if err != nil {
		writeGitUpstreamError(ctx, err)
		return
	}
	if len(repos) == 0 && boolQuery(ctx, "includePublic") && strings.TrimSpace(ctx.Query("search")) != "" {
		repos, err = client.SearchPublicRepositories(ctx.Request.Context(), ctx.Query("search"), page, pageSize)
		if err != nil {
			writeGitUpstreamError(ctx, err)
			return
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"items": repos, "page": page, "pageSize": pageSize})
}

func boolQuery(ctx *gin.Context, key string) bool {
	value := strings.ToLower(strings.TrimSpace(ctx.Query(key)))
	return value == "true" || value == "1" || value == "yes"
}

func (h *Handlers) ListGitBranches(ctx *gin.Context) {
	client, ok := h.gitClientForCurrentUserAccount(ctx, ctx.Param("accountId"))
	if !ok {
		return
	}
	ref := strings.TrimSpace(ctx.Query("ref"))
	cacheKey := gitBranchCacheKey(ctx.Param("accountId"), ctx.Param("owner"), ctx.Param("repo"), ref)
	branches, ok := h.branchCache.get(cacheKey)
	if !ok {
		var err error
		branches, err = client.ListBranches(ctx.Request.Context(), ctx.Param("owner"), ctx.Param("repo"))
		if err != nil {
			writeGitUpstreamError(ctx, err)
			return
		}
		h.branchCache.set(cacheKey, branches)
	}
	limit := positiveInt(ctx.DefaultQuery("limit", "50"), 50)
	result := filterGitBranches(branches, ctx.Query("search"), limit)
	ctx.JSON(http.StatusOK, gin.H{
		"items":        result.items,
		"total":        len(branches),
		"matchedTotal": result.matchedTotal,
		"limited":      len(result.items) < result.matchedTotal,
	})
}

func (h *Handlers) ReadGitFile(ctx *gin.Context) {
	client, ok := h.gitClientForCurrentUserAccount(ctx, ctx.Param("accountId"))
	if !ok {
		return
	}
	filePath := strings.TrimSpace(ctx.Query("path"))
	if filePath == "" {
		writeError(ctx, http.StatusBadRequest, "file path is required")
		return
	}
	file, err := client.ReadFile(ctx.Request.Context(), ctx.Param("owner"), ctx.Param("repo"), filePath, ctx.Query("ref"))
	if err != nil {
		writeGitUpstreamError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, file)
}

func (h *Handlers) ListGitContents(ctx *gin.Context) {
	client, ok := h.gitClientForCurrentUserAccount(ctx, ctx.Param("accountId"))
	if !ok {
		return
	}
	items, err := client.ListContents(ctx.Request.Context(), ctx.Param("owner"), ctx.Param("repo"), ctx.Query("path"), ctx.Query("ref"))
	if err != nil {
		writeGitUpstreamError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, items)
}

func (h *Handlers) GetGitRepositoryBuildOptions(ctx *gin.Context) {
	client, ok := h.gitClientForCurrentUserAccount(ctx, ctx.Param("accountId"))
	if !ok {
		return
	}
	started := time.Now()
	options, err := client.DiscoverBuildOptions(ctx.Request.Context(), ctx.Param("owner"), ctx.Param("repo"), ctx.Query("ref"))
	if err != nil {
		writeGitUpstreamError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"dockerfiles":  options.Dockerfiles,
		"directories":  options.Directories,
		"exposedPorts": options.ExposedPorts,
		"strategy":     options.Strategy,
		"truncated":    options.Truncated,
		"durationMs":   time.Since(started).Milliseconds(),
	})
}

func (h *Handlers) ListRepositoryBindings(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var bindings []repositoryBindingResponse
	query := h.db.Table("repository_bindings").
		Select("repository_bindings.*, git_providers.name as provider_name, git_providers.type as provider_type, git_accounts.username as account_username, users.email as account_owner_email, users.name as account_owner_name, applications.name as application_name").
		Joins("join git_providers on git_providers.id = repository_bindings.git_provider_id and git_providers.deleted_at is null").
		Joins("join git_accounts on git_accounts.id = repository_bindings.git_account_id and git_accounts.deleted_at is null").
		Joins("join users on users.id = git_accounts.user_id and users.deleted_at is null").
		Joins("join applications on applications.id = repository_bindings.application_id and applications.deleted_at is null").
		Where("repository_bindings.project_id = ? and repository_bindings.deleted_at is null", ctx.Param("projectId"))
	query = applySearch(ctx, query, "repository_bindings.owner", "repository_bindings.repo", "applications.name", "git_accounts.username")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if err := query.Order(orderByClause(pagination, map[string]string{
			"repo":      "repository_bindings.repo",
			"owner":     "repository_bindings.owner",
			"createdAt": "repository_bindings.created_at",
		}, "repository_bindings.created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Scan(&bindings).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		for index := range bindings {
			bindings[index].CredentialRef = ""
			bindings[index].WebhookCallbackURL = h.gitWebhookURL(ctx, bindings[index].ID)
		}
		ctx.JSON(http.StatusOK, paginatedResponse(bindings, total, pagination))
		return
	}
	if err := query.Order("repository_bindings.created_at desc").Scan(&bindings).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range bindings {
		bindings[index].CredentialRef = ""
		bindings[index].WebhookCallbackURL = h.gitWebhookURL(ctx, bindings[index].ID)
	}
	ctx.JSON(http.StatusOK, bindings)
}

func (h *Handlers) CreateRepositoryBinding(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}

	var input repositoryBindingInput
	if !bindJSON(ctx, &input) {
		return
	}

	binding, ok := h.repositoryBindingFromInput(ctx, user.ID, input)
	if !ok {
		return
	}
	if !h.ensureRepositoryBindingUnique(ctx, binding, "") {
		return
	}

	if err := h.db.Create(&binding).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if shouldAutoConfigureWebhook(input) && binding.WebhookStatus != "disabled" {
		h.tryConfigureRepositoryWebhook(ctx, user, &binding)
	}
	h.syncApplicationRepositoryURL(binding)
	binding.CredentialRef = ""
	ctx.JSON(http.StatusCreated, binding)
}

func (h *Handlers) UpdateRepositoryBinding(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}

	var existing model.RepositoryBinding
	if err := h.db.First(&existing, "id = ? and project_id = ?", ctx.Param("bindingId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "repository binding not found")
		return
	}

	var input repositoryBindingInput
	if !bindJSON(ctx, &input) {
		return
	}

	binding, ok := h.repositoryBindingFromInput(ctx, user.ID, input)
	if !ok {
		return
	}
	if !h.ensureRepositoryBindingUnique(ctx, binding, existing.ID) {
		return
	}
	webhookTargetChanged := repositoryBindingWebhookTargetChanged(existing, binding)
	wasWebhookCreated := existing.WebhookStatus == "created"
	existing.ApplicationID = binding.ApplicationID
	existing.GitProviderID = binding.GitProviderID
	existing.GitAccountID = binding.GitAccountID
	existing.Owner = binding.Owner
	existing.Repo = binding.Repo
	existing.CloneURL = binding.CloneURL
	existing.DefaultBranch = binding.DefaultBranch
	existing.WebhookStatus = binding.WebhookStatus
	if webhookTargetChanged {
		existing.WebhookID = ""
		existing.WebhookSecret = ""
		existing.LastEvent = ""
		existing.LastCommitSHA = ""
		existing.LastWebhookAt = nil
		if existing.WebhookStatus == "created" {
			existing.WebhookStatus = "pending"
		}
	}
	existing.CredentialRef = ""

	if err := h.db.Save(&existing).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if shouldAutoConfigureWebhook(input) && (webhookTargetChanged || !wasWebhookCreated) && existing.WebhookStatus != "created" && existing.WebhookStatus != "disabled" {
		h.tryConfigureRepositoryWebhook(ctx, user, &existing)
	}
	h.syncApplicationRepositoryURL(existing)
	existing.CredentialRef = ""
	ctx.JSON(http.StatusOK, existing)
}

func (h *Handlers) DeleteRepositoryBinding(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}

	var binding model.RepositoryBinding
	if err := h.db.First(&binding, "id = ? and project_id = ?", ctx.Param("bindingId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "repository binding not found")
		return
	}
	if err := h.db.Delete(&binding).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) CreateRepositoryWebhook(ctx *gin.Context) {
	h.configureRepositoryWebhookFromRequest(ctx)
}

func (h *Handlers) ReconfigureRepositoryWebhook(ctx *gin.Context) {
	h.configureRepositoryWebhookFromRequest(ctx)
}

func (h *Handlers) configureRepositoryWebhookFromRequest(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if _, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer"); !ok {
		return
	}
	var binding model.RepositoryBinding
	if err := h.db.First(&binding, "id = ? and project_id = ?", ctx.Param("bindingId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "repository binding not found")
		return
	}
	if err := h.configureRepositoryWebhook(ctx, user, &binding, true); err != nil {
		if errors.Is(err, errGitClientResponseWritten) {
			return
		}
		writeGitUpstreamError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, binding)
}

func (h *Handlers) ReceiveGitWebhook(ctx *gin.Context) {
	var binding model.RepositoryBinding
	if err := h.db.First(&binding, "id = ?", ctx.Param("bindingId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "repository binding not found")
		return
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 2*1024*1024))
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "invalid webhook body")
		return
	}
	if !verifyGitWebhookSignature(ctx.Request.Header, body, h.secrets.Resolve(binding.WebhookSecret)) {
		writeError(ctx, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	event := gitWebhookEvent(ctx.Request.Header)
	commitSHA := gitWebhookCommitSHA(body)
	pushPayload, isPush := parseGitWebhookPushPayload(ctx.Request.Header, body)
	if pushPayload.CommitSHA != "" {
		commitSHA = pushPayload.CommitSHA
	}
	now := time.Now()
	binding.WebhookStatus = "created"
	binding.LastEvent = event
	binding.LastCommitSHA = commitSHA
	binding.LastWebhookAt = &now
	if err := h.db.Save(&binding).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	result := h.enqueueBuildRunsForWebhook(ctx, binding, pushPayload, isPush)
	ctx.JSON(http.StatusOK, gin.H{
		"accepted":      true,
		"event":         event,
		"commitSha":     commitSHA,
		"matched":       result.Matched,
		"queued":        result.Queued,
		"queuedRunIds":  result.QueuedRunIDs,
		"failed":        result.Failed,
		"skipped":       result.Skipped,
		"ignoredReason": result.IgnoredReason,
	})
}

type webhookBuildTriggerResult struct {
	Matched       int
	Queued        int
	Failed        int
	Skipped       int
	QueuedRunIDs  []string
	IgnoredReason string
}

func (h *Handlers) enqueueBuildRunsForWebhook(ctx *gin.Context, binding model.RepositoryBinding, payload gitWebhookPushPayload, isPush bool) webhookBuildTriggerResult {
	result := webhookBuildTriggerResult{QueuedRunIDs: []string{}}
	if !isPush {
		result.IgnoredReason = "unsupported_event"
		return result
	}
	if payload.Deleted {
		result.IgnoredReason = "deleted_ref"
		return result
	}
	var account model.GitAccount
	if err := h.db.First(&account, "id = ?", binding.GitAccountID).Error; err != nil {
		result.IgnoredReason = "git_account_missing"
		return result
	}
	var actor model.User
	if err := h.db.First(&actor, "id = ?", account.UserID).Error; err != nil {
		result.IgnoredReason = "actor_missing"
		return result
	}
	var targets []model.DeploymentTarget
	if err := h.db.Where(
		"project_id = ? and application_id = ? and repository_binding_id = ? and enabled = ? and delete_status in ?",
		binding.ProjectID,
		binding.ApplicationID,
		binding.ID,
		true,
		[]string{"", "active"},
	).Where("(source_type = ? or source_type = '')", "repository").
		Order("created_at asc").
		Find(&targets).Error; err != nil {
		result.IgnoredReason = "deployment_targets_unavailable"
		return result
	}
	for _, target := range targets {
		run := webhookBuildRunFromTarget(binding, target, actor, payload)
		if !deploymentTargetMatchesBuildRun(target, run) {
			result.Skipped++
			continue
		}
		result.Matched++
		if err := h.prepareBuildRunRequest(actor, &run); err != nil {
			result.Failed++
			continue
		}
		queuedRun, err := h.queueBuildRun(ctx.Request.Context(), actor, run)
		if err != nil {
			result.Failed++
			continue
		}
		result.Queued++
		result.QueuedRunIDs = append(result.QueuedRunIDs, queuedRun.ID)
	}
	if result.Matched == 0 && result.IgnoredReason == "" {
		result.IgnoredReason = "no_matching_deployment_target"
	}
	return result
}

func webhookBuildRunFromTarget(binding model.RepositoryBinding, target model.DeploymentTarget, actor model.User, payload gitWebhookPushPayload) model.BuildRun {
	triggeredByName := firstNonEmpty(payload.TriggeredByName, buildRunActorName(actor), "Git webhook")
	triggeredByEmail := firstNonEmpty(payload.TriggeredByEmail, actor.Email)
	triggerType := "push"
	if payload.SourceTag != "" && payload.SourceBranch == "" {
		triggerType = "tag"
	}
	return model.BuildRun{
		ID:                  id.New("bldr"),
		ProjectID:           binding.ProjectID,
		ApplicationID:       binding.ApplicationID,
		DeploymentTargetID:  target.ID,
		BuildVariableSetIDs: strings.TrimSpace(target.BuildVariableSetIDs),
		Status:              "queued",
		TriggerType:         triggerType,
		SourceBranch:        payload.SourceBranch,
		SourceTag:           payload.SourceTag,
		SourceCommit:        payload.CommitSHA,
		DockerfilePath:      fallback(strings.TrimSpace(target.DockerfilePath), "Dockerfile"),
		BuildContext:        fallback(strings.TrimSpace(target.BuildContext), "."),
		BuildDirectory:      strings.TrimSpace(target.BuildDirectory),
		TargetRegistryID:    strings.TrimSpace(target.TargetRegistryID),
		TargetRepository:    strings.Trim(strings.TrimSpace(target.TargetRepository), "/"),
		TargetTag:           fallback(strings.TrimSpace(target.TargetTag), "latest"),
		CacheConfig:         "",
		CreatedBy:           actor.ID,
		TriggeredByName:     triggeredByName,
		TriggeredByEmail:    triggeredByEmail,
		SourceAuthorName:    payload.SourceAuthorName,
		SourceAuthorEmail:   payload.SourceAuthorEmail,
	}
}

func (h *Handlers) repositoryBindingFromInput(ctx *gin.Context, userID string, input repositoryBindingInput) (model.RepositoryBinding, bool) {
	account, ok := h.findGitAccountForUser(ctx, userID, input.GitAccountID)
	if !ok {
		return model.RepositoryBinding{}, false
	}
	provider, ok := h.findEnabledGitProvider(ctx, account.ProviderID)
	if !ok {
		return model.RepositoryBinding{}, false
	}

	app, ok := h.findApplicationByID(ctx, input.ApplicationID)
	if !ok {
		return model.RepositoryBinding{}, false
	}

	owner := strings.TrimSpace(input.Owner)
	repo := strings.TrimSpace(input.Repo)
	if owner == "" || repo == "" {
		writeError(ctx, http.StatusBadRequest, "请输入仓库 owner 和 repo")
		return model.RepositoryBinding{}, false
	}

	cloneURL := strings.TrimSpace(input.CloneURL)
	if cloneURL == "" {
		cloneURL = defaultCloneURL(provider, owner, repo)
	}

	return model.RepositoryBinding{
		ID:            id.New("rpb"),
		ProjectID:     ctx.Param("projectId"),
		ApplicationID: app.ID,
		GitProviderID: provider.ID,
		GitAccountID:  account.ID,
		Owner:         owner,
		Repo:          repo,
		CloneURL:      cloneURL,
		DefaultBranch: fallback(strings.TrimSpace(input.DefaultBranch), "main"),
		WebhookStatus: normalizeWebhookStatus(input.WebhookStatus),
		CredentialRef: "",
	}, true
}

func (h *Handlers) ensureRepositoryBindingUnique(ctx *gin.Context, binding model.RepositoryBinding, excludeID string) bool {
	query := h.db.Model(&model.RepositoryBinding{}).
		Where("project_id = ? and application_id = ? and git_provider_id = ? and lower(owner) = ? and lower(repo) = ?",
			binding.ProjectID,
			binding.ApplicationID,
			binding.GitProviderID,
			normalizeRepositoryBindingOwner(binding.Owner),
			normalizeRepositoryBindingRepo(binding.Repo),
		)
	if strings.TrimSpace(excludeID) != "" {
		query = query.Where("id <> ?", excludeID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count > 0 {
		writeError(ctx, http.StatusConflict, "该应用已绑定同一个仓库")
		return false
	}
	return true
}

func normalizeRepositoryBindingOwner(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRepositoryBindingRepo(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".git")
}

func shouldAutoConfigureWebhook(input repositoryBindingInput) bool {
	return input.AutoConfigureWebhook == nil || *input.AutoConfigureWebhook
}

func repositoryBindingWebhookTargetChanged(current, next model.RepositoryBinding) bool {
	return current.GitProviderID != next.GitProviderID ||
		current.GitAccountID != next.GitAccountID ||
		current.Owner != next.Owner ||
		current.Repo != next.Repo
}

func (h *Handlers) tryConfigureRepositoryWebhook(ctx *gin.Context, user model.User, binding *model.RepositoryBinding) {
	_ = h.configureRepositoryWebhook(ctx, user, binding, false)
}

func (h *Handlers) configureRepositoryWebhook(ctx *gin.Context, user model.User, binding *model.RepositoryBinding, writeClientErrors bool) error {
	client, err := h.gitClientForUserBinding(ctx, user, *binding, writeClientErrors)
	if err != nil {
		binding.WebhookStatus = "failed"
		_ = h.db.Save(binding).Error
		h.audit(user.ID, "git_webhook.create", binding.ID, false, "git client unavailable")
		return err
	}
	secret := randomHex(32)
	result, err := client.CreateWebhook(ctx.Request.Context(), binding.Owner, binding.Repo, h.gitWebhookURL(ctx, binding.ID), secret)
	if err != nil {
		binding.WebhookStatus = "failed"
		_ = h.db.Save(binding).Error
		h.audit(user.ID, "git_webhook.create", binding.ID, false, "upstream create failed")
		return err
	}
	binding.WebhookStatus = "created"
	binding.WebhookID = result.ID
	binding.WebhookSecret = h.secrets.Store(secret, user.ID, "repository_binding:"+binding.ID+":webhook")
	if binding.WebhookSecret == "" {
		binding.WebhookStatus = "failed"
		_ = h.db.Save(binding).Error
		h.audit(user.ID, "git_webhook.create", binding.ID, false, "secret store failed")
		return fmt.Errorf("webhook secret store failed")
	}
	if err := h.db.Save(binding).Error; err != nil {
		h.audit(user.ID, "git_webhook.create", binding.ID, false, "save failed")
		return err
	}
	h.audit(user.ID, "git_webhook.create", binding.ID, true, binding.WebhookID)
	return nil
}

func (h *Handlers) gitClientForUserBinding(ctx *gin.Context, user model.User, binding model.RepositoryBinding, writeClientErrors bool) (gitprovider.Client, error) {
	if writeClientErrors {
		account, ok := h.findGitAccountForUser(ctx, user.ID, binding.GitAccountID)
		if !ok {
			return gitprovider.Client{}, fmt.Errorf("%w: git account unavailable", errGitClientResponseWritten)
		}
		provider, ok := h.findEnabledGitProvider(ctx, binding.GitProviderID)
		if !ok {
			return gitprovider.Client{}, fmt.Errorf("%w: git provider unavailable", errGitClientResponseWritten)
		}
		if account.ProviderID != provider.ID {
			writeError(ctx, http.StatusBadRequest, "Git 凭据与 Provider 不匹配")
			return gitprovider.Client{}, fmt.Errorf("%w: git provider mismatch", errGitClientResponseWritten)
		}
		if gitAccountNeedsRefresh(account) {
			account, ok = h.refreshGitAccountForUser(ctx, user, account, provider)
			if !ok {
				return gitprovider.Client{}, fmt.Errorf("%w: git account refresh failed", errGitClientResponseWritten)
			}
		}
		token := h.secrets.Resolve(account.AccessTokenRef)
		if token == "" {
			writeError(ctx, http.StatusBadRequest, "git account has no access token")
			return gitprovider.Client{}, fmt.Errorf("%w: git account has no access token", errGitClientResponseWritten)
		}
		return gitprovider.NewClientWithPolicy(provider, token, h.egressPolicyForUser(user)), nil
	}

	var account model.GitAccount
	if err := h.db.First(&account, "id = ?", strings.TrimSpace(binding.GitAccountID)).Error; err != nil {
		return gitprovider.Client{}, fmt.Errorf("git account unavailable: %w", err)
	}
	if normalizeGitAccessScope(account.AccessScope) == "personal" {
		if account.UserID != user.ID {
			return gitprovider.Client{}, fmt.Errorf("git account forbidden")
		}
	} else if !h.canUseScopedResourceByID(user, account.Scope, account.OwnerRef, scopedResourceGitAccount, account.ID) {
		return gitprovider.Client{}, fmt.Errorf("git account forbidden")
	}
	if account.Status != "connected" {
		return gitprovider.Client{}, fmt.Errorf("git account is not connected")
	}
	var provider model.GitProvider
	if err := h.db.First(&provider, "id = ? and enabled = ?", strings.TrimSpace(binding.GitProviderID), true).Error; err != nil {
		return gitprovider.Client{}, fmt.Errorf("git provider unavailable: %w", err)
	}
	if account.ProviderID != provider.ID {
		return gitprovider.Client{}, fmt.Errorf("git provider mismatch")
	}
	if !h.canUseScopedResourceByID(user, provider.Scope, provider.OwnerRef, scopedResourceGitProvider, provider.ID) {
		return gitprovider.Client{}, fmt.Errorf("git provider forbidden")
	}
	token := h.secrets.Resolve(account.AccessTokenRef)
	if token == "" {
		return gitprovider.Client{}, fmt.Errorf("git account has no access token")
	}
	return gitprovider.NewClientWithPolicy(provider, token, h.egressPolicyForUser(user)), nil
}

func (h *Handlers) gitClientForCurrentUserAccount(ctx *gin.Context, accountID string) (gitprovider.Client, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return gitprovider.Client{}, false
	}
	account, ok := h.findGitAccountForUser(ctx, user.ID, accountID)
	if !ok {
		return gitprovider.Client{}, false
	}
	provider, ok := h.findEnabledGitProvider(ctx, account.ProviderID)
	if !ok {
		return gitprovider.Client{}, false
	}
	if gitAccountNeedsRefresh(account) {
		account, ok = h.refreshGitAccountForUser(ctx, user, account, provider)
		if !ok {
			return gitprovider.Client{}, false
		}
	}
	token := h.secrets.Resolve(account.AccessTokenRef)
	if token == "" {
		writeError(ctx, http.StatusBadRequest, "git account has no access token")
		return gitprovider.Client{}, false
	}
	return gitprovider.NewClientWithPolicy(provider, token, h.egressPolicyForUser(user)), true
}
