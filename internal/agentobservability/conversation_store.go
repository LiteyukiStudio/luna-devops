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

type TurnSummary struct {
	ID                string           `json:"id"`
	ConversationID    string           `json:"conversationId"`
	ConversationTitle string           `json:"conversationTitle"`
	User              ConversationUser `gorm:"embedded;embeddedPrefix:user_" json:"user"`
	TurnIndex         int              `json:"turnIndex"`
	Status            string           `json:"status"`
	UserMessage       string           `json:"userMessage"`
	AssistantMessage  string           `json:"assistantMessage"`
	RunID             string           `json:"runId"`
	TraceID           string           `json:"traceId"`
	InputTokens       int64            `json:"inputTokens"`
	OutputTokens      int64            `json:"outputTokens"`
	ToolCallCount     int64            `json:"toolCallCount"`
	DurationMs        float64          `json:"durationMs"`
	CreatedAt         time.Time        `json:"createdAt"`
}

type TurnPeriodSummary struct {
	Total       int64
	SuccessRate float64
}

type ToolPeriodSummary struct {
	Total       int64
	Successful  int64
	Failed      int64
	SuccessRate float64
}

type ToolSummary struct {
	OperationID    string    `json:"operationId"`
	TotalCalls     int64     `json:"totalCalls"`
	SucceededCalls int64     `json:"succeededCalls"`
	FailedCalls    int64     `json:"failedCalls"`
	OtherCalls     int64     `json:"otherCalls"`
	SuccessRate    float64   `json:"successRate"`
	LastCalledAt   time.Time `json:"lastCalledAt"`
}

type ToolCall struct {
	ID                string           `json:"id"`
	OperationID       string           `json:"operationId"`
	Status            string           `json:"status"`
	Arguments         map[string]any   `json:"arguments"`
	Result            any              `json:"result,omitempty"`
	ErrorCode         string           `json:"errorCode,omitempty"`
	DurationMs        float64          `json:"durationMs"`
	TraceID           string           `json:"traceId,omitempty"`
	RunID             string           `json:"runId"`
	TurnID            string           `json:"turnId"`
	TurnIndex         int              `json:"turnIndex"`
	ConversationID    string           `json:"conversationId"`
	ConversationTitle string           `json:"conversationTitle"`
	User              ConversationUser `gorm:"embedded;embeddedPrefix:user_" json:"user"`
	CreatedAt         time.Time        `json:"createdAt"`
}

