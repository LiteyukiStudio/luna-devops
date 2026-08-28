package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultFailureEventTypes = []string{"build.failed", "release.failed", "hook.failed", "gateway.apply_failed", "certificate.failed", "certificate.expired"}

const (
	UserEmailChannelID                = "notification:user-email"
	NotificationFanoutStatusPending   = "pending"
	NotificationFanoutStatusCompleted = "completed"
	DeliveryEnqueueFailedCode         = "notification.delivery_enqueue_failed"
	PersonalDeliveryEnqueueFailedCode = "notification.personal_delivery_enqueue_failed"
)

var ErrEventNotMaterializable = errors.New("platform event is not a notification fanout event")

type DeliveryEnqueuer interface {
	EnqueueNotificationDeliver(ctx context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error)
}

type Service struct {
	DB       *gorm.DB
	Enqueuer DeliveryEnqueuer
}

type deliveryTarget struct {
	ChannelID       string
	AdapterKind     string
	RuleID          string
	TemplateID      string
	RecipientUserID string
}

func DefaultFailureEventTypes() []string {
	return append([]string(nil), defaultFailureEventTypes...)
}

func DefaultUserNotificationPreference(userID string) model.UserNotificationPreference {
	return model.UserNotificationPreference{
		UserID:         strings.TrimSpace(userID),
		EmailEnabled:   true,
		EventTypesJSON: EncodeStringList(DefaultFailureEventTypes()),
	}
}

func (s Service) Emit(ctx context.Context, event Event) ([]model.NotificationDelivery, error) {
	if s.DB == nil {
		return nil, nil
	}
	db := s.DB.WithContext(ctx)
	event = normalizeEvent(event)
	traceContext := telemetry.InjectMap(ctx)
	if strings.TrimSpace(event.TraceID) == "" {
		spanContext := trace.SpanContextFromContext(ctx)
		if spanContext.IsValid() {
			event.TraceID = spanContext.TraceID().String()
		}
	}
	resourceType, resourceID := platformevent.ResourceForType(event.Type, event.Build.ID, event.Release.ID, event.Hook.ID, event.Gateway.ID, event.Certificate.RouteID)
	if platformevent.CategoryForType(event.Type) == "service_binding" {
		resourceType, resourceID = "service_binding", event.ServiceBinding.ID
	}
	recordInput := platformevent.RecordInput{
		ID:                  event.ID,
		Type:                event.Type,
		Severity:            event.Severity,
		ProjectID:           event.Project.ID,
		ApplicationID:       event.Application.ID,
		DeploymentTargetID:  event.DeploymentTarget.ID,
		ResourceType:        resourceType,
		ResourceID:          resourceID,
		ActorID:             event.Actor.ID,
		ResourceOwnerUserID: event.ResourceOwnerUserID,
		Message:             event.Message,
		Detail:              event,
		Links:               event.Links,
		CorrelationID:       event.CorrelationID,
		TraceID:             event.TraceID,
		DedupKey:            event.DedupKey,
		OccurredAt:          event.OccurredAt,
	}

	storedEvent, _, err := (platformevent.Service{DB: db}).RecordNotification(
		ctx,
		recordInput,
		traceContext["traceparent"],
		traceContext["tracestate"],
	)
	if err != nil {
		return nil, err
	}
	return s.ReconcileEvent(ctx, storedEvent.ID)
}

// ReconcileEvent atomically materializes an event before enqueueing the
// resulting deliveries. It is the shared recovery entry point for Emit and
// background reconciliation.
func (s Service) ReconcileEvent(ctx context.Context, eventID string) ([]model.NotificationDelivery, error) {
	deliveries, err := s.MaterializeEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	return deliveries, s.EnqueueDeliveries(ctx, deliveries)
}

