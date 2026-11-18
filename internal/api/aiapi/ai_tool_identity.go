package aiapi

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	aiRunIDHeader      = "X-Luna-AI-Run-ID"
	aiToolCallIDHeader = "X-Luna-AI-Tool-Call-ID"
)

type aiToolExecutionBinding struct {
	OwnerUserID           string `gorm:"column:owner_user_id"`
	ActorSessionID        string `gorm:"column:actor_session_id"`
	ConversationID        string `gorm:"column:conversation_id"`
	RunStatus             string `gorm:"column:run_status"`
	ConversationOwnerID   string `gorm:"column:conversation_owner_id"`
	ConversationProjectID string `gorm:"column:conversation_project_id"`
	OperationID           string `gorm:"column:operation_id"`
	ToolStatus            string `gorm:"column:tool_status"`
	InputMode             string `gorm:"column:input_mode"`
	ApprovalDecision      string `gorm:"column:approval_decision"`
}

// aiToolExecutionIdentityMiddleware turns the Agent's single service identity
// into a short-lived request actor by resolving the immutable Run/ToolCall
// binding from PostgreSQL. The Agent never sends a user ID, session ID, role or
// project grant. Normal platform handlers still perform their existing RBAC and
// resource ownership checks after this middleware.
func (h *Handler) aiToolExecutionIdentityMiddleware() gin.HandlerFunc {
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
		if !h.requireAIAgentService(ctx) {
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
		user, validActor := h.resolveActiveAIPlatformUser(ctx, binding.OwnerUserID, binding.ActorSessionID)
		if !validActor {
			ctx.Abort()
			return
		}
		if !h.aiAccessAllowed(user.Role) {
			writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "AI assistant is restricted to platform administrators")
			ctx.Abort()
			return
		}
		if !aiOperationScopesAllowed(user.Role, operation.RequiredScopes) {
			writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "AI tool operation exceeds the user's authorized scopes")
			ctx.Abort()
			return
		}
		if !aiRequestMatchesConversationProjectWithMode(ctx, operation, binding.ConversationProjectID, binding.InputMode) {
			writeErrorCode(ctx, http.StatusForbidden, "ai.project_scope_mismatch", "AI conversation project scope does not match the requested project")
			ctx.Abort()
			return
		}
		if operation.RequiresApproval && binding.ApprovalDecision != "approve" {
			writeErrorCode(ctx, http.StatusPreconditionRequired, "ai.approval_required", "high-risk AI tool requires approval")
			ctx.Abort()
			return
		}
		requestContext := h.host.WithAIPlatformActor(ctx.Request.Context(), PlatformActor{
			UserID: binding.OwnerUserID, SessionID: binding.ActorSessionID, ProjectID: binding.ConversationProjectID,
		})
		ctx.Request = ctx.Request.WithContext(requestContext)
		ctx.Next()
		if ctx.Request.Method != http.MethodGet {
			success := ctx.Writer.Status() >= http.StatusOK && ctx.Writer.Status() < http.StatusMultipleChoices
			h.auditWithContext(binding.OwnerUserID, "ai.tool.execute", operation.OperationID+":"+toolCallID, success, "AI service-bound platform operation executed", requestContext)
		}
	}
}

func aiOperationScopesAllowed(role string, scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, rawScope := range scopes {
		scope := strings.TrimSpace(rawScope)
		if scope == "" {
			return false
		}
		if scope == "web:read" {
			continue
		}
		normalized := authz.NormalizeOAuthScope(scope)
		if normalized == "" || normalized != scope || !authz.UserCanAuthorizeOAuthScope(role, normalized) {
			return false
		}
	}
	return true
}

func aiRequestMatchesConversationProject(ctx *gin.Context, operation aitool.OpenAPIOperation, conversationProjectID string) bool {
	return aiRequestMatchesConversationProjectWithMode(ctx, operation, conversationProjectID, "")
}

