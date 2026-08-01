package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxPageSize = 100

var (
	validCategories = map[string]struct{}{
		CategoryAction: {}, CategoryProject: {}, CategoryBilling: {},
		CategorySecurity: {}, CategoryDelivery: {}, CategorySystem: {},
	}
	validPriorities = map[string]struct{}{
		PriorityLow: {}, PriorityNormal: {}, PriorityHigh: {}, PriorityCritical: {},
	}
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Publish(ctx context.Context, input PublishInput) (message model.InboxMessage, created bool, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "publish",
		attribute.String("inbox.category", strings.TrimSpace(input.Category)),
		attribute.String("inbox.priority", strings.TrimSpace(input.Priority)),
	)
	defer func() {
		if err != nil {
			telemetry.RecordError(ctx, "inbox.message.publish_failed", err,
				slog.String("inbox.category", strings.TrimSpace(input.Category)),
				slog.String("inbox.priority", strings.TrimSpace(input.Priority)),
			)
		}
		end(err)
	}()

	message, err = newMessage(input)
	if err != nil {
		return model.InboxMessage{}, false, err
	}
	if s == nil || s.db == nil {
		return model.InboxMessage{}, false, ErrInvalidInput
	}
	if message.ActionRequestID != "" {
		var count int64
		if err = s.db.WithContext(ctx).Model(&model.InboxActionRequest{}).
			Where("id = ? AND recipient_user_id = ?", message.ActionRequestID, message.RecipientUserID).
			Count(&count).Error; err != nil {
			return model.InboxMessage{}, false, err
		}
		if count == 0 {
			return model.InboxMessage{}, false, ErrInvalidInput
		}
	}

	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&message)
	if result.Error != nil {
		return model.InboxMessage{}, false, result.Error
	}
	if result.RowsAffected > 0 {
		telemetry.Logger().InfoContext(ctx, "inbox message published",
			slog.String("event.name", "inbox.message.published"),
			slog.String("inbox.category", message.Category),
			slog.String("inbox.priority", message.Priority),
		)
		return message, true, nil
	}
	if message.DedupKey == nil {
		return model.InboxMessage{}, false, errors.New("inbox message insert did not affect a row")
	}

	var existing model.InboxMessage
	if queryErr := s.db.WithContext(ctx).
		Where("recipient_user_id = ? AND dedup_key = ?", message.RecipientUserID, *message.DedupKey).
		First(&existing).Error; queryErr != nil {
		return model.InboxMessage{}, false, queryErr
	}
	telemetry.Logger().InfoContext(ctx, "inbox message deduplicated",
		slog.String("event.name", "inbox.message.deduplicated"),
		slog.String("inbox.category", existing.Category),
	)
	return existing, false, nil
}

func (s *Service) CreateActionRequest(ctx context.Context, input CreateActionRequestInput) (request model.InboxActionRequest, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "create_action_request")
	defer func() {
		if err != nil {
			telemetry.RecordError(ctx, "inbox.action_request.create_failed", err,
				slog.String("inbox.action_type", strings.TrimSpace(input.Type)),
			)
		}
		end(err)
	}()

	request, err = newActionRequest(input)
	if err != nil {
		return model.InboxActionRequest{}, err
	}
	if s == nil || s.db == nil {
		return model.InboxActionRequest{}, ErrInvalidInput
	}
	if err = s.db.WithContext(ctx).Create(&request).Error; err != nil {
		return model.InboxActionRequest{}, err
	}
	telemetry.Logger().InfoContext(ctx, "inbox action request created",
		slog.String("event.name", "inbox.action_request.created"),
		slog.String("inbox.action_type", request.Type),
	)
	return request, nil
}

func (s *Service) UnreadCount(ctx context.Context, userID string) (count int64, err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "unread_count")
	defer func() { end(err) }()
	userID = strings.TrimSpace(userID)
	if userID == "" || s == nil || s.db == nil {
		return 0, ErrInvalidInput
	}
	err = s.db.WithContext(ctx).Model(&model.InboxMessage{}).
		Where("recipient_user_id = ? AND read_at IS NULL AND archived_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

func (s *Service) MarkRead(ctx context.Context, userID, messageID string) (err error) {
	return s.updateMessageTimestamp(ctx, "mark_read", userID, messageID, "read_at")
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", "mark_all_read")
	defer func() { end(err) }()
	userID = strings.TrimSpace(userID)
	if userID == "" || s == nil || s.db == nil {
		return ErrInvalidInput
	}
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&model.InboxMessage{}).
		Where("recipient_user_id = ? AND read_at IS NULL AND archived_at IS NULL", userID).
		Update("read_at", now)
	if result.Error != nil {
		return result.Error
	}
	telemetry.Logger().InfoContext(ctx, "inbox messages marked read",
		slog.String("event.name", "inbox.messages.marked_read"),
		slog.Int64("inbox.message.count", result.RowsAffected),
	)
	return nil
}

