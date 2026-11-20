package aiapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const (
	aiPreviewLockHeader = "X-Luna-AI-Preview-Lock"
	aiPreviewLockTTL    = 24 * time.Hour
)

type aiFacadeToolRequest struct {
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments"`
}

type aiFacadeToolBinding struct {
	OwnerUserID              string `gorm:"column:owner_user_id"`
	ActorSessionID           string `gorm:"column:actor_session_id"`
	ConversationID           string `gorm:"column:conversation_id"`
	RunStatus                string `gorm:"column:run_status"`
	ConversationOwnerID      string `gorm:"column:conversation_owner_id"`
	ConversationProjectID    string `gorm:"column:conversation_project_id"`
	ConversationApprovalMode string `gorm:"column:conversation_approval_mode"`
	OperationID              string `gorm:"column:operation_id"`
	ToolStatus               string `gorm:"column:tool_status"`
	InputMode                string `gorm:"column:input_mode"`
	ArgumentsHash            string `gorm:"column:arguments_hash"`
	ApprovalDecision         string `gorm:"column:approval_decision"`
	RememberedApproval       bool   `gorm:"column:remembered_approval"`
}

// PreviewAIToolInternal handles POST /internal/v1/ai/tools/preview.
// It performs zero-trust actor resolution, RBAC/project sandbox validation,
// dry-run conflict detection, and issues an HMAC-signed X-Luna-AI-Preview-Lock token.
func (h *Handler) PreviewAIToolInternal(ctx *gin.Context) {
	h.handleAIFacadeTool(ctx, true)
}

// ExecuteAIToolInternal handles POST /internal/v1/ai/tools/execute.
// It enforces L0/L1/L2 approval state, X-Luna-AI-Preview-Lock verification for L2,
// single-project conversation sandboxing, and AuditLog recording.
func (h *Handler) ExecuteAIToolInternal(ctx *gin.Context) {
	h.handleAIFacadeTool(ctx, false)
}

