package notificationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type notificationDraftTestInput struct {
	ChannelID     string            `json:"channelId"`
	Scope         string            `json:"scope"`
	PresetID      string            `json:"presetId"`
	AdapterKind   string            `json:"adapterKind"`
	Config        json.RawMessage   `json:"config"`
	Secrets       map[string]string `json:"secrets"`
	TestRecipient string            `json:"testRecipient"`
}

func (h *Handler) ListScopedNotificationPresets(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	ctx.JSON(http.StatusOK, notification.WebhookPresets())
}

func (h *Handler) ListScopedNotificationChannels(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Query("projectId"))
	if projectID != "" {
		if _, _, authorized := h.host.AuthorizeProjectByID(ctx, projectID, authz.ActionNotificationRead); !authorized {
			return
		}
	}
	readableProjectIDs, err := h.readableNotificationProjectIDs(ctx.Request.Context(), user)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}

	pagination := paginationFromQuery(ctx)
	query := h.scopedNotificationChannelsQuery(ctx, user, projectID, readableProjectIDs)
	if scopeFilter := strings.TrimSpace(ctx.Query("scope")); scopeFilter != "" {
		query = query.Where("scope = ?", notification.NormalizeChannelScope(scopeFilter))
	}
	if search := strings.TrimSpace(ctx.Query("search")); search != "" {
		query = query.Where("name ILIKE ? or adapter_kind ILIKE ? or preset_id ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var channels []model.NotificationChannel
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"adapter":   "adapter_kind",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&channels).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.populateLatestNotificationDeliveries(channels, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	responses, err := h.buildScopedChannelResponses(ctx.Request.Context(), user, channels, readableProjectIDs)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handler) CreateScopedNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input notificationChannelInput
	if !bindJSON(ctx, &input) {
		return
	}
	scope, projectIDs, ok := h.authorizeNotificationChannelScopeWrite(ctx, user, input.Scope, input.ProjectIDs)
	if !ok {
		return
	}
	channel, secretValues, ok := h.prepareScopedNotificationChannel(ctx, user, input, model.NotificationChannel{
		ID:          id.New("nch"),
		Scope:       scope,
		OwnerUserID: "",
		CreatedBy:   user.ID,
	})
	if !ok {
		return
	}
	channel.Scope = scope

	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		secretRefs := map[string]string{}
		for key, value := range secretValues {
			ref, storeErr := h.secrets().StoreContextWithDB(ctx.Request.Context(), tx, value, user.ID, "notification_channel:"+channel.ID+":"+key)
			if storeErr != nil {
				return storeErr
			}
			secretRefs[key] = ref
		}
		channel.ConfigJSON = attachSMTPPasswordRef(channel.AdapterKind, channel.ConfigJSON, secretRefs)
		channel.SecretRefsJSON = mustJSON(secretRefs)
		resolver := staticOverlaySecretResolver{overlay: secretValues, fallback: h.secrets()}
		if validateErr := validateNotificationChannel(ctx.Request.Context(), channel, resolver); validateErr != nil {
			return errChannelValidation{err: validateErr}
		}
		if createErr := tx.Create(&channel).Error; createErr != nil {
			return createErr
		}
		if scope == "project" {
			return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, channel.ID, projectIDs, nil)
		}
		return nil
	})
	var validationErr errChannelValidation
	if errors.As(err, &validationErr) {
		writeError(ctx, http.StatusBadRequest, validationErr.err.Error())
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	channel.ProjectIDs = projectIDs
	h.auditWithContext(user.ID, "notification.channel.create", channel.ID, true, scope, ctx.Request.Context())
	resp := notificationChannelResponseForWithPrivacy(channel, true)
	resp.CanManage = true
	ctx.JSON(http.StatusCreated, resp)
}

func (h *Handler) UpdateScopedNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	channelID := strings.TrimSpace(ctx.Param("channelId"))
	var existing model.NotificationChannel
	if err := h.dbFor(ctx).First(&existing, "id = ? and owner_user_id = ?", channelID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	existingProjectIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, existing.ID, ctx.Request.Context())
	if !h.requireManageNotificationChannel(ctx, user, existing, existingProjectIDs) {
		return
	}

	var input notificationChannelInput
	if !bindJSON(ctx, &input) {
		return
	}
	targetScope := strings.TrimSpace(input.Scope)
	if targetScope == "" {
		targetScope = notification.NormalizeChannelScope(existing.Scope)
	}
	targetProjectIDs := input.ProjectIDs
	if targetScope == "project" && len(targetProjectIDs) == 0 && input.ProjectIDs == nil {
		targetProjectIDs = existingProjectIDs
	}
	scope, projectIDs, ok := h.authorizeNotificationChannelScopeWrite(ctx, user, targetScope, targetProjectIDs)
	if !ok {
		return
	}
	updated, secretValues, ok := h.prepareScopedNotificationChannel(ctx, user, input, existing)
	if !ok {
		return
	}
	updated.Scope = scope

	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		secretRefs := decodeStringMap(existing.SecretRefsJSON)
		for key, value := range secretValues {
			resource := "notification_channel:" + updated.ID + ":" + key
			newRef, storeErr := h.secrets().StoreContextWithDB(ctx.Request.Context(), tx, value, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			oldRef := secretRefs[key]
			secretRefs[key] = newRef
			if strings.TrimSpace(oldRef) != "" {
				if delErr := h.secrets().DeleteRefContextWithDB(ctx.Request.Context(), tx, oldRef, resource); delErr != nil {
					return delErr
				}
			}
		}
		updated.ConfigJSON = attachSMTPPasswordRef(updated.AdapterKind, updated.ConfigJSON, secretRefs)
		updated.SecretRefsJSON = mustJSON(secretRefs)
		resolver := staticOverlaySecretResolver{overlay: secretValues, fallback: h.secrets()}
		if validateErr := validateNotificationChannel(ctx.Request.Context(), updated, resolver); validateErr != nil {
			return errChannelValidation{err: validateErr}
		}
		if saveErr := tx.Save(&updated).Error; saveErr != nil {
			return saveErr
		}
		if scope == "project" {
			return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, updated.ID, projectIDs, nil)
		}
		return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, updated.ID, nil, nil)
	})
	var validationErr errChannelValidation
	if errors.As(err, &validationErr) {
		writeError(ctx, http.StatusBadRequest, validationErr.err.Error())
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	updated.ProjectIDs = projectIDs
	h.auditWithContext(user.ID, "notification.channel.update", updated.ID, true, scope, ctx.Request.Context())
	resp := notificationChannelResponseForWithPrivacy(updated, true)
	resp.CanManage = true
	ctx.JSON(http.StatusOK, resp)
}