type TraceContext struct {
	Conversation ConversationSummary `json:"conversation"`
	Turn         ConversationTurn    `json:"turn"`
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

type TurnListResult struct {
	Items      []TurnSummary
	Total      int64
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
	TotalPages int
}

type ToolSummaryListOptions struct {
	Start     time.Time
	Search    string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

type ToolSummaryListResult struct {
	Items      []ToolSummary
	Total      int64
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
	TotalPages int
}

type ToolCallListOptions struct {
	Start       time.Time
	OperationID string
	Page        int
	PageSize    int
	SortBy      string
	SortOrder   string
}

type ToolCallListResult struct {
	Items      []ToolCall
	Total      int64
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
	TotalPages int
}

type ConversationStore struct{ db *gorm.DB }

func NewConversationStore(db *gorm.DB) *ConversationStore { return &ConversationStore{db: db} }

func (s *ConversationStore) SummarizeTurns(ctx context.Context, start time.Time) (TurnPeriodSummary, error) {
	var row struct {
		Total      int64
		Successful int64
		Terminal   int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'completed') AS successful,
			COUNT(*) FILTER (WHERE status IN ('completed', 'failed', 'canceled', 'expired')) AS terminal
		FROM ai.turns
		WHERE created_at >= ?`, start).Scan(&row).Error; err != nil {
		return TurnPeriodSummary{}, err
	}
	return summarizeTurnPeriod(row.Total, row.Successful, row.Terminal), nil
}

func summarizeTurnPeriod(total, successful, terminal int64) TurnPeriodSummary {
	result := TurnPeriodSummary{Total: total}
	if terminal > 0 {
		result.SuccessRate = float64(successful) * 100 / float64(terminal)
	}
	return result
}

func (s *ConversationStore) SummarizeTools(ctx context.Context, start time.Time) (ToolPeriodSummary, error) {
	var row struct {
		Total      int64
		Successful int64
		Failed     int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) AS total,
			COUNT(*) FILTER (WHERE COALESCE(NULLIF(content->>'status', ''), CASE status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE status END) = 'succeeded') AS successful,
			COUNT(*) FILTER (WHERE COALESCE(NULLIF(content->>'status', ''), CASE status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE status END) = 'failed') AS failed
		FROM ai.items
		WHERE type = 'tool_call' AND created_at >= ?`, start).Scan(&row).Error; err != nil {
		return ToolPeriodSummary{}, err
	}
	return summarizeToolPeriod(row.Total, row.Successful, row.Failed), nil
}

func summarizeToolPeriod(total, successful, failed int64) ToolPeriodSummary {
	result := ToolPeriodSummary{Total: total, Successful: successful, Failed: failed}
	if terminal := successful + failed; terminal > 0 {
		result.SuccessRate = float64(successful) * 100 / float64(terminal)
	}
	return result
}

func (s *ConversationStore) ListToolSummaries(ctx context.Context, options ToolSummaryListOptions) (ToolSummaryListResult, error) {
	options = normalizeToolSummaryListOptions(options)
	base := s.db.WithContext(ctx).Table("ai.items AS item").
		Where("item.type = 'tool_call' AND item.created_at >= ? AND COALESCE(item.content->>'operationId', '') <> ''", options.Start)
	if keyword := strings.TrimSpace(options.Search); keyword != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(keyword))
		base = base.Where(`LOWER(item.content->>'operationId') LIKE ? ESCAPE '\'`, "%"+escaped+"%")
	}
	var total int64
	if err := base.Select("COUNT(DISTINCT item.content->>'operationId')").Scan(&total).Error; err != nil {
		return ToolSummaryListResult{}, err
	}
	options.Page = pageWithinTotal(options.Page, options.PageSize, total)
	items := make([]ToolSummary, 0)
	if err := base.Select(`item.content->>'operationId' AS operation_id,
		COUNT(*) AS total_calls,
		COUNT(*) FILTER (WHERE COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) = 'succeeded') AS succeeded_calls,
		COUNT(*) FILTER (WHERE COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) = 'failed') AS failed_calls,
		COUNT(*) FILTER (WHERE COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) NOT IN ('succeeded', 'failed')) AS other_calls,
		CASE WHEN COUNT(*) FILTER (WHERE COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) IN ('succeeded', 'failed')) > 0
			THEN COUNT(*) FILTER (WHERE COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) = 'succeeded') * 100.0 /
				COUNT(*) FILTER (WHERE COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) IN ('succeeded', 'failed'))
			ELSE 0 END AS success_rate,
		MAX(item.created_at) AS last_called_at`).
		Group("item.content->>'operationId'").Order(toolSummarySortClause(options.SortBy, options.SortOrder)).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).Scan(&items).Error; err != nil {
		return ToolSummaryListResult{}, err
	}
	return ToolSummaryListResult{
		Items: items, Total: total, Page: options.Page, PageSize: options.PageSize,
		SortBy: options.SortBy, SortOrder: options.SortOrder, TotalPages: pageCount(total, options.PageSize),
	}, nil
}

