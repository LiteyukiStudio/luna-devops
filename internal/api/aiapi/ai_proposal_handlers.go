package aiapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const proposalPreviewLockTTL = 15 * time.Minute

type AIActionProposalPreviewInput struct {
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments"`
}

type AIActionProposalExecuteInput struct {
	ToolName    string          `json:"toolName"`
	Arguments   json.RawMessage `json:"arguments"`
	PreviewLock string          `json:"previewLock"`
}

type AIActionProposalPreviewOutput struct {
	ToolName        string                 `json:"toolName"`
	ApprovalTier    string                 `json:"approvalTier"`
	ApprovalPreview aitool.ApprovalPreview `json:"approvalPreview"`
	PreviewLock     string                 `json:"previewLock"`
}

type AIActionProposalExecuteOutput struct {
	ToolName string `json:"toolName"`
	Result   any    `json:"result"`
}

func isAllowedProjectProposalTool(tool *aitool.RegisteredTool) bool {
	if tool == nil {
		return false
	}
	if tool.Catalog.VerificationOnly || tool.Catalog.AdminOnly || tool.Catalog.GlobalSessionOnly {
		return false
	}
	if tool.Catalog.Effect == aitool.EffectRead || !tool.HasPreview || len(tool.Catalog.SensitivePaths) > 0 {
		return false
	}
	return model.IsAllowedAIRemediationTool(tool.Catalog.OperationID)
}

func bindProposalArgumentsToProject(rawArgs json.RawMessage, projectID string) (json.RawMessage, *aitool.ToolError) {
	trimmed := bytes.TrimSpace(rawArgs)
	if len(trimmed) == 0 {
		trimmed = []byte(`{}`)
	}
	var argsMap map[string]any
	if err := json.Unmarshal(trimmed, &argsMap); err != nil || argsMap == nil {
		return nil, aitool.BadRequest("ai.tool_arguments_invalid", "arguments must be a JSON object")
	}
	if existing, has := argsMap["project"]; !has || existing == nil {
		argsMap["project"] = projectID
	} else if strVal, ok := existing.(string); ok && strings.TrimSpace(strVal) == "" {
		argsMap["project"] = projectID
	}
	encoded, err := json.Marshal(argsMap)
	if err != nil {
		return nil, aitool.BadRequest("ai.tool_arguments_invalid", "arguments must be valid JSON")
	}
	return encoded, nil
}

// PreviewProjectAIActionProposal handles POST /api/v1/projects/:projectId/ai-action-proposals/preview.
// It runs live tool Preview under the current user's own identity & RBAC within the project sandbox
// and mints a short-lived previewLock strictly bound to the current user's UserID and ProjectID.
func (h *Handler) PreviewProjectAIActionProposal(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	span := trace.SpanFromContext(ctx.Request.Context())
	session, hasSession := h.host.CurrentSessionFromCookie(ctx)
	if !hasSession {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required")
		return
	}
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionProjectRead)
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

	var in AIActionProposalPreviewInput
	if !bindJSON(ctx, &in) {
		return
	}
	toolName := strings.TrimSpace(in.ToolName)
	if toolName == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_not_available", "toolName is required")
		return
	}

	reg := h.host.AIToolRegistry()
	if reg == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable")
		return
	}
	tool, exists := reg.Get(toolName)
	if !exists || !isAllowedProjectProposalTool(tool) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "requested tool is not available for project remediation proposals")
		return
	}
	span.SetAttributes(attribute.String("luna.tool.name", tool.Catalog.OperationID))
	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "tool exceeds authorized scopes")
		return
	}

	normalizedRawArgs, bindErr := bindProposalArgumentsToProject(in.Arguments, project.ID)
	if bindErr != nil {
		writeFacadeToolError(ctx, bindErr)
		return
	}
	effectiveArgs, argsMap, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, normalizedRawArgs, project.ID)
	if sandboxErr != nil {
		span.SetStatus(codes.Error, "proposal sandbox rejected")
		writeFacadeToolError(ctx, sandboxErr)
		return
	}
	argsHash, hashErr := aitool.CanonicalJSONHash(effectiveArgs)
	if hashErr != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "arguments must be valid JSON")
		return
	}

	isProd := h.isTargetProductionStage(ctx, project.ID, argsMap)
	effectiveTier := resolveEffectiveApprovalTier(tool.Catalog, argsMap, isProd)
	runBindingID := "proposal:" + project.ID
	callID := id.New("prop")
	actorSessionID := firstNonEmptyString(strings.TrimSpace(session.ID), "proposal:"+user.ID)

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
		h.dbFor(ctx),
		user,
		actorSessionID,
		"proposal",
		project.ID,
		runBindingID,
		callID,
		"proposal:"+callID,
		"proposal",
		nil,
	)

	res, prevErr := tool.Preview(tc, effectiveArgs)
	if prevErr != nil {
		span.SetStatus(codes.Error, "ai proposal preview failed")
		writeFacadeToolError(ctx, prevErr)
		return
	}
	var previewRes aitool.PreviewResult
	if res != nil {
		previewRes = *res
	}

	now := time.Now()
	claims := aitool.PreviewLockClaims{
		RunID:                 runBindingID,
		ToolCallID:            "proposal",
		OperationID:           tool.Catalog.OperationID,
		ArgumentsHash:         argsHash,
		OwnerUserID:           user.ID,
		ConversationProjectID: project.ID,
		IssuedAt:              now.Unix(),
		ExpiresAt:             now.Add(proposalPreviewLockTTL).Unix(),
	}
	lockToken, signErr := aitool.SignPreviewLock(h.host.MCPPreviewSigningSecret(), claims, previewRes.LockedPayload)
	if signErr != nil {
		span.SetStatus(codes.Error, "preview lock sign failed")
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.preview_lock_sign_failed", "failed to sign preview lock")
		return
	}

	ctx.JSON(http.StatusOK, AIActionProposalPreviewOutput{
		ToolName:        tool.Catalog.OperationID,
		ApprovalTier:    effectiveTier,
		ApprovalPreview: previewRes.Preview,
		PreviewLock:     lockToken,
	})
}