func (h *Handler) handleAIFacadeTool(ctx *gin.Context, isPreview bool) {
	ctx.Header("Cache-Control", "no-store")
	if !h.requireAIAgentService(ctx) {
		return
	}
	runID := strings.TrimSpace(ctx.GetHeader(aiRunIDHeader))
	toolCallID := strings.TrimSpace(ctx.GetHeader(aiToolCallIDHeader))
	if runID == "" || toolCallID == "" || len(runID) > 128 || len(toolCallID) > 128 {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_execution_binding_invalid", "AI tool execution binding is incomplete")
		return
	}

	var req aiFacadeToolRequest
	if !bindJSON(ctx, &req) {
		return
	}
	toolName := strings.TrimSpace(req.ToolName)
	if toolName == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_not_available", "toolName is required")
		return
	}
	if len(bytes.TrimSpace(req.Arguments)) == 0 {
		req.Arguments = json.RawMessage(`{}`)
	}

	span := trace.SpanFromContext(ctx.Request.Context())
	span.SetAttributes(
		attribute.String("luna.run.id", runID),
		attribute.String("luna.tool_call.id", toolCallID),
		attribute.String("luna.tool.name", toolName),
	)

	reg := h.host.AIToolRegistry()
	if reg == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable")
		return
	}
	tool, ok := reg.Get(toolName)
	if !ok {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "requested tool is not registered")
		return
	}

	binding, ok := h.resolveAIFacadeToolBinding(ctx, runID, toolCallID)
	if !ok {
		return
	}
	if binding.OperationID != tool.Catalog.OperationID || binding.RunStatus != "running" {
		writeErrorCode(ctx, http.StatusConflict, "ai.tool_execution_binding_invalid", "AI tool call does not match active Run")
		return
	}
	if isPreview {
		if binding.ToolStatus != "proposed" && binding.ToolStatus != "running" && binding.ToolStatus != "awaiting_approval" {
			writeErrorCode(ctx, http.StatusConflict, "ai.tool_execution_binding_invalid", "AI tool call is not eligible for preview")
			return
		}
	} else if binding.ToolStatus != "running" {
		writeErrorCode(ctx, http.StatusConflict, "ai.tool_execution_binding_invalid", "AI tool call is not in running state")
		return
	}

	wireArgsHash, err := aitool.CanonicalJSONHash(req.Arguments)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "tool arguments must be a valid JSON object")
		return
	}
	if binding.ArgumentsHash != "" && wireArgsHash != binding.ArgumentsHash {
		writeErrorCode(ctx, http.StatusConflict, "ai.tool_execution_binding_invalid", "AI tool arguments do not match persisted arguments_hash")
		return
	}

	if binding.OwnerUserID == "" || binding.OwnerUserID != binding.ConversationOwnerID || binding.ActorSessionID == "" {
		writeErrorCode(ctx, http.StatusForbidden, "ai.authorization_changed", "AI conversation ownership is no longer valid")
		return
	}
	user, validActor := h.resolveActiveAIPlatformUser(ctx, binding.OwnerUserID, binding.ActorSessionID)
	if !validActor {
		return
	}
	if !h.aiAccessAllowed(user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "AI assistant is restricted to platform administrators")
		return
	}
	if tool.Catalog.AdminOnly && !authz.IsPlatformAdmin(user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "platform administrator role is required for this tool")
		return
	}
	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "AI tool operation exceeds the user's authorized scopes")
		return
	}

	effectiveArgs, argsMap, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, req.Arguments, binding.ConversationProjectID)
	if sandboxErr != nil {
		writeFacadeToolError(ctx, sandboxErr)
		return
	}

	isProdTarget := h.isTargetProductionStage(ctx, binding.ConversationProjectID, argsMap)
	effectiveTier := resolveEffectiveApprovalTier(tool.Catalog, argsMap, isProdTarget)

	var lockedPayload json.RawMessage
	if !isPreview {
		if !facadeApprovalSatisfied(effectiveTier, tool.Catalog, isProdTarget, binding) {
			writeErrorCode(ctx, http.StatusPreconditionRequired, "ai.approval_required", "AI tool execution requires approval")
			return
		}
		if effectiveTier == aitool.ApprovalTierL2 {
			lockHeader := strings.TrimSpace(ctx.GetHeader(aiPreviewLockHeader))
			expectedClaims := aitool.PreviewLockClaims{
				RunID:                 runID,
				ToolCallID:            toolCallID,
				OperationID:           tool.Catalog.OperationID,
				ArgumentsHash:         wireArgsHash,
				OwnerUserID:           binding.OwnerUserID,
				ConversationProjectID: binding.ConversationProjectID,
			}
			verifiedPayload, lockErr := aitool.VerifyPreviewLock(h.previewLockSecrets(), lockHeader, expectedClaims, time.Now())
			if lockErr != nil {
				if errors.Is(lockErr, aitool.ErrPreviewLockMissing) {
					writeErrorCode(ctx, http.StatusPreconditionRequired, "ai.approval_required", "L2 tool execution requires a valid preview lock")
					return
				}
				if errors.Is(lockErr, aitool.ErrPreviewLockExpired) {
					writeFacadeToolError(ctx, aitool.NewErrorWithHint(
						http.StatusConflict,
						"ai.preview_lock_expired",
						"approval preview lock has expired",
						"Re-invoke the tool to generate a fresh preview and request approval again.",
					))
					return
				}
				writeFacadeToolError(ctx, aitool.NewError(http.StatusConflict, "ai.preview_lock_invalid", lockErr.Error()))
				return
			}
			lockedPayload = verifiedPayload
		}
	}

	requestCtx := h.host.WithAIPlatformActor(ctx.Request.Context(), PlatformActor{
		UserID:    binding.OwnerUserID,
		SessionID: binding.ActorSessionID,
		ProjectID: binding.ConversationProjectID,
	})
	restore := replaceRequestContext(ctx, requestCtx)
	defer restore()

	tc := aitool.NewToolContext(
		ctx,
		h.dbFor(ctx),
		user,
		binding.ActorSessionID,
		binding.ConversationID,
		binding.ConversationProjectID,
		runID,
		toolCallID,
		ctx.GetHeader("Idempotency-Key"),
		binding.InputMode,
		lockedPayload,
	)

	if isPreview {
		if !tool.HasPreview {
			writeFacadeToolError(ctx, aitool.NewError(http.StatusBadRequest, "ai.tool_preview_unsupported", "tool does not support preview"))
			return
		}
		previewRes, previewErr := tool.Preview(tc, effectiveArgs)
		if previewErr != nil {
			span.SetStatus(codes.Error, "ai tool preview failed")
			writeFacadeToolError(ctx, previewErr)
			return
		}
		now := time.Now()
		claims := aitool.PreviewLockClaims{
			RunID:                 runID,
			ToolCallID:            toolCallID,
			OperationID:           tool.Catalog.OperationID,
			ArgumentsHash:         wireArgsHash,
			OwnerUserID:           binding.OwnerUserID,
			ConversationProjectID: binding.ConversationProjectID,
			IssuedAt:              now.Unix(),
			ExpiresAt:             now.Add(aiPreviewLockTTL).Unix(),
		}
		primarySecret := []byte(strings.TrimSpace(h.host.AIAgentCallbackServiceToken()))
		lockToken, signErr := aitool.SignPreviewLock(primarySecret, claims, previewRes.LockedPayload)
		if signErr != nil {
			writeErrorCode(ctx, http.StatusInternalServerError, "ai.preview_lock_sign_failed", "failed to sign preview lock")
			return
		}
		respPayload := gin.H{
			"approvalPreview": previewRes.Preview,
			"previewLock":     lockToken,
		}
		if !isModeAutoApprovable(binding.ConversationApprovalMode, tool.Catalog, isProdTarget) {
			respPayload["requiresManualApproval"] = true
		}
		ctx.JSON(http.StatusOK, respPayload)
		return
	}

	out, execErr := tool.Execute(tc, effectiveArgs)
	if tool.Catalog.ApprovalTier != aitool.ApprovalTierL0 || tool.Catalog.Effect != aitool.EffectRead || slices.Contains(tool.Catalog.RequiredScopes, "web:read") {
		h.auditWithContext(
			binding.OwnerUserID,
			"ai.tool.execute",
			tool.Catalog.OperationID+":"+toolCallID,
			execErr == nil,
			"AI service-bound domain facade operation executed",
			requestCtx,
		)
	}
	if cc := tc.ResponseHeaders.Get("Cache-Control"); cc != "" {
		ctx.Header("Cache-Control", cc)
	}
	if execErr != nil {
		span.SetStatus(codes.Error, "ai tool execution failed")
		writeFacadeToolError(ctx, execErr)
		return
	}
	ctx.JSON(http.StatusOK, out)
}