func (h *Handler) DeleteScopedNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	channelID := strings.TrimSpace(ctx.Param("channelId"))
	var existing model.NotificationChannel
	if err := h.dbFor(ctx).First(&existing, "id = ? and owner_user_id = ?", channelID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	existingProjectIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, existing.ID, ctx.Request.Context())
	if !h.requireManageNotificationChannel(ctx, user, existing, existingProjectIDs) {
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if bindErr := h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, existing.ID, nil, nil); bindErr != nil {
			return bindErr
		}
		for key, ref := range decodeStringMap(existing.SecretRefsJSON) {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			resource := "notification_channel:" + existing.ID + ":" + key
			if delErr := h.secrets().DeleteRefContextWithDB(ctx.Request.Context(), tx, ref, resource); delErr != nil {
				return delErr
			}
		}
		return tx.Delete(&existing).Error
	})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.delete", existing.ID, true, existing.Scope, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) TestScopedNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	channelID := strings.TrimSpace(ctx.Param("channelId"))
	var channel model.NotificationChannel
	if err := h.dbFor(ctx).First(&channel, "id = ? and owner_user_id = ?", channelID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	projectIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, channel.ID, ctx.Request.Context())
	if !h.requireTestNotificationChannel(ctx, user, channel, projectIDs) {
		return
	}
	if !h.allowPersonalNotificationTest(ctx, user.ID) {
		return
	}
	adapter, err := notification.DefaultRegistry().Adapter(channel.AdapterKind)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	testCtx := notification.WithEgressPolicy(ctx.Request.Context(), notification.EgressPolicyForScope(channel.Scope))
	if recipient := h.resolveTestRecipient(ctx, user); recipient != "" {
		testCtx = notification.WithTestRecipient(testCtx, recipient)
	}
	if err := adapter.Test(testCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), h.secrets()); err != nil {
		h.auditWithContext(user.ID, "notification.channel.test", channel.ID, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusBadGateway, "notification.channel_test_failed", err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.test", channel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) TestDraftScopedNotificationChannel(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input notificationDraftTestInput
	if !bindJSON(ctx, &input) {
		return
	}
	scope := strings.ToLower(strings.TrimSpace(input.Scope))
	if scope == "" {
		scope = "project"
	}
	if scope == "global" {
		if !authz.IsPlatformAdmin(user.Role) {
			writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
			return
		}
	} else if scope == "project" {
		if !authz.IsPlatformAdmin(user.Role) {
			ownedProjects, err := h.manageableNotificationProjectIDs(ctx.Request.Context(), user)
			if err != nil {
				h.host.WriteProjectAuthorizationError(ctx, err)
				return
			}
			if len(ownedProjects) == 0 {
				writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "project owner role is required to test notification channels")
				return
			}
		}
	} else {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.scope_invalid", "notification scope must be project or global")
		return
	}
	if !h.allowPersonalNotificationTest(ctx, user.ID) {
		return
	}

	baseChannel := model.NotificationChannel{
		ID:    "draft",
		Scope: scope,
	}
	if existingID := strings.TrimSpace(input.ChannelID); existingID != "" {
		var existing model.NotificationChannel
		if err := h.dbFor(ctx).First(&existing, "id = ? and owner_user_id = ?", existingID, "").Error; err != nil {
			writeError(ctx, http.StatusNotFound, "notification channel not found")
			return
		}
		existingProjects := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, existing.ID, ctx.Request.Context())
		if !h.requireManageNotificationChannel(ctx, user, existing, existingProjects) {
			return
		}
		baseChannel = existing
	}

	prepared, secretOverlay, ok := h.prepareScopedNotificationChannel(ctx, user, notificationChannelInput{
		Name:        "Draft test",
		Scope:       scope,
		PresetID:    input.PresetID,
		AdapterKind: input.AdapterKind,
		Config:      input.Config,
		Secrets:     input.Secrets,
	}, baseChannel)
	if !ok {
		return
	}
	secretRefs := decodeStringMap(baseChannel.SecretRefsJSON)
	if strings.TrimSpace(input.ChannelID) != "" && !sameNotificationTargetEndpoint(baseChannel, prepared) {
		// Prevent exfiltrating stored secrets to an untrusted new host/URL during draft tests.
		secretRefs = map[string]string{}
	}
	for key := range secretOverlay {
		secretRefs[key] = "draft:" + key
	}
	resolver := staticOverlaySecretResolver{
		overlay:  draftOverlayByRef(secretOverlay),
		fallback: h.secrets(),
	}
	prepared.ConfigJSON = attachSMTPPasswordRef(prepared.AdapterKind, prepared.ConfigJSON, secretRefs)
	prepared.SecretRefsJSON = mustJSON(secretRefs)
	if err := validateNotificationChannel(ctx.Request.Context(), prepared, resolver); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	adapter, err := notification.DefaultRegistry().Adapter(prepared.AdapterKind)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	testCtx := notification.WithEgressPolicy(ctx.Request.Context(), notification.EgressPolicyForScope(scope))
	recipient := strings.TrimSpace(input.TestRecipient)
	if recipient == "" {
		recipient = strings.TrimSpace(user.Email)
	}
	if recipient != "" {
		if _, parseErr := mail.ParseAddress(recipient); parseErr != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.test_recipient_invalid", parseErr.Error())
			return
		}
		testCtx = notification.WithTestRecipient(testCtx, recipient)
	}
	if err := adapter.Test(testCtx, json.RawMessage(prepared.ConfigJSON), json.RawMessage(prepared.SecretRefsJSON), resolver); err != nil {
		h.auditWithContext(user.ID, "notification.channel.test_draft", baseChannel.ID, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusBadGateway, "notification.channel_test_failed", err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.test_draft", baseChannel.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ListProjectNotificationChannels(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationRead)
	if !ok {
		return
	}
	readableProjectIDs, err := h.readableNotificationProjectIDs(ctx.Request.Context(), user)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.scopedNotificationChannelsQuery(ctx, user, project.ID, readableProjectIDs)
	if search := strings.TrimSpace(ctx.Query("search")); search != "" {
		query = query.Where("name ILIKE ? or adapter_kind ILIKE ? or preset_id ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var channels []model.NotificationChannel
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"adapter":   "adapter_kind",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&channels).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.populateLatestNotificationDeliveries(channels, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	responses, err := h.buildScopedChannelResponses(ctx.Request.Context(), user, channels, readableProjectIDs)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handler) BindProjectNotificationChannel(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationManage)
	if !ok {
		return
	}
	if !h.host.EnsureProjectCanMutate(ctx, project) {
		return
	}
	channelID := strings.TrimSpace(ctx.Param("channelId"))
	var channel model.NotificationChannel
	if err := h.dbFor(ctx).First(&channel, "id = ? and owner_user_id = ?", channelID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	if notification.NormalizeChannelScope(channel.Scope) != "project" {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_scope_not_project", "only project-scoped notification channels can be bound to a project space")
		return
	}
	existingProjectIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, channel.ID, ctx.Request.Context())
	if !h.requireManageNotificationChannel(ctx, user, channel, existingProjectIDs) {
		return
	}
	nextProjects := normalizeUniqueStrings(append(append([]string(nil), existingProjectIDs...), project.ID))
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, channel.ID, nextProjects, nil)
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	channel.ProjectIDs = nextProjects
	h.auditWithContext(user.ID, "notification.channel.bind_project", channel.ID, true, project.ID, ctx.Request.Context())
	resp := notificationChannelResponseForWithPrivacy(channel, true)
	resp.CanManage = true
	ctx.JSON(http.StatusOK, resp)
}

func (h *Handler) UnbindProjectNotificationChannel(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationManage)
	if !ok {
		return
	}
	if !h.host.EnsureProjectCanMutate(ctx, project) {
		return
	}
	channelID := strings.TrimSpace(ctx.Param("channelId"))
	var channel model.NotificationChannel
	if err := h.dbFor(ctx).First(&channel, "id = ? and owner_user_id = ?", channelID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification channel not found")
		return
	}
	if notification.NormalizeChannelScope(channel.Scope) != "project" {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_scope_not_project", "global notification channels cannot be unbound from a single project space")
		return
	}
	existingProjectIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, channel.ID, ctx.Request.Context())
	boundToProject := false
	remaining := make([]string, 0, len(existingProjectIDs))
	for _, idValue := range existingProjectIDs {
		if idValue == project.ID {
			boundToProject = true
			continue
		}
		remaining = append(remaining, idValue)
	}
	if !boundToProject {
		writeError(ctx, http.StatusNotFound, "notification channel is not bound to this project space")
		return
	}

	var projectRules []model.NotificationRule
	if err := h.dbFor(ctx).Where("project_id = ?", project.ID).Find(&projectRules).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	for _, rule := range projectRules {
		for _, usedID := range decodeStringList(rule.ChannelIDsJSON) {
			if strings.TrimSpace(usedID) == channel.ID {
				writeErrorCode(ctx, http.StatusConflict, "notification.channel_in_use", "remove this channel from project notification rules before unbinding")
				return
			}
		}
	}

	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if bindErr := h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, channel.ID, remaining, nil); bindErr != nil {
			return bindErr
		}
		if len(remaining) == 0 {
			for key, ref := range decodeStringMap(channel.SecretRefsJSON) {
				if strings.TrimSpace(ref) == "" {
					continue
				}
				resource := "notification_channel:" + channel.ID + ":" + key
				if delErr := h.secrets().DeleteRefContextWithDB(ctx.Request.Context(), tx, ref, resource); delErr != nil {
					return delErr
				}
			}
			return tx.Delete(&channel).Error
		}
		return nil
	})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.channel.unbind_project", channel.ID, true, project.ID, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) ListProjectNotificationRules(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationRead)
	if !ok {
		return
	}
	canManage, err := h.host.ProjectRoleActionAllowed(ctx.Request.Context(), user, project.ID, authz.ActionNotificationManage)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.dbFor(ctx).Model(&model.NotificationRule{}).Where("project_id = ?", project.ID)
	if search := strings.TrimSpace(ctx.Query("search")); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var rules []model.NotificationRule
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&rules).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !canManage && !authz.IsPlatformAdmin(user.Role) {
		for idx := range rules {
			rules[idx].ChannelTargetsJSON = redactChannelTargetsJSON(rules[idx].ChannelTargetsJSON)
		}
	}
	ctx.JSON(http.StatusOK, paginatedResponse(rules, total, pagination))
}