func aiRequestMatchesConversationProjectWithMode(ctx *gin.Context, operation aitool.OpenAPIOperation, conversationProjectID, inputMode string) bool {
	conversationProjectID = strings.TrimSpace(conversationProjectID)
	if conversationProjectID == "" {
		return true
	}

	projectScoped := false
	projectIDs := make([]string, 0, 2)
	for _, parameter := range operation.Parameters {
		isProjectID := parameter.InputName == "projectId" || parameter.WireName == "projectId"
		isProjectIDs := parameter.InputName == "projectIds" || parameter.WireName == "projectIds"
		if !isProjectID && !isProjectIDs {
			continue
		}
		projectScoped = true
		switch parameter.In {
		case "path":
			trimmed := strings.TrimSpace(ctx.Param(parameter.WireName))
			if trimmed == "" {
				return false
			}
			projectIDs = append(projectIDs, trimmed)
		case "query":
			rawValues, exists := ctx.Request.URL.Query()[parameter.WireName]
			if !exists {
				continue
			}
			if len(rawValues) == 0 {
				return false
			}
			for _, value := range rawValues {
				if isProjectIDs {
					parts := strings.Split(value, ",")
					for _, part := range parts {
						trimmed := strings.TrimSpace(part)
						if trimmed == "" {
							return false
						}
						projectIDs = append(projectIDs, trimmed)
					}
					continue
				}
				trimmed := strings.TrimSpace(value)
				if trimmed == "" {
					return false
				}
				projectIDs = append(projectIDs, trimmed)
			}
		}
	}

	properties, _ := operation.InputSchema["properties"].(map[string]any)
	bodySchema, _ := properties["body"].(map[string]any)
	if schemaDeclaresProjectScope(bodySchema) {
		projectScoped = true
		if operation.RequestBody && ctx.Request.Body != nil {
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
				if !collectSchemaScopedProjectIDs(bodySchema, body, &projectIDs) {
					return false
				}
			}
		}
	}

	if !projectScoped {
		return true
	}
	if len(projectIDs) == 0 {
		return strings.EqualFold(strings.TrimSpace(inputMode), "verification") && strings.EqualFold(ctx.Request.Method, http.MethodGet)
	}
	for _, projectID := range projectIDs {
		if projectID != conversationProjectID {
			return false
		}
	}
	return true
}

func schemaDeclaresProjectScope(schema map[string]any) bool {
	if len(schema) == 0 {
		return false
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		for name, rawProp := range properties {
			if name == "projectId" || name == "projectIds" {
				return true
			}
			if propSchema, ok := rawProp.(map[string]any); ok && schemaDeclaresProjectScope(propSchema) {
				return true
			}
		}
	}
	if itemsSchema, ok := schema["items"].(map[string]any); ok && schemaDeclaresProjectScope(itemsSchema) {
		return true
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		for _, branch := range schemaSubschemas(schema[keyword]) {
			if schemaDeclaresProjectScope(branch) {
				return true
			}
		}
	}
	return false
}

func collectSchemaScopedProjectIDs(schema map[string]any, value any, out *[]string) bool {
	if len(schema) == 0 || value == nil {
		return true
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		objectValue, isObject := value.(map[string]any)
		if !isObject {
			return false
		}
		_, declaresProjectID := properties["projectId"]
		_, declaresProjectIDs := properties["projectIds"]
		if rawScopeSchema, declaresScope := properties["scope"]; declaresScope && (declaresProjectID || declaresProjectIDs) {
			scopeCandidate, scopeExists, scopeAmbiguous := lookupJSONObjectField(objectValue, "scope")
			if scopeAmbiguous {
				return false
			}
			if !scopeExists {
				if !schemaScopeOnlyAllowsProject(rawScopeSchema) {
					return false
				}
			} else {
				scopeText, ok := scopeCandidate.(string)
				if !ok {
					return false
				}
				normalizedScope := strings.ToLower(strings.TrimSpace(scopeText))
				expectedProjectScope := "project"
				if scopeSchema, ok := rawScopeSchema.(map[string]any); ok && schemaEnumContains(scopeSchema["enum"], "projects") {
					expectedProjectScope = "projects"
				}
				if normalizedScope != expectedProjectScope {
					return false
				}
			}
		}
		for name, rawProp := range properties {
			switch name {
			case "projectId":
				candidate, exists, ambiguous := lookupJSONObjectField(objectValue, "projectId")
				if ambiguous {
					return false
				}
				if exists {
					text, ok := candidate.(string)
					if !ok {
						return false
					}
					trimmed := strings.TrimSpace(text)
					if trimmed == "" {
						return false
					}
					*out = append(*out, trimmed)
				}
			case "projectIds":
				candidate, exists, ambiguous := lookupJSONObjectField(objectValue, "projectIds")
				if ambiguous {
					return false
				}
				if exists {
					items, ok := candidate.([]any)
					if !ok || len(items) == 0 {
						return false
					}
					for _, elem := range items {
						text, ok := elem.(string)
						if !ok {
							return false
						}
						trimmed := strings.TrimSpace(text)
						if trimmed == "" {
							return false
						}
						*out = append(*out, trimmed)
					}
				}
			default:
				propSchema, ok := rawProp.(map[string]any)
				if !ok || !schemaDeclaresProjectScope(propSchema) {
					continue
				}
				child, exists, ambiguous := lookupJSONObjectField(objectValue, name)
				if ambiguous {
					return false
				}
				if exists {
					if child == nil {
						return false
					}
					if !collectSchemaScopedProjectIDs(propSchema, child, out) {
						return false
					}
				}
			}
		}
	}
	if itemsSchema, ok := schema["items"].(map[string]any); ok && schemaDeclaresProjectScope(itemsSchema) {
		arrayValue, isArray := value.([]any)
		if !isArray {
			return false
		}
		for _, elem := range arrayValue {
			if elem == nil {
				return false
			}
			if !collectSchemaScopedProjectIDs(itemsSchema, elem, out) {
				return false
			}
		}
	}
	for _, branch := range schemaSubschemas(schema["allOf"]) {
		if schemaDeclaresProjectScope(branch) && !collectSchemaScopedProjectIDs(branch, value, out) {
			return false
		}
	}
	for _, keyword := range []string{"oneOf", "anyOf"} {
		for _, branch := range schemaSubschemas(schema[keyword]) {
			if !schemaDeclaresProjectScope(branch) || !schemaMatchesValueKind(branch, value) {
				continue
			}
			if !collectSchemaScopedProjectIDs(branch, value, out) {
				return false
			}
		}
	}
	return true
}