func (h *Handler) resolveAIFacadeToolBinding(ctx *gin.Context, runID, toolCallID string) (aiFacadeToolBinding, bool) {
	var binding aiFacadeToolBinding
	result := h.dbFor(ctx).Raw(`
		SELECT r.owner_user_id,
		       COALESCE(r.actor_session_id, '') AS actor_session_id,
		       r.conversation_id,
		       r.status AS run_status,
		       c.owner_user_id AS conversation_owner_id,
		       COALESCE(c.project_id, '') AS conversation_project_id,
		       COALESCE(c.approval_mode, 'default') AS conversation_approval_mode,
		       tc.operation_id,
		       tc.status AS tool_status,
		       COALESCE(tc.input_mode, '') AS input_mode,
		       COALESCE(tc.arguments_hash, '') AS arguments_hash,
		       COALESCE(tc.approval_decision, '') AS approval_decision,
		       EXISTS (
		           SELECT 1
		           FROM ai.conversation_tool_approvals AS cta
		           WHERE cta.conversation_id = r.conversation_id
		             AND cta.operation_id = tc.operation_id
		       ) AS remembered_approval
		FROM ai.runs AS r
		JOIN ai.conversations AS c ON c.id = r.conversation_id
		JOIN ai.tool_calls AS tc ON tc.run_id = r.id
		WHERE r.id = ? AND tc.id = ?
	`, runID, toolCallID).Scan(&binding)
	if result.Error != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_execution_unavailable", "AI tool execution binding is unavailable")
		return aiFacadeToolBinding{}, false
	}
	if result.RowsAffected != 1 {
		writeErrorCode(ctx, http.StatusNotFound, "ai.tool_call_not_found", "AI ToolCall was not found")
		return aiFacadeToolBinding{}, false
	}
	return binding, true
}

