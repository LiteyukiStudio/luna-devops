package aiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type RuntimeSelfHealPayload struct {
	IncidentID              string         `json:"incidentId"`
	ApplicationID           string         `json:"applicationId"`
	DeploymentTargetID      string         `json:"deploymentTargetId"`
	ToolName                string         `json:"toolName"`
	Arguments               map[string]any `json:"arguments"`
	ExpectedSummary         string         `json:"expectedSummary"`
	BaselineReleaseID       string         `json:"baselineReleaseId,omitempty"`
	BaselineTargetUpdatedAt string         `json:"baselineTargetUpdatedAt,omitempty"`
	CreatedAt               string         `json:"createdAt"`
}

type selfHealLockedEnvelope struct {
	ToolLockedPayload       json.RawMessage `json:"toolLockedPayload,omitempty"`
	BaselineReleaseID       string          `json:"baselineReleaseId,omitempty"`
	BaselineTargetUpdatedAt string          `json:"baselineTargetUpdatedAt,omitempty"`
}

type InboxSelfHealPreviewOutput struct {
	ToolName        string                 `json:"toolName"`
	ApprovalTier    string                 `json:"approvalTier"`
	ApprovalPreview aitool.ApprovalPreview `json:"approvalPreview"`
	PreviewLock     string                 `json:"previewLock"`
	StateDrifted    bool                   `json:"stateDrifted"`
	DriftReason     string                 `json:"driftReason,omitempty"`
}

var inFlightAutoHealTargets sync.Map

type AutoHealInternalInput struct {
	ActorUserID  string          `json:"actorUserId"`
	ProjectID    string          `json:"projectId"`
	IncidentID   string          `json:"incidentId"`
	DiagnosisID  string          `json:"diagnosisId,omitempty"`
	SelfHealMode string          `json:"selfHealMode"`
	AllowedTools []string        `json:"allowedTools"`
	ToolName     string          `json:"toolName"`
	Arguments    json.RawMessage `json:"arguments"`
}

type AutoHealInternalOutput struct {
	ToolName           string `json:"toolName"`
	ApprovalTier       string `json:"approvalTier"`
	PreviewSummary     string `json:"previewSummary"`
	BeforeAfterSummary string `json:"beforeAfterSummary"`
	Result             any    `json:"result"`
}

func isAllowedRuntimeHealTool(tool *aitool.RegisteredTool) bool {
	if tool == nil {
		return false
	}
	if tool.Catalog.VerificationOnly || tool.Catalog.AdminOnly || tool.Catalog.GlobalSessionOnly {
		return false
	}
	if tool.Catalog.Effect == aitool.EffectRead || (!tool.HasPreview && tool.Catalog.ApprovalTier == aitool.ApprovalTierL2) || len(tool.Catalog.SensitivePaths) > 0 {
		return false
	}
	return model.IsAllowedAIRuntimeHealTool(tool.Catalog.OperationID)
}

func projectMemberAllowsToolScopes(db *gorm.DB, user model.User, projectID string, scopes []string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	if db == nil {
		return false
	}
	var member model.ProjectMember
	if err := db.First(&member, "project_id = ? AND user_id = ?", projectID, user.ID).Error; err != nil {
		return false
	}
	for _, scope := range scopes {
		action := authz.Action(strings.TrimSpace(scope))
		if _, hasPolicy := authz.ProjectPolicyForAction(action); hasPolicy && !authz.ProjectRoleAllows(member.Role, action) {
			return false
		}
	}
	return true
}

func runToolPreviewOrFallback(tool *aitool.RegisteredTool, tc *aitool.ToolContext, effectiveArgs json.RawMessage) (aitool.PreviewResult, error) {
	if tool.HasPreview {
		res, err := tool.Preview(tc, effectiveArgs)
		if err != nil {
			return aitool.PreviewResult{}, err
		}
		if res != nil {
			return *res, nil
		}
		return aitool.PreviewResult{}, nil
	}
	return aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        firstNonEmptyString(strings.TrimSpace(tool.Catalog.Summary), tool.Catalog.OperationID),
			TargetResource: tool.Catalog.Domain,
		},
	}, nil
}