// ExecuteProjectAIActionProposal handles POST /api/v1/projects/:projectId/ai-action-proposals/execute.
// It verifies the user- and project-bound previewLock, executes the tool under the current user's live RBAC, and writes AuditLog.
func (h *Handler) ExecuteProjectAIActionProposal(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	span := trace.SpanFromContext(ctx.Request.Context())
	session, hasSession := h.host.CurrentSessionFromCookie(ctx)
	if !hasSession {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required")
		return
	}
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionProjectRead)
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

	var in AIActionProposalExecuteInput
	if !bindJSON(ctx, &in) {
		return
	}
	toolName := strings.TrimSpace(in.ToolName)
	lockToken := strings.TrimSpace(in.PreviewLock)
	if toolName == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_not_available", "toolName is required")
		return
	}
	if lockToken == "" {
		writeErrorCode(ctx, http.StatusPreconditionRequired, "ai.tool_preview_lock_required", "previewLock is required before executing an AI proposal")
		return
	}

	reg := h.host.AIToolRegistry()
	if reg == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable")
		return
	}
	tool, exists := reg.Get(toolName)
	if !exists || !isAllowedProjectProposalTool(tool) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "requested tool is not available for project remediation proposals")
		return
	}
	span.SetAttributes(attribute.String("luna.tool.name", tool.Catalog.OperationID))
	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "tool exceeds authorized scopes")
		return
	}

	normalizedRawArgs, bindErr := bindProposalArgumentsToProject(in.Arguments, project.ID)
	if bindErr != nil {
		writeFacadeToolError(ctx, bindErr)
		return
	}
	effectiveArgs, _, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, normalizedRawArgs, project.ID)
	if sandboxErr != nil {
		span.SetStatus(codes.Error, "proposal sandbox rejected")
		writeFacadeToolError(ctx, sandboxErr)
		return
	}
	argsHash, hashErr := aitool.CanonicalJSONHash(effectiveArgs)
	if hashErr != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "arguments must be valid JSON")
		return
	}

	runBindingID := "proposal:" + project.ID
	lockedPayload, verifyErr := aitool.VerifyPreviewLock(
		[][]byte{h.host.MCPPreviewSigningSecret()},
		lockToken,
		aitool.PreviewLockClaims{
			RunID:                 runBindingID,
			ToolCallID:            "proposal",
			OperationID:           tool.Catalog.OperationID,
			ArgumentsHash:         argsHash,
			OwnerUserID:           user.ID,
			ConversationProjectID: project.ID,
		},
		time.Now(),
	)
	if verifyErr != nil {
		span.SetStatus(codes.Error, "preview lock verification failed")
		writeFacadeToolError(ctx, mapPreviewLockVerifyError(verifyErr))
		return
	}

	callID := id.New("prop")
	actorSessionID := firstNonEmptyString(strings.TrimSpace(session.ID), "proposal:"+user.ID)
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
		h.dbFor(ctx),
		user,
		actorSessionID,
		"proposal",
		project.ID,
		runBindingID,
		callID,
		"proposal:"+callID,
		"proposal",
		lockedPayload,
	)

	out, execErr := tool.Execute(tc, effectiveArgs)
	h.auditWithContext(
		user.ID,
		"ai.proposal.execute."+tool.Catalog.OperationID,
		project.ID,
		execErr == nil,
		callID,
		requestCtx,
	)
	if execErr != nil {
		span.SetStatus(codes.Error, "ai proposal execution failed")
		writeFacadeToolError(ctx, execErr)
		return
	}

	ctx.JSON(http.StatusOK, AIActionProposalExecuteOutput{
		ToolName: tool.Catalog.OperationID,
		Result:   out,
	})
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
