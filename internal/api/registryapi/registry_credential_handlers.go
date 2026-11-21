package registryapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handler) ListRegistryCredentials(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
	if !ok {
		return
	}
	registry, ok := h.findAccessibleRegistry(ctx, user, ctx.Param("registryId"))
	if !ok {
		return
	}
	query := h.dbFor(ctx).Model(&model.RegistryCredential{}).Where("registry_id = ?", registry.ID)
	query, ok = h.applyScopedResourceListVisibility(
		ctx,
		query,
		scopedResourceRegistryCredential,
		user,
		strings.TrimSpace(ctx.Query("projectId")),
		visibility,
	)
	if !ok {
		return
	}
	h.listRegistryCredentials(ctx, user, query)
}

func (h *Handler) ListAllRegistryCredentials(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
	if !ok {
		return
	}
	query, ok := h.applyScopedResourceListVisibility(
		ctx,
		h.dbFor(ctx).Model(&model.RegistryCredential{}),
		scopedResourceRegistryCredential,
		user,
		"",
		visibility,
	)
	if !ok {
		return
	}
	h.listRegistryCredentials(ctx, user, query)
}

func (h *Handler) listRegistryCredentials(ctx *gin.Context, user model.User, query *gorm.DB) {
	var credentials []model.RegistryCredential
	query = applySearch(ctx, query, "name", "username")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "username": "username", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"username":  "username",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&credentials).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.attachRegistryCredentialProjects(credentials, ctx.Request.Context())
	responses, err := h.credentialResponsesForUser(user, credentials, ctx.Request.Context())
	if err != nil {
		h.writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handler) CreateRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	var input registryCredentialInput
	if !bindJSON(ctx, &input) {
		return
	}
	if strings.TrimSpace(input.Password) == "" && strings.TrimSpace(input.Token) == "" {
		writeError(ctx, http.StatusBadRequest, "请填写 Registry 密码或 Token")
		return
	}
	if !h.secretAvailable() {
		writeError(ctx, http.StatusInternalServerError, "SECRET_ENCRYPTION_KEY is required to save registry credentials in production")
		return
	}
	scope, ownerRef, projectIDs, scopeOK := h.normalizeCredentialScopeWithinParent(ctx, user, input.Scope, input.ProjectIDs, registry.Scope, registry.ProjectIDs, "只有平台管理员可以创建全局镜像凭据")
	if !scopeOK {
		return
	}

	credential := model.RegistryCredential{
		ID:                 id.New("regc"),
		RegistryID:         registry.ID,
		Name:               fallback(strings.TrimSpace(input.Name), "default"),
		Username:           strings.TrimSpace(input.Username),
		Usage:              normalizeCredentialUsage(input.Usage),
		Scope:              scope,
		OwnerRef:           ownerRef,
		ProjectIDs:         projectIDs,
		RepositoryTemplate: normalizeImageRepositoryTemplate(input.RepositoryTemplate),
		TagTemplate:        normalizeImageTagTemplate(input.TagTemplate),
		CreatedBy:          user.ID,
	}

	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRegistry model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRegistry, "id = ?", registry.ID).Error; err != nil {
			return err
		}
		if err := ensureCredentialFitsRegistryScopeTx(tx, lockedRegistry, credential); err != nil {
			return err
		}
		if strings.TrimSpace(input.Password) != "" {
			ref, storeErr := h.storeSecretWithDB(ctx.Request.Context(), tx, input.Password, user.ID, "registry_credential:"+lockedRegistry.ID+":password")
			if storeErr != nil {
				return storeErr
			}
			credential.PasswordRef = ref
		}
		if strings.TrimSpace(input.Token) != "" {
			ref, storeErr := h.storeSecretWithDB(ctx.Request.Context(), tx, input.Token, user.ID, "registry_credential:"+lockedRegistry.ID+":token")
			if storeErr != nil {
				return storeErr
			}
			credential.TokenRef = ref
		}
		if err := tx.Create(&credential).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, credential.ID, sortedProjectIDs(credential.ProjectIDs), nil); err != nil {
			return err
		}
		if lockedRegistry.CredentialRef == "" && credentialAllowsPush(credential.Usage) {
			return tx.Model(&model.ArtifactRegistry{}).Where("id = ?", lockedRegistry.ID).Update("credential_ref", credential.ID).Error
		}
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(ctx, http.StatusNotFound, "artifact registry not found")
			return
		}
		if errors.Is(err, errRegistryCredentialScopeConflict) {
			writeErrorCode(ctx, http.StatusConflict, "registry.credential_scope_conflict", "registry credential scope exceeds the parent registry scope")
			return
		}
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "registry_credential.create", credential.ID, true, credential.Scope, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, credentialResponseWithManage(credential, true))
}

