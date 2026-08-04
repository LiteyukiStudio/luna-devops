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
	ID               string    `json:"id"`
	TurnIndex        int       `json:"turnIndex"`
	Status           string    `json:"status"`
	UserMessage      string    `json:"userMessage"`
	AssistantMessage string    `json:"assistantMessage"`
	RunID            string    `json:"runId"`
	TraceID          string    `json:"traceId"`
	DurationMs       float64   `json:"durationMs"`
	CreatedAt        time.Time `json:"createdAt"`
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
	assistantMessages, err := s.loadAssistantMessages(ctx, runIDs)
	if err != nil {
		return ConversationDetail{}, err
	}
	turns := make([]ConversationTurn, 0, len(rows))
	for _, row := range rows {
		turn := ConversationTurn{ID: row.ID, TurnIndex: row.TurnIndex, Status: row.Status, UserMessage: row.Input, AssistantMessage: assistantMessages[row.RunID], RunID: row.RunID, TraceID: traceIDFromContext(row.TraceContext), CreatedAt: row.CreatedAt}
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

func (s *ConversationStore) loadAssistantMessages(ctx context.Context, runIDs []string) (map[string]string, error) {
	messages := make(map[string]string, len(runIDs))
	if len(runIDs) == 0 {
		return messages, nil
	}
	type itemRow struct {
		RunID   string
		Content []byte
	}
	var rows []itemRow
	if err := s.db.WithContext(ctx).Table("ai.items").Select("run_id, content").
		Where("run_id IN ? AND type = ?", runIDs, "assistant_message").Order("run_id ASC, timeline_index ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		text := messageText(row.Content)
		if text == "" {
			continue
		}
		if messages[row.RunID] != "" {
			messages[row.RunID] += "\n"
		}
		messages[row.RunID] += text
	}
	return messages, nil
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