func (h *Handler) enforceConversationProjectSandbox(
	ctx *gin.Context,
	tool *aitool.RegisteredTool,
	rawArgs json.RawMessage,
	conversationProjectID string,
) (json.RawMessage, map[string]any, *aitool.ToolError) {
	var argsMap map[string]any
	if err := json.Unmarshal(rawArgs, &argsMap); err != nil || argsMap == nil {
		return nil, nil, aitool.BadRequest("ai.tool_arguments_invalid", "arguments must be a JSON object")
	}
	schemaProps, _ := tool.Catalog.InputSchema["properties"].(map[string]any)
	conversationProjectID = strings.TrimSpace(conversationProjectID)
	if conversationProjectID == "" {
		if schemaProps != nil {
			for key := range argsMap {
				if _, ok := schemaProps[key]; !ok {
					return nil, nil, aitool.BadRequest("ai.tool_arguments_invalid", "unknown argument "+key)
				}
			}
		}
		return rawArgs, argsMap, nil
	}

	mismatchErr := func(detail string) *aitool.ToolError {
		err := aitool.Forbidden("ai.project_scope_mismatch", detail)
		err.Hint = "Switch to an unbound global conversation when operating across projects or global platform resources."
		return err
	}

	if tool.Catalog.AdminOnly || tool.Catalog.GlobalSessionOnly {
		return nil, nil, mismatchErr("global or platform administration tool is not allowed in a single-project conversation")
	}

	var convProject model.Project
	if err := h.dbFor(ctx).First(&convProject, "id = ? AND deleted_at IS NULL", conversationProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, mismatchErr("bound conversation project no longer exists")
		}
		return nil, nil, aitool.NewError(http.StatusServiceUnavailable, "project.lookup_unavailable", "project storage is unavailable")
	}

	matchesBoundProject := func(v string) bool {
		trimmed := strings.TrimSpace(v)
		return trimmed != "" && (trimmed == convProject.ID || trimmed == convProject.Identifier)
	}

	hasNonEmptyProjects := false
	seenScope := ""
	for key, rawVal := range argsMap {
		if rawVal == nil {
			continue
		}
		switch strings.ToLower(key) {
		case "visibility":
			if vis, ok := rawVal.(string); ok && strings.EqualFold(strings.TrimSpace(vis), "all") {
				return nil, nil, mismatchErr("visibility=all is not allowed in a single-project conversation")
			}
		case "scope":
			if scopeVal, ok := rawVal.(string); ok {
				seenScope = strings.ToLower(strings.TrimSpace(scopeVal))
				if seenScope == "global" {
					return nil, nil, mismatchErr("global scope resources cannot be mutated from a single-project conversation")
				}
			}
		case "project":
			projStr, ok := rawVal.(string)
			if !ok || (strings.TrimSpace(projStr) != "" && !matchesBoundProject(projStr)) {
				return nil, nil, mismatchErr("requested project does not match the bound conversation project")
			}
		case "projects", "addprojects", "removeprojects":
			arr, ok := rawVal.([]any)
			if !ok {
				return nil, nil, aitool.BadRequest("ai.tool_arguments_invalid", key+" must be an array of strings")
			}
			if strings.EqualFold(key, "projects") && len(arr) == 0 {
				return nil, nil, mismatchErr("projects cannot be empty in a single-project conversation")
			}
			for _, elem := range arr {
				s, ok := elem.(string)
				if !ok || !matchesBoundProject(s) {
					return nil, nil, mismatchErr(key + " contains a project outside the bound conversation project")
				}
			}
			if strings.EqualFold(key, "projects") && len(arr) > 0 {
				hasNonEmptyProjects = true
			}
		case "upsertgrants":
			if grants, ok := rawVal.([]any); ok {
				for _, g := range grants {
					if gMap, ok := g.(map[string]any); ok {
						matchedGrant := false
						for gk, gv := range gMap {
							if strings.EqualFold(gk, "project") {
								p, ok := gv.(string)
								if !ok || !matchesBoundProject(p) {
									return nil, nil, mismatchErr("upsertGrants[].project does not match the bound conversation project")
								}
								matchedGrant = true
							}
						}
						if !matchedGrant {
							return nil, nil, mismatchErr("upsertGrants[].project is required in a single-project conversation")
						}
					}
				}
			}
		}
	}

	if schemaProps != nil {
		if _, declaresProjects := schemaProps["projects"]; declaresProjects {
			if seenScope == "user" && hasNonEmptyProjects {
				return nil, nil, mismatchErr("scope=user cannot carry project bindings")
			}
			if seenScope == "project" && !hasNonEmptyProjects {
				return nil, nil, mismatchErr("scope=project requires projects matching the bound conversation project")
			}
		}
		for key := range argsMap {
			if _, ok := schemaProps[key]; !ok {
				return nil, nil, aitool.BadRequest("ai.tool_arguments_invalid", "unknown argument "+key)
			}
		}
	}

	// Auto-inject conversationProjectID for optional-project L0 read tools when omitted.
	if tool.Catalog.ApprovalTier == aitool.ApprovalTierL0 && !tool.AllowPersonalProjectOmit {
		if schemaProps != nil {
			if _, hasProjectProp := schemaProps["project"]; hasProjectProp && !schemaFieldIsRequired(tool.Catalog.InputSchema, "project") {
				currentProj, _ := argsMap["project"].(string)
				if strings.TrimSpace(currentProj) == "" {
					cloned := make(map[string]any, len(argsMap)+1)
					for k, v := range argsMap {
						cloned[k] = v
					}
					cloned["project"] = convProject.ID
					encoded, err := json.Marshal(cloned)
					if err == nil {
						return encoded, cloned, nil
					}
				}
			}
		}
	}

	encoded, err := json.Marshal(argsMap)
	if err == nil {
		return encoded, argsMap, nil
	}
	return rawArgs, argsMap, nil
}