func (s *ConversationStore) ListToolCalls(ctx context.Context, options ToolCallListOptions) (ToolCallListResult, error) {
	options = normalizeToolCallListOptions(options)
	base := s.db.WithContext(ctx).Table("ai.items AS item").
		Joins("JOIN ai.runs AS r ON r.id = item.run_id").
		Joins("JOIN ai.turns AS t ON t.id = r.turn_id").
		Joins("JOIN ai.conversations AS c ON c.id = t.conversation_id").
		Joins("LEFT JOIN users AS u ON u.id = c.owner_user_id AND u.deleted_at IS NULL").
		Where("item.type = 'tool_call' AND item.created_at >= ? AND item.content->>'operationId' = ?", options.Start, options.OperationID)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return ToolCallListResult{}, err
	}
	options.Page = pageWithinTotal(options.Page, options.PageSize, total)
	type toolCallRow struct {
		ID                string
		OperationID       string
		Status            string
		Arguments         []byte
		Result            []byte
		ErrorCode         string
		DurationMs        float64
		ItemTraceID       string
		TraceContext      []byte
		RunID             string
		TurnID            string
		TurnIndex         int
		ConversationID    string
		ConversationTitle string
		UserID            string
		UserName          string
		UserEmail         string
		UserAvatarURL     string
		CreatedAt         time.Time
	}
	var rows []toolCallRow
	selectSQL := `COALESCE(NULLIF(item.content->>'toolCallId', ''), item.id) AS id,
		item.content->>'operationId' AS operation_id,
		COALESCE(NULLIF(item.content->>'status', ''), CASE item.status WHEN 'failed' THEN 'failed' WHEN 'completed' THEN 'succeeded' ELSE item.status END) AS status,
		COALESCE(item.content->'arguments', '{}'::jsonb) AS arguments, item.content->'result' AS result,
		COALESCE(item.content->>'errorCode', '') AS error_code,
		CASE WHEN COALESCE(item.content->>'durationMs', '') ~ '^[0-9]+([.][0-9]+)?$'
			THEN (item.content->>'durationMs')::double precision ELSE 0 END AS duration_ms,
		COALESCE(item.content->>'traceId', '') AS item_trace_id,
		COALESCE(r.trace_context, '{}'::jsonb) AS trace_context,
		r.id AS run_id, t.id AS turn_id, t.turn_index, c.id AS conversation_id,
		c.title AS conversation_title, c.owner_user_id AS user_id,
		COALESCE(u.name, '') AS user_name, COALESCE(u.email, '') AS user_email,
		COALESCE(u.avatar_url, '') AS user_avatar_url, item.created_at`
	if err := base.Select(selectSQL).Order(toolCallSortClause(options.SortBy, options.SortOrder)).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).Scan(&rows).Error; err != nil {
		return ToolCallListResult{}, err
	}
	items := make([]ToolCall, 0, len(rows))
	for _, row := range rows {
		traceID := validTraceID(row.ItemTraceID)
		if traceID == "" {
			traceID = traceIDFromContext(row.TraceContext)
		}
		items = append(items, ToolCall{
			ID: row.ID, OperationID: row.OperationID, Status: row.Status,
			Arguments: decodeSanitizedToolObject(row.Arguments), Result: decodeSanitizedToolValue(row.Result),
			ErrorCode: row.ErrorCode, DurationMs: row.DurationMs, TraceID: traceID,
			RunID: row.RunID, TurnID: row.TurnID, TurnIndex: row.TurnIndex,
			ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
			User:      ConversationUser{ID: row.UserID, Name: row.UserName, Email: row.UserEmail, AvatarURL: row.UserAvatarURL},
			CreatedAt: row.CreatedAt,
		})
	}
	return ToolCallListResult{
		Items: items, Total: total, Page: options.Page, PageSize: options.PageSize,
		SortBy: options.SortBy, SortOrder: options.SortOrder, TotalPages: pageCount(total, options.PageSize),
	}, nil
}