func (h *Handler) CreateProjectNotificationRule(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationManage)
	if !ok {
		return
	}
	if !h.host.EnsureProjectCanMutate(ctx, project) {
		return
	}
	var input NotificationRuleInput
	if !bindJSON(ctx, &input) {
		return
	}
	rule, ok := h.NotificationRuleFromInput(ctx, input, model.NotificationRule{
		ID:        id.New("nrl"),
		ProjectID: project.ID,
		CreatedBy: user.ID,
	})
	if !ok {
		return
	}
	if !h.validateProjectRuleScopeReferences(ctx, project.ID, rule.FilterJSON) {
		return
	}
	if err := h.dbFor(ctx).Create(&rule).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.project_rule.create", rule.ID, true, project.ID, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, rule)
}

func (h *Handler) UpdateProjectNotificationRule(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationManage)
	if !ok {
		return
	}
	if !h.host.EnsureProjectCanMutate(ctx, project) {
		return
	}
	ruleID := strings.TrimSpace(ctx.Param("ruleId"))
	var existing model.NotificationRule
	if err := h.dbFor(ctx).First(&existing, "id = ? and project_id = ?", ruleID, project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification rule not found")
		return
	}
	var input NotificationRuleInput
	if !bindJSON(ctx, &input) {
		return
	}
	rule, ok := h.NotificationRuleFromInput(ctx, input, existing)
	if !ok {
		return
	}
	if !h.validateProjectRuleScopeReferences(ctx, project.ID, rule.FilterJSON) {
		return
	}
	if err := h.dbFor(ctx).Save(&rule).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "notification.project_rule.update", rule.ID, true, project.ID, ctx.Request.Context())
	ctx.JSON(http.StatusOK, rule)
}

