package notification

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

var defaultFailureEventTypes = []string{"build.failed", "release.failed", "hook.failed", "gateway.apply_failed"}

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

func (s Service) Emit(ctx context.Context, event Event) ([]model.NotificationDelivery, error) {
	if s.DB == nil {
		return nil, nil
	}
	event = normalizeEvent(event)
	eventData, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	var rules []model.NotificationRule
	query := s.DB.Where("enabled = ?", true)
	if strings.TrimSpace(event.Project.ID) != "" {
		query = query.Where("project_id in ?", []string{"", event.Project.ID})
	} else {
		query = query.Where("project_id = ?", "")
	}
	if err := query.Order("created_at asc").Find(&rules).Error; err != nil {
		return nil, err
	}

	deliveries := make([]model.NotificationDelivery, 0)
	for _, rule := range rules {
		if !ruleMatchesEvent(rule, event) {
			continue
		}
		channelIDs := decodeStringList(rule.ChannelIDsJSON)
		if len(channelIDs) == 0 {
			continue
		}
		var channels []model.NotificationChannel
		if err := s.DB.Where("id in ? and enabled = ?", channelIDs, true).Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			template := model.NotificationTemplate{}
			templateID := strings.TrimSpace(rule.TemplateID)
			if templateID != "" {
				_ = s.DB.First(&template, "id = ? and enabled = ?", templateID, true).Error
			}
			if template.ID == "" {
				template = DefaultTemplateFor(channel.AdapterKind, event.Type, strings.TrimSpace(rule.Locale))
				templateID = ""
			}
			delivery := model.NotificationDelivery{
				ID:          id.New("ndl"),
				ProjectID:   event.Project.ID,
				EventID:     event.ID,
				EventType:   event.Type,
				Severity:    event.Severity,
				ChannelID:   channel.ID,
				AdapterKind: channel.AdapterKind,
				RuleID:      rule.ID,
				TemplateID:  templateID,
				EventJSON:   string(eventData),
				Status:      "pending",
				QueuedAt:    time.Now(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := s.DB.Create(&delivery).Error; err != nil {
				return deliveries, err
			}
			deliveries = append(deliveries, delivery)
			if s.Enqueuer != nil {
				if _, err := s.Enqueuer.EnqueueNotificationDeliver(ctx, tasks.NotificationDeliverPayload{DeliveryID: delivery.ID}); err != nil {
					_ = s.DB.Model(&delivery).Updates(map[string]any{"status": "enqueue_failed", "error_message": err.Error()}).Error
					return deliveries, err
				}
			}
		}
		_ = s.DB.Model(&rule).Update("last_matched_event_id", event.ID).Error
	}
	return deliveries, nil
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
