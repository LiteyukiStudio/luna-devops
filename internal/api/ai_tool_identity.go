package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	aiRunIDHeader      = "X-Luna-AI-Run-ID"
	aiToolCallIDHeader = "X-Luna-AI-Tool-Call-ID"
)

type aiPlatformActorContextKey struct{}

type aiPlatformActor struct {
	UserID    string
	SessionID string
	ProjectID string
}

type aiToolExecutionBinding struct {
	OwnerUserID           string `gorm:"column:owner_user_id"`
	ActorSessionID        string `gorm:"column:actor_session_id"`
	ConversationID        string `gorm:"column:conversation_id"`
	RunStatus             string `gorm:"column:run_status"`
	ConversationOwnerID   string `gorm:"column:conversation_owner_id"`
	ConversationProjectID string `gorm:"column:conversation_project_id"`
	OperationID           string `gorm:"column:operation_id"`
	ToolStatus            string `gorm:"column:tool_status"`
	ApprovalDecision      string `gorm:"column:approval_decision"`
}

// aiToolExecutionIdentityMiddleware turns the Agent's single service identity
// into a short-lived request actor by resolving the immutable Run/ToolCall
// binding from PostgreSQL. The Agent never sends a user ID, session ID, role or
// project grant. Normal platform handlers still perform their existing RBAC and
// resource ownership checks after this middleware.
func (h *Handlers) aiToolExecutionIdentityMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		runID := strings.TrimSpace(ctx.GetHeader(aiRunIDHeader))
		toolCallID := strings.TrimSpace(ctx.GetHeader(aiToolCallIDHeader))
		if runID == "" && toolCallID == "" {
			ctx.Next()
			return
		}
		if runID == "" || toolCallID == "" || len(runID) > 128 || len(toolCallID) > 128 {
			writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_execution_binding_invalid", "AI tool execution binding is incomplete")
			ctx.Abort()
			return
		}
		if !requireAIAgentService(ctx) {
			ctx.Abort()
			return
		}
		operation, ok := aiOperationForRequest(ctx)
		if !ok {
			writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "request is not an Agent-eligible platform operation")
			ctx.Abort()
			return
		}
		binding, ok := h.resolveAIToolExecutionBinding(ctx, runID, toolCallID)
		if !ok {
			return
		}
		if binding.OperationID != operation.OperationID || binding.RunStatus != "running" || binding.ToolStatus != "running" {
			writeErrorCode(ctx, http.StatusConflict, "ai.tool_execution_binding_invalid", "AI tool execution does not match the active Run and ToolCall")
			ctx.Abort()
			return
		}
		if binding.OwnerUserID == "" || binding.OwnerUserID != binding.ConversationOwnerID || binding.ActorSessionID == "" {
			writeErrorCode(ctx, http.StatusForbidden, "ai.authorization_changed", "AI conversation ownership is no longer valid")
			ctx.Abort()
			return
		}
		if !aiRequestMatchesConversationProject(ctx, operation, binding.ConversationProjectID) {
			writeErrorCode(ctx, http.StatusForbidden, "ai.project_scope_mismatch", "AI conversation project scope does not match the requested project")
			ctx.Abort()
			return
		}
		if operation.RequiresApproval && binding.ApprovalDecision != "approve" && binding.ApprovalDecision != "approve_always" &&
			!h.hasAIToolApprovalExemption(ctx, binding.OwnerUserID, operation.OperationID) {
			writeErrorCode(ctx, http.StatusPreconditionRequired, "ai.approval_required", "high-risk AI tool requires approval")
			ctx.Abort()
			return
		}
		requestContext := context.WithValue(ctx.Request.Context(), aiPlatformActorContextKey{}, aiPlatformActor{
			UserID: binding.OwnerUserID, SessionID: binding.ActorSessionID, ProjectID: binding.ConversationProjectID,
		})
		ctx.Request = ctx.Request.WithContext(requestContext)
		ctx.Next()
		success := ctx.Writer.Status() >= http.StatusOK && ctx.Writer.Status() < http.StatusMultipleChoices
		h.auditWithContext(binding.OwnerUserID, "ai.tool.execute", operation.OperationID+":"+toolCallID, success, "AI service-bound platform operation executed", requestContext)
	}
}

func aiRequestMatchesConversationProject(ctx *gin.Context, operation aitool.OpenAPIOperation, conversationProjectID string) bool {
	conversationProjectID = strings.TrimSpace(conversationProjectID)
	if conversationProjectID == "" {
		return true
	}

	projectScoped := false
	projectIDs := make([]string, 0, 2)
	for _, parameter := range operation.Parameters {
		if parameter.InputName != "projectId" && parameter.WireName != "projectId" {
			continue
		}
		projectScoped = true
		switch parameter.In {
		case "path":
			projectIDs = appendNonEmptyProjectID(projectIDs, ctx.Param(parameter.WireName))
		case "query":
			for _, value := range ctx.QueryArray(parameter.WireName) {
				projectIDs = appendNonEmptyProjectID(projectIDs, value)
			}
		}
	}

	properties, _ := operation.InputSchema["properties"].(map[string]any)
	if _, exists := properties["projectId"]; exists {
		projectScoped = true
	}
	if _, exists := properties["projectIds"]; exists {
		projectScoped = true
	}
	if projectScoped && operation.RequestBody && ctx.Request.Body != nil {
		const maxToolJSONBody = 8 << 20
		raw, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxToolJSONBody+1))
		ctx.Request.Body = io.NopCloser(bytes.NewReader(raw))
		if err != nil || len(raw) > maxToolJSONBody {
			return false
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			var body any
			if json.Unmarshal(raw, &body) != nil {
				return false
			}
			projectIDs = collectRequestProjectIDs(projectIDs, body)
		}
	}

	if !projectScoped {
		return true
	}
	if len(projectIDs) == 0 {
		return false
	}
	for _, projectID := range projectIDs {
		if projectID != conversationProjectID {
			return false
		}
	}
	return true
}