func (h *Handler) DeleteProjectNotificationRule(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationManage)
	if !ok {
		return
	}
	if !h.host.EnsureProjectCanMutate(ctx, project) {
		return
	}
	ruleID := strings.TrimSpace(ctx.Param("ruleId"))
	result := h.dbFor(ctx).Where("id = ? and project_id = ?", ruleID, project.ID).Delete(&model.NotificationRule{})
	if result.Error != nil {
		writeError(ctx, http.StatusInternalServerError, result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		writeError(ctx, http.StatusNotFound, "notification rule not found")
		return
	}
	h.auditWithContext(user.ID, "notification.project_rule.delete", ruleID, true, project.ID, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) ListProjectNotificationDeliveries(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationRead)
	if !ok {
		return
	}
	canManage, err := h.host.ProjectRoleActionAllowed(ctx.Request.Context(), user, project.ID, authz.ActionNotificationManage)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.dbFor(ctx).Model(&model.NotificationDelivery{}).Where("project_id = ? and recipient_user_id = ?", project.ID, "")
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if eventType := strings.TrimSpace(ctx.Query("eventType")); eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	if channelID := strings.TrimSpace(ctx.Query("channelId")); channelID != "" {
		query = query.Where("channel_id = ?", channelID)
	}
	if ruleID := strings.TrimSpace(ctx.Query("ruleId")); ruleID != "" {
		query = query.Where("rule_id = ?", ruleID)
	}
	if eventID := strings.TrimSpace(ctx.Query("eventId")); eventID != "" {
		query = query.Where("event_id = ?", eventID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var deliveries []model.NotificationDelivery
	if err := query.Order(orderByClause(pagination, map[string]string{
		"createdAt":      "created_at",
		"queuedAt":       "queued_at",
		"finishedAt":     "finished_at",
		"durationMillis": "duration_millis",
		"status":         "status",
		"eventType":      "event_type",
	}, "created_at desc")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&deliveries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	includePrivateRecipients := canManage || authz.IsPlatformAdmin(user.Role)
	h.enrichNotificationDeliveries(deliveries, includePrivateRecipients, ctx.Request.Context())
	ctx.JSON(http.StatusOK, paginatedResponse(deliveries, total, pagination))
}

func (h *Handler) RetryProjectNotificationDelivery(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionNotificationManage)
	if !ok {
		return
	}
	if !h.host.EnsureProjectCanMutate(ctx, project) {
		return
	}
	deliveryID := strings.TrimSpace(ctx.Param("deliveryId"))
	var existing model.NotificationDelivery
	if err := h.dbFor(ctx).First(&existing, "id = ? and project_id = ? and recipient_user_id = ?", deliveryID, project.ID, "").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "notification delivery not found")
		return
	}
	svc := notification.Service{DB: h.dbFor(ctx), Enqueuer: h.host.DeliveryEnqueuer()}
	retried, err := svc.RetryDelivery(ctx.Request.Context(), existing.ID)
	if errors.Is(err, notification.ErrDeliveryActive) {
		writeErrorCode(ctx, http.StatusConflict, "notification.delivery_active", err.Error())
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	items := []model.NotificationDelivery{retried}
	h.enrichNotificationDeliveries(items, true, ctx.Request.Context())
	h.auditWithContext(user.ID, "notification.project_delivery.retry", retried.ID, true, project.ID, ctx.Request.Context())
	ctx.JSON(http.StatusOK, items[0])
}

// --- Helpers ---

type errChannelValidation struct {
	err error
}

func (e errChannelValidation) Error() string { return e.err.Error() }

type staticOverlaySecretResolver struct {
	overlay  map[string]string
	fallback notification.SecretResolver
}

func (r staticOverlaySecretResolver) ResolveContext(ctx context.Context, ref string) string {
	if value, ok := r.overlay[ref]; ok {
		return value
	}
	if r.fallback != nil {
		return r.fallback.ResolveContext(ctx, ref)
	}
	return ""
}

func draftOverlayByRef(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out["draft:"+key] = value
		out[key] = value
	}
	return out
}

