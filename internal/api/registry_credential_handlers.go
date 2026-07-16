package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListRegistryCredentials(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	var credentials []model.RegistryCredential
	query := h.db.Model(&model.RegistryCredential{}).Where("registry_id = ?", registry.ID)
	query = h.applyScopedResourceVisibilityForUser(query, scopedResourceRegistryCredential, user)
	query = applySearch(ctx, query, "name", "username")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
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
		h.attachRegistryCredentialProjects(credentials)
		ctx.JSON(http.StatusOK, paginatedResponse(credentialResponses(credentials), total, pagination))
		return
	}
	if err := query.Order("created_at desc").Find(&credentials).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.attachRegistryCredentialProjects(credentials)
	ctx.JSON(http.StatusOK, credentialResponses(credentials))
}

func (h *Handlers) CreateRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	var input registryCredentialInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeRegistryCredentialUpdate) {
		return
	}
	if strings.TrimSpace(input.Password) != "" || strings.TrimSpace(input.Token) != "" {
		if err := secret.ValidateEncryptionConfig(); err != nil {
			status := http.StatusInternalServerError
			message := err.Error()
			if errors.Is(err, secret.ErrMissingEncryptionKey) {
				message = "SECRET_ENCRYPTION_KEY is required to save registry credentials in production"
			}
			writeError(ctx, status, message)
			return
		}
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
		PasswordRef:        h.secrets.Store(input.Password, user.ID, "registry_credential:"+registry.ID+":password"),
		TokenRef:           h.secrets.Store(input.Token, user.ID, "registry_credential:"+registry.ID+":token"),
		Usage:              normalizeCredentialUsage(input.Usage),
		Scope:              scope,
		OwnerRef:           ownerRef,
		ProjectIDs:         projectIDs,
		RepositoryTemplate: normalizeImageRepositoryTemplate(input.RepositoryTemplate),
		TagTemplate:        normalizeImageTagTemplate(input.TagTemplate),
		CreatedBy:          user.ID,
	}
	if credential.PasswordRef == "" && credential.TokenRef == "" {
		writeError(ctx, http.StatusBadRequest, "请填写 Registry 密码或 Token")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&credential).Error; err != nil {
			return err
		}
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, credential.ID, sortedProjectIDs(credential.ProjectIDs), nil); err != nil {
			return err
		}
		if registry.CredentialRef == "" {
			return tx.Model(&registry).Update("credential_ref", credential.ID).Error
		}
		return nil
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "registry_credential.create", credential.ID, true, credential.Scope)
	ctx.JSON(http.StatusCreated, credentialResponse(credential))
}

func (h *Handlers) UpdateRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}

	var credential model.RegistryCredential
	if err := h.db.First(&credential, "id = ? and registry_id = ?", ctx.Param("credentialId"), registry.ID).Error; err != nil {
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
	if !h.requireStepUp(ctx, user, stepUpPurposeRegistryCredentialUpdate) {
		return
	}
	if strings.TrimSpace(input.Password) != "" || strings.TrimSpace(input.Token) != "" {
		if err := secret.ValidateEncryptionConfig(); err != nil {
			status := http.StatusInternalServerError
			message := err.Error()
			if errors.Is(err, secret.ErrMissingEncryptionKey) {
				message = "SECRET_ENCRYPTION_KEY is required to save registry credentials in production"
			}
			writeError(ctx, status, message)
			return
		}
	}

	scope, ownerRef, projectIDs, scopeOK := h.normalizeCredentialScopeWithinParent(ctx, user, input.Scope, input.ProjectIDs, registry.Scope, registry.ProjectIDs, "只有平台管理员可以创建全局镜像凭据")
	if !scopeOK {
		return
	}

	passwordRef := credential.PasswordRef
	if strings.TrimSpace(input.Password) != "" {
		passwordRef = h.secrets.Store(input.Password, user.ID, "registry_credential:"+registry.ID+":password")
	}
	tokenRef := credential.TokenRef
	if strings.TrimSpace(input.Token) != "" {
		tokenRef = h.secrets.Store(input.Token, user.ID, "registry_credential:"+registry.ID+":token")
	}
	if passwordRef == "" && tokenRef == "" {
		writeError(ctx, http.StatusBadRequest, "请填写 Registry 密码或 Token")
		return
	}

	credential.Name = fallback(strings.TrimSpace(input.Name), "default")
	credential.Username = strings.TrimSpace(input.Username)
	credential.PasswordRef = passwordRef
	credential.TokenRef = tokenRef
	credential.Usage = normalizeCredentialUsage(input.Usage)
	credential.Scope = scope
	credential.OwnerRef = ownerRef
	credential.ProjectIDs = projectIDs
	credential.RepositoryTemplate = normalizeImageRepositoryTemplate(input.RepositoryTemplate)
	credential.TagTemplate = normalizeImageTagTemplate(input.TagTemplate)

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&credential).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceRegistryCredential, credential.ID, sortedProjectIDs(credential.ProjectIDs), nil)
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "registry_credential.update", credential.ID, true, credential.Scope)
	ctx.JSON(http.StatusOK, credentialResponse(credential))
}

func (h *Handlers) DeleteRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}

	var credential model.RegistryCredential
	if err := h.db.First(&credential, "id = ? and registry_id = ?", ctx.Param("credentialId"), registry.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "registry credential not found")
		return
	}
	if !h.canManageRegistryCredential(ctx, user, registry, credential) {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceRegistryCredential, credential.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&credential).Error; err != nil {
			return err
		}
		if registry.CredentialRef == credential.ID {
			return tx.Model(&registry).Update("credential_ref", "").Error
		}
		return nil
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "registry_credential.delete", credential.ID, true, credential.Scope)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) registryCredentialFor(user model.User, registry model.ArtifactRegistry) (model.RegistryCredential, bool) {
	var credential model.RegistryCredential
	if registry.CredentialRef != "" {
		query := h.applyScopedResourceVisibilityForUser(h.db.Model(&model.RegistryCredential{}), scopedResourceRegistryCredential, user)
		if err := query.First(&credential, "id = ? and registry_id = ?", registry.CredentialRef, registry.ID).Error; err == nil {
			return credential, true
		}
	}
	query := h.applyScopedResourceVisibilityForUser(h.db.Model(&model.RegistryCredential{}), scopedResourceRegistryCredential, user)
	if query.Where("registry_id = ?", registry.ID).Order("case scope when 'user' then 0 when 'project' then 1 else 2 end, created_at desc").First(&credential).Error == nil {
		return credential, true
	}
	return credential, false
}

func (h *Handlers) registryPushCredentialFor(user model.User, registry model.ArtifactRegistry) (model.RegistryCredential, bool) {
	return h.registryPushCredentialForProject(user, registry, "")
}

func (h *Handlers) registryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string) (model.RegistryCredential, bool) {
	usages := []string{"push", "push-pull"}
	var credential model.RegistryCredential
	visibleCredentials := func() *gorm.DB {
		query := h.db.Model(&model.RegistryCredential{})
		if strings.TrimSpace(projectID) != "" {
			return h.applyScopedResourceVisibilityForProject(query, scopedResourceRegistryCredential, user, projectID)
		}
		return h.applyScopedResourceVisibilityForUser(query, scopedResourceRegistryCredential, user)
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

func (h *Handlers) attachRegistryCredentialProjects(credentials []model.RegistryCredential) {
	ids := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		ids = append(ids, credential.ID)
	}
	projectMap := h.scopedResourceProjectIDMap(scopedResourceRegistryCredential, ids)
	for index := range credentials {
		credentials[index].ProjectIDs = projectMap[credentials[index].ID]
	}
}