func parsePositiveIntArg(raw any) (int, bool) {
	switch v := raw.(type) {
	case float64:
		if math.Trunc(v) != v || v < 1 || v > 1000 {
			return 0, false
		}
		return int(v), true
	case int:
		if v < 1 || v > 1000 {
			return 0, false
		}
		return v, true
	case int64:
		if v < 1 || v > 1000 {
			return 0, false
		}
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil || i < 1 || i > 1000 {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// validateSafeRuntimeHealArguments enforces strict argument-level guardrails against indirect prompt
// injection or hallucination: runtime self-healing may NEVER override container images/commands/security,
// nor scale a deployment target down to 0 replicas.
func validateSafeRuntimeHealArguments(toolName string, argsMap map[string]any) error {
	switch strings.TrimSpace(toolName) {
	case "delivery_scale_target":
		if _, ok := parsePositiveIntArg(argsMap["replicas"]); !ok {
			return aitool.Forbidden("ai.self_heal_not_permitted", "runtime self-healing requires replicas >= 1")
		}
	case "delivery_deploy_target":
		if rawImg, exists := argsMap["imageRef"]; exists && rawImg != nil {
			if imgStr, ok := rawImg.(string); !ok || strings.TrimSpace(imgStr) != "" {
				return aitool.Forbidden("ai.self_heal_not_permitted", "runtime self-healing cannot override container imageRef")
			}
		}
	case "delivery_update_target_runtime":
		allowedKeys := map[string]struct{}{
			"project":       {},
			"application":   {},
			"target":        {},
			"cpuRequest":    {},
			"memoryRequest": {},
			"autoScaling":   {},
		}
		for key, val := range argsMap {
			if val == nil {
				continue
			}
			if _, ok := allowedKeys[key]; !ok {
				return aitool.Forbidden("ai.self_heal_not_permitted", fmt.Sprintf("runtime self-healing cannot mutate field %q", key))
			}
		}
		if rawAuto, exists := argsMap["autoScaling"]; exists && rawAuto != nil {
			autoMap, ok := rawAuto.(map[string]any)
			if !ok {
				return aitool.BadRequest("ai.tool_arguments_invalid", "autoScaling must be an object")
			}
			allowedAutoKeys := map[string]struct{}{
				"enabled":       {},
				"minReplicas":   {},
				"maxReplicas":   {},
				"cpuPercent":    {},
				"memoryPercent": {},
			}
			for k, v := range autoMap {
				if v == nil {
					continue
				}
				if _, ok := allowedAutoKeys[k]; !ok {
					return aitool.Forbidden("ai.self_heal_not_permitted", fmt.Sprintf("runtime self-healing cannot mutate autoScaling.%s", k))
				}
			}
			if minRep, exists := autoMap["minReplicas"]; exists && minRep != nil {
				if _, ok := parsePositiveIntArg(minRep); !ok {
					return aitool.Forbidden("ai.self_heal_not_permitted", "runtime self-healing requires autoScaling.minReplicas >= 1")
				}
			}
			if maxRep, exists := autoMap["maxReplicas"]; exists && maxRep != nil {
				if _, ok := parsePositiveIntArg(maxRep); !ok {
					return aitool.Forbidden("ai.self_heal_not_permitted", "runtime self-healing requires autoScaling.maxReplicas >= 1")
				}
			}
		}
	}
	return nil
}

func matchesApplicationIdentifier(candidate string, app model.Application) bool {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return true
	}
	return trimmed == app.ID ||
		(app.Identifier != "" && strings.EqualFold(trimmed, app.Identifier)) ||
		(app.Name != "" && strings.EqualFold(trimmed, app.Name))
}

func matchesTargetIdentifier(candidate string, target model.DeploymentTarget) bool {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return true
	}
	return trimmed == target.ID ||
		(target.Stage != "" && strings.EqualFold(trimmed, target.Stage)) ||
		(target.Name != "" && strings.EqualFold(trimmed, target.Name)) ||
		(target.KubernetesName != "" && strings.EqualFold(trimmed, target.KubernetesName))
}

func bindRuntimeHealArgumentsToTarget(
	rawArgs json.RawMessage,
	projectID string,
	app model.Application,
	target model.DeploymentTarget,
) (json.RawMessage, error) {
	boundProjectRaw, err := bindProposalArgumentsToProject(rawArgs, projectID)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(boundProjectRaw, &decoded); err != nil || decoded == nil {
		return nil, aitool.BadRequest("ai.tool_arguments_invalid", "arguments must be a valid JSON object")
	}
	if rawApp, exists := decoded["application"]; exists && rawApp != nil {
		appStr, ok := rawApp.(string)
		if !ok || !matchesApplicationIdentifier(appStr, app) {
			return nil, aitool.Forbidden("ai.project_scope_mismatch", "self-heal arguments target a different application than the incident")
		}
	}
	if rawTarget, exists := decoded["target"]; exists && rawTarget != nil {
		targetStr, ok := rawTarget.(string)
		if !ok || !matchesTargetIdentifier(targetStr, target) {
			return nil, aitool.Forbidden("ai.project_scope_mismatch", "self-heal arguments target a different deployment target than the incident")
		}
	}
	decoded["project"] = projectID
	decoded["application"] = app.ID
	decoded["target"] = target.ID

	normalized, marshalErr := json.Marshal(decoded)
	if marshalErr != nil {
		return nil, aitool.BadRequest("ai.tool_arguments_invalid", "failed to encode arguments")
	}
	return normalized, nil
}

type runtimeSelfHealDriftCheck struct {
	TerminalStale       bool
	Drifted             bool
	Reason              string
	CurrentReleaseID    string
	CurrentTargetUpdate string
}

func inspectRuntimeSelfHealState(
	db *gorm.DB,
	projectID string,
	incidentID string,
	targetID string,
	baselineReleaseID string,
	baselineTargetUpdatedAt string,
) runtimeSelfHealDriftCheck {
	if db == nil {
		return runtimeSelfHealDriftCheck{}
	}
	incID := strings.TrimSpace(incidentID)
	if incID == "" {
		return runtimeSelfHealDriftCheck{TerminalStale: true, Reason: "incident_not_found"}
	}
	var inc model.RuntimeAlertIncident
	if err := db.First(&inc, "id = ? AND project_id = ?", incID, projectID).Error; err != nil {
		return runtimeSelfHealDriftCheck{TerminalStale: true, Reason: "incident_not_found"}
	}
	if inc.Status != "active" {
		return runtimeSelfHealDriftCheck{TerminalStale: true, Reason: "incident_resolved"}
	}

	tgtID := strings.TrimSpace(targetID)
	if tgtID == "" || (strings.TrimSpace(inc.DeploymentTargetID) != "" && tgtID != strings.TrimSpace(inc.DeploymentTargetID)) {
		return runtimeSelfHealDriftCheck{TerminalStale: true, Reason: "target_not_found"}
	}
	var target model.DeploymentTarget
	if err := db.First(&target, "id = ? AND project_id = ? AND deleted_at IS NULL", tgtID, projectID).Error; err != nil {
		return runtimeSelfHealDriftCheck{TerminalStale: true, Reason: "target_not_found"}
	}
	if !target.Enabled || (target.DeleteStatus != "" && target.DeleteStatus != "active") {
		return runtimeSelfHealDriftCheck{TerminalStale: true, Reason: "target_unavailable"}
	}

	out := runtimeSelfHealDriftCheck{
		CurrentTargetUpdate: target.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	var latest model.Release
	if err := db.Where("project_id = ? AND deployment_target_id = ?", projectID, tgtID).
		Order("created_at desc").
		First(&latest).Error; err == nil {
		out.CurrentReleaseID = latest.ID
	}

	if baseUpdated := strings.TrimSpace(baselineTargetUpdatedAt); baseUpdated != "" && out.CurrentTargetUpdate != baseUpdated {
		out.Drifted = true
		out.Reason = "target_modified"
	}
	if baseRel := strings.TrimSpace(baselineReleaseID); baseRel != "" && out.CurrentReleaseID != "" && out.CurrentReleaseID != baseRel {
		out.Drifted = true
		out.Reason = "newer_release_created"
	}
	return out
}

func expireStaleSelfHealActionRequest(db *gorm.DB, reqRow model.InboxActionRequest) {
	if db == nil || strings.TrimSpace(reqRow.ID) == "" {
		return
	}
	now := time.Now().UTC()
	_ = db.Model(&model.InboxActionRequest{}).
		Where("id = ? AND status = ?", reqRow.ID, inbox.ActionRequestStatusPending).
		Updates(map[string]any{
			"status":      inbox.ActionRequestStatusExpired,
			"row_version": reqRow.RowVersion + 1,
			"updated_at":  now,
		}).Error
}

func formatBeforeAfterSummary(preview aitool.ApprovalPreview) string {
	var parts []string
	if summary := strings.TrimSpace(preview.Summary); summary != "" {
		parts = append(parts, summary)
	}
	for _, ch := range preview.Changes {
		field := strings.TrimSpace(ch.Field)
		if field == "" {
			continue
		}
		if ch.Before != nil {
			parts = append(parts, fmt.Sprintf("%s: %v -> %v", field, ch.Before, ch.After))
		} else if ch.After != nil {
			parts = append(parts, fmt.Sprintf("%s: %v", field, ch.After))
		}
	}
	return strings.Join(parts, "; ")
}

// PreviewInboxSelfHealAction handles POST /api/v1/inbox/action-requests/:requestId/self-heal-preview.
// It runs a live Preview under the clicking user's identity & RBAC, reports whether target state has drifted,
// and mints a fresh user-bound previewLock for confirming self-healing.
func (h *Handler) PreviewInboxSelfHealAction(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	span := trace.SpanFromContext(ctx.Request.Context())
	session, hasSession := h.host.CurrentSessionFromCookie(ctx)
	if !hasSession {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required")
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Disabled {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return
	}
	if !h.aiAssistantEnabled() {
		writeErrorCode(ctx, http.StatusForbidden, "ai.disabled", "AI assistant is disabled")
		return
	}
	if !h.aiAccessAllowed(user.Role) {
		writeErrorKey(ctx, http.StatusForbidden, requestLanguage(ctx), "auth.permission.denied")
		return
	}

	requestID := strings.TrimSpace(ctx.Param("requestId"))
	if requestID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.request_id_required", "requestId is required")
		return
	}
	db := h.dbFor(ctx)
	reqRow, err := inbox.NewService(db).GetActionRequest(ctx.Request.Context(), user.ID, requestID)
	if err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "inbox.not_found", "inbox action request was not found")
		return
	}
	if reqRow.Type != inbox.ActionRequestTypeRuntimeSelfHeal {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.request_invalid", "action request is not a runtime self-heal proposal")
		return
	}
	if reqRow.ExpiresAt != nil && !reqRow.ExpiresAt.After(time.Now()) {
		writeErrorCode(ctx, http.StatusGone, "inbox.action_request_expired", "inbox action request has expired")
		return
	}
	if reqRow.Status != inbox.ActionRequestStatusPending {
		writeErrorCode(ctx, http.StatusConflict, "inbox.action_request_stale", "inbox action request is no longer pending")
		return
	}

	_, project, authOK := h.host.AuthorizeProjectByID(ctx, reqRow.ProjectID, authz.ActionProjectRead)
	if !authOK {
		return
	}

	var payload RuntimeSelfHealPayload
	if err := json.Unmarshal([]byte(reqRow.PayloadJSON), &payload); err != nil || strings.TrimSpace(payload.ToolName) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.request_invalid", "self-heal proposal payload is invalid")
		return
	}
	if strings.TrimSpace(payload.DeploymentTargetID) == "" ||
		(strings.TrimSpace(reqRow.ResourceID) != "" && strings.TrimSpace(payload.DeploymentTargetID) != strings.TrimSpace(reqRow.ResourceID)) {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.request_invalid", "self-heal proposal target binding is invalid")
		return
	}

	driftState := inspectRuntimeSelfHealState(
		db,
		project.ID,
		payload.IncidentID,
		payload.DeploymentTargetID,
		payload.BaselineReleaseID,
		payload.BaselineTargetUpdatedAt,
	)
	if driftState.TerminalStale {
		if driftState.Reason == "incident_resolved" {
			expireStaleSelfHealActionRequest(db, reqRow)
		}
		writeErrorCode(ctx, http.StatusConflict, "inbox.action_request_stale", "incident or deployment target is no longer active")
		return
	}

	var target model.DeploymentTarget
	if err := db.First(&target, "id = ? AND project_id = ? AND deleted_at IS NULL", payload.DeploymentTargetID, project.ID).Error; err != nil {
		writeErrorCode(ctx, http.StatusConflict, "inbox.action_request_stale", "deployment target was not found")
		return
	}
	var app model.Application
	appID := firstNonEmptyString(strings.TrimSpace(payload.ApplicationID), target.ApplicationID)
	if err := db.First(&app, "id = ? AND project_id = ? AND deleted_at IS NULL", appID, project.ID).Error; err != nil {
		writeErrorCode(ctx, http.StatusConflict, "inbox.action_request_stale", "application was not found")
		return
	}

	reg := h.host.AIToolRegistry()
	if reg == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable")
		return
	}
	tool, exists := reg.Get(strings.TrimSpace(payload.ToolName))
	if !exists || !isAllowedRuntimeHealTool(tool) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "requested tool is not available for runtime self-healing")
		return
	}
	span.SetAttributes(attribute.String("luna.tool.name", tool.Catalog.OperationID))
	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) || !projectMemberAllowsToolScopes(db, user, project.ID, tool.Catalog.RequiredScopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "tool exceeds authorized scopes")
		return
	}

	rawArgs, _ := json.Marshal(payload.Arguments)
	normalizedRawArgs, bindErr := bindRuntimeHealArgumentsToTarget(rawArgs, project.ID, app, target)
	if bindErr != nil {
		writeFacadeToolError(ctx, bindErr)
		return
	}
	effectiveArgs, argsMap, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, normalizedRawArgs, project.ID)
	if sandboxErr != nil {
		span.SetStatus(codes.Error, "self-heal sandbox rejected")
		writeFacadeToolError(ctx, sandboxErr)
		return
	}
	if safeErr := validateSafeRuntimeHealArguments(tool.Catalog.OperationID, argsMap); safeErr != nil {
		writeFacadeToolError(ctx, safeErr)
		return
	}
	argsHash, hashErr := aitool.CanonicalJSONHash(effectiveArgs)
	if hashErr != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "arguments must be valid JSON")
		return
	}

	isProd := isProductionStageValue(target.Stage)
	effectiveTier := resolveEffectiveApprovalTier(tool.Catalog, argsMap, isProd)
	runBindingID := "inbox-self-heal:" + reqRow.ID
	callID := id.New("ish")
	actorSessionID := firstNonEmptyString(strings.TrimSpace(session.ID), "inbox-self-heal:"+user.ID)

	requestCtx := h.host.WithAIPlatformActor(ctx.Request.Context(), PlatformActor{
		UserID:    user.ID,
		SessionID: actorSessionID,
		ProjectID: project.ID,
	})
	restore := replaceRequestContext(ctx, requestCtx)
	defer restore()

	h.host.SetCurrentUser(ctx, user)
	tc := aitool.NewToolContext(
		ctx,
		db,
		user,
		actorSessionID,
		"inbox-self-heal",
		project.ID,
		runBindingID,
		callID,
		"inbox-self-heal:"+callID,
		"inbox",
		nil,
	)

	previewRes, prevErr := runToolPreviewOrFallback(tool, tc, effectiveArgs)
	if prevErr != nil {
		span.SetStatus(codes.Error, "inbox self-heal preview failed")
		writeFacadeToolError(ctx, prevErr)
		return
	}

	rawToolLocked, _ := json.Marshal(previewRes.LockedPayload)
	envelope := selfHealLockedEnvelope{
		ToolLockedPayload:       rawToolLocked,
		BaselineReleaseID:       driftState.CurrentReleaseID,
		BaselineTargetUpdatedAt: driftState.CurrentTargetUpdate,
	}

	now := time.Now()
	claims := aitool.PreviewLockClaims{
		RunID:                 runBindingID,
		ToolCallID:            "self-heal",
		OperationID:           tool.Catalog.OperationID,
		ArgumentsHash:         argsHash,
		OwnerUserID:           user.ID,
		ConversationProjectID: project.ID,
		IssuedAt:              now.Unix(),
		ExpiresAt:             now.Add(proposalPreviewLockTTL).Unix(),
	}
	lockToken, signErr := aitool.SignPreviewLock(h.host.MCPPreviewSigningSecret(), claims, envelope)
	if signErr != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.preview_lock_sign_failed", "failed to sign preview lock")
		return
	}

	ctx.JSON(http.StatusOK, InboxSelfHealPreviewOutput{
		ToolName:        tool.Catalog.OperationID,
		ApprovalTier:    effectiveTier,
		ApprovalPreview: previewRes.Preview,
		PreviewLock:     lockToken,
		StateDrifted:    driftState.Drifted,
		DriftReason:     driftState.Reason,
	})
}