func schemaFieldIsRequired(schema map[string]any, field string) bool {
	switch req := schema["required"].(type) {
	case []string:
		for _, item := range req {
			if item == field {
				return true
			}
		}
	case []any:
		for _, item := range req {
			if s, ok := item.(string); ok && s == field {
				return true
			}
		}
	}
	return false
}

func (h *Handler) isTargetProductionStage(ctx *gin.Context, conversationProjectID string, argsMap map[string]any) bool {
	if argsContainProductionStage(argsMap) {
		return true
	}
	rawTarget, ok := argsMap["target"].(string)
	if !ok {
		return false
	}
	targetVal := strings.TrimSpace(rawTarget)
	if targetVal == "" {
		return false
	}
	db := h.dbFor(ctx)
	if db == nil {
		return false
	}
	if strings.HasPrefix(targetVal, "dplt_") {
		var stage string
		if err := db.Table("deployment_targets").
			Select("stage").
			Where("id = ? AND deleted_at IS NULL", targetVal).
			Limit(1).
			Scan(&stage).Error; err == nil && isProductionStageValue(stage) {
			return true
		}
		return false
	}
	projectFilter := strings.TrimSpace(conversationProjectID)
	if projectFilter == "" {
		if rawProj, ok := argsMap["project"].(string); ok && strings.TrimSpace(rawProj) != "" {
			var resolvedProjID string
			if err := db.Table("projects").
				Select("id").
				Where("(id = ? OR identifier = ?) AND deleted_at IS NULL", strings.TrimSpace(rawProj), strings.TrimSpace(rawProj)).
				Limit(1).
				Scan(&resolvedProjID).Error; err == nil {
				projectFilter = strings.TrimSpace(resolvedProjID)
			}
		}
	}
	if projectFilter == "" {
		return false
	}
	var stages []string
	if err := db.Table("deployment_targets").
		Select("stage").
		Where("project_id = ? AND deleted_at IS NULL AND (id = ? OR stage = ? OR name = ? OR kubernetes_name = ?)", projectFilter, targetVal, targetVal, targetVal, targetVal).
		Scan(&stages).Error; err == nil {
		for _, stage := range stages {
			if isProductionStageValue(stage) {
				return true
			}
		}
	}
	return false
}

