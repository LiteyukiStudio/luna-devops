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
	if !h.canManageRegistryCredentials(ctx, user, registry) {
		return
	}

	var credentials []model.RegistryCredential
	query := h.db.Model(&model.RegistryCredential{}).Where("registry_id = ?", registry.ID)
	if registry.Scope == "global" {
		query = query.Where("created_by = ? and access_scope = ?", user.ID, "personal")
	} else {
		query = query.Where("access_scope = ? or (access_scope = ? and created_by = ?)", "registry", "personal", user.ID)
	}
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
		ctx.JSON(http.StatusOK, paginatedResponse(credentialResponses(credentials), total, pagination))
		return
	}
	if err := query.Order("created_at desc").Find(&credentials).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, credentialResponses(credentials))
}

func (h *Handlers) CreateRegistryCredential(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}
	if !h.canManageRegistry(ctx, user, registry) {
		return
	}

	var input registryCredentialInput
	if !bindJSON(ctx, &input) {
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
	accessScope := normalizeCredentialAccessScope(input.AccessScope)
	if accessScope == "registry" {
		if registry.Scope == "global" {
			writeError(ctx, http.StatusBadRequest, "全局镜像站凭据只能设为个人使用")
			return
		}
		if !h.canManageRegistry(ctx, user, registry) {
			return
		}
	} else if registry.Scope != "global" && !h.canManageRegistryCredentials(ctx, user, registry) {
		return
	}

	credential := model.RegistryCredential{
		ID:                 id.New("regc"),
		RegistryID:         registry.ID,
		Name:               fallback(strings.TrimSpace(input.Name), "default"),
		Username:           strings.TrimSpace(input.Username),
		PasswordRef:        h.secrets.Store(input.Password, user.ID, "registry_credential:"+registry.ID+":password"),
		TokenRef:           h.secrets.Store(input.Token, user.ID, "registry_credential:"+registry.ID+":token"),
		Scope:              normalizeCredentialScope(input.Scope),
		AccessScope:        accessScope,
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
		if registry.CredentialRef == "" {
			return tx.Model(&registry).Update("credential_ref", credential.ID).Error
		}
		return nil
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "registry_credential.create", credential.ID, true, credential.AccessScope)
	ctx.JSON(http.StatusCreated, credentialResponse(credential))
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
	h.audit(user.ID, "registry_credential.delete", credential.ID, true, credential.AccessScope)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) registryCredentialFor(user model.User, registry model.ArtifactRegistry) (model.RegistryCredential, bool) {
	var credential model.RegistryCredential
	if registry.CredentialRef != "" {
		if err := h.db.First(&credential, "id = ? and registry_id = ? and (access_scope = ? or created_by = ?)", registry.CredentialRef, registry.ID, "registry", user.ID).Error; err == nil {
			return credential, true
		}
	}
	if err := h.db.Where("registry_id = ? and access_scope = ? and created_by = ?", registry.ID, "personal", user.ID).Order("created_at desc").First(&credential).Error; err == nil {
		return credential, true
	}
	if registry.Scope != "global" && h.db.Where("registry_id = ? and access_scope = ?", registry.ID, "registry").Order("created_at desc").First(&credential).Error == nil {
		return credential, true
	}
	return credential, false
}

func (h *Handlers) registryPushCredentialFor(user model.User, registry model.ArtifactRegistry) (model.RegistryCredential, bool) {
	scopes := []string{"push", "push-pull"}
	var credential model.RegistryCredential
	if registry.CredentialRef != "" {
		if err := h.db.First(&credential, "id = ? and registry_id = ? and scope in ? and (access_scope = ? or created_by = ?)", registry.CredentialRef, registry.ID, scopes, "registry", user.ID).Error; err == nil {
			return credential, true
		}
	}
	if err := h.db.Where("registry_id = ? and access_scope = ? and created_by = ? and scope in ?", registry.ID, "personal", user.ID, scopes).Order("created_at desc").First(&credential).Error; err == nil {
		return credential, true
	}
	if registry.Scope != "global" && h.db.Where("registry_id = ? and access_scope = ? and scope in ?", registry.ID, "registry", scopes).Order("created_at desc").First(&credential).Error == nil {
		return credential, true
	}
	return credential, false
}