func (h *Handler) UpdateRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}

	var credential model.RegistryCredential
	if err := h.dbFor(ctx).First(&credential, "id = ? and registry_id = ?", ctx.Param("credentialId"), registry.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "registry credential not found")
		return
	}
	if !h.canManageRegistryCredential(ctx, user, registry, credential) {
		return
	}

	var input registryCredentialInput
	if !bindJSON(ctx, &input) {
		return
	}
	hasNewPassword := strings.TrimSpace(input.Password) != ""
	hasNewToken := strings.TrimSpace(input.Token) != ""
	if hasNewPassword || hasNewToken {
		if !h.secretAvailable() {
			writeError(ctx, http.StatusInternalServerError, "SECRET_ENCRYPTION_KEY is required to save registry credentials in production")
			return
		}
	}

	scope, ownerRef, projectIDs, scopeOK := h.normalizeCredentialScopeWithinParent(ctx, user, input.Scope, input.ProjectIDs, registry.Scope, registry.ProjectIDs, "只有平台管理员可以创建全局镜像凭据")
	if !scopeOK {
		return
	}

	var updated model.RegistryCredential
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRegistry model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRegistry, "id = ?", registry.ID).Error; err != nil {
			return err
		}
		var lockedCred model.RegistryCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedCred, "id = ? and registry_id = ?", credential.ID, lockedRegistry.ID).Error; err != nil {
			return err
		}
		passwordRef := lockedCred.PasswordRef
		if hasNewPassword {
			resource := "registry_credential:" + lockedRegistry.ID + ":password"
			newRef, storeErr := h.storeSecretWithDB(ctx.Request.Context(), tx, input.Password, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(lockedCred.PasswordRef) != "" && lockedCred.PasswordRef != newRef {
				if delErr := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, lockedCred.PasswordRef, resource); delErr != nil {
					return delErr
				}
			}
			passwordRef = newRef
		}
		tokenRef := lockedCred.TokenRef
		if hasNewToken {
			resource := "registry_credential:" + lockedRegistry.ID + ":token"
			newRef, storeErr := h.storeSecretWithDB(ctx.Request.Context(), tx, input.Token, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			if strings.TrimSpace(lockedCred.TokenRef) != "" && lockedCred.TokenRef != newRef {
				if delErr := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, lockedCred.TokenRef, resource); delErr != nil {
					return delErr
				}
			}
			tokenRef = newRef
		}
		if passwordRef == "" && tokenRef == "" {
			return errRegistryCredentialSecretMissing
		}

		lockedCred.Name = fallback(strings.TrimSpace(input.Name), "default")
		lockedCred.Username = strings.TrimSpace(input.Username)
		lockedCred.PasswordRef = passwordRef
		lockedCred.TokenRef = tokenRef
		lockedCred.Usage = normalizeCredentialUsage(input.Usage)
		lockedCred.Scope = scope
		lockedCred.OwnerRef = ownerRef
		lockedCred.ProjectIDs = projectIDs
		lockedCred.RepositoryTemplate = normalizeImageRepositoryTemplate(input.RepositoryTemplate)
		lockedCred.TagTemplate = normalizeImageTagTemplate(input.TagTemplate)
		if err := ensureCredentialFitsRegistryScopeTx(tx, lockedRegistry, lockedCred); err != nil {
			return err
		}

		if err := tx.Save(&lockedCred).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, lockedCred.ID, sortedProjectIDs(lockedCred.ProjectIDs), nil); err != nil {
			return err
		}
		if err := reconcileRegistryPushCredentialsTx(tx, lockedRegistry, &lockedCred); err != nil {
			return err
		}
		updated = lockedCred
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "registry credential not found")
		return
	}
	if errors.Is(err, errRegistryCredentialSecretMissing) {
		writeError(ctx, http.StatusBadRequest, "请填写 Registry 密码或 Token")
		return
	}
	if errors.Is(err, errRegistryCredentialScopeConflict) {
		writeErrorCode(ctx, http.StatusConflict, "registry.credential_scope_conflict", "registry credential scope exceeds the parent registry scope")
		return
	}
	if errors.Is(err, errRegistryCredentialInUse) {
		writeErrorCode(ctx, http.StatusConflict, "registry.credential_in_use", "registry credential is required by repository deployment targets")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "registry_credential.update", updated.ID, true, updated.Scope, ctx.Request.Context())
	ctx.JSON(http.StatusOK, credentialResponseWithManage(updated, true))
}

