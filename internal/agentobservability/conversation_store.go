package agentobservability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrConversationNotFound = errors.New("agent observability conversation not found")

type ConversationUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl"`
}

type ConversationSummary struct {
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	User       ConversationUser `gorm:"embedded;embeddedPrefix:user_" json:"user"`
	TurnCount  int64            `json:"turnCount"`
	TraceCount int64            `json:"traceCount"`
	CreatedAt  time.Time        `json:"createdAt"`
	UpdatedAt  time.Time        `json:"updatedAt"`
}

type ConversationTurn struct {
	ID               string             `json:"id"`
	TurnIndex        int                `json:"turnIndex"`
	Status           string             `json:"status"`
	UserMessage      string             `json:"userMessage"`
	AssistantMessage string             `json:"assistantMessage"`
	RunID            string             `json:"runId"`
	TraceID          string             `json:"traceId"`
	DurationMs       float64            `json:"durationMs"`
	CreatedAt        time.Time          `json:"createdAt"`
	Loops            []ConversationLoop `json:"loops"`
}

type ConversationLoop struct {
	LoopIndex int                   `json:"loopIndex"`
	Items     []ConversationRunItem `json:"items"`
}

type ConversationRunItem struct {
	ID            string                `json:"id"`
	TimelineIndex int                   `json:"timelineIndex"`
	Type          string                `json:"type"`
	Status        string                `json:"status"`
	Text          string                `json:"text"`
	ToolCall      *ConversationToolCall `json:"toolCall,omitempty"`
	CreatedAt     time.Time             `json:"createdAt"`
}

type ConversationToolCall struct {
	ID          string         `json:"id"`
	OperationID string         `json:"operationId"`
	Status      string         `json:"status"`
	Arguments   map[string]any `json:"arguments"`
	Result      any            `json:"result,omitempty"`
	ErrorCode   string         `json:"errorCode,omitempty"`
	DurationMs  float64        `json:"durationMs,omitempty"`
	TraceID     string         `json:"traceId,omitempty"`
}

type ConversationDetail struct {
	ConversationSummary
	Turns          []ConversationTurn `json:"turns"`
	TurnPage       int                `json:"turnPage"`
	TurnPageSize   int                `json:"turnPageSize"`
	TotalTurnPages int                `json:"totalTurnPages"`
}

type ConversationListOptions struct {
	Start     time.Time
	Search    string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

type ConversationListResult struct {
	Items      []ConversationSummary
	Total      int64
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
	TotalPages int
}

type ConversationStore struct{ db *gorm.DB }

func NewConversationStore(db *gorm.DB) *ConversationStore { return &ConversationStore{db: db} }

func (s *ConversationStore) List(ctx context.Context, options ConversationListOptions) (ConversationListResult, error) {
	options = normalizeConversationListOptions(options)
	base := s.db.WithContext(ctx).Table("ai.conversations AS c").
		Joins("LEFT JOIN users AS u ON u.id = c.owner_user_id AND u.deleted_at IS NULL").
		Where("c.updated_at >= ?", options.Start)
	if keyword := strings.TrimSpace(options.Search); keyword != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(keyword))
		pattern := "%" + escaped + "%"
		base = base.Where("LOWER(c.title) LIKE ? ESCAPE '\\' OR LOWER(COALESCE(u.name, '')) LIKE ? ESCAPE '\\' OR LOWER(COALESCE(u.email, '')) LIKE ? ESCAPE '\\'", pattern, pattern, pattern)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return ConversationListResult{}, err
	}
	var items []ConversationSummary
	selectSQL := `c.id, c.title, c.created_at, c.updated_at,
		c.owner_user_id AS user_id, COALESCE(u.name, '') AS user_name,
		COALESCE(u.email, '') AS user_email, COALESCE(u.avatar_url, '') AS user_avatar_url,
		(SELECT COUNT(*) FROM ai.turns t WHERE t.conversation_id = c.id) AS turn_count,
		(SELECT COUNT(*) FROM ai.runs r WHERE r.conversation_id = c.id AND COALESCE(r.trace_context->>'traceparent', '') <> '') AS trace_count`
	if err := base.Select(selectSQL).
		Order(conversationSortClause(options.SortBy, options.SortOrder)).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).
		Scan(&items).Error; err != nil {
		return ConversationListResult{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(options.PageSize) - 1) / int64(options.PageSize))
	}
	return ConversationListResult{Items: items, Total: total, Page: options.Page, PageSize: options.PageSize, SortBy: options.SortBy, SortOrder: options.SortOrder, TotalPages: totalPages}, nil
}

