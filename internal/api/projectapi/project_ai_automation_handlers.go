package projectapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProjectAIAutomationConfigResponse struct {
	ProjectID                   string            `json:"projectId"`
	DeliveryDiagnosisEnabled    bool              `json:"deliveryDiagnosisEnabled"`
	DeliveryBillingUserID       string            `json:"deliveryBillingUserId"`
	DeliveryBillingUserName     string            `json:"deliveryBillingUserName"`
	DeliveryCooldownSeconds     int               `json:"deliveryCooldownSeconds"`
	DeliveryDailyLimit          int               `json:"deliveryDailyLimit"`
	RuntimeDiagnosisEnabled     bool              `json:"runtimeDiagnosisEnabled"`
	RuntimeBillingUserID        string            `json:"runtimeBillingUserId"`
	RuntimeBillingUserName      string            `json:"runtimeBillingUserName"`
	RuntimeCooldownSeconds      int               `json:"runtimeCooldownSeconds"`
	RuntimeDailyLimit           int               `json:"runtimeDailyLimit"`
	SelfHealMode                string            `json:"selfHealMode"`
	StageSelfHealModes          map[string]string `json:"stageSelfHealModes"`
	AllowedAutoHealTools        []string          `json:"allowedAutoHealTools"`
	MaxConsecutiveAutoHeals     int               `json:"maxConsecutiveAutoHeals"`
	CircuitBreakerWindowSeconds int               `json:"circuitBreakerWindowSeconds"`
	UpdatedBy                   string            `json:"updatedBy"`
	UpdatedAt                   time.Time         `json:"updatedAt"`
}