// DecideRuntimeSelfHealInboxAction handles accept/reject decisions on inbox.ActionRequestTypeRuntimeSelfHeal.
// It NEVER reuses a rule owner's previewLock; on accept, it runs live Preview under the clicking user's
// own identity & RBAC (or verifies a fresh re-preview lock minted for this exact clicking user), enforces
// drift checks, executes the remediation tool once across all members, and writes AuditLog.
func (h *Handler) DecideRuntimeSelfHealInboxAction(
	ctx context.Context,
	db *gorm.DB,
	user model.User,
	reqRow model.InboxActionRequest,
	decision string,
	expectedVersion int64,
) ([]string, error) {
	if reqRow.ExpiresAt != nil && !reqRow.ExpiresAt.After(time.Now()) {
		return nil, inbox.ErrActionRequestExpired
	}
	if reqRow.Status != inbox.ActionRequestStatusPending || reqRow.RowVersion != expectedVersion {
		return nil, inbox.ErrActionRequestStale
	}

	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision == "reject" {
		now := time.Now().UTC()
		res := db.WithContext(ctx).Model(&model.InboxActionRequest{}).
			Where("id = ? AND recipient_user_id = ? AND status = ? AND row_version = ?", reqRow.ID, user.ID, inbox.ActionRequestStatusPending, expectedVersion).
			Updates(map[string]any{
				"status":       inbox.ActionRequestStatusRejected,
				"row_version":  expectedVersion + 1,
				"responded_at": &now,
				"updated_at":   now,
			})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			return nil, inbox.ErrActionRequestStale
		}
		h.auditWithContext(user.ID, "inbox.self_heal.reject", reqRow.ProjectID, true, reqRow.ID, ctx)
		return []string{user.ID}, nil
	}
	if decision != "accept" {
		return nil, inbox.ErrInvalidInput
	}

	meta, hasMeta := inbox.DecisionMetaFromContext(ctx)
	ginCtx, carrierOK := meta.RequestCarrier.(*gin.Context)
	if !hasMeta || !carrierOK || ginCtx == nil {
		return nil, inbox.ErrSelfHealSessionRequired
	}
	session, hasSession := h.host.CurrentSessionFromCookie(ginCtx)
	if !hasSession {
		return nil, inbox.ErrSelfHealSessionRequired
	}
	if user.Disabled {
		return nil, inbox.ErrSelfHealForbidden
	}
	if !h.aiAssistantEnabled() || !h.aiAccessAllowed(user.Role) {
		return nil, inbox.ErrSelfHealForbidden
	}

	var project model.Project
	if err := db.WithContext(ctx).First(&project, "id = ? AND deleted_at IS NULL", reqRow.ProjectID).Error; err != nil {
		return nil, inbox.ErrNotFound
	}
	if !authz.IsPlatformAdmin(user.Role) {
		var memberCount int64
		if err := db.WithContext(ctx).Model(&model.ProjectMember{}).
			Where("project_id = ? AND user_id = ?", project.ID, user.ID).
			Count(&memberCount).Error; err != nil || memberCount == 0 {
			return nil, inbox.ErrSelfHealForbidden
		}
	}

	var payload RuntimeSelfHealPayload
	if err := json.Unmarshal([]byte(reqRow.PayloadJSON), &payload); err != nil || strings.TrimSpace(payload.ToolName) == "" {
		return nil, inbox.ErrInvalidInput
	}
	if strings.TrimSpace(payload.DeploymentTargetID) == "" ||
		(strings.TrimSpace(reqRow.ResourceID) != "" && strings.TrimSpace(payload.DeploymentTargetID) != strings.TrimSpace(reqRow.ResourceID)) {
		return nil, inbox.ErrInvalidInput
	}

	driftState := inspectRuntimeSelfHealState(
		db.WithContext(ctx),
		project.ID,
		payload.IncidentID,
		payload.DeploymentTargetID,
		payload.BaselineReleaseID,
		payload.BaselineTargetUpdatedAt,
	)
	if driftState.TerminalStale {
		if driftState.Reason == "incident_resolved" {
			expireStaleSelfHealActionRequest(db.WithContext(ctx), reqRow)
		}
		return nil, inbox.ErrActionRequestStale
	}

	var target model.DeploymentTarget
	if err := db.WithContext(ctx).First(&target, "id = ? AND project_id = ? AND deleted_at IS NULL", payload.DeploymentTargetID, project.ID).Error; err != nil {
		return nil, inbox.ErrActionRequestStale
	}
	var app model.Application
	appID := firstNonEmptyString(strings.TrimSpace(payload.ApplicationID), target.ApplicationID)
	if err := db.WithContext(ctx).First(&app, "id = ? AND project_id = ? AND deleted_at IS NULL", appID, project.ID).Error; err != nil {
		return nil, inbox.ErrActionRequestStale
	}

	reg := h.host.AIToolRegistry()
	if reg == nil {
		return nil, errors.New("AI tool registry is unavailable")
	}
	tool, exists := reg.Get(strings.TrimSpace(payload.ToolName))
	if !exists || !isAllowedRuntimeHealTool(tool) {
		return nil, inbox.ErrSelfHealForbidden
	}
	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) || !projectMemberAllowsToolScopes(db.WithContext(ctx), user, project.ID, tool.Catalog.RequiredScopes) {
		return nil, inbox.ErrSelfHealForbidden
	}

	rawArgs, _ := json.Marshal(payload.Arguments)
	normalizedRawArgs, bindErr := bindRuntimeHealArgumentsToTarget(rawArgs, project.ID, app, target)
	if bindErr != nil {
		return nil, bindErr
	}
	effectiveArgs, argsMap, sandboxErr := h.enforceConversationProjectSandbox(ginCtx, tool, normalizedRawArgs, project.ID)
	if sandboxErr != nil {
		return nil, sandboxErr
	}
	if safeErr := validateSafeRuntimeHealArguments(tool.Catalog.OperationID, argsMap); safeErr != nil {
		return nil, safeErr
	}
	argsHash, hashErr := aitool.CanonicalJSONHash(effectiveArgs)
	if hashErr != nil {
		return nil, inbox.ErrInvalidInput
	}

	suppliedLock := strings.TrimSpace(meta.PreviewLock)
	if driftState.Drifted && suppliedLock == "" {
		return nil, inbox.ErrSelfHealStateDrifted
	}

	runBindingID := "inbox-self-heal:" + reqRow.ID
	callID := id.New("ish")
	actorSessionID := firstNonEmptyString(strings.TrimSpace(session.ID), "inbox-self-heal:"+user.ID)
	requestCtx := h.host.WithAIPlatformActor(ctx, PlatformActor{
		UserID:    user.ID,
		SessionID: actorSessionID,
		ProjectID: project.ID,
	})
	restore := replaceRequestContext(ginCtx, requestCtx)
	defer restore()
	h.host.SetCurrentUser(ginCtx, user)

	var verifiedLockedPayload json.RawMessage
	if suppliedLock != "" {
		locked, verifyErr := aitool.VerifyPreviewLock(
			[][]byte{h.host.MCPPreviewSigningSecret()},
			suppliedLock,
			aitool.PreviewLockClaims{
				RunID:                 runBindingID,
				ToolCallID:            "self-heal",
				OperationID:           tool.Catalog.OperationID,
				ArgumentsHash:         argsHash,
				OwnerUserID:           user.ID,
				ConversationProjectID: project.ID,
			},
			time.Now(),
		)
		if verifyErr != nil {
			return nil, mapPreviewLockVerifyError(verifyErr)
		}
		var env selfHealLockedEnvelope
		if err := json.Unmarshal(locked, &env); err == nil && (env.BaselineTargetUpdatedAt != "" || env.BaselineReleaseID != "" || len(env.ToolLockedPayload) > 0) {
			postPreviewDrift := inspectRuntimeSelfHealState(
				db.WithContext(requestCtx),
				project.ID,
				payload.IncidentID,
				payload.DeploymentTargetID,
				env.BaselineReleaseID,
				env.BaselineTargetUpdatedAt,
			)
			if postPreviewDrift.TerminalStale {
				return nil, inbox.ErrActionRequestStale
			}
			if postPreviewDrift.Drifted {
				return nil, inbox.ErrSelfHealStateDrifted
			}
			verifiedLockedPayload = env.ToolLockedPayload
		} else {
			verifiedLockedPayload = locked
		}
	} else {
		previewTC := aitool.NewToolContext(
			ginCtx,
			db.WithContext(requestCtx),
			user,
			actorSessionID,
			"inbox-self-heal",
			project.ID,
			runBindingID,
			callID,
			"inbox-self-heal:preview:"+callID,
			"inbox",
			nil,
		)
		previewRes, prevErr := runToolPreviewOrFallback(tool, previewTC, effectiveArgs)
		if prevErr != nil {
			return nil, prevErr
		}
		lockedRaw := previewRes.LockedPayload
		now := time.Now()
		mintedLock, signErr := aitool.SignPreviewLock(
			h.host.MCPPreviewSigningSecret(),
			aitool.PreviewLockClaims{
				RunID:                 runBindingID,
				ToolCallID:            "self-heal",
				OperationID:           tool.Catalog.OperationID,
				ArgumentsHash:         argsHash,
				OwnerUserID:           user.ID,
				ConversationProjectID: project.ID,
				IssuedAt:              now.Unix(),
				ExpiresAt:             now.Add(proposalPreviewLockTTL).Unix(),
			},
			lockedRaw,
		)
		if signErr != nil {
			return nil, signErr
		}
		locked, verifyErr := aitool.VerifyPreviewLock(
			[][]byte{h.host.MCPPreviewSigningSecret()},
			mintedLock,
			aitool.PreviewLockClaims{
				RunID:                 runBindingID,
				ToolCallID:            "self-heal",
				OperationID:           tool.Catalog.OperationID,
				ArgumentsHash:         argsHash,
				OwnerUserID:           user.ID,
				ConversationProjectID: project.ID,
			},
			now,
		)
		if verifyErr != nil {
			return nil, mapPreviewLockVerifyError(verifyErr)
		}
		verifiedLockedPayload = locked
	}

	// Atomically claim the action request (and serialize by incidentId so two members cannot double-execute).
	alreadyHealedByPeer := false
	claimErr := db.WithContext(requestCtx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector != nil && tx.Dialector.Name() == "postgres" && strings.TrimSpace(payload.IncidentID) != "" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "inbox_self_heal:"+payload.IncidentID).Error; err != nil {
				return err
			}
		}
		var liveInc model.RuntimeAlertIncident
		if err := tx.First(&liveInc, "id = ? AND project_id = ?", payload.IncidentID, project.ID).Error; err != nil || liveInc.Status != "active" {
			return inbox.ErrActionRequestStale
		}
		if strings.TrimSpace(payload.IncidentID) != "" {
			var peerCompleted int64
			if err := tx.Model(&model.InboxActionRequest{}).
				Where("project_id = ? AND type = ? AND id <> ? AND status IN ? AND payload_json ->> 'incidentId' = ?",
					project.ID,
					inbox.ActionRequestTypeRuntimeSelfHeal,
					reqRow.ID,
					[]string{inbox.ActionRequestStatusProcessing, inbox.ActionRequestStatusCompleted},
					strings.TrimSpace(payload.IncidentID),
				).Count(&peerCompleted).Error; err == nil && peerCompleted > 0 {
				alreadyHealedByPeer = true
				return nil
			}
		}
		res := tx.Model(&model.InboxActionRequest{}).
			Where("id = ? AND recipient_user_id = ? AND status = ? AND row_version = ?",
				reqRow.ID, user.ID, inbox.ActionRequestStatusPending, expectedVersion).
			Updates(map[string]any{
				"status":     inbox.ActionRequestStatusProcessing,
				"updated_at": time.Now().UTC(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return inbox.ErrActionRequestStale
		}
		return nil
	})
	if claimErr != nil {
		return nil, claimErr
	}
	if alreadyHealedByPeer {
		return nil, inbox.ErrActionRequestStale
	}

	execTC := aitool.NewToolContext(
		ginCtx,
		db.WithContext(requestCtx),
		user,
		actorSessionID,
		"inbox-self-heal",
		project.ID,
		runBindingID,
		callID,
		"inbox-self-heal:exec:"+callID,
		"inbox",
		verifiedLockedPayload,
	)
	_, execErr := tool.Execute(execTC, effectiveArgs)
	h.auditWithContext(
		user.ID,
		"inbox.self_heal.execute."+tool.Catalog.OperationID,
		project.ID,
		execErr == nil,
		reqRow.ID+":"+callID,
		requestCtx,
	)
	if execErr != nil {
		_ = db.WithContext(requestCtx).Model(&model.InboxActionRequest{}).
			Where("id = ? AND status = ?", reqRow.ID, inbox.ActionRequestStatusProcessing).
			Updates(map[string]any{
				"status":     inbox.ActionRequestStatusPending,
				"updated_at": time.Now().UTC(),
			}).Error
		return nil, execErr
	}

	now := time.Now().UTC()
	notifiedUsers := []string{user.ID}
	_ = db.WithContext(requestCtx).Model(&model.InboxActionRequest{}).
		Where("id = ?", reqRow.ID).
		Updates(map[string]any{
			"status":       inbox.ActionRequestStatusCompleted,
			"row_version":  expectedVersion + 1,
			"responded_at": &now,
			"updated_at":   now,
		}).Error

	if strings.TrimSpace(payload.IncidentID) != "" {
		var siblings []model.InboxActionRequest
		if err := db.WithContext(requestCtx).
			Where("project_id = ? AND type = ? AND id <> ? AND status = ?",
				project.ID, inbox.ActionRequestTypeRuntimeSelfHeal, reqRow.ID, inbox.ActionRequestStatusPending).
			Find(&siblings).Error; err == nil {
			for _, sib := range siblings {
				var sibPayload RuntimeSelfHealPayload
				if json.Unmarshal([]byte(sib.PayloadJSON), &sibPayload) == nil &&
					strings.TrimSpace(sibPayload.IncidentID) == strings.TrimSpace(payload.IncidentID) {
					_ = db.WithContext(requestCtx).Model(&model.InboxActionRequest{}).
						Where("id = ? AND status = ?", sib.ID, inbox.ActionRequestStatusPending).
						Updates(map[string]any{
							"status":       inbox.ActionRequestStatusCompleted,
							"row_version":  sib.RowVersion + 1,
							"responded_at": &now,
							"updated_at":   now,
						}).Error
					notifiedUsers = append(notifiedUsers, sib.RecipientUserID)
				}
			}
		}
	}
	return notifiedUsers, nil
}

