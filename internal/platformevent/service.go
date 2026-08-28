package platformevent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusInProgress = "in_progress"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusCanceled   = "canceled"
)

type RecordInput struct {
	ID                  string
	Type                string
	Severity            string
	ProjectID           string
	ApplicationID       string
	DeploymentTargetID  string
	ResourceType        string
	ResourceID          string
	ActorID             string
	ResourceOwnerUserID string
	Message             string
	Detail              any
	Links               map[string]string
	CorrelationID       string
	TraceID             string
	DedupKey            string
	OccurredAt          time.Time
}

type Service struct {
	DB *gorm.DB
}

func (s Service) Record(ctx context.Context, input RecordInput) (model.PlatformEvent, bool, error) {
	event := NewRecord(input)
	return s.record(ctx, event)
}

// RecordNotification persists the authoritative event and marks only events
// emitted through the notification service for durable fanout processing.
func (s Service) RecordNotification(ctx context.Context, input RecordInput, traceparent string, tracestate string) (model.PlatformEvent, bool, error) {
	event := NewRecord(input)
	event.NotificationFanoutStatus = "pending"
	event.FanoutTraceparent = traceparent
	event.FanoutTracestate = tracestate
	return s.record(ctx, event)
}

func (s Service) record(ctx context.Context, event model.PlatformEvent) (model.PlatformEvent, bool, error) {
	if s.DB == nil {
		return event, false, nil
	}

	result := s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
	if result.Error != nil {
		return model.PlatformEvent{}, false, result.Error
	}
	if result.RowsAffected > 0 {
		return event, true, nil
	}
	if event.DedupKey != nil {
		var existing model.PlatformEvent
		if err := s.DB.WithContext(ctx).First(&existing, "dedup_key = ?", *event.DedupKey).Error; err == nil {
			return existing, false, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return model.PlatformEvent{}, false, err
		}
	}
	var existing model.PlatformEvent
	if err := s.DB.WithContext(ctx).First(&existing, "id = ?", event.ID).Error; err != nil {
		return model.PlatformEvent{}, false, err
	}
	return existing, false, nil
}

func NewRecord(input RecordInput) model.PlatformEvent {
	eventType := strings.TrimSpace(input.Type)
	eventID := strings.TrimSpace(input.ID)
	if eventID == "" {
		eventID = id.New("evt")
	}
	severity := strings.TrimSpace(input.Severity)
	if severity == "" {
		severity = "info"
	}
	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	detailJSON := redactSensitiveText(marshalJSON(input.Detail, "{}"))
	linksJSON := redactSensitiveText(marshalJSON(input.Links, "{}"))
	var dedupKey *string
	if value := strings.TrimSpace(input.DedupKey); value != "" {
		dedupKey = &value
	}
	return model.PlatformEvent{
		ID:                  eventID,
		Type:                eventType,
		Category:            CategoryForType(eventType),
		Severity:            severity,
		Status:              StatusForType(eventType),
		ProjectID:           strings.TrimSpace(input.ProjectID),
		ApplicationID:       strings.TrimSpace(input.ApplicationID),
		DeploymentTargetID:  strings.TrimSpace(input.DeploymentTargetID),
		ResourceType:        strings.TrimSpace(input.ResourceType),
		ResourceID:          strings.TrimSpace(input.ResourceID),
		ActorID:             strings.TrimSpace(input.ActorID),
		ResourceOwnerUserID: strings.TrimSpace(input.ResourceOwnerUserID),
		SummaryKey:          "events.types." + strings.ReplaceAll(eventType, ".", "_"),
		Message:             redactSensitiveText(strings.TrimSpace(input.Message)),
		DetailJSON:          detailJSON,
		LinksJSON:           linksJSON,
		CorrelationID:       strings.TrimSpace(input.CorrelationID),
		TraceID:             strings.TrimSpace(input.TraceID),
		DedupKey:            dedupKey,
		OccurredAt:          occurredAt,
		CreatedAt:           time.Now(),
	}
}

func CategoryForType(eventType string) string {
	category, _, _ := strings.Cut(strings.TrimSpace(eventType), ".")
	if category == "" {
		return "other"
	}
	return category
}

func StatusForType(eventType string) string {
	category, result, _ := strings.Cut(strings.TrimSpace(eventType), ".")
	if category == "service_binding" {
		switch result {
		case "invalid":
			return StatusFailed
		case "created", "updated", "deleted", "recovered":
			return StatusSucceeded
		}
	}
	switch result {
	case "queued", "started", "pending", "created", "updated":
		return StatusInProgress
	case "succeeded", "applied", "issued", "renewed":
		return StatusSucceeded
	case "failed", "apply_failed", "expired":
		return StatusFailed
	case "canceled":
		return StatusCanceled
	default:
		return StatusSucceeded
	}
}

func ResourceForType(eventType string, buildID string, releaseID string, hookID string, gatewayID string, certificateID string) (string, string) {
	switch CategoryForType(eventType) {
	case "build":
		return "build", strings.TrimSpace(buildID)
	case "release":
		return "release", strings.TrimSpace(releaseID)
	case "hook":
		return "hook", strings.TrimSpace(hookID)
	case "gateway":
		return "gateway", strings.TrimSpace(gatewayID)
	case "certificate":
		return "certificate", strings.TrimSpace(certificateID)
	default:
		return CategoryForType(eventType), ""
	}
}

func marshalJSON(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}
