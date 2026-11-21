package projectapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProjectAIAutomationConfigResponse struct {
	ProjectID                string    `json:"projectId"`
	DeliveryDiagnosisEnabled bool      `json:"deliveryDiagnosisEnabled"`
	DeliveryBillingUserID    string    `json:"deliveryBillingUserId"`
	DeliveryBillingUserName  string    `json:"deliveryBillingUserName"`
	DeliveryCooldownSeconds  int       `json:"deliveryCooldownSeconds"`
	DeliveryDailyLimit       int       `json:"deliveryDailyLimit"`
	UpdatedBy                string    `json:"updatedBy"`
	UpdatedAt                time.Time `json:"updatedAt"`
}

type UpdateProjectAIAutomationConfigInput struct {
	DeliveryDiagnosisEnabled *bool   `json:"deliveryDiagnosisEnabled"`
	DeliveryBillingUserID    *string `json:"deliveryBillingUserId"`
	DeliveryCooldownSeconds  *int    `json:"deliveryCooldownSeconds"`
	DeliveryDailyLimit       *int    `json:"deliveryDailyLimit"`
}

func (h *Handler) GetProjectAIAutomationConfig(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	_, project, ok := h.authorizeProject(ctx, authz.ActionNotificationRead)
	if !ok {
		return
	}
	cfg, err := h.loadOrDefaultAIAutomationConfig(ctx, project.ID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.presentAIAutomationConfig(ctx, cfg))
}

func (h *Handler) UpdateProjectAIAutomationConfig(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, project, ok := h.authorizeProject(ctx, authz.ActionNotificationManage)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}

	var input UpdateProjectAIAutomationConfigInput
	if !bindJSON(ctx, &input) {
		return
	}

	existing, err := h.loadOrDefaultAIAutomationConfig(ctx, project.ID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	next := existing
	next.ProjectID = project.ID

	if input.DeliveryDiagnosisEnabled != nil {
		next.DeliveryDiagnosisEnabled = *input.DeliveryDiagnosisEnabled
	}
	if input.DeliveryCooldownSeconds != nil {
		if *input.DeliveryCooldownSeconds < 0 || *input.DeliveryCooldownSeconds > 86400 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.cooldown_invalid", "deliveryCooldownSeconds must be between 0 and 86400")
			return
		}
		next.DeliveryCooldownSeconds = *input.DeliveryCooldownSeconds
	}
	if input.DeliveryDailyLimit != nil {
		if *input.DeliveryDailyLimit < 1 || *input.DeliveryDailyLimit > 500 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.daily_limit_invalid", "deliveryDailyLimit must be between 1 and 500")
			return
		}
		next.DeliveryDailyLimit = *input.DeliveryDailyLimit
	}

	if input.DeliveryBillingUserID != nil {
		requestedBillingUser := strings.TrimSpace(*input.DeliveryBillingUserID)
		if requestedBillingUser != "" &&
			requestedBillingUser != user.ID &&
			requestedBillingUser != existing.DeliveryBillingUserID {
			// Prevent binding another user's personal wallet without their own action.
			writeErrorCode(ctx, http.StatusForbidden, "ai_automation.billing_user_must_be_self", "background AI billing owner can only be bound to your own personal wallet")
			return
		}
		next.DeliveryBillingUserID = requestedBillingUser
	}
	if next.DeliveryDiagnosisEnabled && strings.TrimSpace(next.DeliveryBillingUserID) == "" {
		next.DeliveryBillingUserID = user.ID
	}

	escalatesSpend := (!existing.DeliveryDiagnosisEnabled && next.DeliveryDiagnosisEnabled) ||
		next.DeliveryCooldownSeconds < existing.DeliveryCooldownSeconds ||
		next.DeliveryDailyLimit > existing.DeliveryDailyLimit
	bindsNewWallet := strings.TrimSpace(next.DeliveryBillingUserID) != "" &&
		strings.TrimSpace(next.DeliveryBillingUserID) != strings.TrimSpace(existing.DeliveryBillingUserID)

	// Binding/changing a personal wallet or escalating spend exposure requires an interactive browser session, not a notification-only PAT/OAuth token.
	if bindsNewWallet || escalatesSpend {
		if _, hasSession := h.currentSessionFromCookie(ctx); !hasSession {
			writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required to bind or escalate personal wallet AI spend")
			return
		}
	}

	// Prevent a co-manager from re-enabling or increasing spend exposure on another user's already-bound personal wallet.
	if existingOwner := strings.TrimSpace(existing.DeliveryBillingUserID); existingOwner != "" &&
		existingOwner != user.ID &&
		strings.TrimSpace(next.DeliveryBillingUserID) == existingOwner &&
		escalatesSpend {
		writeErrorCode(ctx, http.StatusForbidden, "ai_automation.billing_user_must_be_self", "rebind deliveryBillingUserId to your own personal wallet before enabling or increasing AI spend limits")
		return
	}

	if strings.TrimSpace(next.DeliveryBillingUserID) != "" {
		validOwner := true
		var billingUser model.User
		if err := h.dbFor(ctx).First(&billingUser, "id = ? AND disabled = ?", next.DeliveryBillingUserID, false).Error; err != nil {
			validOwner = false
		} else if !authz.IsPlatformAdmin(billingUser.Role) {
			var memberCount int64
			if err := h.dbFor(ctx).Model(&model.ProjectMember{}).
				Where("project_id = ? AND user_id = ?", project.ID, billingUser.ID).
				Count(&memberCount).Error; err != nil || memberCount == 0 {
				validOwner = false
			}
		}
		if !validOwner {
			if next.DeliveryDiagnosisEnabled {
				writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.billing_user_invalid", "billing user must be an active member of this project")
				return
			}
			// Allow disabling AI diagnosis cleanly even if the previously bound owner left or was disabled.
			next.DeliveryBillingUserID = ""
		}
	}

	now := time.Now().UTC()
	next.UpdatedBy = user.ID
	next.UpdatedAt = now
	if next.CreatedAt.IsZero() {
		next.CreatedAt = now
	}

	if err := h.dbFor(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "project_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"delivery_diagnosis_enabled",
			"delivery_billing_user_id",
			"delivery_cooldown_seconds",
			"delivery_daily_limit",
			"updated_by",
			"updated_at",
		}),
	}).Create(&next).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	h.auditWithContext(user.ID, "project.ai_automation_config.update", project.ID, true, next.DeliveryBillingUserID, ctx.Request.Context())
	ctx.JSON(http.StatusOK, h.presentAIAutomationConfig(ctx, next))
}