type UpdateProjectAIAutomationConfigInput struct {
	DeliveryDiagnosisEnabled    *bool              `json:"deliveryDiagnosisEnabled"`
	DeliveryBillingUserID       *string            `json:"deliveryBillingUserId"`
	DeliveryCooldownSeconds     *int               `json:"deliveryCooldownSeconds"`
	DeliveryDailyLimit          *int               `json:"deliveryDailyLimit"`
	RuntimeDiagnosisEnabled     *bool              `json:"runtimeDiagnosisEnabled"`
	RuntimeBillingUserID        *string            `json:"runtimeBillingUserId"`
	RuntimeCooldownSeconds      *int               `json:"runtimeCooldownSeconds"`
	RuntimeDailyLimit           *int               `json:"runtimeDailyLimit"`
	SelfHealMode                *string            `json:"selfHealMode"`
	StageSelfHealModes          *map[string]string `json:"stageSelfHealModes"`
	AllowedAutoHealTools        *[]string          `json:"allowedAutoHealTools"`
	MaxConsecutiveAutoHeals     *int               `json:"maxConsecutiveAutoHeals"`
	CircuitBreakerWindowSeconds *int               `json:"circuitBreakerWindowSeconds"`
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
		requested := strings.TrimSpace(*input.DeliveryBillingUserID)
		if requested != "" && requested != user.ID && requested != strings.TrimSpace(existing.DeliveryBillingUserID) {
			writeErrorCode(ctx, http.StatusForbidden, "ai_automation.billing_user_must_be_self", "background AI billing owner can only be bound to your own personal wallet")
			return
		}
		next.DeliveryBillingUserID = requested
	}
	if next.DeliveryDiagnosisEnabled && strings.TrimSpace(next.DeliveryBillingUserID) == "" {
		next.DeliveryBillingUserID = user.ID
	}

	if input.RuntimeDiagnosisEnabled != nil {
		next.RuntimeDiagnosisEnabled = *input.RuntimeDiagnosisEnabled
	}
	if input.RuntimeCooldownSeconds != nil {
		if *input.RuntimeCooldownSeconds < 0 || *input.RuntimeCooldownSeconds > 86400 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.cooldown_invalid", "runtimeCooldownSeconds must be between 0 and 86400")
			return
		}
		next.RuntimeCooldownSeconds = *input.RuntimeCooldownSeconds
	}
	if input.RuntimeDailyLimit != nil {
		if *input.RuntimeDailyLimit < 1 || *input.RuntimeDailyLimit > 500 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.daily_limit_invalid", "runtimeDailyLimit must be between 1 and 500")
			return
		}
		next.RuntimeDailyLimit = *input.RuntimeDailyLimit
	}
	if input.RuntimeBillingUserID != nil {
		requested := strings.TrimSpace(*input.RuntimeBillingUserID)
		if requested != "" && requested != user.ID && requested != strings.TrimSpace(existing.RuntimeBillingUserID) {
			writeErrorCode(ctx, http.StatusForbidden, "ai_automation.billing_user_must_be_self", "runtime AI billing owner can only be bound to your own personal wallet")
			return
		}
		next.RuntimeBillingUserID = requested
	}
	if next.RuntimeDiagnosisEnabled && strings.TrimSpace(next.RuntimeBillingUserID) == "" {
		next.RuntimeBillingUserID = user.ID
	}

	if input.SelfHealMode != nil {
		mode := strings.TrimSpace(*input.SelfHealMode)
		if !model.IsValidAISelfHealMode(mode) {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.self_heal_mode_invalid", "selfHealMode must be suggest_only, auto_l1, or auto_l2")
			return
		}
		next.SelfHealMode = mode
	}
	if input.StageSelfHealModes != nil {
		cleanMap := map[string]string{}
		for rawStage, rawMode := range *input.StageSelfHealModes {
			stage := strings.ToLower(strings.TrimSpace(rawStage))
			mode := strings.TrimSpace(rawMode)
			if stage == "" || mode == "" {
				continue
			}
			if !isAllowedStageKey(stage) || !model.IsValidAISelfHealMode(mode) {
				writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.self_heal_mode_invalid", "stageSelfHealModes contains an invalid stage or self-heal mode")
				return
			}
			if (stage == "prod" || stage == "production") && mode == model.AISelfHealModeAutoL1 {
				writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.self_heal_mode_invalid", "auto_l1 cannot be configured for production stage")
				return
			}
			cleanMap[stage] = mode
		}
		encoded, _ := json.Marshal(cleanMap)
		next.StageSelfHealModesJSON = string(encoded)
	}
	if input.AllowedAutoHealTools != nil {
		cleanTools := make([]string, 0, len(*input.AllowedAutoHealTools))
		seen := map[string]struct{}{}
		for _, rawTool := range *input.AllowedAutoHealTools {
			name := strings.TrimSpace(rawTool)
			if name == "" {
				continue
			}
			if !model.IsAllowedAIRuntimeHealTool(name) {
				writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.auto_heal_tool_invalid", "allowedAutoHealTools contains an unsupported tool")
				return
			}
			if _, exists := seen[name]; !exists {
				seen[name] = struct{}{}
				cleanTools = append(cleanTools, name)
			}
		}
		encoded, _ := json.Marshal(cleanTools)
		next.AllowedAutoHealToolsJSON = string(encoded)
	}
	if input.MaxConsecutiveAutoHeals != nil {
		if *input.MaxConsecutiveAutoHeals < 1 || *input.MaxConsecutiveAutoHeals > 10 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.circuit_breaker_invalid", "maxConsecutiveAutoHeals must be between 1 and 10")
			return
		}
		next.MaxConsecutiveAutoHeals = *input.MaxConsecutiveAutoHeals
	}
	if input.CircuitBreakerWindowSeconds != nil {
		if *input.CircuitBreakerWindowSeconds < 60 || *input.CircuitBreakerWindowSeconds > 86400 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.circuit_breaker_invalid", "circuitBreakerWindowSeconds must be between 60 and 86400")
			return
		}
		next.CircuitBreakerWindowSeconds = *input.CircuitBreakerWindowSeconds
	}

	escalatesDeliverySpend := (!existing.DeliveryDiagnosisEnabled && next.DeliveryDiagnosisEnabled) ||
		next.DeliveryCooldownSeconds < existing.DeliveryCooldownSeconds ||
		next.DeliveryDailyLimit > existing.DeliveryDailyLimit
	escalatesRuntimeSpendOrHeal := (!existing.RuntimeDiagnosisEnabled && next.RuntimeDiagnosisEnabled) ||
		next.RuntimeCooldownSeconds < existing.RuntimeCooldownSeconds ||
		next.RuntimeDailyLimit > existing.RuntimeDailyLimit ||
		selfHealExposureIncreased(existing, next)

	bindsNewDeliveryWallet := strings.TrimSpace(next.DeliveryBillingUserID) != "" &&
		strings.TrimSpace(next.DeliveryBillingUserID) != strings.TrimSpace(existing.DeliveryBillingUserID)
	bindsNewRuntimeWallet := strings.TrimSpace(next.RuntimeBillingUserID) != "" &&
		strings.TrimSpace(next.RuntimeBillingUserID) != strings.TrimSpace(existing.RuntimeBillingUserID)

	if bindsNewDeliveryWallet || bindsNewRuntimeWallet || escalatesDeliverySpend || escalatesRuntimeSpendOrHeal {
		if _, hasSession := h.currentSessionFromCookie(ctx); !hasSession {
			writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required to bind a personal wallet or escalate AI automation")
			return
		}
	}

	// Configuring or activating auto_l1 / auto_l2 requires deployment write permission in addition to notification:manage.
	if maxConfiguredSelfHealRank(next) > 0 && (selfHealExposureIncreased(existing, next) || bindsNewRuntimeWallet || (!existing.RuntimeDiagnosisEnabled && next.RuntimeDiagnosisEnabled)) {
		allowed, roleErr := h.ProjectRoleActionAllowed(ctx.Request.Context(), user, project.ID, authz.ActionDeploymentUpdate)
		if roleErr != nil || !allowed {
			writeErrorKey(ctx, http.StatusForbidden, requestLanguage(ctx), "auth.permission.denied")
			return
		}
	}

	if existingOwner := strings.TrimSpace(existing.DeliveryBillingUserID); existingOwner != "" &&
		existingOwner != user.ID &&
		strings.TrimSpace(next.DeliveryBillingUserID) == existingOwner &&
		escalatesDeliverySpend {
		writeErrorCode(ctx, http.StatusForbidden, "ai_automation.billing_user_must_be_self", "rebind deliveryBillingUserId to your own personal wallet before enabling or increasing AI spend limits")
		return
	}

	if existingRuntimeOwner := strings.TrimSpace(existing.RuntimeBillingUserID); existingRuntimeOwner != "" &&
		existingRuntimeOwner != user.ID &&
		strings.TrimSpace(next.RuntimeBillingUserID) == existingRuntimeOwner &&
		escalatesRuntimeSpendOrHeal {
		writeErrorCode(ctx, http.StatusForbidden, "ai_automation.billing_user_must_be_self", "rebind runtimeBillingUserId to your own personal wallet before enabling or escalating runtime AI self-healing")
		return
	}

	if ok := h.validateOrClearBillingOwner(ctx, project.ID, &next.DeliveryBillingUserID, next.DeliveryDiagnosisEnabled, false); !ok {
		return
	}
	requireDeployPerm := maxConfiguredSelfHealRank(next) > 0
	if ok := h.validateOrClearBillingOwner(ctx, project.ID, &next.RuntimeBillingUserID, next.RuntimeDiagnosisEnabled, requireDeployPerm); !ok {
		return
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
			"runtime_diagnosis_enabled",
			"runtime_billing_user_id",
			"runtime_cooldown_seconds",
			"runtime_daily_limit",
			"self_heal_mode",
			"stage_self_heal_modes_json",
			"allowed_auto_heal_tools_json",
			"max_consecutive_auto_heals",
			"circuit_breaker_window_seconds",
			"updated_by",
			"updated_at",
		}),
	}).Create(&next).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	auditDetail := "deliveryOwner=" + next.DeliveryBillingUserID + ",runtimeOwner=" + next.RuntimeBillingUserID + ",selfHealMode=" + next.SelfHealMode
	h.auditWithContext(user.ID, "project.ai_automation_config.update", project.ID, true, auditDetail, ctx.Request.Context())
	ctx.JSON(http.StatusOK, h.presentAIAutomationConfig(ctx, next))
}