// MaterializeEvent resolves the current eligible targets for an authoritative
// notification event and persists all deliveries atomically. Enqueueing is
// deliberately left to the caller and may only happen after this transaction
// commits.
func (s Service) MaterializeEvent(ctx context.Context, eventID string) ([]model.NotificationDelivery, error) {
	if s.DB == nil {
		return nil, errors.New("notification database is unavailable")
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, fmt.Errorf("%w: empty event id", ErrEventNotMaterializable)
	}

	var deliveries []model.NotificationDelivery
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var storedEvent model.PlatformEvent
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&storedEvent, "id = ?", eventID).Error; err != nil {
			return err
		}
		switch storedEvent.NotificationFanoutStatus {
		case NotificationFanoutStatusPending, NotificationFanoutStatusCompleted:
		default:
			return fmt.Errorf("%w: event %s has status %q", ErrEventNotMaterializable, storedEvent.ID, storedEvent.NotificationFanoutStatus)
		}

		event, err := eventFromStoredPlatformEvent(storedEvent)
		if err != nil {
			return err
		}
		eventData, err := json.Marshal(event)
		if err != nil {
			return err
		}

		sharedTargets, err := s.sharedDeliveryTargets(ctx, tx, event)
		if err != nil {
			return err
		}
		personalTargets, err := s.personalDeliveryTargets(ctx, tx, storedEvent)
		if err != nil {
			return err
		}
		targets := make([]deliveryTarget, 0, len(sharedTargets)+len(personalTargets))
		targets = append(targets, sharedTargets...)
		targets = append(targets, personalTargets...)

		persisted := make([]model.NotificationDelivery, 0, len(targets))
		for _, target := range targets {
			delivery, enqueue, persistErr := persistDelivery(tx, event, eventData, storedEvent, target)
			if persistErr != nil {
				return persistErr
			}
			if enqueue {
				persisted = append(persisted, delivery)
			}
		}
		completed := tx.Model(&model.PlatformEvent{}).
			Where("id = ? and notification_fanout_status in ?", storedEvent.ID, []string{
				NotificationFanoutStatusPending,
				NotificationFanoutStatusCompleted,
			}).
			Update("notification_fanout_status", NotificationFanoutStatusCompleted)
		if completed.Error != nil {
			return completed.Error
		}
		if completed.RowsAffected != 1 {
			return fmt.Errorf("complete notification fanout event %s: authoritative row changed", storedEvent.ID)
		}
		deliveries = persisted
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deliveries, nil
}

func eventFromStoredPlatformEvent(storedEvent model.PlatformEvent) (Event, error) {
	var event Event
	if err := json.Unmarshal([]byte(storedEvent.DetailJSON), &event); err != nil {
		return Event{}, fmt.Errorf("decode recorded notification event: %w", err)
	}
	event.ID = storedEvent.ID
	event.Type = storedEvent.Type
	event.Severity = storedEvent.Severity
	event.Project.ID = storedEvent.ProjectID
	event.Application.ID = storedEvent.ApplicationID
	event.DeploymentTarget.ID = storedEvent.DeploymentTargetID
	event.Actor.ID = strings.TrimSpace(storedEvent.ActorID)
	event.ResourceOwnerUserID = strings.TrimSpace(storedEvent.ResourceOwnerUserID)
	event.CorrelationID = storedEvent.CorrelationID
	event.TraceID = storedEvent.TraceID
	event.OccurredAt = storedEvent.OccurredAt
	event.Message = storedEvent.Message
	return event, nil
}

func (s Service) sharedDeliveryTargets(ctx context.Context, db *gorm.DB, event Event) ([]deliveryTarget, error) {
	db = db.WithContext(ctx)
	var rules []model.NotificationRule
	query := db.Where("enabled = ?", true)
	if strings.TrimSpace(event.Project.ID) != "" {
		query = query.Where("project_id in ?", []string{"", event.Project.ID})
	} else {
		query = query.Where("project_id = ?", "")
	}
	if err := query.Order("created_at asc").Find(&rules).Error; err != nil {
		return nil, err
	}

	targets := make([]deliveryTarget, 0)
	seenChannels := map[string]bool{}
	for _, rule := range rules {
		if !ruleMatchesEvent(rule, event) {
			continue
		}
		channelIDs := decodeStringList(rule.ChannelIDsJSON)
		if len(channelIDs) == 0 {
			continue
		}
		var channels []model.NotificationChannel
		if err := db.Where("id in ? and enabled = ? and owner_user_id = ?", channelIDs, true, "").Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			if seenChannels[channel.ID] {
				continue
			}
			seenChannels[channel.ID] = true
			template := model.NotificationTemplate{}
			templateID := strings.TrimSpace(rule.TemplateID)
			if templateID != "" {
				_ = db.First(&template, "id = ? and enabled = ?", templateID, true).Error
			}
			if template.ID == "" {
				templateID = ""
			}
			targets = append(targets, deliveryTarget{
				ChannelID:   channel.ID,
				AdapterKind: channel.AdapterKind,
				RuleID:      rule.ID,
				TemplateID:  templateID,
			})
		}
	}
	return targets, nil
}