func (h *Handler) scopedNotificationChannelsQuery(ctx *gin.Context, user model.User, projectID string, readableProjectIDs []string) *gorm.DB {
	db := h.dbFor(ctx)
	query := db.Model(&model.NotificationChannel{}).Where("owner_user_id = ?", "")
	if projectID != "" {
		subquery := db.Model(&model.ScopedResourceProjectBinding{}).
			Select("resource_id").
			Where("resource_type = ? and project_id = ?", notification.ScopedResourceChannelType, projectID)
		return query.Where("scope = 'global' or (scope = 'project' and id in (?))", subquery)
	}
	if authz.IsPlatformAdmin(user.Role) {
		return query.Where("scope in ?", []string{"global", "project"})
	}
	if len(readableProjectIDs) == 0 {
		return query.Where("scope = 'global'")
	}
	subquery := db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? and project_id in ?", notification.ScopedResourceChannelType, readableProjectIDs)
	return query.Where("scope = 'global' or (scope = 'project' and id in (?))", subquery)
}

func (h *Handler) readableNotificationProjectIDs(ctx context.Context, user model.User) ([]string, error) {
	if authz.IsPlatformAdmin(user.Role) {
		return nil, nil
	}
	candidateIDs := h.host.ProjectIDsForUser(ctx, user.ID)
	out := make([]string, 0, len(candidateIDs))
	for _, projectID := range candidateIDs {
		allowed, err := h.host.ProjectRoleActionAllowed(ctx, user, projectID, authz.ActionNotificationRead)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, projectID)
		}
	}
	return normalizeUniqueStrings(out), nil
}

func (h *Handler) manageableNotificationProjectIDs(ctx context.Context, user model.User) ([]string, error) {
	candidateIDs := h.host.ProjectIDsForUser(ctx, user.ID)
	out := make([]string, 0, len(candidateIDs))
	for _, projectID := range candidateIDs {
		allowed, err := h.host.ProjectRoleActionAllowed(ctx, user, projectID, authz.ActionNotificationManage)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, projectID)
		}
	}
	return normalizeUniqueStrings(out), nil
}

func (h *Handler) authorizeNotificationChannelScopeWrite(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string) (string, []string, bool) {
	scope := strings.ToLower(strings.TrimSpace(rawScope))
	if scope == "" {
		scope = "project"
	}
	switch scope {
	case "global":
		if !authz.IsPlatformAdmin(user.Role) {
			writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
			return "", nil, false
		}
		return "global", nil, true
	case "project":
		projectIDs := normalizeUniqueStrings(rawProjectIDs)
		if len(projectIDs) == 0 {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.project_required", "at least one project space is required")
			return "", nil, false
		}
		for _, projectID := range projectIDs {
			var project model.Project
			if err := h.dbFor(ctx).First(&project, "id = ?", projectID).Error; err != nil {
				writeError(ctx, http.StatusNotFound, "project not found")
				return "", nil, false
			}
			if !h.host.EnsureProjectCanMutate(ctx, project) {
				return "", nil, false
			}
			if authz.IsPlatformAdmin(user.Role) {
				continue
			}
			allowed, err := h.host.ProjectRoleActionAllowed(ctx.Request.Context(), user, projectID, authz.ActionNotificationManage)
			if err != nil {
				h.host.WriteProjectAuthorizationError(ctx, err)
				return "", nil, false
			}
			if !allowed {
				writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "project owner role is required for all bound project spaces")
				return "", nil, false
			}
		}
		return "project", projectIDs, true
	default:
		writeErrorCode(ctx, http.StatusBadRequest, "notification.scope_invalid", "notification scope must be project or global")
		return "", nil, false
	}
}