func isAllowedStageKey(stage string) bool {
	switch stage {
	case "dev", "test", "staging", "prod":
		return true
	default:
		return false
	}
}

func maxConfiguredSelfHealRank(cfg model.ProjectAIAutomationConfig) int {
	maxRank := model.SelfHealModeRank(cfg.SelfHealMode)
	for _, mode := range cfg.StageSelfHealModes() {
		if r := model.SelfHealModeRank(mode); r > maxRank {
			maxRank = r
		}
	}
	return maxRank
}

func selfHealExposureIncreased(prev, next model.ProjectAIAutomationConfig) bool {
	if model.SelfHealModeRank(next.SelfHealMode) > model.SelfHealModeRank(prev.SelfHealMode) {
		return true
	}
	stageSet := map[string]struct{}{
		"dev":     {},
		"test":    {},
		"staging": {},
		"prod":    {},
	}
	for stage := range prev.StageSelfHealModes() {
		stageSet[stage] = struct{}{}
	}
	for stage := range next.StageSelfHealModes() {
		stageSet[stage] = struct{}{}
	}
	for stage := range stageSet {
		if model.SelfHealModeRank(next.EffectiveSelfHealModeForStage(stage)) > model.SelfHealModeRank(prev.EffectiveSelfHealModeForStage(stage)) {
			return true
		}
	}
	if maxConfiguredSelfHealRank(next) > 0 {
		if next.MaxConsecutiveAutoHeals > prev.MaxConsecutiveAutoHeals ||
			next.CircuitBreakerWindowSeconds < prev.CircuitBreakerWindowSeconds {
			return true
		}
		prevTools := prev.AllowedAutoHealTools()
		nextTools := next.AllowedAutoHealTools()
		if len(prevTools) > 0 && len(nextTools) == 0 {
			return true
		}
		for _, tool := range nextTools {
			if !prev.AllowsAutoHealTool(tool) {
				return true
			}
		}
	}
	return false
}

