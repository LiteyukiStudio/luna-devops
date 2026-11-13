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

var defaultFailureEventTypes = []string{
	"build.failed",
	"build.webhook_rejected",
	"release.failed",
	"runtime.pod_oom_killed",
	"runtime.pod_crash_loop",
	"runtime.pod_start_failed",
	"runtime.workload_degraded",
	"runtime.recovered",
	"hook.failed",
	"gateway.apply_failed",
	"certificate.failed",
	"certificate.expired",
}

const (
	UserEmailChannelID                = "notification:user-email"
	ScopedResourceChannelType         = "notification_channel"
	NotificationFanoutStatusPending   = "pending"
	NotificationFanoutStatusCompleted = "completed"
	DeliveryEnqueueFailedCode         = "notification.delivery_enqueue_failed"
	PersonalDeliveryEnqueueFailedCode = "notification.personal_delivery_enqueue_failed"
)

var (
	ErrEventNotMaterializable = errors.New("platform event is not a notification fanout event")
	ErrDeliveryActive         = errors.New("notification delivery is currently active")
)

type DeliveryEnqueuer interface {
	EnqueueNotificationDeliver(ctx context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error)
}

type Service struct {
	DB       *gorm.DB
	Enqueuer DeliveryEnqueuer
}

type deliveryTarget struct {
	ChannelID          string
	AdapterKind        string
	RuleID             string
	TemplateID         string
	RecipientUserID    string
	TargetOverrideJSON string
	TimeZone           string
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
	if event.Type == "build.webhook_rejected" {
		resourceType, resourceID = "repository_binding", event.Build.ID
	} else if platformevent.CategoryForType(event.Type) == "service_binding" {
		resourceType, resourceID = "service_binding", event.ServiceBinding.ID
	} else if platformevent.CategoryForType(event.Type) == "runtime" && resourceID == "" {
		resourceType, resourceID = "deployment_target", firstNonEmptyString(event.DeploymentTarget.ID, event.Runtime.IncidentID)
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
		// Order project-specific rules first so project locale/timezone/template preferences take precedence.
		query = query.Where("project_id in ?", []string{"", event.Project.ID}).Order("CASE WHEN project_id <> '' THEN 0 ELSE 1 END, created_at asc")
	} else {
		query = query.Where("project_id = ?", "").Order("created_at asc")
	}
	if err := query.Find(&rules).Error; err != nil {
		return nil, err
	}

	targets := make([]deliveryTarget, 0)
	targetIndexByChannel := map[string]int{}
	for _, rule := range rules {
		if !RuleMatchesEvent(rule, event) {
			continue
		}
		channelIDs := decodeStringList(rule.ChannelIDsJSON)
		if len(channelIDs) == 0 {
			continue
		}
		channelTargets, _ := DecodeChannelTargets([]byte(rule.ChannelTargetsJSON))
		var channels []model.NotificationChannel
		if err := db.Where("id in ? and enabled = ? and owner_user_id = ?", channelIDs, true, "").Find(&channels).Error; err != nil {
			return nil, err
		}
		boundProjectChannels := map[string]bool{}
		if strings.TrimSpace(rule.ProjectID) != "" {
			var bindings []model.ScopedResourceProjectBinding
			if err := db.Where(
				"resource_type = ? and project_id = ? and resource_id in ?",
				ScopedResourceChannelType,
				strings.TrimSpace(rule.ProjectID),
				channelIDs,
			).Find(&bindings).Error; err != nil {
				return nil, err
			}
			for _, binding := range bindings {
				boundProjectChannels[binding.ResourceID] = true
			}
		}
		for _, channel := range channels {
			if !channelEligibleForRule(channel, rule.ProjectID, boundProjectChannels) {
				continue
			}
			override := effectiveChannelTargetOverride(channel, channelTargets[channel.ID])
			if channel.AdapterKind == AdapterKindSMTP {
				if _, recipients, err := ApplySMTPTargetOverride(json.RawMessage(channel.ConfigJSON), override); err != nil || len(recipients) == 0 {
					continue
				}
			}
			if existingIdx, exists := targetIndexByChannel[channel.ID]; exists {
				if channel.AdapterKind == AdapterKindSMTP {
					merged := MergeChannelTargetOverride(
						DecodeChannelTargetOverride(targets[existingIdx].TargetOverrideJSON),
						override,
					)
					targets[existingIdx].TargetOverrideJSON = EncodeChannelTargetOverride(merged)
				}
				continue
			}
			template := model.NotificationTemplate{}
			templateID := strings.TrimSpace(rule.TemplateID)
			if templateID != "" {
				_ = db.First(&template, "id = ? and enabled = ?", templateID, true).Error
			}
			if template.ID == "" {
				templateID = ""
			}
			targetIndexByChannel[channel.ID] = len(targets)
			targets = append(targets, deliveryTarget{
				ChannelID:          channel.ID,
				AdapterKind:        channel.AdapterKind,
				RuleID:             rule.ID,
				TemplateID:         templateID,
				TargetOverrideJSON: EncodeChannelTargetOverride(override),
				TimeZone:           NormalizeTimeZone(rule.TimeZone),
			})
		}
	}
	return targets, nil
}

func channelEligibleForRule(channel model.NotificationChannel, ruleProjectID string, boundProjectChannels map[string]bool) bool {
	if strings.TrimSpace(channel.OwnerUserID) != "" {
		return false
	}
	scope := NormalizeChannelScope(channel.Scope)
	ruleProjectID = strings.TrimSpace(ruleProjectID)
	switch scope {
	case "global":
		return true
	case "project":
		if ruleProjectID == "" {
			return false
		}
		if boundProjectChannels[channel.ID] {
			return true
		}
		// Backward-compatible fallback if legacy project_id was populated directly on channel.
		return strings.TrimSpace(channel.ProjectID) == ruleProjectID
	default:
		return false
	}
}