func (s *ConversationStore) FindTraceContext(ctx context.Context, traceID string) (*TraceContext, error) {
	traceID = validTraceID(traceID)
	if traceID == "" {
		return nil, ErrConversationNotFound
	}
	type traceRow struct {
		ConversationID        string
		ConversationTitle     string
		ConversationCreatedAt time.Time
		ConversationUpdatedAt time.Time
		UserID                string
		UserName              string
		UserEmail             string
		UserAvatarURL         string
		TurnCount             int64
		TraceCount            int64
		TurnID                string
		TurnIndex             int
		TurnStatus            string
		TurnInput             string
		TurnCreatedAt         time.Time
		RunID                 string
		TraceContext          []byte
		StartedAt             *time.Time
		CompletedAt           *time.Time
	}
	var row traceRow
	result := s.db.WithContext(ctx).Raw(`
		SELECT c.id AS conversation_id, c.title AS conversation_title,
			c.created_at AS conversation_created_at, c.updated_at AS conversation_updated_at,
			c.owner_user_id AS user_id, COALESCE(u.name, '') AS user_name,
			COALESCE(u.email, '') AS user_email, COALESCE(u.avatar_url, '') AS user_avatar_url,
			(SELECT COUNT(*) FROM ai.turns counted_turn WHERE counted_turn.conversation_id = c.id) AS turn_count,
			(SELECT COUNT(*) FROM ai.runs counted_run WHERE counted_run.conversation_id = c.id AND COALESCE(counted_run.trace_context->>'traceparent', '') <> '') AS trace_count,
			t.id AS turn_id, t.turn_index, t.status AS turn_status, t.input AS turn_input,
			t.created_at AS turn_created_at, r.id AS run_id, r.trace_context, r.started_at, r.completed_at
		FROM ai.runs r
		JOIN ai.turns t ON t.id = r.turn_id
		JOIN ai.conversations c ON c.id = r.conversation_id
		LEFT JOIN users u ON u.id = c.owner_user_id AND u.deleted_at IS NULL
		WHERE LOWER(SPLIT_PART(COALESCE(r.trace_context->>'traceparent', ''), '-', 2)) = ?
			OR EXISTS (
				SELECT 1 FROM ai.items item
				WHERE item.run_id = r.id AND item.type = 'tool_call'
					AND LOWER(COALESCE(item.content->>'traceId', '')) = ?
			)
		ORDER BY r.created_at DESC
		LIMIT 1`, traceID, traceID).Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrConversationNotFound
	}
	runItems, err := s.loadRunItems(ctx, []string{row.RunID})
	if err != nil {
		return nil, err
	}
	loops := buildConversationLoops(runItems[row.RunID])
	turn := ConversationTurn{
		ID: row.TurnID, TurnIndex: row.TurnIndex, Status: row.TurnStatus, UserMessage: row.TurnInput,
		AssistantMessage: assistantMessageFromLoops(loops), RunID: row.RunID,
		TraceID: traceIDFromContext(row.TraceContext), CreatedAt: row.TurnCreatedAt, Loops: loops,
	}
	if row.StartedAt != nil && row.CompletedAt != nil {
		turn.DurationMs = float64(row.CompletedAt.Sub(*row.StartedAt).Microseconds()) / 1000
	}
	return &TraceContext{
		Conversation: ConversationSummary{
			ID: row.ConversationID, Title: row.ConversationTitle,
			User:      ConversationUser{ID: row.UserID, Name: row.UserName, Email: row.UserEmail, AvatarURL: row.UserAvatarURL},
			TurnCount: row.TurnCount, TraceCount: row.TraceCount,
			CreatedAt: row.ConversationCreatedAt, UpdatedAt: row.ConversationUpdatedAt,
		},
		Turn: turn,
	}, nil
}

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
	options.Page = pageWithinTotal(options.Page, options.PageSize, total)
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
	totalPages := pageCount(total, options.PageSize)
	return ConversationListResult{Items: items, Total: total, Page: options.Page, PageSize: options.PageSize, SortBy: options.SortBy, SortOrder: options.SortOrder, TotalPages: totalPages}, nil
}