func lookupJSONObjectField(objectValue map[string]any, target string) (any, bool, bool) {
	var matched any
	count := 0
	for key, val := range objectValue {
		if strings.EqualFold(key, target) {
			matched = val
			count++
			if count > 1 {
				return nil, true, true
			}
		}
	}
	return matched, count == 1, false
}

func schemaScopeOnlyAllowsProject(raw any) bool {
	schema, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	switch values := schema["enum"].(type) {
	case []string:
		if len(values) == 0 {
			return false
		}
		for _, item := range values {
			normalized := strings.ToLower(strings.TrimSpace(item))
			if normalized != "project" && normalized != "projects" {
				return false
			}
		}
		return true
	case []any:
		if len(values) == 0 {
			return false
		}
		for _, item := range values {
			text, ok := item.(string)
			if !ok {
				return false
			}
			normalized := strings.ToLower(strings.TrimSpace(text))
			if normalized != "project" && normalized != "projects" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func schemaEnumContains(rawEnum any, target string) bool {
	switch values := rawEnum.(type) {
	case []string:
		for _, item := range values {
			if strings.EqualFold(strings.TrimSpace(item), target) {
				return true
			}
		}
	case []any:
		for _, item := range values {
			if text, ok := item.(string); ok && strings.EqualFold(strings.TrimSpace(text), target) {
				return true
			}
		}
	}
	return false
}

func schemaMatchesValueKind(schema map[string]any, value any) bool {
	if _, hasProps := schema["properties"].(map[string]any); hasProps {
		_, isObject := value.(map[string]any)
		return isObject
	}
	if _, hasItems := schema["items"].(map[string]any); hasItems {
		_, isArray := value.([]any)
		return isArray
	}
	return true
}

func schemaSubschemas(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, entry := range typed {
			if branch, ok := entry.(map[string]any); ok {
				result = append(result, branch)
			}
		}
		return result
	default:
		return nil
	}
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

func (h *Handler) resolveAIToolExecutionBinding(ctx *gin.Context, runID, toolCallID string) (aiToolExecutionBinding, bool) {
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
		       COALESCE(tc.input_mode, '') AS input_mode,
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

func constantTimeTokenEqual(expected, actual string) bool {
	return expected != "" && len(expected) == len(actual) && subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func (h *Handler) requireAIAgentService(ctx *gin.Context) bool {
	actual := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	expected := strings.TrimSpace(h.host.AIAgentCallbackServiceToken())
	previous := strings.TrimSpace(h.host.AIAgentPreviousCallbackServiceToken())
	if expected == "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.agent_service_not_configured", "Agent service identity is not configured")
		return false
	}
	if !constantTimeTokenEqual(expected, actual) && !constantTimeTokenEqual(previous, actual) {
		writeErrorCode(ctx, http.StatusUnauthorized, "ai.agent_service_unauthorized", "Agent service identity is invalid")
		return false
	}
	return true
}

func (h *Handler) resolveActiveAIPlatformUser(ctx *gin.Context, userID, sessionID string) (model.User, bool) {
	now := time.Now()
	var user model.User
	if h.dbFor(ctx).First(&user, "id = ? and disabled = ?", userID, false).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}
	var session model.UserSession
	if h.dbFor(ctx).First(&session, "id = ? and user_id = ? and expires_at > ?", sessionID, userID, now).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, false
	}
	h.host.SetCurrentUser(ctx, user)
	return user, true
}

func (h *Handler) currentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	actor, ok := h.host.AIPlatformActor(ctx.Request.Context())
	if !ok || actor.UserID == "" || actor.SessionID == "" {
		return model.User{}, false
	}
	user, _ := h.resolveActiveAIPlatformUser(ctx, actor.UserID, actor.SessionID)
	return user, true
}