func (h *Handler) canManageNotificationChannel(ctx context.Context, user model.User, channel model.NotificationChannel, boundProjectIDs []string) (bool, error) {
	if authz.IsPlatformAdmin(user.Role) {
		return true, nil
	}
	if notification.NormalizeChannelScope(channel.Scope) != "project" {
		return false, nil
	}
	if len(boundProjectIDs) == 0 {
		return channel.CreatedBy == user.ID, nil
	}
	for _, projectID := range boundProjectIDs {
		allowed, err := h.host.ProjectRoleActionAllowed(ctx, user, projectID, authz.ActionNotificationManage)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

func (h *Handler) requireManageNotificationChannel(ctx *gin.Context, user model.User, channel model.NotificationChannel, boundProjectIDs []string) bool {
	canManage, err := h.canManageNotificationChannel(ctx.Request.Context(), user, channel, boundProjectIDs)
	if err != nil {
		h.host.WriteProjectAuthorizationError(ctx, err)
		return false
	}
	if !canManage {
		writeErrorCode(ctx, http.StatusForbidden, "notification.channel_shared_owner_required", "project owner permission is required on all bound project spaces to modify this channel")
		return false
	}
	return true
}

func (h *Handler) requireTestNotificationChannel(ctx *gin.Context, user model.User, channel model.NotificationChannel, boundProjectIDs []string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	if notification.NormalizeChannelScope(channel.Scope) != "project" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return false
	}
	for _, projectID := range boundProjectIDs {
		allowed, err := h.host.ProjectRoleActionAllowed(ctx.Request.Context(), user, projectID, authz.ActionNotificationManage)
		if err != nil {
			h.host.WriteProjectAuthorizationError(ctx, err)
			return false
		}
		if allowed {
			return true
		}
	}
	writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "project owner role is required to test this notification channel")
	return false
}

func (h *Handler) prepareScopedNotificationChannel(
	ctx *gin.Context,
	user model.User,
	input notificationChannelInput,
	channel model.NotificationChannel,
) (model.NotificationChannel, map[string]string, bool) {
	presetID := strings.TrimSpace(input.PresetID)
	if presetID == "" {
		presetID = strings.TrimSpace(channel.PresetID)
	}
	secretValues := map[string]string{}
	for key, value := range input.Secrets {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			secretValues[key] = value
		}
	}

	if presetID != "" {
		preset, found := findWebhookPreset(presetID)
		if !found {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.preset_not_found", "notification preset not found")
			return model.NotificationChannel{}, nil, false
		}
		channel.PresetID = preset.ID
		channel.AdapterKind = preset.AdapterKind
		channel.ConfigJSON = preset.ConfigTemplate
		channel.Name = firstNonEmpty(input.Name, channel.Name, preset.Name)
		existingRefs := decodeStringMap(channel.SecretRefsJSON)
		for _, field := range preset.SecretFields {
			if strings.TrimSpace(secretValues[field]) == "" && strings.TrimSpace(existingRefs[field]) == "" {
				writeErrorCode(ctx, http.StatusBadRequest, "notification.secret_required", fmt.Sprintf("notification preset secret %s is required", field))
				return model.NotificationChannel{}, nil, false
			}
		}
	} else {
		channel.Name = strings.TrimSpace(input.Name)
		channel.AdapterKind = strings.TrimSpace(firstNonEmpty(input.AdapterKind, channel.AdapterKind))
		if channel.Name == "" || channel.AdapterKind == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.channel_required", "notification channel name and adapter kind are required")
			return model.NotificationChannel{}, nil, false
		}
		configRaw := normalizedJSON(input.Config, channel.ConfigJSON)
		configMap := map[string]any{}
		_ = json.Unmarshal(configRaw, &configMap)
		if password, ok := configMap["password"].(string); ok && strings.TrimSpace(password) != "" {
			secretValues["password"] = strings.TrimSpace(password)
			delete(configMap, "password")
			if marshaled, err := json.Marshal(configMap); err == nil {
				configRaw = marshaled
			}
		}
		channel.ConfigJSON = string(configRaw)
	}

	if input.Enabled != nil {
		channel.Enabled = *input.Enabled
	} else if channel.CreatedAt.IsZero() {
		channel.Enabled = true
	}
	channel.CreatedBy = firstNonEmpty(channel.CreatedBy, user.ID)
	return channel, secretValues, true
}

func attachSMTPPasswordRef(adapterKind string, configJSON string, secretRefs map[string]string) string {
	if adapterKind != notification.AdapterKindSMTP {
		return configJSON
	}
	ref := strings.TrimSpace(secretRefs["password"])
	if ref == "" {
		return configJSON
	}
	config := map[string]any{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return configJSON
	}
	config["passwordRef"] = ref
	data, err := json.Marshal(config)
	if err != nil {
		return configJSON
	}
	return string(data)
}

func sameNotificationTargetEndpoint(left, right model.NotificationChannel) bool {
	if left.AdapterKind != right.AdapterKind {
		return false
	}
	leftMap, _ := decodeJSONValue(left.ConfigJSON).(map[string]any)
	rightMap, _ := decodeJSONValue(right.ConfigJSON).(map[string]any)
	if left.AdapterKind == notification.AdapterKindSMTP {
		leftHost := strings.ToLower(strings.TrimSpace(fmt.Sprint(leftMap["host"])))
		rightHost := strings.ToLower(strings.TrimSpace(fmt.Sprint(rightMap["host"])))
		leftPort := fmt.Sprint(leftMap["port"])
		rightPort := fmt.Sprint(rightMap["port"])
		return leftHost == rightHost && (leftPort == rightPort || rightPort == "<nil>" || rightPort == "0")
	}
	leftURL := strings.TrimSpace(fmt.Sprint(leftMap["url"]))
	rightURL := strings.TrimSpace(fmt.Sprint(rightMap["url"]))
	return leftURL == rightURL
}