func (s *ConversationStore) ListTurns(ctx context.Context, options ConversationListOptions) (TurnListResult, error) {
	options = normalizeTurnListOptions(options)
	base := s.db.WithContext(ctx).Table("ai.turns AS t").
		Joins("JOIN ai.conversations AS c ON c.id = t.conversation_id").
		Joins("LEFT JOIN users AS u ON u.id = c.owner_user_id AND u.deleted_at IS NULL").
		Joins("LEFT JOIN ai.runs AS r ON r.id = t.selected_run_id").
		Where("t.created_at >= ?", options.Start)
	if keyword := strings.TrimSpace(options.Search); keyword != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(keyword))
		pattern := "%" + escaped + "%"
		base = base.Where(`LOWER(c.title) LIKE ? ESCAPE '\' OR LOWER(COALESCE(u.name, '')) LIKE ? ESCAPE '\'
			OR LOWER(COALESCE(u.email, '')) LIKE ? ESCAPE '\' OR LOWER(t.input) LIKE ? ESCAPE '\'
			OR LOWER(t.status) LIKE ? ESCAPE '\' OR LOWER(t.id) LIKE ? ESCAPE '\'
			OR LOWER(COALESCE(r.id, '')) LIKE ? ESCAPE '\'`, pattern, pattern, pattern, pattern, pattern, pattern, pattern)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return TurnListResult{}, err
	}
	options.Page = pageWithinTotal(options.Page, options.PageSize, total)
	type turnSummaryRow struct {
		ID                string
		ConversationID    string
		ConversationTitle string
		UserID            string
		UserName          string
		UserEmail         string
		UserAvatarURL     string
		TurnIndex         int
		Status            string
		UserMessage       string
		RunID             string
		TraceContext      []byte
		StartedAt         *time.Time
		CompletedAt       *time.Time
		CreatedAt         time.Time
	}
	var rows []turnSummaryRow
	selectSQL := `t.id, t.conversation_id, c.title AS conversation_title,
		c.owner_user_id AS user_id, COALESCE(u.name, '') AS user_name,
		COALESCE(u.email, '') AS user_email, COALESCE(u.avatar_url, '') AS user_avatar_url,
		t.turn_index, t.status, t.input AS user_message, t.created_at,
		COALESCE(r.id, '') AS run_id, COALESCE(r.trace_context, '{}'::jsonb) AS trace_context,
		r.started_at, r.completed_at`
	if err := base.Select(selectSQL).
		Order(turnSortClause(options.SortBy, options.SortOrder)).
		Limit(options.PageSize).Offset((options.Page - 1) * options.PageSize).
		Scan(&rows).Error; err != nil {
		return TurnListResult{}, err
	}
	runIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.RunID != "" {
			runIDs = append(runIDs, row.RunID)
		}
	}
	assistantMessages, err := s.loadAssistantMessages(ctx, runIDs)
	if err != nil {
		return TurnListResult{}, err
	}
	statsByRun, err := s.loadRunStats(ctx, runIDs)
	if err != nil {
		return TurnListResult{}, err
	}
	items := make([]TurnSummary, 0, len(rows))
	for _, row := range rows {
		stats := statsByRun[row.RunID]
		item := TurnSummary{
			ID: row.ID, ConversationID: row.ConversationID, ConversationTitle: row.ConversationTitle,
			User:      ConversationUser{ID: row.UserID, Name: row.UserName, Email: row.UserEmail, AvatarURL: row.UserAvatarURL},
			TurnIndex: row.TurnIndex, Status: row.Status, UserMessage: row.UserMessage, RunID: row.RunID,
			AssistantMessage: assistantMessages[row.RunID], TraceID: traceIDFromContext(row.TraceContext),
			InputTokens: stats.InputTokens, OutputTokens: stats.OutputTokens,
			ToolCallCount: stats.ToolCallCount, CreatedAt: row.CreatedAt,
		}
		if row.StartedAt != nil && row.CompletedAt != nil {
			item.DurationMs = float64(row.CompletedAt.Sub(*row.StartedAt).Microseconds()) / 1000
		}
		items = append(items, item)
	}
	totalPages := pageCount(total, options.PageSize)
	return TurnListResult{Items: items, Total: total, Page: options.Page, PageSize: options.PageSize, SortBy: options.SortBy, SortOrder: options.SortOrder, TotalPages: totalPages}, nil
}

type runStats struct {
	InputTokens   int64
	OutputTokens  int64
	ToolCallCount int64
}

func (s *ConversationStore) loadAssistantMessages(ctx context.Context, runIDs []string) (map[string]string, error) {
	messages := make(map[string][]string, len(runIDs))
	if len(runIDs) == 0 {
		return map[string]string{}, nil
	}
	var rows []struct {
		RunID   string
		Content []byte
	}
	if err := s.db.WithContext(ctx).Table("ai.items").Select("run_id, content").
		Where("run_id IN ? AND type = ?", runIDs, "assistant_message").
		Order("run_id ASC, timeline_index ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if text := messageText(row.Content); strings.TrimSpace(text) != "" {
			messages[row.RunID] = append(messages[row.RunID], text)
		}
	}
	result := make(map[string]string, len(messages))
	for runID, parts := range messages {
		result[runID] = strings.Join(parts, "\n")
	}
	return result, nil
}