func effectiveChannelTargetOverride(channel model.NotificationChannel, ruleOverride ChannelTargetOverride) ChannelTargetOverride {
	if channel.AdapterKind != AdapterKindSMTP {
		return ChannelTargetOverride{}
	}
	cfg, err := ParseSMTPConfigForValidation(json.RawMessage(channel.ConfigJSON))
	if err != nil {
		return ruleOverride
	}
	effective := ChannelTargetOverride{
		To: cfg.To,
		Cc: cfg.Cc,
	}
	if len(ruleOverride.To) > 0 {
		effective.To = ruleOverride.To
	}
	if len(ruleOverride.Cc) > 0 {
		effective.Cc = ruleOverride.Cc
	}
	return effective
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
		ID:                 id.New("ndl"),
		ProjectID:          event.Project.ID,
		RecipientUserID:    target.RecipientUserID,
		EventID:            event.ID,
		EventType:          event.Type,
		Severity:           event.Severity,
		ChannelID:          target.ChannelID,
		AdapterKind:        target.AdapterKind,
		RuleID:             target.RuleID,
		TemplateID:         target.TemplateID,
		TargetOverrideJSON: firstNonEmptyString(target.TargetOverrideJSON, "{}"),
		TimeZone:           target.TimeZone,
		EventJSON:          string(eventData),
		Traceparent:        storedEvent.FanoutTraceparent,
		Tracestate:         storedEvent.FanoutTracestate,
		Status:             "pending",
		QueuedAt:           now,
		CreatedAt:          now,
		UpdatedAt:          now,
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

func RuleMatchesEvent(rule model.NotificationRule, event Event) bool {
	if !rule.Enabled {
		return false
	}
	return ruleMatchesEvent(rule, event)
}

func ruleMatchesEvent(rule model.NotificationRule, event Event) bool {
	if len(event.EligibleRuleIDs) > 0 && !containsString(event.EligibleRuleIDs, rule.ID) {
		return false
	}
	eventTypes := decodeStringList(rule.EventTypesJSON)
	if len(eventTypes) == 0 {
		return false
	}
	eventTypeMatched := containsString(eventTypes, event.Type)
	if !eventTypeMatched && event.Type == "runtime.recovered" && rule.NotifyOnRecovery && strings.TrimSpace(event.Runtime.IncidentKind) != "" {
		eventTypeMatched = containsString(eventTypes, event.Runtime.IncidentKind)
	}
	if !eventTypeMatched {
		return false
	}
	filter, err := DecodeRuleFilter([]byte(rule.FilterJSON))
	if err != nil {
		return false
	}
	if strings.TrimSpace(rule.ProjectID) != "" {
		if strings.TrimSpace(event.Project.ID) != strings.TrimSpace(rule.ProjectID) {
			return false
		}
		return stringListMatches(filter.Severities, event.Severity) &&
			stringListMatches(filter.ApplicationIDs, event.Application.ID) &&
			stringListMatches(filter.DeploymentTargetIDs, event.DeploymentTarget.ID)
	}
	projectMatches := filter.Scope == RuleScopeAll || containsString(filter.ProjectIDs, event.Project.ID)
	return projectMatches && stringListMatches(filter.Severities, event.Severity) &&
		stringListMatches(filter.ApplicationIDs, event.Application.ID) &&
		stringListMatches(filter.DeploymentTargetIDs, event.DeploymentTarget.ID)
}

func (s Service) RetryDelivery(ctx context.Context, deliveryID string) (model.NotificationDelivery, error) {
	if s.DB == nil {
		return model.NotificationDelivery{}, errors.New("notification database is unavailable")
	}
	db := s.DB.WithContext(ctx)
	var delivery model.NotificationDelivery
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&delivery, "id = ?", strings.TrimSpace(deliveryID)).Error; err != nil {
			return err
		}
		if delivery.Status == "sending" {
			staleBefore := now.Add(-tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout)
			if delivery.StartedAt != nil && delivery.StartedAt.After(staleBefore) {
				return ErrDeliveryActive
			}
		}
		traceContext := telemetry.InjectMap(ctx)
		updates := map[string]any{
			"status":        "pending",
			"attempt_count": 0,
			"error_message": "",
			"queued_at":     now,
			"started_at":    nil,
			"finished_at":   nil,
			"updated_at":    now,
		}
		if tp := strings.TrimSpace(traceContext["traceparent"]); tp != "" {
			updates["traceparent"] = tp
			updates["tracestate"] = traceContext["tracestate"]
			delivery.Traceparent = tp
			delivery.Tracestate = traceContext["tracestate"]
		}
		if err := tx.Model(&model.NotificationDelivery{}).Where("id = ?", delivery.ID).Updates(updates).Error; err != nil {
			return err
		}
		delivery.Status = "pending"
		delivery.AttemptCount = 0
		delivery.ErrorMessage = ""
		delivery.QueuedAt = now
		delivery.StartedAt = nil
		delivery.FinishedAt = nil
		delivery.UpdatedAt = now
		return nil
	})
	if err != nil {
		return model.NotificationDelivery{}, err
	}
	_ = s.EnqueueDeliveries(ctx, []model.NotificationDelivery{delivery})
	_ = db.First(&delivery, "id = ?", delivery.ID).Error
	return delivery, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
