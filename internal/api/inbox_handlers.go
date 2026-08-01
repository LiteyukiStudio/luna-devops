package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
)

const (
	inboxFilterAll    = "all"
	inboxFilterUnread = "unread"
	inboxFilterAction = "action"
)

var inboxCategories = map[string]struct{}{
	"action":   {},
	"project":  {},
	"billing":  {},
	"security": {},
	"delivery": {},
	"system":   {},
}

type inboxActionRequestResponse struct {
	ID               string     `json:"id"`
	Type             string     `json:"type"`
	Status           string     `json:"status"`
	RowVersion       int64      `json:"rowVersion"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	AllowedDecisions []string   `json:"allowedDecisions"`
}

type inboxMessageResponse struct {
	ID              string                      `json:"id"`
	Type            string                      `json:"type"`
	Category        string                      `json:"category"`
	Priority        string                      `json:"priority"`
	ActorID         string                      `json:"actorId"`
	ProjectID       string                      `json:"projectId"`
	ResourceType    string                      `json:"resourceType"`
	ResourceID      string                      `json:"resourceId"`
	TitleKey        string                      `json:"titleKey"`
	ContentKey      string                      `json:"contentKey"`
	Params          map[string]any              `json:"params"`
	ActionRequestID string                      `json:"actionRequestId"`
	DeepLink        string                      `json:"deepLink"`
	GroupKey        string                      `json:"groupKey"`
	ReadAt          *time.Time                  `json:"readAt"`
	ExpiresAt       *time.Time                  `json:"expiresAt"`
	CreatedAt       time.Time                   `json:"createdAt"`
	UpdatedAt       time.Time                   `json:"updatedAt"`
	ActionRequest   *inboxActionRequestResponse `json:"actionRequest,omitempty"`
}

type inboxDecisionInput struct {
	Decision        string `json:"decision"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handlers) ListInboxMessages(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	filter, category, ok := parseInboxFilters(ctx)
	if !ok {
		return
	}
	pagination := normalizedInboxPagination(ctx)
	result, err := h.inboxService().List(ctx.Request.Context(), inbox.ListInput{
		UserID:    user.ID,
		Page:      pagination.Page,
		PageSize:  pagination.PageSize,
		SortBy:    pagination.SortBy,
		SortOrder: pagination.SortOrder,
		Filter:    filter,
		Category:  category,
	})
	if err != nil {
		writeInboxError(ctx, err)
		return
	}

	responses, err := h.inboxMessageResponses(ctx.Request.Context(), user.ID, result.Items)
	if err != nil {
		writeInboxError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, gin.H{
		"items":      responses,
		"page":       result.Page,
		"pageSize":   result.PageSize,
		"sortBy":     result.SortBy,
		"sortOrder":  result.SortOrder,
		"total":      result.Total,
		"totalPages": result.TotalPages,
	})
}

func (h *Handlers) GetInboxUnreadCount(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	count, err := h.inboxService().UnreadCount(ctx.Request.Context(), user.ID)
	if err != nil {
		writeInboxError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, gin.H{"unreadCount": count})
}

func (h *Handlers) GetInboxMessage(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	message, err := h.inboxService().Get(ctx.Request.Context(), user.ID, strings.TrimSpace(ctx.Param("messageId")))
	if err != nil {
		writeInboxError(ctx, err)
		return
	}
	response, err := h.inboxMessageResponse(ctx.Request.Context(), user.ID, message)
	if err != nil {
		writeInboxError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) MarkInboxMessageRead(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if err := h.inboxService().MarkRead(ctx.Request.Context(), user.ID, strings.TrimSpace(ctx.Param("messageId"))); err != nil {
		writeInboxError(ctx, err)
		return
	}
	defaultInboxBroker.Notify(user.ID, strings.TrimSpace(ctx.Param("messageId")))
	ctx.Status(http.StatusNoContent)
	ctx.Writer.WriteHeaderNow()
}

func (h *Handlers) MarkAllInboxMessagesRead(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if err := h.inboxService().MarkAllRead(ctx.Request.Context(), user.ID); err != nil {
		writeInboxError(ctx, err)
		return
	}
	defaultInboxBroker.Notify(user.ID, "")
	ctx.Status(http.StatusNoContent)
	ctx.Writer.WriteHeaderNow()
}

func (h *Handlers) ArchiveInboxMessage(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	messageID := strings.TrimSpace(ctx.Param("messageId"))
	if err := h.inboxService().Archive(ctx.Request.Context(), user.ID, messageID); err != nil {
		writeInboxError(ctx, err)
		return
	}
	defaultInboxBroker.Notify(user.ID, messageID)
	ctx.Status(http.StatusNoContent)
	ctx.Writer.WriteHeaderNow()
}

func (h *Handlers) DecideInboxActionRequest(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input inboxDecisionInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	if (input.Decision != "accept" && input.Decision != "reject") || input.ExpectedVersion < 1 {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.decision_invalid", "decision or expectedVersion is invalid")
		return
	}
	if input.Decision == "accept" && h.configs != nil && !h.requireStepUp(ctx, user, stepUpPurposeBillingOwnerTransfer) {
		return
	}
	if h.inboxDecision == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "inbox.decision_unavailable", "inbox decision handler is unavailable")
		return
	}
	requestID := strings.TrimSpace(ctx.Param("requestId"))
	if requestID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.request_id_required", "requestId is required")
		return
	}
	if err := h.inboxDecision(ctx.Request.Context(), user, requestID, input.Decision, input.ExpectedVersion); err != nil {
		writeInboxError(ctx, err)
		return
	}
	actionRequest, err := h.inboxService().GetActionRequest(ctx.Request.Context(), user.ID, requestID)
	if err != nil {
		writeInboxError(ctx, err)
		return
	}
	defaultInboxBroker.Notify(user.ID, "")
	if actionRequest.RequesterUserID != user.ID {
		defaultInboxBroker.Notify(actionRequest.RequesterUserID, "")
	}
	ctx.JSON(http.StatusOK, inboxActionRequestResponseFor(actionRequest))
}