func (s *Service) Archive(ctx context.Context, userID, messageID string) (err error) {
	return s.updateMessageTimestamp(ctx, "archive", userID, messageID, "archived_at")
}

func (s *Service) updateMessageTimestamp(ctx context.Context, operation, userID, messageID, column string) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "inbox", operation)
	defer func() { end(err) }()
	userID = strings.TrimSpace(userID)
	messageID = strings.TrimSpace(messageID)
	if userID == "" || messageID == "" || s == nil || s.db == nil {
		return ErrInvalidInput
	}
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&model.InboxMessage{}).
		Where("id = ? AND recipient_user_id = ?", messageID, userID).
		Update(column, now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	eventName := "inbox.message.marked_read"
	if operation == "archive" {
		eventName = "inbox.message.archived"
	}
	telemetry.Logger().InfoContext(ctx, "inbox message state updated",
		slog.String("event.name", eventName),
	)
	return nil
}

func newMessage(input PublishInput) (model.InboxMessage, error) {
	recipientUserID := strings.TrimSpace(input.RecipientUserID)
	messageType := strings.TrimSpace(input.Type)
	category := strings.TrimSpace(input.Category)
	priority := strings.TrimSpace(input.Priority)
	titleKey := strings.TrimSpace(input.TitleKey)
	contentKey := strings.TrimSpace(input.ContentKey)
	if recipientUserID == "" || messageType == "" || titleKey == "" || contentKey == "" {
		return model.InboxMessage{}, ErrInvalidInput
	}
	if _, ok := validCategories[category]; !ok {
		return model.InboxMessage{}, ErrInvalidInput
	}
	if _, ok := validPriorities[priority]; !ok {
		return model.InboxMessage{}, ErrInvalidInput
	}
	deepLink := strings.TrimSpace(input.DeepLink)
	if !validDeepLink(deepLink) {
		return model.InboxMessage{}, ErrInvalidDeepLink
	}
	paramsJSON, err := marshalObject(input.Params)
	if err != nil {
		return model.InboxMessage{}, errors.Join(ErrInvalidInput, err)
	}
	var dedupKey *string
	if value := strings.TrimSpace(input.DedupKey); value != "" {
		dedupKey = &value
	}
	return model.InboxMessage{
		ID:              id.New("imsg"),
		RecipientUserID: recipientUserID,
		Type:            messageType,
		Category:        category,
		Priority:        priority,
		ActorID:         strings.TrimSpace(input.ActorID),
		ProjectID:       strings.TrimSpace(input.ProjectID),
		ResourceType:    strings.TrimSpace(input.ResourceType),
		ResourceID:      strings.TrimSpace(input.ResourceID),
		TitleKey:        titleKey,
		ContentKey:      contentKey,
		ParamsJSON:      paramsJSON,
		ActionRequestID: strings.TrimSpace(input.ActionRequestID),
		DeepLink:        deepLink,
		GroupKey:        strings.TrimSpace(input.GroupKey),
		DedupKey:        dedupKey,
		ExpiresAt:       input.ExpiresAt,
	}, nil
}

func newActionRequest(input CreateActionRequestInput) (model.InboxActionRequest, error) {
	requestType := strings.TrimSpace(input.Type)
	requesterUserID := strings.TrimSpace(input.RequesterUserID)
	recipientUserID := strings.TrimSpace(input.RecipientUserID)
	if requestType == "" || requesterUserID == "" || recipientUserID == "" {
		return model.InboxActionRequest{}, ErrInvalidInput
	}
	if requestType != ActionRequestTypeBillingOwnerTransfer {
		return model.InboxActionRequest{}, ErrInvalidInput
	}
	payloadJSON, err := marshalObject(input.Payload)
	if err != nil {
		return model.InboxActionRequest{}, errors.Join(ErrInvalidInput, err)
	}
	return model.InboxActionRequest{
		ID:              id.New("iar"),
		Type:            requestType,
		RequesterUserID: requesterUserID,
		RecipientUserID: recipientUserID,
		ProjectID:       strings.TrimSpace(input.ProjectID),
		ResourceType:    strings.TrimSpace(input.ResourceType),
		ResourceID:      strings.TrimSpace(input.ResourceID),
		PayloadJSON:     payloadJSON,
		Status:          ActionRequestStatusPending,
		RowVersion:      1,
		ExpiresAt:       input.ExpiresAt,
	}, nil
}

func marshalObject(value map[string]any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func validDeepLink(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return false
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || strings.Contains(parsed.Path, "\\") {
		return false
	}
	return true
}