// ExecuteAutoHealInternal handles POST /internal/v1/ai/tools/auto-heal.
// Called by luna-worker when an active runtime incident matches project auto_l1 / auto_l2 self-healing policy.
// It verifies service token (+ optional signed actor headers), verifies the active RuntimeAlertIncident and
// authoritative DeploymentTarget in PostgreSQL, enforces live RBAC + tier/whitelist/argument safety + circuit breaker,
// runs Preview + previewLock verification + Execute, and records an AuditLog entry.
func (h *Handler) ExecuteAutoHealInternal(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	span := trace.SpanFromContext(ctx.Request.Context())
	if !h.requireAIAgentService(ctx) {
		return
	}

	var in AutoHealInternalInput
	if !bindJSON(ctx, &in) {
		return
	}
	actorUserID := strings.TrimSpace(in.ActorUserID)
	projectID := strings.TrimSpace(in.ProjectID)
	incidentID := strings.TrimSpace(in.IncidentID)
	toolName := strings.TrimSpace(in.ToolName)
	if actorUserID == "" || projectID == "" || incidentID == "" || toolName == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "actorUserId, projectId, incidentId, and toolName are required")
		return
	}
	if actorOverride, _, actorOK, actorConfigured := h.host.AIActorOverride(ctx); actorConfigured {
		if !actorOK || actorOverride.UserID != actorUserID || (strings.TrimSpace(actorOverride.ProjectID) != "" && strings.TrimSpace(actorOverride.ProjectID) != projectID) {
			writeErrorCode(ctx, http.StatusForbidden, "ai.project_scope_mismatch", "signed actor does not match auto-heal request")
			return
		}
	}

	if !h.aiAssistantEnabled() {
		writeErrorCode(ctx, http.StatusForbidden, "ai.disabled", "AI assistant is disabled")
		return
	}

	db := h.dbFor(ctx)
	var liveCfg model.ProjectAIAutomationConfig
	if err := db.First(&liveCfg, "project_id = ?", projectID).Error; err != nil || !liveCfg.RuntimeDiagnosisEnabled {
		writeErrorCode(ctx, http.StatusForbidden, "ai.self_heal_not_permitted", "project runtime AI self-healing is disabled")
		return
	}
	if strings.TrimSpace(liveCfg.RuntimeBillingUserID) == "" || strings.TrimSpace(liveCfg.RuntimeBillingUserID) != actorUserID {
		writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "actorUserId does not match the project's bound runtimeBillingUserId")
		return
	}

	var inc model.RuntimeAlertIncident
	if err := db.First(&inc, "id = ? AND project_id = ? AND status = ?", incidentID, projectID, "active").Error; err != nil {
		writeErrorCode(ctx, http.StatusConflict, "ai.incident_not_active", "runtime alert incident is no longer active")
		return
	}
	var target model.DeploymentTarget
	if err := db.First(&target, "id = ? AND project_id = ? AND deleted_at IS NULL", inc.DeploymentTargetID, projectID).Error; err != nil ||
		!target.Enabled || (target.DeleteStatus != "" && target.DeleteStatus != "active") {
		writeErrorCode(ctx, http.StatusConflict, "ai.target_unavailable", "deployment target is unavailable for self-healing")
		return
	}
	var app model.Application
	appID := firstNonEmptyString(strings.TrimSpace(inc.ApplicationID), target.ApplicationID)
	if err := db.First(&app, "id = ? AND project_id = ? AND deleted_at IS NULL", appID, projectID).Error; err != nil {
		writeErrorCode(ctx, http.StatusConflict, "ai.target_unavailable", "application is unavailable for self-healing")
		return
	}

	var user model.User
	if err := db.First(&user, "id = ? AND disabled = ?", actorUserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return
	}
	if !h.aiAccessAllowed(user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "user is not permitted to use AI automation")
		return
	}
	if !authz.IsPlatformAdmin(user.Role) {
		var memberCount int64
		if err := db.Model(&model.ProjectMember{}).
			Where("project_id = ? AND user_id = ?", projectID, user.ID).
			Count(&memberCount).Error; err != nil || memberCount == 0 {
			writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "billing owner is no longer a member of this project")
			return
		}
	}

	reg := h.host.AIToolRegistry()
	if reg == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable")
		return
	}
	tool, exists := reg.Get(toolName)
	if !exists || !isAllowedRuntimeHealTool(tool) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "tool is not permitted for automatic runtime self-healing")
		return
	}
	span.SetAttributes(attribute.String("luna.tool.name", tool.Catalog.OperationID))

	if !liveCfg.AllowsAutoHealTool(tool.Catalog.OperationID) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.self_heal_tool_not_whitelisted", "tool is not in project allowedAutoHealTools whitelist")
		return
	}
	if len(in.AllowedTools) > 0 {
		allowed := false
		for _, item := range in.AllowedTools {
			if strings.TrimSpace(item) == tool.Catalog.OperationID {
				allowed = true
				break
			}
		}
		if !allowed {
			writeErrorCode(ctx, http.StatusForbidden, "ai.self_heal_tool_not_whitelisted", "tool is not in project allowedAutoHealTools whitelist")
			return
		}
	}

	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) || !projectMemberAllowsToolScopes(db, user, projectID, tool.Catalog.RequiredScopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "tool exceeds authorized scopes")
		return
	}

	normalizedRawArgs, bindErr := bindRuntimeHealArgumentsToTarget(in.Arguments, projectID, app, target)
	if bindErr != nil {
		writeFacadeToolError(ctx, bindErr)
		return
	}
	effectiveArgs, argsMap, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, normalizedRawArgs, projectID)
	if sandboxErr != nil {
		span.SetStatus(codes.Error, "auto-heal sandbox rejected")
		writeFacadeToolError(ctx, sandboxErr)
		return
	}
	if safeErr := validateSafeRuntimeHealArguments(tool.Catalog.OperationID, argsMap); safeErr != nil {
		writeFacadeToolError(ctx, safeErr)
		return
	}
	argsHash, hashErr := aitool.CanonicalJSONHash(effectiveArgs)
	if hashErr != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "arguments must be valid JSON")
		return
	}

	mode := liveCfg.EffectiveSelfHealModeForStage(target.Stage)
	if requestedMode := strings.TrimSpace(in.SelfHealMode); requestedMode != "" && model.SelfHealModeRank(requestedMode) < model.SelfHealModeRank(mode) {
		mode = requestedMode
	}
	if mode != model.AISelfHealModeAutoL1 && mode != model.AISelfHealModeAutoL2 {
		writeErrorCode(ctx, http.StatusForbidden, "ai.self_heal_not_permitted", "selfHealMode does not permit automatic execution")
		return
	}

	isProd := isProductionStageValue(target.Stage)
	effectiveTier := resolveEffectiveApprovalTier(tool.Catalog, argsMap, isProd)
	if mode == model.AISelfHealModeAutoL1 && (isProd || effectiveTier == aitool.ApprovalTierL2) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.self_heal_tier_exceeded", "auto_l1 policy does not permit L2 or production write operations")
		return
	}

	if _, loaded := inFlightAutoHealTargets.LoadOrStore(target.ID, struct{}{}); loaded {
		writeErrorCode(ctx, http.StatusConflict, "ai.auto_heal_in_progress", "another auto-heal operation is currently running on this target")
		return
	}
	defer inFlightAutoHealTargets.Delete(target.ID)

	// Enforce serialized anti-flapping circuit-breaker check + atomic diagnosis claim at the API boundary.
	windowSecs := liveCfg.CircuitBreakerWindowSeconds
	if windowSecs < 60 {
		windowSecs = model.DefaultRuntimeCircuitBreakerWindowSeconds
	}
	maxConsecutive := liveCfg.MaxConsecutiveAutoHeals
	if maxConsecutive < 1 {
		maxConsecutive = model.DefaultRuntimeMaxConsecutiveAutoHeals
	}
	diagnosisID := strings.TrimSpace(in.DiagnosisID)
	circuitTripped := false
	alreadyClaimed := false
	_ = db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector != nil && tx.Dialector.Name() == "postgres" {
			_ = tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "ai_auto_heal_target:"+target.ID).Error
		}
		var recentSucceeded int64
		cutoff := time.Now().UTC().Add(-time.Duration(windowSecs) * time.Second)
		if err := tx.Model(&model.AIRuntimeIncidentDiagnosis{}).
			Where("deployment_target_id = ? AND auto_heal_status = ? AND created_at >= ?",
				target.ID, model.AIAutoHealStatusSucceeded, cutoff).
			Count(&recentSucceeded).Error; err == nil && recentSucceeded >= int64(maxConsecutive) {
			circuitTripped = true
			return nil
		}
		if diagnosisID != "" {
			res := tx.Model(&model.AIRuntimeIncidentDiagnosis{}).
				Where("id = ? AND project_id = ? AND status = ? AND auto_heal_status = ? AND auto_heal_summary = ''",
					diagnosisID, projectID, model.AIDiagnosisStatusRunning, model.AIAutoHealStatusNone).
				Update("auto_heal_summary", "in_progress")
			if res.Error != nil || res.RowsAffected == 0 {
				alreadyClaimed = true
			}
		}
		return nil
	})
	if circuitTripped {
		writeErrorCode(ctx, http.StatusConflict, "ai.self_heal_circuit_broken", "auto-heal circuit breaker tripped for target")
		return
	}
	if alreadyClaimed {
		writeErrorCode(ctx, http.StatusConflict, "ai.auto_heal_in_progress", "diagnosis auto-heal execution was already claimed")
		return
	}
	releaseClaimOnFailure := func() {
		if diagnosisID != "" {
			_ = db.Model(&model.AIRuntimeIncidentDiagnosis{}).
				Where("id = ? AND auto_heal_summary = ?", diagnosisID, "in_progress").
				Update("auto_heal_summary", "").Error
		}
	}

	runBindingID := "auto-heal:" + projectID + ":" + incidentID
	callID := id.New("aheal")
	actorSessionID := "auto-heal:" + user.ID

	requestCtx := h.host.WithAIPlatformActor(ctx.Request.Context(), PlatformActor{
		UserID:    user.ID,
		SessionID: actorSessionID,
		ProjectID: projectID,
	})
	restore := replaceRequestContext(ctx, requestCtx)
	defer restore()
	h.host.SetCurrentUser(ctx, user)

	previewTC := aitool.NewToolContext(
		ctx,
		db.WithContext(requestCtx),
		user,
		actorSessionID,
		"auto-heal",
		projectID,
		runBindingID,
		callID,
		"auto-heal:preview:"+callID,
		"auto-heal",
		nil,
	)
	resolvedPreview, prevErr := runToolPreviewOrFallback(tool, previewTC, effectiveArgs)
	if prevErr != nil {
		releaseClaimOnFailure()
		span.SetStatus(codes.Error, "auto-heal preview failed")
		writeFacadeToolError(ctx, prevErr)
		return
	}

	now := time.Now()
	lockClaims := aitool.PreviewLockClaims{
		RunID:                 runBindingID,
		ToolCallID:            callID,
		OperationID:           tool.Catalog.OperationID,
		ArgumentsHash:         argsHash,
		OwnerUserID:           user.ID,
		ConversationProjectID: projectID,
		IssuedAt:              now.Unix(),
		ExpiresAt:             now.Add(5 * time.Minute).Unix(),
	}
	mintedLock, signErr := aitool.SignPreviewLock(h.host.MCPPreviewSigningSecret(), lockClaims, resolvedPreview.LockedPayload)
	if signErr != nil {
		releaseClaimOnFailure()
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.preview_lock_sign_failed", "failed to sign auto-heal preview lock")
		return
	}
	verifiedPayload, verifyErr := aitool.VerifyPreviewLock(
		[][]byte{h.host.MCPPreviewSigningSecret()},
		mintedLock,
		aitool.PreviewLockClaims{
			RunID:                 runBindingID,
			ToolCallID:            callID,
			OperationID:           tool.Catalog.OperationID,
			ArgumentsHash:         argsHash,
			OwnerUserID:           user.ID,
			ConversationProjectID: projectID,
		},
		now,
	)
	if verifyErr != nil {
		releaseClaimOnFailure()
		writeFacadeToolError(ctx, mapPreviewLockVerifyError(verifyErr))
		return
	}

	execTC := aitool.NewToolContext(
		ctx,
		db.WithContext(requestCtx),
		user,
		actorSessionID,
		"auto-heal",
		projectID,
		runBindingID,
		callID,
		"auto-heal:exec:"+callID,
		"auto-heal",
		verifiedPayload,
	)
	out, execErr := tool.Execute(execTC, effectiveArgs)
	h.auditWithContext(
		user.ID,
		"ai.self_heal.auto_execute."+tool.Catalog.OperationID,
		projectID,
		execErr == nil,
		incidentID+":"+callID,
		requestCtx,
	)
	if execErr != nil {
		releaseClaimOnFailure()
		span.SetStatus(codes.Error, "auto-heal execute failed")
		writeFacadeToolError(ctx, execErr)
		return
	}

	ctx.JSON(http.StatusOK, AutoHealInternalOutput{
		ToolName:           tool.Catalog.OperationID,
		ApprovalTier:       effectiveTier,
		PreviewSummary:     resolvedPreview.Preview.Summary,
		BeforeAfterSummary: formatBeforeAfterSummary(resolvedPreview.Preview),
		Result:             out,
	})
}