func (h *Handler) validateOrClearBillingOwner(
	ctx *gin.Context,
	projectID string,
	ownerUserID *string,
	enabled bool,
	requireDeployPerm bool,
) bool {
	uid := strings.TrimSpace(*ownerUserID)
	if uid == "" {
		return true
	}
	valid := true
	var billingUser model.User
	if err := h.dbFor(ctx).First(&billingUser, "id = ? AND disabled = ?", uid, false).Error; err != nil {
		valid = false
	} else if !authz.IsPlatformAdmin(billingUser.Role) {
		var member model.ProjectMember
		if err := h.dbFor(ctx).First(&member, "project_id = ? AND user_id = ?", projectID, billingUser.ID).Error; err != nil {
			valid = false
		} else if requireDeployPerm {
			allowed, err := h.ProjectRoleActionAllowed(ctx.Request.Context(), billingUser, projectID, authz.ActionDeploymentUpdate)
			if err != nil || !allowed {
				valid = false
			}
		}
	}
	if !valid {
		if enabled {
			writeErrorCode(ctx, http.StatusBadRequest, "ai_automation.billing_user_invalid", "billing user must be an active authorized member of this project")
			return false
		}
		*ownerUserID = ""
	}
	return true
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
		ProjectID:                   cfg.ProjectID,
		DeliveryDiagnosisEnabled:    cfg.DeliveryDiagnosisEnabled,
		DeliveryBillingUserID:       cfg.DeliveryBillingUserID,
		DeliveryCooldownSeconds:     cfg.DeliveryCooldownSeconds,
		DeliveryDailyLimit:          cfg.DeliveryDailyLimit,
		RuntimeDiagnosisEnabled:     cfg.RuntimeDiagnosisEnabled,
		RuntimeBillingUserID:        cfg.RuntimeBillingUserID,
		RuntimeCooldownSeconds:      cfg.RuntimeCooldownSeconds,
		RuntimeDailyLimit:           cfg.RuntimeDailyLimit,
		SelfHealMode:                cfg.EffectiveSelfHealModeForStage(""),
		StageSelfHealModes:          cfg.StageSelfHealModes(),
		AllowedAutoHealTools:        cfg.AllowedAutoHealTools(),
		MaxConsecutiveAutoHeals:     cfg.MaxConsecutiveAutoHeals,
		CircuitBreakerWindowSeconds: cfg.CircuitBreakerWindowSeconds,
		UpdatedBy:                   cfg.UpdatedBy,
		UpdatedAt:                   cfg.UpdatedAt,
	}
	if strings.TrimSpace(cfg.DeliveryBillingUserID) != "" {
		resp.DeliveryBillingUserName = h.lookupUserDisplayName(ctx, cfg.DeliveryBillingUserID)
	}
	if strings.TrimSpace(cfg.RuntimeBillingUserID) != "" {
		resp.RuntimeBillingUserName = h.lookupUserDisplayName(ctx, cfg.RuntimeBillingUserID)
	}
	return resp
}

func (h *Handler) lookupUserDisplayName(ctx *gin.Context, userID string) string {
	var u model.User
	if err := h.dbFor(ctx).Select("id", "name", "email").First(&u, "id = ?", strings.TrimSpace(userID)).Error; err == nil {
		if name := strings.TrimSpace(u.Name); name != "" {
			return name
		}
		return strings.TrimSpace(u.Email)
	}
	return ""
}

