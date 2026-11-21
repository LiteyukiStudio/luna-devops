package notificationapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
)

type opsListInboxInput struct {
	Filter    *string `json:"filter,omitempty" enum:"all|unread|action" default:"all" desc:"过滤范围：all（全部未归档）、unread（未读）、action（待处理请求）"`
	Category  *string `json:"category,omitempty" enum:"action|project|billing|security|delivery|system" desc:"按消息类别过滤"`
	MessageID *string `json:"messageId,omitempty" desc:"精确读取单条未归档站内信详情及 actionRequest.rowVersion"`
	Page      *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize  *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type opsMarkInboxReadInput struct {
	MarkAll   *bool   `json:"markAll,omitempty" desc:"设为 true 时将当前用户全部未读站内信标记为已读"`
	MessageID *string `json:"messageId,omitempty" desc:"markAll 未设为 true 时必填，指定标记已读的站内信 ID"`
}

type opsArchiveInboxMessageInput struct {
	MessageID string `json:"messageId" desc:"待归档隐藏的站内信 ID"`
}

type opsDecideInboxActionInput struct {
	RequestID       string `json:"requestId" desc:"待办确认请求 ActionRequest ID"`
	Decision        string `json:"decision" enum:"accept|reject" desc:"确认决策：accept 或 reject"`
	ExpectedVersion int64  `json:"expectedVersion" min:"1" desc:"乐观锁版本号 rowVersion（从 ops_list_inbox 获取）"`
}

type lockedInboxActionDecision struct {
	RequestID       string `json:"requestId"`
	Decision        string `json:"decision"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) registerOpsInboxAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsListInboxInput, map[string]any]{
		Name:           "ops_list_inbox",
		Domain:         aitool.DomainOps,
		Title:          "查看个人站内信与待办",
		Summary:        "查看当前用户的站内信列表、未读总数及单条待办请求（ActionRequest）的乐观锁版本 rowVersion",
		Tags:           []string{"inbox", "notifications"},
		AliasesZH:      []string{"查看站内信", "未读消息", "查看待办确认请求"},
		AliasesEN:      []string{"list inbox", "get inbox message", "unread inbox"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionUserRead)},
		Execute:        h.aiOpsListInbox,
	})

	aitool.Register(reg, aitool.Spec[opsMarkInboxReadInput, map[string]any]{
		Name:           "ops_mark_inbox_read",
		Domain:         aitool.DomainOps,
		Title:          "标记站内信已读",
		Summary:        "幂等标记单条或全部未读站内信为已读",
		Tags:           []string{"inbox"},
		AliasesZH:      []string{"站内信标为已读", "全部标为已读"},
		AliasesEN:      []string{"mark inbox read", "mark all inbox read"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Idempotent:     true,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_inbox",
				Family:          "resource",
				WhenResultHas:   "messageId",
				IDFrom:          "result.messageId",
				ReadIDArgument:  "messageId",
				StaticArguments: map[string]any{"filter": "all"},
				Expect:          "present",
			},
			{
				OperationID:     "ops_list_inbox",
				Family:          "resource",
				Singleton:       true,
				StaticArguments: map[string]any{"filter": "unread"},
				Expect:          "present",
			},
		},
		Execute: h.aiOpsMarkInboxRead,
	})

	aitool.Register(reg, aitool.Spec[opsArchiveInboxMessageInput, map[string]any]{
		Name:           "ops_archive_inbox_message",
		Domain:         aitool.DomainOps,
		Title:          "归档隐藏站内信",
		Summary:        "归档隐藏单条站内信（归档后不可恢复取消）",
		Tags:           []string{"inbox"},
		AliasesZH:      []string{"归档站内信", "隐藏站内信"},
		AliasesEN:      []string{"archive inbox message"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_inbox",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "messageId",
				StaticArguments: map[string]any{"filter": "all"},
				Expect:          "absent",
			},
		},
		Preview: h.aiPreviewOpsArchiveInboxMessage,
		Execute: h.aiExecuteOpsArchiveInboxMessage,
	})

	aitool.Register(reg, aitool.Spec[opsDecideInboxActionInput, map[string]any]{
		Name:           "ops_decide_inbox_action",
		Domain:         aitool.DomainOps,
		Title:          "处理站内信待办确认请求",
		Summary:        "接受或拒绝站内信中的待办确认请求（如项目计费归属转移）",
		Tags:           []string{"inbox", "billing"},
		AliasesZH:      []string{"确认待办请求", "接受计费归属转移", "拒绝站内信请求"},
		AliasesEN:      []string{"decide inbox action", "accept billing transfer"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionUserWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_inbox",
				Family:          "resource",
				WhenResultHas:   "messageId",
				IDFrom:          "result.messageId",
				ReadIDArgument:  "messageId",
				StaticArguments: map[string]any{"filter": "all"},
				Expect:          "present",
			},
			{
				OperationID:     "ops_list_inbox",
				Family:          "resource",
				Singleton:       true,
				StaticArguments: map[string]any{"filter": "all"},
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewOpsDecideInboxAction,
		Execute: h.aiExecuteOpsDecideInboxAction,
	})
}

func (h *Handler) aiOpsListInbox(tc *aitool.ToolContext, in opsListInboxInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	svc := h.inboxService()
	if svc == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "inbox.service_unavailable", "inbox service is unavailable")
	}
	tc.SetNoStore()

	filter := strings.ToLower(notifStringOrDefault(in.Filter, inboxFilterAll))
	if filter != inboxFilterAll && filter != inboxFilterUnread && filter != inboxFilterAction {
		return nil, aitool.BadRequest("inbox.filter_invalid", "filter must be all, unread, or action")
	}
	category := ""
	if in.Category != nil && strings.TrimSpace(*in.Category) != "" {
		category = strings.ToLower(strings.TrimSpace(*in.Category))
		if _, ok := inboxCategories[category]; !ok {
			return nil, aitool.BadRequest("inbox.category_invalid", "unsupported inbox category")
		}
	}

	unreadCount, err := svc.UnreadCount(tc.Context(), user.ID)
	if err != nil {
		return nil, mapInboxToolError(err)
	}

	if in.MessageID != nil && strings.TrimSpace(*in.MessageID) != "" {
		msgID := strings.TrimSpace(*in.MessageID)
		msg, getErr := svc.Get(tc.Context(), user.ID, msgID)
		if getErr != nil {
			return nil, mapInboxToolError(getErr)
		}
		if msg.ArchivedAt != nil {
			return nil, aitool.NotFound("inbox.not_found", "inbox message was not found")
		}
		resp, respErr := h.inboxMessageResponse(tc.Context(), user.ID, msg)
		if respErr != nil {
			return nil, mapInboxToolError(respErr)
		}
		return map[string]any{
			"id":          resp.ID,
			"message":     resp,
			"items":       []InboxMessageResponse{resp},
			"unreadCount": unreadCount,
			"page":        1,
			"pageSize":    1,
			"sortBy":      "createdAt",
			"sortOrder":   "desc",
			"total":       int64(1),
			"totalPages":  1,
		}, nil
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	listRes, err := svc.List(tc.Context(), inbox.ListInput{
		UserID:    user.ID,
		Page:      page,
		PageSize:  pageSize,
		SortBy:    "createdAt",
		SortOrder: "desc",
		Filter:    filter,
		Category:  category,
	})
	if err != nil {
		return nil, mapInboxToolError(err)
	}
	items, err := h.inboxMessageResponses(tc.Context(), user.ID, listRes.Items)
	if err != nil {
		return nil, mapInboxToolError(err)
	}

	env := aitool.PageEnvelope(items, listRes.Page, listRes.PageSize, listRes.SortBy, listRes.SortOrder, listRes.Total)
	env["unreadCount"] = unreadCount
	return env, nil
}

func (h *Handler) aiOpsMarkInboxRead(tc *aitool.ToolContext, in opsMarkInboxReadInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	svc := h.inboxService()
	if svc == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "inbox.service_unavailable", "inbox service is unavailable")
	}

	markAll := notifBoolOrDefault(in.MarkAll, false)
	msgID := ""
	if in.MessageID != nil {
		msgID = strings.TrimSpace(*in.MessageID)
	}
	if markAll && msgID != "" {
		return nil, aitool.BadRequest("inbox.request_invalid", "markAll=true and messageId are mutually exclusive")
	}
	if !markAll && msgID == "" {
		return nil, aitool.BadRequest("inbox.request_invalid", "messageId is required when markAll is not true")
	}

	if markAll {
		if err := svc.MarkAllRead(tc.Context(), user.ID); err != nil {
			return nil, mapInboxToolError(err)
		}
		defaultInboxBroker.Notify(user.ID, "")
	} else {
		msg, getErr := svc.Get(tc.Context(), user.ID, msgID)
		if getErr != nil {
			return nil, mapInboxToolError(getErr)
		}
		if msg.ArchivedAt != nil {
			return nil, aitool.NotFound("inbox.not_found", "inbox message was not found")
		}
		if err := svc.MarkRead(tc.Context(), user.ID, msgID); err != nil {
			return nil, mapInboxToolError(err)
		}
		defaultInboxBroker.Notify(user.ID, msgID)
	}

	unreadCount, err := svc.UnreadCount(tc.Context(), user.ID)
	if err != nil {
		return nil, mapInboxToolError(err)
	}
	out := map[string]any{
		"markedAll":   markAll,
		"unreadCount": unreadCount,
	}
	if msgID != "" {
		out["messageId"] = msgID
	}
	return out, nil
}

func (h *Handler) aiPreviewOpsArchiveInboxMessage(tc *aitool.ToolContext, in opsArchiveInboxMessageInput) (*aitool.PreviewResult, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	svc := h.inboxService()
	if svc == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "inbox.service_unavailable", "inbox service is unavailable")
	}
	msgID := strings.TrimSpace(in.MessageID)
	if msgID == "" {
		return nil, aitool.BadRequest("inbox.request_invalid", "messageId is required")
	}
	msg, err := svc.Get(tc.Context(), user.ID, msgID)
	if err != nil {
		return nil, mapInboxToolError(err)
	}
	if msg.ArchivedAt != nil {
		return nil, aitool.NotFound("inbox.not_found", "inbox message was not found")
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Archive inbox message %s (%s)", msg.ID, msg.Type),
			TargetResource: "inbox_message",
			TargetID:       msg.ID,
			Changes: []aitool.PreviewChange{
				{Field: "archived", Before: false, After: true, Action: "archive"},
			},
			Warnings: []string{"Archived inbox messages are permanently hidden and cannot be unarchived."},
		},
		LockedPayload: map[string]string{"messageId": msg.ID},
	}, nil
}

func (h *Handler) aiExecuteOpsArchiveInboxMessage(tc *aitool.ToolContext, in opsArchiveInboxMessageInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	svc := h.inboxService()
	if svc == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "inbox.service_unavailable", "inbox service is unavailable")
	}
	msgID := strings.TrimSpace(in.MessageID)
	msg, err := svc.Get(tc.Context(), user.ID, msgID)
	if err != nil {
		return nil, mapInboxToolError(err)
	}
	if msg.ArchivedAt != nil {
		return nil, aitool.NotFound("inbox.not_found", "inbox message was not found")
	}
	if err := svc.Archive(tc.Context(), user.ID, msgID); err != nil {
		return nil, mapInboxToolError(err)
	}
	defaultInboxBroker.Notify(user.ID, msgID)
	h.auditWithContext(user.ID, "inbox.message.archive", msgID, true, "", tc.Context())

	return map[string]any{
		"id":       msgID,
		"archived": true,
	}, nil
}

func (h *Handler) validateInboxDecisionState(tc *aitool.ToolContext, user model.User, in opsDecideInboxActionInput) (model.InboxActionRequest, error) {
	decision := strings.ToLower(strings.TrimSpace(in.Decision))
	if (decision != "accept" && decision != "reject") || in.ExpectedVersion < 1 {
		return model.InboxActionRequest{}, aitool.BadRequest("inbox.decision_invalid", "decision must be accept or reject and expectedVersion must be >= 1")
	}
	if !h.inboxDecisionAvailable() {
		return model.InboxActionRequest{}, aitool.NewError(http.StatusServiceUnavailable, "inbox.decision_unavailable", "inbox decision handler is unavailable")
	}
	svc := h.inboxService()
	if svc == nil {
		return model.InboxActionRequest{}, aitool.NewError(http.StatusServiceUnavailable, "inbox.service_unavailable", "inbox service is unavailable")
	}
	reqID := strings.TrimSpace(in.RequestID)
	if reqID == "" {
		return model.InboxActionRequest{}, aitool.BadRequest("inbox.request_id_required", "requestId is required")
	}
	actionReq, err := svc.GetActionRequest(tc.Context(), user.ID, reqID)
	if err != nil {
		return model.InboxActionRequest{}, mapInboxToolError(err)
	}
	if boundProj := strings.TrimSpace(tc.ConversationProjectID); boundProj != "" && strings.TrimSpace(actionReq.ProjectID) != boundProj {
		return model.InboxActionRequest{}, aitool.Forbidden("ai.project_scope_mismatch", "inbox action request belongs to another project")
	}
	if actionReq.ExpiresAt != nil && !actionReq.ExpiresAt.After(time.Now()) {
		return model.InboxActionRequest{}, aitool.NewError(http.StatusGone, "inbox.billing_owner_transfer_expired", "inbox action request has expired")
	}
	if actionReq.Status != "pending" || actionReq.RowVersion != in.ExpectedVersion {
		return model.InboxActionRequest{}, aitool.NewErrorWithHint(
			http.StatusConflict,
			"inbox.billing_owner_transfer_stale",
			fmt.Sprintf("action request %s status=%q rowVersion=%d does not match expected pending version %d", actionReq.ID, actionReq.Status, actionReq.RowVersion, in.ExpectedVersion),
			"Call ops_list_inbox to inspect the latest status and rowVersion before retrying.",
		)
	}
	return actionReq, nil
}

func (h *Handler) aiPreviewOpsDecideInboxAction(tc *aitool.ToolContext, in opsDecideInboxActionInput) (*aitool.PreviewResult, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	actionReq, err := h.validateInboxDecisionState(tc, user, in)
	if err != nil {
		return nil, err
	}
	decision := strings.ToLower(strings.TrimSpace(in.Decision))

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("%s inbox action request %s (type=%s, expectedVersion=%d)", strings.ToUpper(decision), actionReq.ID, actionReq.Type, actionReq.RowVersion),
			TargetResource: "inbox_action_request",
			TargetID:       actionReq.ID,
			ProjectID:      actionReq.ProjectID,
			Changes: []aitool.PreviewChange{
				{Field: "decision", Before: actionReq.Status, After: decision, Action: decision},
				{Field: "expectedVersion", Before: actionReq.RowVersion, After: actionReq.RowVersion + 1, Action: "lock"},
			},
		},
		LockedPayload: lockedInboxActionDecision{
			RequestID:       actionReq.ID,
			Decision:        decision,
			ExpectedVersion: actionReq.RowVersion,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsDecideInboxAction(tc *aitool.ToolContext, in opsDecideInboxActionInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	decision := strings.ToLower(strings.TrimSpace(in.Decision))
	expectedVersion := in.ExpectedVersion
	requestID := strings.TrimSpace(in.RequestID)

	var locked lockedInboxActionDecision
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ExpectedVersion >= 1 {
		expectedVersion = locked.ExpectedVersion
		if locked.RequestID != "" {
			requestID = locked.RequestID
		}
		if locked.Decision != "" {
			decision = locked.Decision
		}
	}

	if err := h.decideInboxAction(tc.Context(), user, requestID, decision, expectedVersion); err != nil {
		return nil, mapInboxToolError(err)
	}
	actionReq, err := h.inboxService().GetActionRequest(tc.Context(), user.ID, requestID)
	if err != nil {
		return nil, mapInboxToolError(err)
	}

	defaultInboxBroker.Notify(user.ID, "")
	if actionReq.RequesterUserID != user.ID {
		defaultInboxBroker.Notify(actionReq.RequesterUserID, "")
	}

	out := map[string]any{
		"id":         actionReq.ID,
		"requestId":  actionReq.ID,
		"type":       actionReq.Type,
		"status":     actionReq.Status,
		"rowVersion": actionReq.RowVersion,
		"expiresAt":  actionReq.ExpiresAt,
	}
	if strings.TrimSpace(actionReq.ProjectID) != "" {
		out["projectId"] = actionReq.ProjectID
	}

	db := h.aiDB(tc)
	if db != nil {
		var linkedMsg model.InboxMessage
		if err := db.Where("recipient_user_id = ? AND action_request_id = ? AND archived_at IS NULL", user.ID, actionReq.ID).
			Order("created_at desc").
			First(&linkedMsg).Error; err == nil && linkedMsg.ID != "" {
			out["messageId"] = linkedMsg.ID
		}
	}
	return out, nil
}