func (h *Handler) loadOrDefaultAIAutomationConfig(ctx *gin.Context, projectID string) (model.ProjectAIAutomationConfig, error) {
	var cfg model.ProjectAIAutomationConfig
	err := h.dbFor(ctx).First(&cfg, "project_id = ?", projectID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.DefaultProjectAIAutomationConfig(projectID), nil
		}
		return model.ProjectAIAutomationConfig{}, err
	}
	return cfg, nil
}

func (h *Handler) presentAIAutomationConfig(ctx *gin.Context, cfg model.ProjectAIAutomationConfig) ProjectAIAutomationConfigResponse {
	resp := ProjectAIAutomationConfigResponse{
		ProjectID:                cfg.ProjectID,
		DeliveryDiagnosisEnabled: cfg.DeliveryDiagnosisEnabled,
		DeliveryBillingUserID:    cfg.DeliveryBillingUserID,
		DeliveryCooldownSeconds:  cfg.DeliveryCooldownSeconds,
		DeliveryDailyLimit:       cfg.DeliveryDailyLimit,
		UpdatedBy:                cfg.UpdatedBy,
		UpdatedAt:                cfg.UpdatedAt,
	}
	if strings.TrimSpace(cfg.DeliveryBillingUserID) != "" {
		var u model.User
		if err := h.dbFor(ctx).Select("id", "name", "email").First(&u, "id = ?", cfg.DeliveryBillingUserID).Error; err == nil {
			resp.DeliveryBillingUserName = strings.TrimSpace(u.Name)
			if resp.DeliveryBillingUserName == "" {
				resp.DeliveryBillingUserName = strings.TrimSpace(u.Email)
			}
		}
	}
	return resp
}