type ProjectAIDiagnosisRecordResponse struct {
	ID                    string                  `json:"id"`
	ProjectID             string                  `json:"projectId"`
	ApplicationID         string                  `json:"applicationId"`
	ApplicationName       string                  `json:"applicationName"`
	DeploymentTargetID    string                  `json:"deploymentTargetId"`
	DeploymentTargetName  string                  `json:"deploymentTargetName"`
	Category              string                  `json:"category"`
	SubjectType           string                  `json:"subjectType"`
	SubjectID             string                  `json:"subjectId"`
	Status                string                  `json:"status"`
	TriggerCode           string                  `json:"triggerCode"`
	RootCauseSummary      string                  `json:"rootCauseSummary"`
	Confidence            string                  `json:"confidence"`
	Suggestions           []string                `json:"suggestions"`
	ProposedAction        *model.AIProposedAction `json:"proposedAction,omitempty"`
	AutoHealMode          string                  `json:"autoHealMode,omitempty"`
	AutoHealStatus        string                  `json:"autoHealStatus,omitempty"`
	AutoHealSummary       string                  `json:"autoHealSummary,omitempty"`
	CircuitBreakerTripped bool                    `json:"circuitBreakerTripped,omitempty"`
	ErrorCode             string                  `json:"errorCode,omitempty"`
	DeepLink              string                  `json:"deepLink"`
	CreatedAt             time.Time               `json:"createdAt"`
	FinishedAt            *time.Time              `json:"finishedAt,omitempty"`
}