func (h *Handler) DeleteRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}

	var credential model.RegistryCredential
	if err := h.dbFor(ctx).First(&credential, "id = ? and registry_id = ?", ctx.Param("credentialId"), registry.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "registry credential not found")
		return
	}
	if !h.canManageRegistryCredential(ctx, user, registry, credential) {
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedRegistry model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRegistry, "id = ?", registry.ID).Error; err != nil {
			return err
		}
		var lockedCred model.RegistryCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedCred, "id = ? and registry_id = ?", credential.ID, lockedRegistry.ID).Error; err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceRegistryCredential, lockedCred.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		if err := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, lockedCred.PasswordRef, "registry_credential:"+lockedCred.RegistryID+":password"); err != nil {
			return err
		}
		if err := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, lockedCred.TokenRef, "registry_credential:"+lockedCred.RegistryID+":token"); err != nil {
			return err
		}
		if err := tx.Delete(&lockedCred).Error; err != nil {
			return err
		}
		if lockedRegistry.CredentialRef == lockedCred.ID {
			if err := tx.Model(&model.ArtifactRegistry{}).Where("id = ?", lockedRegistry.ID).Update("credential_ref", "").Error; err != nil {
				return err
			}
			lockedRegistry.CredentialRef = ""
		}
		return reconcileRegistryPushCredentialsTx(tx, lockedRegistry, nil)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "registry credential not found")
		return
	}
	if errors.Is(err, errRegistryCredentialInUse) {
		writeErrorCode(ctx, http.StatusConflict, "registry.credential_in_use", "registry credential is required by repository deployment targets")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "registry_credential.delete", credential.ID, true, credential.Scope, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

var (
	errRegistryCredentialInUse         = errors.New("registry credential is required by repository deployment targets")
	errRegistryCredentialSecretMissing = errors.New("registry credential password or token is required")
)

func credentialAllowsPush(usage string) bool {
	normalized := normalizeCredentialUsage(usage)
	return normalized == "push" || normalized == "push-pull"
}

func ensureCredentialFitsRegistryScopeTx(tx *gorm.DB, registry model.ArtifactRegistry, credential model.RegistryCredential) error {
	switch registry.Scope {
	case "global":
		return nil
	case "user":
		if credential.Scope != "user" || strings.TrimSpace(credential.OwnerRef) != strings.TrimSpace(registry.OwnerRef) {
			return errRegistryCredentialScopeConflict
		}
		return nil
	case "project":
		if credential.Scope == "global" {
			return errRegistryCredentialScopeConflict
		}
		if credential.Scope == "project" {
			var boundProjects []string
			if err := tx.Model(&model.ScopedResourceProjectBinding{}).
				Where("resource_type = ? and resource_id = ?", scopedResourceArtifactRegistry, registry.ID).
				Pluck("project_id", &boundProjects).Error; err != nil {
				return err
			}
			allowed := make(map[string]bool, len(boundProjects))
			for _, projectID := range boundProjects {
				allowed[strings.TrimSpace(projectID)] = true
			}
			for _, projectID := range credential.ProjectIDs {
				if !allowed[strings.TrimSpace(projectID)] {
					return errRegistryCredentialScopeConflict
				}
			}
		}
		return nil
	default:
		return errRegistryCredentialScopeConflict
	}
}

func credentialCoversProjectTx(tx *gorm.DB, credential model.RegistryCredential, projectID string) (bool, error) {
	var count int64
	switch credential.Scope {
	case "global":
		return true, nil
	case "project":
		err := tx.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? and resource_id = ? and project_id = ?", scopedResourceRegistryCredential, credential.ID, projectID).
			Count(&count).Error
		return count > 0, err
	case "user":
		err := tx.Model(&model.ProjectMember{}).
			Where("project_id = ? and user_id = ?", projectID, credential.OwnerRef).
			Count(&count).Error
		return count > 0, err
	default:
		return false, nil
	}
}

func reconcileRegistryPushCredentialsTx(tx *gorm.DB, registry model.ArtifactRegistry, updatedCred *model.RegistryCredential) error {
	var referencingProjectIDs []string
	if err := tx.Model(&model.DeploymentTarget{}).
		Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
		Where("deployment_targets.target_registry_id = ? and (deployment_targets.source_type = '' or deployment_targets.source_type = ?)", registry.ID, "repository").
		Distinct("deployment_targets.project_id").
		Pluck("deployment_targets.project_id", &referencingProjectIDs).Error; err != nil {
		return err
	}

	if updatedCred != nil && registry.CredentialRef == updatedCred.ID {
		clearRef := !credentialAllowsPush(updatedCred.Usage)
		if !clearRef {
			for _, projectID := range referencingProjectIDs {
				covered, err := credentialCoversProjectTx(tx, *updatedCred, projectID)
				if err != nil {
					return err
				}
				if !covered {
					clearRef = true
					break
				}
			}
		}
		if clearRef {
			if err := tx.Model(&model.ArtifactRegistry{}).Where("id = ?", registry.ID).Update("credential_ref", "").Error; err != nil {
				return err
			}
		}
	}

	for _, projectID := range referencingProjectIDs {
		var count int64
		err := tx.Model(&model.RegistryCredential{}).
			Where("registry_id = ? and usage in ?", registry.ID, []string{"push", "push-pull"}).
			Where(
				"scope = ? or (scope = ? and exists (select 1 from scoped_resource_project_bindings srpb where srpb.resource_type = ? and srpb.resource_id = registry_credentials.id and srpb.project_id = ?)) or (scope = ? and exists (select 1 from project_members pm where pm.project_id = ? and pm.user_id = registry_credentials.owner_ref))",
				"global",
				"project", scopedResourceRegistryCredential, projectID,
				"user", projectID,
			).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return errRegistryCredentialInUse
		}
	}
	return nil
}