func (h *Handlers) inboxService() inboxService {
	return h.inbox
}

func (h *Handlers) inboxMessageResponse(ctx context.Context, userID string, message model.InboxMessage) (inboxMessageResponse, error) {
	response := inboxMessageResponseFor(message)
	if strings.TrimSpace(message.ActionRequestID) == "" {
		return response, nil
	}
	request, err := h.inboxService().GetActionRequest(ctx, userID, message.ActionRequestID)
	if err != nil {
		return inboxMessageResponse{}, err
	}
	actionResponse := inboxActionRequestResponseFor(request)
	response.ActionRequest = &actionResponse
	return response, nil
}

func (h *Handlers) inboxMessageResponses(ctx context.Context, userID string, messages []model.InboxMessage) ([]inboxMessageResponse, error) {
	requestIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		if requestID := strings.TrimSpace(message.ActionRequestID); requestID != "" {
			requestIDs = append(requestIDs, requestID)
		}
	}
	requests, err := h.inboxService().GetActionRequests(ctx, userID, requestIDs)
	if err != nil {
		return nil, err
	}
	responses := make([]inboxMessageResponse, 0, len(messages))
	for _, message := range messages {
		response := inboxMessageResponseFor(message)
		if request, exists := requests[message.ActionRequestID]; exists {
			actionResponse := inboxActionRequestResponseFor(request)
			response.ActionRequest = &actionResponse
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func inboxMessageResponseFor(message model.InboxMessage) inboxMessageResponse {
	params := map[string]any{}
	if err := json.Unmarshal([]byte(message.ParamsJSON), &params); err != nil || params == nil {
		params = map[string]any{}
	}
	return inboxMessageResponse{
		ID:              message.ID,
		Type:            message.Type,
		Category:        message.Category,
		Priority:        message.Priority,
		ActorID:         message.ActorID,
		ProjectID:       message.ProjectID,
		ResourceType:    message.ResourceType,
		ResourceID:      message.ResourceID,
		TitleKey:        message.TitleKey,
		ContentKey:      message.ContentKey,
		Params:          params,
		ActionRequestID: message.ActionRequestID,
		DeepLink:        message.DeepLink,
		GroupKey:        message.GroupKey,
		ReadAt:          message.ReadAt,
		ExpiresAt:       message.ExpiresAt,
		CreatedAt:       message.CreatedAt,
		UpdatedAt:       message.UpdatedAt,
	}
}

func inboxActionRequestResponseFor(request model.InboxActionRequest) inboxActionRequestResponse {
	allowed := []string{}
	if request.Status == "pending" {
		allowed = []string{"accept", "reject"}
	}
	return inboxActionRequestResponse{
		ID:               request.ID,
		Type:             request.Type,
		Status:           request.Status,
		RowVersion:       request.RowVersion,
		ExpiresAt:        request.ExpiresAt,
		AllowedDecisions: allowed,
	}
}

func parseInboxFilters(ctx *gin.Context) (string, string, bool) {
	filter := strings.ToLower(strings.TrimSpace(ctx.DefaultQuery("filter", inboxFilterAll)))
	if filter != inboxFilterAll && filter != inboxFilterUnread && filter != inboxFilterAction {
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.filter_invalid", "unsupported inbox filter")
		return "", "", false
	}
	category := strings.ToLower(strings.TrimSpace(ctx.Query("category")))
	if category != "" {
		if _, exists := inboxCategories[category]; !exists {
			writeErrorCode(ctx, http.StatusBadRequest, "inbox.category_invalid", "unsupported inbox category")
			return "", "", false
		}
	}
	return filter, category, true
}

func normalizedInboxPagination(ctx *gin.Context) paginationParams {
	pagination := paginationFromQuery(ctx)
	switch strings.TrimSpace(pagination.SortBy) {
	case "createdAt", "updatedAt", "priority":
	default:
		pagination.SortBy = "createdAt"
	}
	return pagination
}

func writeInboxError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, inbox.ErrInvalidInput), errors.Is(err, inbox.ErrInvalidDeepLink):
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.request_invalid", "inbox request is invalid")
	case errors.Is(err, inbox.ErrNotFound):
		writeErrorCode(ctx, http.StatusNotFound, "inbox.not_found", "inbox resource was not found")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferInvalid):
		writeErrorCode(ctx, http.StatusBadRequest, "inbox.billing_owner_transfer_invalid", "billing owner transfer input is invalid")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferForbidden):
		writeErrorCode(ctx, http.StatusForbidden, "inbox.billing_owner_transfer_forbidden", "billing owner transfer is forbidden")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferConflict):
		writeErrorCode(ctx, http.StatusConflict, "inbox.billing_owner_transfer_conflict", "billing owner transfer conflicts with current state")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferStale):
		writeErrorCode(ctx, http.StatusConflict, "inbox.billing_owner_transfer_stale", "billing owner transfer request is stale")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferNotFound):
		writeErrorCode(ctx, http.StatusNotFound, "inbox.billing_owner_transfer_not_found", "billing owner transfer request was not found")
	case errors.Is(err, projectservice.ErrBillingOwnerTransferExpired):
		writeErrorCode(ctx, http.StatusGone, "inbox.billing_owner_transfer_expired", "billing owner transfer request expired")
	default:
		writeErrorCode(ctx, http.StatusInternalServerError, "inbox.operation_failed", "inbox operation failed")
	}
}