func findWebhookPreset(presetID string) (notification.WebhookPreset, bool) {
	presetID = strings.TrimSpace(presetID)
	for _, preset := range notification.WebhookPresets() {
		if preset.ID == presetID {
			return preset, true
		}
	}
	return notification.WebhookPreset{}, false
}

func (h *Handler) populateChannelProjectIDs(channels []model.NotificationChannel, ctx context.Context) {
	ids := make([]string, 0, len(channels))
	for _, channel := range channels {
		if notification.NormalizeChannelScope(channel.Scope) == "project" {
			ids = append(ids, channel.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	byChannel := h.host.ScopedResourceProjectIDMap(notification.ScopedResourceChannelType, ids, ctx)
	for index := range channels {
		if projects := byChannel[channels[index].ID]; len(projects) > 0 {
			channels[index].ProjectIDs = projects
		}
	}
}

func (h *Handler) buildScopedChannelResponses(
	ctx context.Context,
	user model.User,
	channels []model.NotificationChannel,
	readableProjectIDs []string,
) ([]notificationChannelResponse, error) {
	ids := make([]string, 0, len(channels))
	for _, channel := range channels {
		if notification.NormalizeChannelScope(channel.Scope) == "project" {
			ids = append(ids, channel.ID)
		}
	}
	allBindings := h.host.ScopedResourceProjectIDMap(notification.ScopedResourceChannelType, ids, ctx)
	readableSet := make(map[string]bool, len(readableProjectIDs))
	for _, projectID := range readableProjectIDs {
		readableSet[projectID] = true
	}

	responses := make([]notificationChannelResponse, 0, len(channels))
	for _, channel := range channels {
		boundIDs := allBindings[channel.ID]
		canManage, err := h.canManageNotificationChannel(ctx, user, channel, boundIDs)
		if err != nil {
			return nil, err
		}
		if authz.IsPlatformAdmin(user.Role) {
			channel.ProjectIDs = boundIDs
		} else {
			visibleIDs := make([]string, 0, len(boundIDs))
			for _, projectID := range boundIDs {
				if readableSet[projectID] {
					visibleIDs = append(visibleIDs, projectID)
				}
			}
			channel.ProjectIDs = visibleIDs
		}
		resp := notificationChannelResponseForWithPrivacy(channel, canManage)
		resp.CanManage = canManage
		responses = append(responses, resp)
	}
	return responses, nil
}

func (h *Handler) resolveTestRecipient(ctx *gin.Context, user model.User) string {
	var input notificationChannelTestInput
	if ctx.Request != nil && ctx.Request.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 4096))
		if err == nil && len(strings.TrimSpace(string(bodyBytes))) > 0 {
			_ = json.Unmarshal(bodyBytes, &input)
		}
	}
	recipient := strings.TrimSpace(input.TestRecipient)
	if recipient == "" {
		recipient = strings.TrimSpace(user.Email)
	}
	if recipient == "" {
		return ""
	}
	if _, err := mail.ParseAddress(recipient); err != nil {
		return ""
	}
	return recipient
}

func normalizeProjectRuleFilterJSON(raw json.RawMessage, projectID string) json.RawMessage {
	projectID = strings.TrimSpace(projectID)
	payload := map[string]any{}
	if len(raw) > 0 && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "null" {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return raw
		}
	}
	payload["scope"] = string(notification.RuleScopeProjects)
	payload["projectIds"] = []string{projectID}
	data, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return data
}

func (h *Handler) validateProjectRuleScopeReferences(ctx *gin.Context, projectID string, filterJSON string) bool {
	filter, err := notification.DecodeRuleFilter([]byte(filterJSON))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_filter_invalid", err.Error())
		return false
	}
	if len(filter.ApplicationIDs) > 0 {
		var appCount int64
		if err := h.dbFor(ctx).Model(&model.Application{}).Where("project_id = ? and id in ?", projectID, filter.ApplicationIDs).Count(&appCount).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return false
		}
		if appCount != int64(len(filter.ApplicationIDs)) {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_application_not_found", "selected application does not belong to this project space")
			return false
		}
	}
	if len(filter.DeploymentTargetIDs) > 0 {
		var targetCount int64
		if err := h.dbFor(ctx).Model(&model.DeploymentTarget{}).Where("project_id = ? and id in ?", projectID, filter.DeploymentTargetIDs).Count(&targetCount).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return false
		}
		if targetCount != int64(len(filter.DeploymentTargetIDs)) {
			writeErrorCode(ctx, http.StatusBadRequest, "notification.rule_deployment_target_not_found", "selected deployment target does not belong to this project space")
			return false
		}
	}
	return true
}

func redactChannelTargetsJSON(raw string) string {
	targets, err := notification.DecodeChannelTargets([]byte(raw))
	if err != nil || len(targets) == 0 {
		return "{}"
	}
	redacted := make(map[string]notification.ChannelTargetOverride, len(targets))
	for channelID, item := range targets {
		redacted[channelID] = notification.ChannelTargetOverride{
			To: maskEmailSlice(item.To),
			Cc: maskEmailSlice(item.Cc),
		}
	}
	encoded, err := notification.EncodeChannelTargets(redacted)
	if err != nil {
		return "{}"
	}
	return encoded
}