func (s Service) personalDeliveryTargets(ctx context.Context, db *gorm.DB, event model.PlatformEvent) ([]deliveryTarget, error) {
	db = db.WithContext(ctx)
	targets := make([]deliveryTarget, 0, 4)
	for _, recipientUserID := range PersonalRecipientUserIDs(event.ActorID, event.ResourceOwnerUserID) {
		policy, code, err := LoadPersonalRecipientPolicy(ctx, db, recipientUserID)
		if err != nil {
			return nil, err
		}
		if code != "" {
			continue
		}
		if code, err = policy.CheckEvent(ctx, db, event); err != nil {
			return nil, err
		} else if code != "" {
			continue
		}

		if policy.CheckAdapter(AdapterKindSMTP) == "" {
			targets = append(targets, deliveryTarget{
				ChannelID:       UserEmailChannelID,
				AdapterKind:     AdapterKindSMTP,
				RecipientUserID: policy.User.ID,
			})
		}
		var channels []model.NotificationChannel
		if err := db.Where("owner_user_id = ? and adapter_kind = ? and enabled = ?", policy.User.ID, AdapterKindWebhook, true).
			Order("created_at asc").Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			targets = append(targets, deliveryTarget{
				ChannelID:       channel.ID,
				AdapterKind:     AdapterKindWebhook,
				RecipientUserID: policy.User.ID,
			})
		}
	}
	return targets, nil
}

func persistDelivery(db *gorm.DB, event Event, eventData []byte, storedEvent model.PlatformEvent, target deliveryTarget) (model.NotificationDelivery, bool, error) {
	now := time.Now()
	delivery := model.NotificationDelivery{
		ID:              id.New("ndl"),
		ProjectID:       event.Project.ID,
		RecipientUserID: target.RecipientUserID,
		EventID:         event.ID,
		EventType:       event.Type,
		Severity:        event.Severity,
		ChannelID:       target.ChannelID,
		AdapterKind:     target.AdapterKind,
		RuleID:          target.RuleID,
		TemplateID:      target.TemplateID,
		EventJSON:       string(eventData),
		Traceparent:     storedEvent.FanoutTraceparent,
		Tracestate:      storedEvent.FanoutTracestate,
		Status:          "pending",
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "event_id"},
			{Name: "channel_id"},
			{Name: "recipient_user_id"},
		},
		DoNothing: true,
	}).Create(&delivery)
	if result.Error != nil {
		return model.NotificationDelivery{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		var existing model.NotificationDelivery
		if err := db.First(&existing,
			"event_id = ? and channel_id = ? and recipient_user_id = ?",
			event.ID,
			target.ChannelID,
			target.RecipientUserID,
		).Error; err != nil {
			return model.NotificationDelivery{}, false, err
		}
		switch existing.Status {
		case "pending", "enqueue_failed":
			return existing, true, nil
		default:
			return model.NotificationDelivery{}, false, nil
		}
	}
	return delivery, true, nil
}

// EnqueueDeliveries applies the single delivery enqueue state machine after
// callers have committed their materialization or reconciliation claims.
func (s Service) EnqueueDeliveries(ctx context.Context, deliveries []model.NotificationDelivery) error {
	if s.Enqueuer == nil || len(deliveries) == 0 {
		return nil
	}
	if s.DB == nil {
		return errors.New("notification database is unavailable")
	}
	db := s.DB.WithContext(ctx)
	errs := make([]error, 0)
	for index := range deliveries {
		delivery := &deliveries[index]
		if delivery.Status == "enqueue_failed" {
			now := time.Now()
			if err := db.Model(&model.NotificationDelivery{}).
				Where("id = ? and status = ?", delivery.ID, "enqueue_failed").
				Updates(map[string]any{"status": "pending", "error_message": "", "queued_at": now}).Error; err != nil {
				errs = append(errs, fmt.Errorf("prepare notification delivery %s for enqueue: %w", delivery.ID, err))
			} else {
				delivery.Status = "pending"
				delivery.ErrorMessage = ""
				delivery.QueuedAt = now
			}
		}
		enqueueCtx := ctx
		if strings.TrimSpace(delivery.Traceparent) != "" {
			carrier := map[string]string{"traceparent": delivery.Traceparent}
			if delivery.Tracestate != "" {
				carrier["tracestate"] = delivery.Tracestate
			}
			enqueueCtx = telemetry.ExtractMap(ctx, carrier)
		}
		_, err := s.Enqueuer.EnqueueNotificationDeliver(enqueueCtx, tasks.NotificationDeliverPayload{DeliveryID: delivery.ID})
		if err == nil || errors.Is(err, asynq.ErrDuplicateTask) {
			continue
		}
		errorMessage := DeliveryEnqueueFailedCode
		if strings.TrimSpace(delivery.RecipientUserID) != "" {
			errorMessage = PersonalDeliveryEnqueueFailedCode
		}
		failedAt := time.Now()
		delivery.Status = "enqueue_failed"
		delivery.ErrorMessage = errorMessage
		delivery.QueuedAt = failedAt
		if updateErr := db.Model(&model.NotificationDelivery{}).
			Where("id = ? and status in ?", delivery.ID, []string{"pending", "enqueue_failed"}).
			Updates(map[string]any{
				"status":        "enqueue_failed",
				"error_message": errorMessage,
				"queued_at":     failedAt,
			}).Error; updateErr != nil {
			errs = append(errs, fmt.Errorf("record notification delivery %s enqueue failure: %w", delivery.ID, updateErr))
		}
		errs = append(errs, fmt.Errorf("enqueue notification delivery %s: %w", delivery.ID, err))
	}
	return errors.Join(errs...)
}

