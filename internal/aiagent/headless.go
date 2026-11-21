package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HeadlessTaskType string

const (
	HeadlessTaskIncidentDiagnosis        HeadlessTaskType = "incident_diagnosis"
	HeadlessTaskDeliveryFailureDiagnosis HeadlessTaskType = "delivery_failure_diagnosis"
	HeadlessTaskRepoConfigInfer          HeadlessTaskType = "repo_config_infer"
)

type HeadlessToolTier string

const (
	HeadlessToolTierNone HeadlessToolTier = "none"
	HeadlessToolTierL0   HeadlessToolTier = "L0"
)

type HeadlessInferenceRequest struct {
	ActorUserID    string           `json:"actorUserId,omitempty"`
	ProjectID      string           `json:"projectId,omitempty"`
	TaskType       HeadlessTaskType `json:"taskType"`
	ContextPayload map[string]any   `json:"contextPayload"`
	MaxToolTier    HeadlessToolTier `json:"maxToolTier,omitempty"`
	MaxSteps       int              `json:"maxSteps,omitempty"`
	ModelID        string           `json:"modelId,omitempty"`
	OutputSchema   map[string]any   `json:"outputSchema,omitempty"`
}

type HeadlessUsage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}

type HeadlessInferenceResponse struct {
	RunID     string           `json:"runId"`
	TaskType  HeadlessTaskType `json:"taskType"`
	ModelID   string           `json:"modelId"`
	ModelName string           `json:"modelName"`
	Output    json.RawMessage  `json:"output"`
	Usage     HeadlessUsage    `json:"usage"`
}

type UpstreamError struct {
	StatusCode int
	Code       string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("ai agent returned %d (%s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("ai agent returned %d", e.StatusCode)
}

const maxHeadlessResponseBytes = 512 * 1024

func RunHeadlessInference(
	ctx context.Context,
	client Client,
	actor ActorContext,
	input HeadlessInferenceRequest,
) (*HeadlessInferenceResponse, error) {
	if client == nil {
		return nil, ErrUnavailable
	}
	if strings.TrimSpace(actor.UserID) == "" {
		return nil, errors.New("aiagent: actor UserID is required")
	}
	if strings.TrimSpace(actor.SessionID) == "" {
		actor.SessionID = "headless:" + strings.TrimSpace(actor.UserID)
	}
	if strings.TrimSpace(actor.Locale) == "" {
		actor.Locale = "zh-CN"
	}
	if strings.TrimSpace(actor.RequestID) == "" {
		actor.RequestID = "req_headless"
	}
	if input.ActorUserID == "" {
		input.ActorUserID = actor.UserID
	}
	if input.ProjectID == "" && actor.ProjectID != "" {
		input.ProjectID = actor.ProjectID
	}
	if input.MaxToolTier == "" {
		input.MaxToolTier = HeadlessToolTierNone
	}
	if input.ContextPayload == nil {
		input.ContextPayload = map[string]any{}
	}

	rawBody, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	callCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		callCtx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	resp, err := client.Do(callCtx, actor, Request{
		Method:             http.MethodPost,
		Path:               "/internal/v1/inference",
		Body:               rawBody,
		ContentType:        "application/json",
		UseContextDeadline: true,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxHeadlessResponseBytes))
	if readErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, readErr)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(respBytes, &envelope)
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Code:       strings.TrimSpace(envelope.Error.Code),
		}
	}

	var parsed HeadlessInferenceResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("%w: invalid inference payload: %v", ErrUnavailable, err)
	}
	return &parsed, nil
}