func maskEmailSlice(emails []string) []string {
	out := make([]string, 0, len(emails))
	for _, value := range emails {
		out = append(out, maskEmailAddress(value))
	}
	return out
}

func maskEmailAddress(raw string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "***"
	}
	local, domain, found := strings.Cut(parsed.Address, "@")
	if !found || domain == "" {
		return "***"
	}
	if len(local) <= 1 {
		return "*@" + domain
	}
	return local[:1] + "***@" + domain
}

func (h *Handler) enrichNotificationDeliveries(deliveries []model.NotificationDelivery, includePrivateRecipients bool, ctx context.Context) {
	if len(deliveries) == 0 {
		return
	}
	channelIDSet := map[string]struct{}{}
	ruleIDSet := map[string]struct{}{}
	for _, delivery := range deliveries {
		if chID := strings.TrimSpace(delivery.ChannelID); chID != "" && chID != notification.UserEmailChannelID {
			channelIDSet[chID] = struct{}{}
		}
		if rlID := strings.TrimSpace(delivery.RuleID); rlID != "" {
			ruleIDSet[rlID] = struct{}{}
		}
	}
	channelsByID := map[string]model.NotificationChannel{}
	if len(channelIDSet) > 0 {
		ids := mapKeys(channelIDSet)
		var channels []model.NotificationChannel
		if err := h.dbWithContext(ctx).Unscoped().Where("id in ?", ids).Find(&channels).Error; err == nil {
			for _, channel := range channels {
				channelsByID[channel.ID] = channel
			}
		}
	}
	rulesByID := map[string]model.NotificationRule{}
	if len(ruleIDSet) > 0 {
		ids := mapKeys(ruleIDSet)
		var rules []model.NotificationRule
		if err := h.dbWithContext(ctx).Unscoped().Where("id in ?", ids).Find(&rules).Error; err == nil {
			for _, rule := range rules {
				rulesByID[rule.ID] = rule
			}
		}
	}

	for index := range deliveries {
		delivery := &deliveries[index]
		if channel, ok := channelsByID[delivery.ChannelID]; ok {
			delivery.ChannelName = channel.Name
		}
		if rule, ok := rulesByID[delivery.RuleID]; ok {
			delivery.RuleName = rule.Name
		}
		delivery.RecipientSummary = summarizeDeliveryTarget(*delivery, channelsByID[delivery.ChannelID], includePrivateRecipients)
		if !includePrivateRecipients {
			delivery.TargetOverrideJSON = redactChannelTargetOverrideJSON(delivery.TargetOverrideJSON)
			delivery.RequestSnapshot = redactRequestSnapshotEmails(delivery.RequestSnapshot)
		}
	}
}

func (h *Handler) enrichPersonalNotificationDeliveries(deliveries []model.NotificationDelivery, user model.User, ctx context.Context) {
	if len(deliveries) == 0 {
		return
	}
	h.enrichNotificationDeliveries(deliveries, true, ctx)
	for index := range deliveries {
		if deliveries[index].ChannelID == notification.UserEmailChannelID {
			deliveries[index].RecipientSummary = strings.TrimSpace(user.Email)
		}
	}
}

func summarizeDeliveryTarget(delivery model.NotificationDelivery, channel model.NotificationChannel, includePrivateRecipients bool) string {
	if delivery.AdapterKind == notification.AdapterKindSMTP {
		override := notification.DecodeChannelTargetOverride(delivery.TargetOverrideJSON)
		recipients := append([]string(nil), override.To...)
		if len(recipients) == 0 && strings.TrimSpace(channel.ConfigJSON) != "" {
			if cfg, err := notification.ParseSMTPConfigForValidation(json.RawMessage(channel.ConfigJSON)); err == nil {
				recipients = append(recipients, cfg.To...)
			}
		}
		if len(recipients) == 0 {
			return ""
		}
		if !includePrivateRecipients {
			recipients = maskEmailSlice(recipients)
		}
		if len(recipients) > 2 {
			return fmt.Sprintf("%s +%d", strings.Join(recipients[:2], ", "), len(recipients)-2)
		}
		return strings.Join(recipients, ", ")
	}
	if strings.TrimSpace(channel.PresetID) != "" {
		return channel.PresetID
	}
	return delivery.AdapterKind
}

func redactChannelTargetOverrideJSON(raw string) string {
	override := notification.DecodeChannelTargetOverride(raw)
	if len(override.To) == 0 && len(override.Cc) == 0 {
		return "{}"
	}
	return notification.EncodeChannelTargetOverride(notification.ChannelTargetOverride{
		To: maskEmailSlice(override.To),
		Cc: maskEmailSlice(override.Cc),
	})
}

func redactRequestSnapshotEmails(raw string) string {
	snapshot, ok := decodeJSONValue(raw).(map[string]any)
	if !ok || len(snapshot) == 0 {
		return raw
	}
	for _, field := range []string{"to", "cc", "from"} {
		switch typed := snapshot[field].(type) {
		case []any:
			masked := make([]string, 0, len(typed))
			for _, item := range typed {
				masked = append(masked, maskEmailAddress(fmt.Sprint(item)))
			}
			snapshot[field] = masked
		case string:
			if strings.TrimSpace(typed) != "" {
				snapshot[field] = maskEmailAddress(typed)
			}
		}
	}
	return mustJSON(snapshot)
}

func normalizeUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func mapKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return out
}

var _ = clause.Locking{}
var _ = time.Now