func normalizeEvent(event Event) Event {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = id.New("nev")
	}
	if strings.TrimSpace(event.Severity) == "" {
		event.Severity = SeverityError
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	return event
}

func ruleMatchesEvent(rule model.NotificationRule, event Event) bool {
	eventTypes := decodeStringList(rule.EventTypesJSON)
	if len(eventTypes) == 0 || !containsString(eventTypes, event.Type) {
		return false
	}
	filter, err := DecodeRuleFilter([]byte(rule.FilterJSON))
	if err != nil {
		return false
	}
	projectMatches := filter.Scope == RuleScopeAll || containsString(filter.ProjectIDs, event.Project.ID)
	return projectMatches && stringListMatches(filter.Severities, event.Severity) &&
		stringListMatches(filter.ApplicationIDs, event.Application.ID) &&
		stringListMatches(filter.DeploymentTargetIDs, event.DeploymentTarget.ID)
}

func DefaultTemplateFor(adapterKind string, eventType string, locale string) model.NotificationTemplate {
	name := "Default " + eventType
	if strings.TrimSpace(eventType) == "" {
		name = "Default notification"
	}
	template := model.NotificationTemplate{
		Name:        name,
		EventType:   eventType,
		AdapterKind: adapterKind,
		Locale:      locale,
		Enabled:     true,
	}
	switch adapterKind {
	case AdapterKindSMTP:
		template.SubjectTemplate = "[{{.Event.Severity}}] {{.Event.Type}}"
		template.BodyTemplate = "{{.Event.Message}}\n\nProject: {{.Event.Project.Name}}\nApplication: {{.Event.Application.Name}}\nDeployment: {{.Event.DeploymentTarget.Name}}\nTime: {{time .Event.OccurredAt \"2006-01-02 15:04:05 MST\"}}"
	default:
		template.JSONBodyTemplate = `{
  "text": "[{{.Event.Severity}}] {{.Event.Type}}\n{{.Event.Message}}\nProject: {{.Event.Project.Name}}\nApplication: {{.Event.Application.Name}}\nDeployment: {{.Event.DeploymentTarget.Name}}"
}`
	}
	return template
}

func DefaultTemplateForChannel(channel model.NotificationChannel, event Event, locale string) model.NotificationTemplate {
	template := DefaultTemplateFor(channel.AdapterKind, event.Type, locale)
	if channel.AdapterKind == AdapterKindWebhook {
		cfg, err := parseWebhookConfig(json.RawMessage(channel.ConfigJSON))
		if err == nil && strings.TrimSpace(cfg.TestJSONBodyTemplate) != "" {
			template.JSONBodyTemplate = cfg.TestJSONBodyTemplate
		}
	}
	return template
}

func TemplateFromModel(template model.NotificationTemplate) Template {
	return Template{
		Subject: template.SubjectTemplate,
		Body:    template.BodyTemplate,
		JSON:    template.JSONBodyTemplate,
	}
}

func decodeStringList(raw string) []string {
	values := []string{}
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &values)
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func EncodeStringList(values []string) string {
	data, _ := json.Marshal(values)
	return string(data)
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func stringListMatches(values []string, target string) bool {
	return len(values) == 0 || containsString(values, target)
}
