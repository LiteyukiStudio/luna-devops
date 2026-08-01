package inbox

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"gorm.io/gorm"
)

func (s *Service) List(ctx context.Context, input ListInput) (result ListResult, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "list")
	defer func() { end(err) }()

	input.UserID = strings.TrimSpace(input.UserID)
	if input.UserID == "" || s == nil || s.db == nil {
		return ListResult{}, ErrInvalidInput
	}
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 20
	}
	if input.PageSize > maxPageSize {
		input.PageSize = maxPageSize
	}
	sortColumn, sortBy, ok := inboxSortColumn(input.SortBy)
	if !ok {
		return ListResult{}, ErrInvalidInput
	}
	sortOrder := strings.ToLower(strings.TrimSpace(input.SortOrder))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return ListResult{}, ErrInvalidInput
	}
	filter := strings.ToLower(strings.TrimSpace(input.Filter))
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "unread" && filter != "action" {
		return ListResult{}, ErrInvalidInput
	}
	category := strings.TrimSpace(input.Category)
	if category != "" {
		if _, exists := validCategories[category]; !exists {
			return ListResult{}, ErrInvalidInput
		}
	}

	query := s.db.WithContext(ctx).Model(&model.InboxMessage{}).
		Where("recipient_user_id = ? AND archived_at IS NULL", input.UserID)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	switch filter {
	case "unread":
		query = query.Where("read_at IS NULL")
	case "action":
		query = query.Where(
			"EXISTS (SELECT 1 FROM inbox_action_requests AS action_request WHERE action_request.id = inbox_messages.action_request_id AND action_request.recipient_user_id = ? AND action_request.status IN ?)",
			input.UserID,
			[]string{ActionRequestStatusPending, ActionRequestStatusProcessing},
		)
	}
	if err = query.Count(&result.Total).Error; err != nil {
		return ListResult{}, err
	}
	result.Page = input.Page
	result.PageSize = input.PageSize
	result.SortBy = sortBy
	result.SortOrder = sortOrder
	result.TotalPages = int(math.Ceil(float64(result.Total) / float64(input.PageSize)))
	result.Items = make([]model.InboxMessage, 0)
	err = query.Order(sortColumn + " " + sortOrder + ", id " + sortOrder).
		Offset((input.Page - 1) * input.PageSize).
		Limit(input.PageSize).
		Find(&result.Items).Error
	return result, err
}

func (s *Service) Get(ctx context.Context, userID, messageID string) (message model.InboxMessage, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "get")
	defer func() { end(err) }()
	userID = strings.TrimSpace(userID)
	messageID = strings.TrimSpace(messageID)
	if userID == "" || messageID == "" || s == nil || s.db == nil {
		return model.InboxMessage{}, ErrInvalidInput
	}
	err = s.db.WithContext(ctx).
		Where("id = ? AND recipient_user_id = ?", messageID, userID).
		First(&message).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return message, err
}

func (s *Service) GetActionRequest(ctx context.Context, userID, requestID string) (request model.InboxActionRequest, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "get_action_request")
	defer func() { end(err) }()
	userID = strings.TrimSpace(userID)
	requestID = strings.TrimSpace(requestID)
	if userID == "" || requestID == "" || s == nil || s.db == nil {
		return model.InboxActionRequest{}, ErrInvalidInput
	}
	err = s.db.WithContext(ctx).
		Where("id = ? AND recipient_user_id = ?", requestID, userID).
		First(&request).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrNotFound
	}
	return request, err
}

func (s *Service) GetActionRequests(ctx context.Context, userID string, requestIDs []string) (requests map[string]model.InboxActionRequest, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "get_action_requests")
	defer func() { end(err) }()
	userID = strings.TrimSpace(userID)
	if userID == "" || s == nil || s.db == nil {
		return nil, ErrInvalidInput
	}
	uniqueIDs := make([]string, 0, len(requestIDs))
	seen := make(map[string]struct{}, len(requestIDs))
	for _, requestID := range requestIDs {
		requestID = strings.TrimSpace(requestID)
		if requestID == "" {
			continue
		}
		if _, exists := seen[requestID]; exists {
			continue
		}
		seen[requestID] = struct{}{}
		uniqueIDs = append(uniqueIDs, requestID)
	}
	requests = make(map[string]model.InboxActionRequest, len(uniqueIDs))
	if len(uniqueIDs) == 0 {
		return requests, nil
	}
	var items []model.InboxActionRequest
	if err = s.db.WithContext(ctx).
		Where("recipient_user_id = ? AND id IN ?", userID, uniqueIDs).
		Find(&items).Error; err != nil {
		return nil, err
	}
	for _, request := range items {
		requests[request.ID] = request
	}
	return requests, nil
}

func inboxSortColumn(value string) (column, normalized string, ok bool) {
	switch strings.TrimSpace(value) {
	case "", "createdAt":
		return "created_at", "createdAt", true
	case "updatedAt":
		return "updated_at", "updatedAt", true
	case "priority":
		return "CASE priority WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'normal' THEN 2 WHEN 'low' THEN 1 ELSE 0 END", "priority", true
	default:
		return "", "", false
	}
}