func (h *Handler) ListProjectAIDiagnoses(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	_, project, ok := h.authorizeProject(ctx, authz.ActionNotificationRead)
	if !ok {
		return
	}

	category := strings.ToLower(strings.TrimSpace(ctx.Query("category")))
	if category != "" && category != "all" && category != "delivery" && category != "runtime" {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "category must be all, delivery, or runtime")
		return
	}
	status := strings.ToLower(strings.TrimSpace(ctx.Query("status")))
	if status != "" && status != model.AIDiagnosisStatusRunning && status != model.AIDiagnosisStatusSucceeded && status != model.AIDiagnosisStatusFailed {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "status must be running, succeeded, or failed")
		return
	}

	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"createdAt": "created_at",
	}, "createdAt")
	dir := "desc"
	if strings.EqualFold(pagination.SortOrder, "asc") {
		dir = "asc"
	}

	db := h.dbFor(ctx)
	includeDelivery := category == "" || category == "all" || category == "delivery"
	includeRuntime := category == "" || category == "all" || category == "runtime"

	type indexRow struct {
		Source string `gorm:"column:source"`
		ID     string `gorm:"column:id"`
	}

	var total int64
	var pageIndex []indexRow

	if includeDelivery {
		q := db.Model(&model.AIDeliveryDiagnosis{}).Where("project_id = ?", project.ID)
		if status != "" {
			q = q.Where("status = ?", status)
		}
		var cnt int64
		if err := q.Count(&cnt).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		total += cnt
	}
	if includeRuntime {
		q := db.Model(&model.AIRuntimeIncidentDiagnosis{}).Where("project_id = ?", project.ID)
		if status != "" {
			q = q.Where("status = ?", status)
		}
		var cnt int64
		if err := q.Count(&cnt).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		total += cnt
	}

	if total == 0 {
		ctx.JSON(http.StatusOK, paginatedResponse([]ProjectAIDiagnosisRecordResponse{}, 0, pagination))
		return
	}

	switch {
	case includeDelivery && includeRuntime:
		var args []any
		deliverySQL := "SELECT 'delivery' AS source, id, created_at FROM ai_delivery_diagnoses WHERE project_id = ?"
		args = append(args, project.ID)
		if status != "" {
			deliverySQL += " AND status = ?"
			args = append(args, status)
		}
		runtimeSQL := "SELECT 'runtime' AS source, id, created_at FROM ai_runtime_incident_diagnoses WHERE project_id = ?"
		args = append(args, project.ID)
		if status != "" {
			runtimeSQL += " AND status = ?"
			args = append(args, status)
		}
		unionSQL := "SELECT source, id FROM (" + deliverySQL + " UNION ALL " + runtimeSQL + ") AS combined ORDER BY created_at " + dir + ", id " + dir + " LIMIT ? OFFSET ?"
		args = append(args, pagination.PageSize, pagination.Offset())
		if err := db.Raw(unionSQL, args...).Scan(&pageIndex).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	case includeDelivery:
		q := db.Model(&model.AIDeliveryDiagnosis{}).
			Select("'delivery' AS source, id").
			Where("project_id = ?", project.ID)
		if status != "" {
			q = q.Where("status = ?", status)
		}
		if err := q.Order("created_at " + dir + ", id " + dir).
			Limit(pagination.PageSize).
			Offset(pagination.Offset()).
			Scan(&pageIndex).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	case includeRuntime:
		q := db.Model(&model.AIRuntimeIncidentDiagnosis{}).
			Select("'runtime' AS source, id").
			Where("project_id = ?", project.ID)
		if status != "" {
			q = q.Where("status = ?", status)
		}
		if err := q.Order("created_at " + dir + ", id " + dir).
			Limit(pagination.PageSize).
			Offset(pagination.Offset()).
			Scan(&pageIndex).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}

	var deliveryIDs, runtimeIDs []string
	for _, idx := range pageIndex {
		if idx.Source == "delivery" {
			deliveryIDs = append(deliveryIDs, idx.ID)
		} else if idx.Source == "runtime" {
			runtimeIDs = append(runtimeIDs, idx.ID)
		}
	}

	deliveryByID := map[string]model.AIDeliveryDiagnosis{}
	if len(deliveryIDs) > 0 {
		var rows []model.AIDeliveryDiagnosis
		if err := db.Where("project_id = ? AND id IN ?", project.ID, deliveryIDs).Find(&rows).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		for _, r := range rows {
			deliveryByID[r.ID] = r
		}
	}

	runtimeByID := map[string]model.AIRuntimeIncidentDiagnosis{}
	if len(runtimeIDs) > 0 {
		var rows []model.AIRuntimeIncidentDiagnosis
		if err := db.Where("project_id = ? AND id IN ?", project.ID, runtimeIDs).Find(&rows).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		for _, r := range rows {
			runtimeByID[r.ID] = r
		}
	}

	appIDSet := map[string]struct{}{}
	targetIDSet := map[string]struct{}{}
	for _, r := range deliveryByID {
		if id := strings.TrimSpace(r.ApplicationID); id != "" {
			appIDSet[id] = struct{}{}
		}
		if id := strings.TrimSpace(r.DeploymentTargetID); id != "" {
			targetIDSet[id] = struct{}{}
		}
	}
	for _, r := range runtimeByID {
		if id := strings.TrimSpace(r.ApplicationID); id != "" {
			appIDSet[id] = struct{}{}
		}
		if id := strings.TrimSpace(r.DeploymentTargetID); id != "" {
			targetIDSet[id] = struct{}{}
		}
	}

	appNames := map[string]string{}
	if len(appIDSet) > 0 {
		ids := make([]string, 0, len(appIDSet))
		for id := range appIDSet {
			ids = append(ids, id)
		}
		var apps []model.Application
		if err := db.Unscoped().Select("id", "name", "identifier").Where("project_id = ? AND id IN ?", project.ID, ids).Find(&apps).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		for _, a := range apps {
			appNames[a.ID] = fallback(strings.TrimSpace(a.Name), strings.TrimSpace(a.Identifier))
		}
	}

	targetNames := map[string]string{}
	if len(targetIDSet) > 0 {
		ids := make([]string, 0, len(targetIDSet))
		for id := range targetIDSet {
			ids = append(ids, id)
		}
		var targets []model.DeploymentTarget
		if err := db.Unscoped().Select("id", "name", "stage").Where("project_id = ? AND id IN ?", project.ID, ids).Find(&targets).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		for _, tgt := range targets {
			targetNames[tgt.ID] = fallback(strings.TrimSpace(tgt.Name), strings.TrimSpace(tgt.Stage))
		}
	}

	items := make([]ProjectAIDiagnosisRecordResponse, 0, len(pageIndex))
	for _, idx := range pageIndex {
		if idx.Source == "delivery" {
			row, found := deliveryByID[idx.ID]
			if !found {
				continue
			}
			view := row.ToView()
			items = append(items, ProjectAIDiagnosisRecordResponse{
				ID:                   row.ID,
				ProjectID:            row.ProjectID,
				ApplicationID:        row.ApplicationID,
				ApplicationName:      appNames[row.ApplicationID],
				DeploymentTargetID:   row.DeploymentTargetID,
				DeploymentTargetName: targetNames[row.DeploymentTargetID],
				Category:             "delivery",
				SubjectType:          row.SubjectType,
				SubjectID:            row.SubjectID,
				Status:               row.Status,
				TriggerCode:          row.FailureCode,
				RootCauseSummary:     view.RootCauseSummary,
				Confidence:           view.Confidence,
				Suggestions:          view.Suggestions,
				ProposedAction:       view.ProposedAction,
				ErrorCode:            row.ErrorCode,
				DeepLink:             aiDiagnosisRecordDeepLink(row.ProjectID, row.ApplicationID, row.DeploymentTargetID, row.SubjectType, row.SubjectID),
				CreatedAt:            row.CreatedAt,
				FinishedAt:           row.FinishedAt,
			})
		} else if idx.Source == "runtime" {
			row, found := runtimeByID[idx.ID]
			if !found {
				continue
			}
			suggestions := []string{}
			if raw := strings.TrimSpace(row.SuggestionsJSON); raw != "" {
				_ = json.Unmarshal([]byte(raw), &suggestions)
			}
			if suggestions == nil {
				suggestions = []string{}
			}
			var action *model.AIProposedAction
			if raw := strings.TrimSpace(row.ProposedActionJSON); raw != "" {
				var parsed model.AIProposedAction
				if err := json.Unmarshal([]byte(raw), &parsed); err == nil && strings.TrimSpace(parsed.ToolName) != "" {
					if parsed.Arguments == nil {
						parsed.Arguments = map[string]any{}
					}
					action = &parsed
				}
			}
			healSummary := strings.TrimSpace(row.AutoHealSummary)
			if healSummary == "in_progress" {
				healSummary = ""
			}
			items = append(items, ProjectAIDiagnosisRecordResponse{
				ID:                    row.ID,
				ProjectID:             row.ProjectID,
				ApplicationID:         row.ApplicationID,
				ApplicationName:       appNames[row.ApplicationID],
				DeploymentTargetID:    row.DeploymentTargetID,
				DeploymentTargetName:  targetNames[row.DeploymentTargetID],
				Category:              "runtime",
				SubjectType:           "runtime_incident",
				SubjectID:             row.IncidentID,
				Status:                row.Status,
				TriggerCode:           row.AnomalyType,
				RootCauseSummary:      row.RootCauseSummary,
				Confidence:            row.Confidence,
				Suggestions:           suggestions,
				ProposedAction:        action,
				AutoHealMode:          row.AutoHealMode,
				AutoHealStatus:        row.AutoHealStatus,
				AutoHealSummary:       healSummary,
				CircuitBreakerTripped: row.CircuitBreakerTripped,
				ErrorCode:             row.ErrorCode,
				DeepLink:              aiDiagnosisRecordDeepLink(row.ProjectID, row.ApplicationID, row.DeploymentTargetID, "runtime_incident", row.IncidentID),
				CreatedAt:             row.CreatedAt,
				FinishedAt:            row.FinishedAt,
			})
		}
	}

	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}

func aiDiagnosisRecordDeepLink(projectID, applicationID, targetID, subjectType, subjectID string) string {
	projectID = strings.TrimSpace(projectID)
	applicationID = strings.TrimSpace(applicationID)
	targetID = strings.TrimSpace(targetID)
	subjectID = strings.TrimSpace(subjectID)
	if projectID == "" {
		return ""
	}
	if applicationID == "" {
		return "/projects/" + url.PathEscape(projectID)
	}
	base := "/projects/" + url.PathEscape(projectID) + "/apps/" + url.PathEscape(applicationID)
	switch subjectType {
	case model.AIDeliverySubjectBuildRun:
		if subjectID != "" {
			return base + "?tab=builds#buildRunId=" + url.QueryEscape(subjectID)
		}
		return base + "?tab=builds"
	case model.AIDeliverySubjectRelease:
		if subjectID != "" {
			return base + "?tab=deployments#releaseId=" + url.QueryEscape(subjectID)
		}
		return base + "?tab=deployments"
	case "runtime_incident":
		if targetID != "" {
			return base + "?tab=deployments#targetId=" + url.QueryEscape(targetID)
		}
		return base + "?tab=deployments"
	default:
		return base
	}
}