func argsContainProductionStage(argsMap map[string]any) bool {
	for key, rawVal := range argsMap {
		if strings.EqualFold(key, "stage") || strings.EqualFold(key, "target") {
			if val, ok := rawVal.(string); ok && isProductionStageValue(val) {
				return true
			}
		}
		if strings.EqualFold(key, "body") {
			if nested, ok := rawVal.(map[string]any); ok && argsContainProductionStage(nested) {
				return true
			}
		}
	}
	return false
}

func isProductionStageValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "prod" || normalized == "production"
}

func resolveEffectiveApprovalTier(cat aitool.CatalogOperation, argsMap map[string]any, isProdTarget bool) string {
	tier := strings.ToUpper(strings.TrimSpace(cat.ApprovalTier))
	if tier == "" {
		if cat.RequiresApproval {
			tier = aitool.ApprovalTierL2
		} else {
			tier = aitool.ApprovalTierL0
		}
	}
	if tier == aitool.ApprovalTierL1 && (isProdTarget || argsContainProductionStage(argsMap)) {
		return aitool.ApprovalTierL2
	}
	return tier
}

func isModeAutoApprovable(mode string, cat aitool.CatalogOperation, isProdTarget bool) bool {
	switch strings.TrimSpace(mode) {
	case "full_access":
		return true
	case "auto":
		return cat.AutoModeExempt && !isProdTarget
	default:
		return false
	}
}

func facadeApprovalSatisfied(tier string, cat aitool.CatalogOperation, isProdTarget bool, binding aiFacadeToolBinding) bool {
	decision := strings.TrimSpace(binding.ApprovalDecision)
	mode := strings.TrimSpace(binding.ConversationApprovalMode)
	if mode == "" {
		mode = "default"
	}
	if tier == aitool.ApprovalTierL0 {
		return true
	}
	if decision == "approve" {
		return true
	}
	if decision == "mode_full_access" {
		return mode == "full_access"
	}
	if decision == "mode_auto" {
		return (mode == "auto" || mode == "full_access") && cat.AutoModeExempt && !isProdTarget
	}
	if tier == aitool.ApprovalTierL1 && !isProdTarget && mode != "strict" {
		return decision == "approve_and_remember" ||
			(decision == "remembered" && binding.RememberedApproval)
	}
	return false
}

func (h *Handler) previewLockSecrets() [][]byte {
	var secrets [][]byte
	if cur := strings.TrimSpace(h.host.AIAgentCallbackServiceToken()); cur != "" {
		secrets = append(secrets, []byte(cur))
	}
	if prev := strings.TrimSpace(h.host.AIAgentPreviousCallbackServiceToken()); prev != "" {
		secrets = append(secrets, []byte(prev))
	}
	return secrets
}

func writeFacadeToolError(ctx *gin.Context, err error) {
	span := trace.SpanFromContext(ctx.Request.Context())
	if te, ok := aitool.AsToolError(err); ok {
		span.SetAttributes(attribute.String("error.code", te.Code))
		if len(bytes.TrimSpace(te.RawBody)) > 0 {
			ctx.Data(te.Status, "application/json; charset=utf-8", te.RawBody)
			return
		}
		payload := gin.H{
			"code":    te.Code,
			"message": te.Message,
		}
		if te.Hint != "" {
			payload["hint"] = te.Hint
		}
		if te.Retryable != nil {
			payload["retryable"] = *te.Retryable
		}
		if len(te.Candidates) > 0 {
			payload["candidates"] = te.Candidates
		}
		if len(te.Details) > 0 {
			payload["details"] = te.Details
		}
		ctx.JSON(te.Status, payload)
		return
	}
	span.SetAttributes(attribute.String("error.code", "ai.tool_failed"))
	writeErrorCode(ctx, http.StatusInternalServerError, "ai.tool_failed", err.Error())
}