func (s *ConversationStore) loadRunStats(ctx context.Context, runIDs []string) (map[string]runStats, error) {
	stats := make(map[string]runStats, len(runIDs))
	if len(runIDs) == 0 {
		return stats, nil
	}
	var rows []struct {
		RunID         string
		InputTokens   int64
		OutputTokens  int64
		ToolCallCount int64
	}
	if err := s.db.WithContext(ctx).Raw(`
		SELECT run.id AS run_id,
			COALESCE((SELECT SUM(CASE WHEN event.data->'usage'->>'inputTokens' ~ '^[0-9]+$' THEN (event.data->'usage'->>'inputTokens')::bigint ELSE 0 END)
				FROM ai.run_events event WHERE event.run_id = run.id AND event.type = 'model.completed'), 0) AS input_tokens,
			COALESCE((SELECT SUM(CASE WHEN event.data->'usage'->>'outputTokens' ~ '^[0-9]+$' THEN (event.data->'usage'->>'outputTokens')::bigint ELSE 0 END)
				FROM ai.run_events event WHERE event.run_id = run.id AND event.type = 'model.completed'), 0) AS output_tokens,
			(SELECT COUNT(*) FROM ai.items item WHERE item.run_id = run.id AND item.type = 'tool_call') AS tool_call_count
		FROM ai.runs run
		WHERE run.id IN ?`, runIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		stats[row.RunID] = runStats{InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, ToolCallCount: row.ToolCallCount}
	}
	return stats, nil
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
	turnPage = pageWithinTotal(turnPage, turnPageSize, summary.TurnCount)

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
	totalTurnPages := pageCount(summary.TurnCount, turnPageSize)
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

func normalizeTurnListOptions(options ConversationListOptions) ConversationListOptions {
	options.Page, options.PageSize = normalizePage(options.Page, options.PageSize)
	if options.SortBy != "conversation" && options.SortBy != "user" && options.SortBy != "status" && options.SortBy != "duration" {
		options.SortBy = "createdAt"
	}
	if options.SortOrder != "asc" {
		options.SortOrder = "desc"
	}
	return options
}

func normalizeToolSummaryListOptions(options ToolSummaryListOptions) ToolSummaryListOptions {
	options.Page, options.PageSize = normalizePage(options.Page, options.PageSize)
	if options.SortBy != "operationId" && options.SortBy != "totalCalls" && options.SortBy != "successRate" && options.SortBy != "failedCalls" {
		options.SortBy = "lastCalledAt"
	}
	if options.SortOrder != "asc" {
		options.SortOrder = "desc"
	}
	return options
}

func normalizeToolCallListOptions(options ToolCallListOptions) ToolCallListOptions {
	options.Page, options.PageSize = normalizePage(options.Page, options.PageSize)
	if options.SortBy != "status" && options.SortBy != "user" && options.SortBy != "conversation" {
		options.SortBy = "createdAt"
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

func turnSortClause(sortBy, sortOrder string) string {
	columns := map[string]string{
		"createdAt": "t.created_at", "conversation": "c.title", "user": "u.name", "status": "t.status",
		"duration": "(EXTRACT(EPOCH FROM (r.completed_at - r.started_at)) * 1000)",
	}
	column := columns[sortBy]
	if column == "" {
		column = "t.created_at"
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return fmt.Sprintf("%s %s, t.id %s", column, sortOrder, sortOrder)
}

func toolSummarySortClause(sortBy, sortOrder string) string {
	columns := map[string]string{
		"operationId": "operation_id", "totalCalls": "total_calls", "successRate": "success_rate",
		"failedCalls": "failed_calls", "lastCalledAt": "last_called_at",
	}
	column := columns[sortBy]
	if column == "" {
		column = "last_called_at"
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return fmt.Sprintf("%s %s, operation_id asc", column, sortOrder)
}

func toolCallSortClause(sortBy, sortOrder string) string {
	columns := map[string]string{
		"createdAt": "item.created_at", "status": "status", "user": "u.name", "conversation": "c.title",
	}
	column := columns[sortBy]
	if column == "" {
		column = "item.created_at"
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return fmt.Sprintf("%s %s, item.id %s", column, sortOrder, sortOrder)
}

func decodeSanitizedToolObject(raw []byte) map[string]any {
	value := decodeSanitizedToolValue(raw)
	result, _ := value.(map[string]any)
	if result == nil {
		return map[string]any{}
	}
	return result
}

func decodeSanitizedToolValue(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return sanitizeToolValue(value, 0)
}

func pageCount(total int64, pageSize int) int {
	if total <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func pageWithinTotal(page, pageSize int, total int64) int {
	pages := pageCount(total, pageSize)
	if pages == 0 {
		return 1
	}
	if page > pages {
		return pages
	}
	return page
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