func collectRequestProjectIDs(projectIDs []string, value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "projectId":
				if projectID, ok := child.(string); ok {
					projectIDs = appendNonEmptyProjectID(projectIDs, projectID)
				}
			case "projectIds":
				if values, ok := child.([]any); ok {
					for _, candidate := range values {
						if projectID, ok := candidate.(string); ok {
							projectIDs = appendNonEmptyProjectID(projectIDs, projectID)
						}
					}
				}
			default:
				projectIDs = collectRequestProjectIDs(projectIDs, child)
			}
		}
	case []any:
		for _, child := range typed {
			projectIDs = collectRequestProjectIDs(projectIDs, child)
		}
	}
	return projectIDs
}

func appendNonEmptyProjectID(projectIDs []string, value string) []string {
	if value = strings.TrimSpace(value); value != "" {
		return append(projectIDs, value)
	}
	return projectIDs
}

func aiConversationProjectID(ctx *gin.Context) string {
	actor, _ := ctx.Request.Context().Value(aiPlatformActorContextKey{}).(aiPlatformActor)
	return actor.ProjectID
}

func aiOperationForRequest(ctx *gin.Context) (aitool.OpenAPIOperation, bool) {
	operations, err := aitool.PlatformCatalog()
	if err != nil {
		return aitool.OpenAPIOperation{}, false
	}
	for _, operation := range operations {
		if operation.Method == ctx.Request.Method && openAPIPathToGin(operation.Path) == ctx.FullPath() {
			return operation, true
		}
	}
	return aitool.OpenAPIOperation{}, false
}

func openAPIPathToGin(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[index] = ":" + strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
		}
	}
	return strings.Join(parts, "/")
}

func (h *Handlers) resolveAIToolExecutionBinding(ctx *gin.Context, runID, toolCallID string) (aiToolExecutionBinding, bool) {
	var binding aiToolExecutionBinding
	result := h.dbFor(ctx).Raw(`
		SELECT r.owner_user_id,
		       COALESCE(r.actor_session_id, '') AS actor_session_id,
		       r.conversation_id,
		       r.status AS run_status,
		       c.owner_user_id AS conversation_owner_id,
		       COALESCE(c.project_id, '') AS conversation_project_id,
		       tc.operation_id,
		       tc.status AS tool_status,
		       COALESCE(tc.approval_decision, '') AS approval_decision
		FROM ai.runs AS r
		JOIN ai.conversations AS c ON c.id = r.conversation_id
		JOIN ai.tool_calls AS tc ON tc.run_id = r.id
		WHERE r.id = ? AND tc.id = ?
	`, runID, toolCallID).Scan(&binding)
	if result.Error != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_execution_unavailable", "AI tool execution binding is unavailable")
		ctx.Abort()
		return aiToolExecutionBinding{}, false
	}
	if result.RowsAffected != 1 {
		writeErrorCode(ctx, http.StatusNotFound, "ai.tool_call_not_found", "AI ToolCall was not found")
		ctx.Abort()
		return aiToolExecutionBinding{}, false
	}
	return binding, true
}

func (h *Handlers) hasAIToolApprovalExemption(ctx *gin.Context, userID, operationID string) bool {
	var count int64
	return h.dbFor(ctx).Raw(`
		SELECT count(*) FROM ai.tool_approval_exemptions
		WHERE user_id = ? AND operation_id = ?
	`, userID, operationID).Scan(&count).Error == nil && count == 1
}

func requireAIAgentService(ctx *gin.Context) bool {
	actual := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	internalKeys, err := aiagent.LoadInternalKeys()
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.agent_service_not_configured", "Agent service identity is not configured")
		return false
	}
	expected := internalKeys.CallbackServiceToken
	if expected == "" || len(expected) != len(actual) || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		writeErrorCode(ctx, http.StatusUnauthorized, "ai.agent_service_unauthorized", "Agent service identity is invalid")
		return false
	}
	return true
}

func (h *Handlers) currentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	actor, ok := ctx.Request.Context().Value(aiPlatformActorContextKey{}).(aiPlatformActor)
	if !ok || actor.UserID == "" || actor.SessionID == "" {
		return model.User{}, false
	}
	now := time.Now()
	var user model.User
	if h.dbFor(ctx).First(&user, "id = ? and disabled = ?", actor.UserID, false).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, true
	}
	var session model.UserSession
	if h.dbFor(ctx).First(&session, "id = ? and user_id = ? and expires_at > ?", actor.SessionID, actor.UserID, now).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, true
	}
	ctx.Set(currentUserContextKey, user)
	return user, true
}