func (s *ConversationStore) Get(ctx context.Context, conversationID string, turnPage, turnPageSize int) (ConversationDetail, error) {
	turnPage, turnPageSize = normalizePage(turnPage, turnPageSize)
	var summary ConversationSummary
	selectSQL := `c.id, c.title, c.created_at, c.updated_at,
		c.owner_user_id AS user_id, COALESCE(u.name, '') AS user_name,
		COALESCE(u.email, '') AS user_email, COALESCE(u.avatar_url, '') AS user_avatar_url,
		(SELECT COUNT(*) FROM ai.turns t WHERE t.conversation_id = c.id) AS turn_count,
		(SELECT COUNT(*) FROM ai.runs r WHERE r.conversation_id = c.id AND COALESCE(r.trace_context->>'traceparent', '') <> '') AS trace_count`
	result := s.db.WithContext(ctx).Table("ai.conversations AS c").Select(selectSQL).
		Joins("LEFT JOIN users AS u ON u.id = c.owner_user_id AND u.deleted_at IS NULL").
		Where("c.id = ?", conversationID).Scan(&summary)
	if result.Error != nil {
		return ConversationDetail{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ConversationDetail{}, ErrConversationNotFound
	}

	type turnRow struct {
		ID           string
		TurnIndex    int
		Status       string
		Input        string
		RunID        string
		TraceContext []byte
		StartedAt    *time.Time
		CompletedAt  *time.Time
		CreatedAt    time.Time
	}
	var rows []turnRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT t.id, t.turn_index, t.status, t.input, t.created_at,
			COALESCE(r.id, '') AS run_id, COALESCE(r.trace_context, '{}'::jsonb) AS trace_context,
			r.started_at, r.completed_at
		FROM ai.turns t
		LEFT JOIN ai.runs r ON r.id = t.selected_run_id
		WHERE t.conversation_id = ?
		ORDER BY t.turn_index ASC
		LIMIT ? OFFSET ?`, conversationID, turnPageSize, (turnPage-1)*turnPageSize).Scan(&rows).Error; err != nil {
		return ConversationDetail{}, err
	}

	runIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.RunID != "" {
			runIDs = append(runIDs, row.RunID)
		}
	}
	runItems, err := s.loadRunItems(ctx, runIDs)
	if err != nil {
		return ConversationDetail{}, err
	}
	turns := make([]ConversationTurn, 0, len(rows))
	for _, row := range rows {
		loops := buildConversationLoops(runItems[row.RunID])
		turn := ConversationTurn{ID: row.ID, TurnIndex: row.TurnIndex, Status: row.Status, UserMessage: row.Input, AssistantMessage: assistantMessageFromLoops(loops), RunID: row.RunID, TraceID: traceIDFromContext(row.TraceContext), CreatedAt: row.CreatedAt, Loops: loops}
		if row.StartedAt != nil && row.CompletedAt != nil {
			turn.DurationMs = float64(row.CompletedAt.Sub(*row.StartedAt).Microseconds()) / 1000
		}
		turns = append(turns, turn)
	}
	totalTurnPages := 0
	if summary.TurnCount > 0 {
		totalTurnPages = int((summary.TurnCount + int64(turnPageSize) - 1) / int64(turnPageSize))
	}
	return ConversationDetail{ConversationSummary: summary, Turns: turns, TurnPage: turnPage, TurnPageSize: turnPageSize, TotalTurnPages: totalTurnPages}, nil
}

func (s *ConversationStore) loadRunItems(ctx context.Context, runIDs []string) (map[string][]ConversationRunItem, error) {
	items := make(map[string][]ConversationRunItem, len(runIDs))
	if len(runIDs) == 0 {
		return items, nil
	}
	type itemRow struct {
		ID            string
		RunID         string
		TimelineIndex int
		Type          string
		Status        string
		Content       []byte
		CreatedAt     time.Time
	}
	var rows []itemRow
	if err := s.db.WithContext(ctx).Table("ai.items").Select("id, run_id, timeline_index, type, status, content, created_at").
		Where("run_id IN ? AND type IN ?", runIDs, []string{"reasoning_summary", "assistant_message", "tool_call"}).Order("run_id ASC, timeline_index ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		item := ConversationRunItem{ID: row.ID, TimelineIndex: row.TimelineIndex, Type: row.Type, Status: row.Status, CreatedAt: row.CreatedAt}
		if row.Type == "tool_call" {
			item.ToolCall = toolCallFromContent(row.ID, row.Status, row.Content)
		} else {
			item.Text = runItemText(row.Type, row.Content)
		}
		items[row.RunID] = append(items[row.RunID], item)
	}
	return items, nil
}

func buildConversationLoops(items []ConversationRunItem) []ConversationLoop {
	loops := make([]ConversationLoop, 0)
	for _, item := range items {
		startsLoop := item.Type == "reasoning_summary"
		if startsLoop || len(loops) == 0 {
			loops = append(loops, ConversationLoop{LoopIndex: len(loops) + 1})
		}
		loops[len(loops)-1].Items = append(loops[len(loops)-1].Items, item)
	}
	return loops
}

func assistantMessageFromLoops(loops []ConversationLoop) string {
	messages := make([]string, 0)
	for _, loop := range loops {
		for _, item := range loop.Items {
			if item.Type == "assistant_message" && strings.TrimSpace(item.Text) != "" {
				messages = append(messages, item.Text)
			}
		}
	}
	return strings.Join(messages, "\n")
}

func runItemText(itemType string, raw []byte) string {
	if itemType == "reasoning_summary" {
		var content struct {
			Summary string `json:"summary"`
		}
		if json.Unmarshal(raw, &content) == nil {
			return content.Summary
		}
	}
	return messageText(raw)
}

func toolCallFromContent(itemID, itemStatus string, raw []byte) *ConversationToolCall {
	var content struct {
		ToolCallID  string         `json:"toolCallId"`
		OperationID string         `json:"operationId"`
		Status      string         `json:"status"`
		Arguments   map[string]any `json:"arguments"`
		Result      any            `json:"result"`
		ErrorCode   string         `json:"errorCode"`
		DurationMs  float64        `json:"durationMs"`
		TraceID     string         `json:"traceId"`
	}
	if json.Unmarshal(raw, &content) != nil {
		return &ConversationToolCall{ID: itemID, Status: itemStatus, Arguments: map[string]any{}}
	}
	status := content.Status
	if status == "" {
		status = itemStatus
	}
	return &ConversationToolCall{
		ID: content.ToolCallID, OperationID: content.OperationID, Status: status,
		Arguments: sanitizeToolObject(content.Arguments), Result: sanitizeToolValue(content.Result, 0),
		ErrorCode: content.ErrorCode, DurationMs: content.DurationMs, TraceID: validTraceID(content.TraceID),
	}
}

func sanitizeToolObject(value map[string]any) map[string]any {
	result, _ := sanitizeToolValue(value, 0).(map[string]any)
	if result == nil {
		return map[string]any{}
	}
	return result
}

func sanitizeToolValue(value any, depth int) any {
	if depth >= 6 {
		return "[TRUNCATED]"
	}
	switch typed := value.(type) {
	case string:
		if len(typed) > 2000 {
			return typed[:2000]
		}
		return typed
	case []any:
		if len(typed) > 50 {
			typed = typed[:50]
		}
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitizeToolValue(item, depth+1))
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		count := 0
		for key, item := range typed {
			if sensitiveToolKey(key) {
				continue
			}
			result[key] = sanitizeToolValue(item, depth+1)
			count++
			if count >= 50 {
				break
			}
		}
		return result
	default:
		return value
	}
}

func sensitiveToolKey(key string) bool {
	lower := strings.ToLower(key)
	for _, fragment := range []string{"authorization", "cookie", "password", "secret", "token", "credential", "api_key", "apikey", "kubeconfig"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func validTraceID(value string) string {
	if len(value) != 32 || value == strings.Repeat("0", 32) {
		return ""
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return ""
		}
	}
	return strings.ToLower(value)
}

func normalizeConversationListOptions(options ConversationListOptions) ConversationListOptions {
	options.Page, options.PageSize = normalizePage(options.Page, options.PageSize)
	if options.SortBy != "title" && options.SortBy != "user" && options.SortBy != "turnCount" {
		options.SortBy = "updatedAt"
	}
	if options.SortOrder != "asc" {
		options.SortOrder = "desc"
	}
	return options
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func conversationSortClause(sortBy, sortOrder string) string {
	columns := map[string]string{"updatedAt": "c.updated_at", "title": "c.title", "user": "u.name", "turnCount": "turn_count"}
	column := columns[sortBy]
	if column == "" {
		column = "c.updated_at"
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return fmt.Sprintf("%s %s", column, sortOrder)
}

func traceIDFromContext(raw []byte) string {
	var values map[string]string
	if json.Unmarshal(raw, &values) != nil {
		return ""
	}
	parts := strings.Split(values["traceparent"], "-")
	if len(parts) != 4 || len(parts[1]) != 32 {
		return ""
	}
	for _, char := range parts[1] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return ""
		}
	}
	return strings.ToLower(parts[1])
}

func messageText(raw []byte) string {
	var content struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if json.Unmarshal(raw, &content) != nil {
		return ""
	}
	parts := make([]string, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}
