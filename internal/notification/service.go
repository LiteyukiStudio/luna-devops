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
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultFailureEventTypes = []string{"build.failed", "release.failed", "hook.failed", "gateway.apply_failed", "certificate.failed", "certificate.expired"}

const UserEmailChannelID = "notification:user-email"

type DeliveryEnqueuer interface {
	EnqueueNotificationDeliver(ctx context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error)
}

type Service struct {
	DB       *gorm.DB
	Enqueuer DeliveryEnqueuer
}

type RuleFilter struct {
	Severities          []string `json:"severities"`
	ProjectIDs          []string `json:"projectIds"`
	ApplicationIDs      []string `json:"applicationIds"`
	DeploymentTargetIDs []string `json:"deploymentTargetIds"`
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
	resourceType, resourceID := platformevent.ResourceForType(event.Type, event.Build.ID, event.Release.ID, event.Hook.ID, event.Gateway.ID, event.Certificate.RouteID)
	if platformevent.CategoryForType(event.Type) == "service_binding" {
		resourceType, resourceID = "service_binding", event.ServiceBinding.ID
	}
	storedEvent, created, err := (platformevent.Service{DB: db}).Record(ctx, platformevent.RecordInput{
		ID:                 event.ID,
		Type:               event.Type,
		Severity:           event.Severity,
		ProjectID:          event.Project.ID,
		ApplicationID:      event.Application.ID,
		DeploymentTargetID: event.DeploymentTarget.ID,
		ResourceType:       resourceType,
		ResourceID:         resourceID,
		ActorID:            event.Actor.ID,
		Message:            event.Message,
		Detail:             event,
		Links:              event.Links,
		CorrelationID:      event.CorrelationID,
		TraceID:            event.TraceID,
		DedupKey:           event.DedupKey,
		OccurredAt:         event.OccurredAt,
	})
	if err != nil {
		return nil, err
	}
	if !created {
		var authoritative Event
		if err := json.Unmarshal([]byte(storedEvent.DetailJSON), &authoritative); err != nil {
			return nil, fmt.Errorf("decode recorded notification event: %w", err)
		}
		event = normalizeEvent(authoritative)
	}
	event.ID = storedEvent.ID
	event.Actor.ID = strings.TrimSpace(storedEvent.ActorID)
	eventData, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

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

	deliveries := make([]model.NotificationDelivery, 0)
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
				template = DefaultTemplateFor(channel.AdapterKind, event.Type, strings.TrimSpace(rule.Locale))
				templateID = ""
			}
			delivery, created, err := s.createDelivery(ctx, event, eventData, deliveryTarget{
				ChannelID:   channel.ID,
				AdapterKind: channel.AdapterKind,
				RuleID:      rule.ID,
				TemplateID:  templateID,
			})
			if created {
				deliveries = append(deliveries, delivery)
			}
			if err != nil {
				return deliveries, err
			}
		}
	}

	personalTargets, err := s.personalDeliveryTargets(ctx, event)
	if err != nil {
		return deliveries, err
	}
	for _, target := range personalTargets {
		delivery, created, err := s.createDelivery(ctx, event, eventData, target)
		if created {
			deliveries = append(deliveries, delivery)
		}
		if err != nil {
			return deliveries, err
		}
	}
	return deliveries, nil
}

func (s Service) personalDeliveryTargets(ctx context.Context, event Event) ([]deliveryTarget, error) {
	actorID := strings.TrimSpace(event.Actor.ID)
	if actorID == "" {
		return nil, nil
	}
	db := s.DB.WithContext(ctx)
	var user model.User
	if err := db.Where("id = ? and disabled = ?", actorID, false).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	preference := DefaultUserNotificationPreference(user.ID)
	if err := db.First(&preference, "user_id = ?", user.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if !containsString(decodeStringList(preference.EventTypesJSON), event.Type) {
		return nil, nil
	}

	targets := make([]deliveryTarget, 0, 2)
	if preference.EmailEnabled && strings.TrimSpace(user.Email) != "" {
		targets = append(targets, deliveryTarget{
			ChannelID:       UserEmailChannelID,
			AdapterKind:     AdapterKindSMTP,
			RecipientUserID: user.ID,
		})
	}
	var channels []model.NotificationChannel
	if err := db.Where("owner_user_id = ? and adapter_kind = ? and enabled = ?", user.ID, AdapterKindWebhook, true).
		Order("created_at asc").Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		targets = append(targets, deliveryTarget{
			ChannelID:       channel.ID,
			AdapterKind:     AdapterKindWebhook,
			RecipientUserID: user.ID,
		})
	}
	return targets, nil
}

func (s Service) createDelivery(ctx context.Context, event Event, eventData []byte, target deliveryTarget) (model.NotificationDelivery, bool, error) {
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
		Status:          "pending",
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	result := s.DB.WithContext(ctx).Clauses(clause.OnConflict{
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
		if err := s.DB.WithContext(ctx).First(&existing,
			"event_id = ? and channel_id = ? and recipient_user_id = ?",
			event.ID,
			target.ChannelID,
			target.RecipientUserID,
		).Error; err != nil {
			return model.NotificationDelivery{}, false, err
		}
		if existing.Status != "enqueue_failed" || s.Enqueuer == nil {
			return model.NotificationDelivery{}, false, nil
		}
		claimed := s.DB.WithContext(ctx).Model(&model.NotificationDelivery{}).
			Where("id = ? and status = ?", existing.ID, "enqueue_failed").
			Updates(map[string]any{
				"status":        "pending",
				"error_message": "",
				"queued_at":     now,
			})
		if claimed.Error != nil {
			return model.NotificationDelivery{}, false, claimed.Error
		}
		if claimed.RowsAffected == 0 {
			return model.NotificationDelivery{}, false, nil
		}
		delivery = existing
		delivery.Status = "pending"
		delivery.ErrorMessage = ""
		delivery.QueuedAt = now
	}
	if s.Enqueuer != nil {
		if _, err := s.Enqueuer.EnqueueNotificationDeliver(ctx, tasks.NotificationDeliverPayload{DeliveryID: delivery.ID}); err != nil {
			errorMessage := err.Error()
			if strings.TrimSpace(target.RecipientUserID) != "" {
				errorMessage = "notification.personal_delivery_enqueue_failed"
			}
			_ = s.DB.WithContext(ctx).Model(&delivery).Updates(map[string]any{"status": "enqueue_failed", "error_message": errorMessage}).Error
			return delivery, true, err
		}
	}
	return delivery, true, nil
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
	if len(eventTypes) > 0 && !containsString(eventTypes, event.Type) {
		return false
	}
	var filter RuleFilter
	if strings.TrimSpace(rule.FilterJSON) != "" {
		_ = json.Unmarshal([]byte(rule.FilterJSON), &filter)
	}
	return stringListMatches(filter.Severities, event.Severity) &&
		stringListMatches(filter.ProjectIDs, event.Project.ID) &&
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

func EncodeRuleFilter(filter RuleFilter) string {
	data, _ := json.Marshal(filter)
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