func (h *Handler) registryCredentialFor(user model.User, registry model.ArtifactRegistry, ctx context.Context) (model.RegistryCredential, bool) {
	var credential model.RegistryCredential
	if registry.CredentialRef != "" {
		query := h.applyScopedResourceVisibilityForUser(h.dbWithContext(ctx).Model(&model.RegistryCredential{}), scopedResourceRegistryCredential, user, ctx)
		if err := query.First(&credential, "id = ? and registry_id = ?", registry.CredentialRef, registry.ID).Error; err == nil {
			return credential, true
		}
	}
	query := h.applyScopedResourceVisibilityForUser(h.dbWithContext(ctx).Model(&model.RegistryCredential{}), scopedResourceRegistryCredential, user, ctx)
	if query.Where("registry_id = ?", registry.ID).Order("case scope when 'user' then 0 when 'project' then 1 else 2 end, created_at desc").First(&credential).Error == nil {
		return credential, true
	}
	return credential, false
}

func (h *Handler) registryPushCredentialFor(user model.User, registry model.ArtifactRegistry, ctx context.Context) (model.RegistryCredential, bool) {
	return h.registryPushCredentialForProject(user, registry, "", ctx)
}

func (h *Handler) registryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	if strings.TrimSpace(projectID) != "" && !h.registryUsableInProject(h.dbWithContext(ctx), user, registry, strings.TrimSpace(projectID), ctx) {
		return model.RegistryCredential{}, false
	}
	usages := []string{"push", "push-pull"}
	var credential model.RegistryCredential
	visibleCredentials := func() *gorm.DB {
		query := h.dbWithContext(ctx).Model(&model.RegistryCredential{})
		if strings.TrimSpace(projectID) != "" {
			return h.applyScopedResourceVisibilityForProject(query, scopedResourceRegistryCredential, user, projectID, ctx)
		}
		return h.applyScopedResourceVisibilityForUser(query, scopedResourceRegistryCredential, user, ctx)
	}
	if registry.CredentialRef != "" {
		query := visibleCredentials()
		if err := query.First(&credential, "id = ? and registry_id = ? and usage in ?", registry.CredentialRef, registry.ID, usages).Error; err == nil {
			return credential, true
		}
	}
	query := visibleCredentials()
	if query.Where("registry_id = ? and usage in ?", registry.ID, usages).Order("case scope when 'user' then 0 when 'project' then 1 else 2 end, created_at desc").First(&credential).Error == nil {
		return credential, true
	}
	return credential, false
}

func (h *Handler) attachRegistryCredentialProjects(credentials []model.RegistryCredential, ctx context.Context) {
	ids := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		ids = append(ids, credential.ID)
	}
	projectMap := h.scopedResourceProjectIDMap(scopedResourceRegistryCredential, ids, ctx)
	for index := range credentials {
		credentials[index].ProjectIDs = projectMap[credentials[index].ID]
	}
}
