package notificationapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type opsListProjectNotificationsInput struct {
	Project                 *string `json:"project,omitempty" desc:"项目 ID 或 identifier（overview/rules/deliveries/presets 必填；仅全局会话且 section=channels 时可省略以跨项目盘点可管理的项目渠道）"`
	Section                 *string `json:"section,omitempty" enum:"overview|channels|rules|deliveries|presets" default:"overview" desc:"查询子视图"`
	IncludeBindableChannels *bool   `json:"includeBindableChannels,omitempty" default:"false" desc:"全局会话下 section=channels 且传入 project 时，额外返回其他项目可并集复用的候选渠道"`
	Channel                 *string `json:"channel,omitempty" desc:"channels/deliveries 按渠道 ID 或名称精确过滤"`
	Rule                    *string `json:"rule,omitempty" desc:"rules/deliveries 按规则 ID 或名称精确过滤"`
	Status                  *string `json:"status,omitempty" desc:"deliveries 按投递状态筛选（如 pending|sending|succeeded|failed|enqueue_failed）"`
	EventType               *string `json:"eventType,omitempty" desc:"deliveries 按事件类型筛选"`
	EventID                 *string `json:"eventId,omitempty" desc:"deliveries 按关联平台事件 ID 筛选"`
	Page                    *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize                *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type opsSaveProjectNotificationChannelInput struct {
	Project          string            `json:"project" desc:"项目 ID 或 identifier"`
	Channel          *string           `json:"channel,omitempty" desc:"省略为新建；传入渠道 ID 或唯一名称为增量更新或将共享渠道并集绑定到当前项目"`
	Name             *string           `json:"name,omitempty" desc:"渠道名称（新建必填）"`
	PresetID         *string           `json:"presetId,omitempty" enum:"feishu-bot|lark-bot|wecom-bot|gotify|dingtalk-bot|slack-webhook|discord-webhook" desc:"7 类预设之一（自定义 Webhook 或 SMTP 省略；创建后不可跨类型变更）"`
	AdapterKind      *string           `json:"adapterKind,omitempty" enum:"webhook|smtp" desc:"适配器类型（默认 webhook）"`
	UpsertConfig     map[string]any    `json:"upsertConfig,omitempty" desc:"增量合并自定义渠道非敏感配置（严禁明文密码或敏感鉴权头字面量）"`
	RemoveConfigKeys []string          `json:"removeConfigKeys,omitempty" desc:"移除指定可选配置键"`
	UpsertSecrets    map[string]string `json:"upsertSecrets,omitempty" sensitive:"true" desc:"增量写入签名密钥、Token 或 SMTP password"`
	ClearSecretKeys  []string          `json:"clearSecretKeys,omitempty" desc:"显式清除指定密钥键"`
	Enabled          *bool             `json:"enabled,omitempty" desc:"是否启用（新建默认 true）"`
}

type opsUnbindProjectNotificationChannelInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Channel string `json:"channel" desc:"待从当前项目解绑的共享通知渠道 ID 或名称"`
}

type opsDeleteProjectNotificationChannelInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Channel string `json:"channel" desc:"待彻底删除的单项目独占通知渠道 ID 或名称"`
}

type projectRuleFilterPatchInput struct {
	SetSeverities      []string `json:"setSeverities,omitempty" enum:"info|warning|error" desc:"设置过滤的事件严重级别"`
	ClearSeverities    *bool    `json:"clearSeverities,omitempty" desc:"清空严重级别过滤条件"`
	AddApplications    []string `json:"addApplications,omitempty" desc:"增量添加应用过滤（支持应用 ID、identifier 或名称）"`
	RemoveApplications []string `json:"removeApplications,omitempty" desc:"增量移除应用过滤"`
	ClearApplications  *bool    `json:"clearApplications,omitempty" desc:"清空应用过滤条件"`
	AddTargets         []string `json:"addTargets,omitempty" desc:"增量添加部署目标过滤（支持 dplt_xxx 或 <appIdentifier>:<stage>）"`
	RemoveTargets      []string `json:"removeTargets,omitempty" desc:"增量移除部署目标过滤"`
	ClearTargets       *bool    `json:"clearTargets,omitempty" desc:"清空部署目标过滤条件"`
	ClearFilter        *bool    `json:"clearFilter,omitempty" desc:"重置为整项目范围过滤"`
}

type opsSaveProjectNotificationRuleInput struct {
	Project               string                        `json:"project" desc:"项目 ID 或 identifier"`
	Rule                  *string                       `json:"rule,omitempty" desc:"省略为新建；传入规则 ID 或同项目唯一名称为增量更新"`
	Name                  *string                       `json:"name,omitempty" desc:"规则名称（新建必填）"`
	AddEventTypes         []string                      `json:"addEventTypes,omitempty" desc:"新增订阅的事件类型"`
	RemoveEventTypes      []string                      `json:"removeEventTypes,omitempty" desc:"移除已订阅的事件类型"`
	AddChannels           []string                      `json:"addChannels,omitempty" desc:"新增投递渠道 ID 或名称"`
	RemoveChannels        []string                      `json:"removeChannels,omitempty" desc:"移除投递渠道"`
	UpsertChannelTargets  []ruleChannelTargetPatchInput `json:"upsertChannelTargets,omitempty" desc:"配置 SMTP 渠道收件人覆盖"`
	RemoveChannelTargets  []string                      `json:"removeChannelTargets,omitempty" desc:"按渠道移除收件人覆盖"`
	Filter                *projectRuleFilterPatchInput  `json:"filter,omitempty" desc:"纯增量规则过滤条件"`
	Locale                *string                       `json:"locale,omitempty" enum:"zh-CN|en-US" desc:"通知语言"`
	TimeZone              *string                       `json:"timeZone,omitempty" desc:"IANA 时区名"`
	RemindIntervalMinutes *int                          `json:"remindIntervalMinutes,omitempty" min:"0" max:"10080" desc:"重复提醒间隔分钟数（0..10080，新建默认 60）"`
	MaxReminders          *int                          `json:"maxReminders,omitempty" min:"0" max:"100" desc:"最大提醒次数（0..100，新建默认 3）"`
	NotifyOnRecovery      *bool                         `json:"notifyOnRecovery,omitempty" desc:"恢复时是否发送通知"`
	Enabled               *bool                         `json:"enabled,omitempty" desc:"是否启用（新建默认 true）"`
}

type opsDeleteProjectNotificationRuleInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Rule    string `json:"rule" desc:"项目通知规则 ID 或唯一名称"`
}

type opsTestOrRetryProjectNotificationInput struct {
	Project         string  `json:"project" desc:"项目 ID 或 identifier"`
	Channel         *string `json:"channel,omitempty" desc:"测试已绑定至当前项目的项目级通知渠道 ID 或名称（与 retryDeliveryId 互斥）"`
	TestRecipient   *string `json:"testRecipient,omitempty" desc:"SMTP 渠道测试收件邮箱"`
	RetryDeliveryID *string `json:"retryDeliveryId,omitempty" desc:"重试归属当前项目的项目通知失败投递记录 ID（与 channel 互斥）"`
}

type lockedProjectNotifMutation struct {
	ProjectID         string `json:"projectId"`
	ResolvedChannelID string `json:"resolvedChannelId,omitempty"`
	ResolvedRuleID    string `json:"resolvedRuleId,omitempty"`
	DeliveryID        string `json:"deliveryId,omitempty"`
}

func (h *Handler) registerOpsProjectNotificationAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsListProjectNotificationsInput, map[string]any]{
		Name:           "ops_list_project_notifications",
		Domain:         aitool.DomainOps,
		Title:          "查看项目通知渠道、规则与投递记录",
		Summary:        "查看指定项目空间的通知预设、已绑定/可绑定通知渠道、项目通知规则或最近投递记录，亦支持全局会话跨项目盘点可管理渠道",
		Tags:           []string{"notifications", "projects"},
		AliasesZH:      []string{"查看项目通知", "项目通知渠道列表", "项目通知规则", "项目通知投递记录"},
		AliasesEN:      []string{"list project notifications", "project notification channels", "project notification rules"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionNotificationRead)},
		Execute:        h.aiOpsListProjectNotifications,
	})

	aitool.Register(reg, aitool.Spec[opsSaveProjectNotificationChannelInput, map[string]any]{
		Name:           "ops_save_project_notification_channel",
		Domain:         aitool.DomainOps,
		Title:          "保存或绑定项目通知渠道",
		Summary:        "创建或纯增量修改项目作用域通知渠道，或将已有可管理共享渠道并集绑定到当前项目",
		Tags:           []string{"notifications", "projects"},
		AliasesZH:      []string{"创建项目通知渠道", "修改项目通知渠道", "绑定共享通知渠道"},
		AliasesEN:      []string{"save project notification channel", "bind project notification channel"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_project_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "channel",
				StaticArguments: map[string]any{"section": "channels"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewOpsSaveProjectNotificationChannel,
		Execute: h.aiExecuteOpsSaveProjectNotificationChannel,
	})

	aitool.Register(reg, aitool.Spec[opsUnbindProjectNotificationChannelInput, map[string]any]{
		Name:           "ops_unbind_project_notification_channel",
		Domain:         aitool.DomainOps,
		Title:          "解绑跨项目共享通知渠道",
		Summary:        "将跨多项目共享的通知渠道从当前项目解绑（不删除渠道本身）",
		Tags:           []string{"notifications", "projects"},
		AliasesZH:      []string{"解绑项目通知渠道", "移除共享通知渠道绑定"},
		AliasesEN:      []string{"unbind project notification channel"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_project_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "channel",
				StaticArguments: map[string]any{"section": "channels"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "absent",
			},
		},
		Preview: h.aiPreviewOpsUnbindProjectNotificationChannel,
		Execute: h.aiExecuteOpsUnbindProjectNotificationChannel,
	})

	aitool.Register(reg, aitool.Spec[opsDeleteProjectNotificationChannelInput, map[string]any]{
		Name:           "ops_delete_project_notification_channel",
		Domain:         aitool.DomainOps,
		Title:          "删除单项目独占通知渠道",
		Summary:        "彻底删除单项目独占的通知渠道（仍绑定其他项目或被规则引用时阻断删除）",
		Tags:           []string{"notifications", "projects"},
		AliasesZH:      []string{"删除项目通知渠道"},
		AliasesEN:      []string{"delete project notification channel"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_project_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "channel",
				StaticArguments: map[string]any{"section": "channels"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "absent",
			},
		},
		Preview: h.aiPreviewOpsDeleteProjectNotificationChannel,
		Execute: h.aiExecuteOpsDeleteProjectNotificationChannel,
	})

	aitool.Register(reg, aitool.Spec[opsSaveProjectNotificationRuleInput, map[string]any]{
		Name:           "ops_save_project_notification_rule",
		Domain:         aitool.DomainOps,
		Title:          "保存项目通知规则",
		Summary:        "创建或纯增量更新项目通知订阅规则",
		Tags:           []string{"notifications", "projects", "rules"},
		AliasesZH:      []string{"创建项目通知规则", "修改项目通知规则", "配置项目告警订阅"},
		AliasesEN:      []string{"save project notification rule", "create project notification rule"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_project_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "rule",
				StaticArguments: map[string]any{"section": "rules"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewOpsSaveProjectNotificationRule,
		Execute: h.aiExecuteOpsSaveProjectNotificationRule,
	})

	aitool.Register(reg, aitool.Spec[opsDeleteProjectNotificationRuleInput, map[string]any]{
		Name:           "ops_delete_project_notification_rule",
		Domain:         aitool.DomainOps,
		Title:          "删除项目通知规则",
		Summary:        "删除项目通知订阅规则",
		Tags:           []string{"notifications", "projects", "rules"},
		AliasesZH:      []string{"删除项目通知规则"},
		AliasesEN:      []string{"delete project notification rule"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "ops_list_project_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "rule",
				StaticArguments: map[string]any{"section": "rules"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "absent",
			},
		},
		Preview: h.aiPreviewOpsDeleteProjectNotificationRule,
		Execute: h.aiExecuteOpsDeleteProjectNotificationRule,
	})

	aitool.Register(reg, aitool.Spec[opsTestOrRetryProjectNotificationInput, map[string]any]{
		Name:           "ops_test_or_retry_project_notification",
		Domain:         aitool.DomainOps,
		Title:          "测试项目通知渠道或重试项目投递",
		Summary:        "发送项目通知渠道测试消息或重试失败的项目通知投递记录",
		Tags:           []string{"notifications", "projects"},
		AliasesZH:      []string{"测试项目通知渠道", "重试项目通知投递"},
		AliasesEN:      []string{"test project notification channel", "retry project notification delivery"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Preview:        h.aiPreviewOpsTestOrRetryProjectNotification,
		Execute:        h.aiExecuteOpsTestOrRetryProjectNotification,
	})
}

func newScratchGin(tc *aitool.ToolContext) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req, _ := http.NewRequestWithContext(tc.Context(), http.MethodPost, "/internal/v1/ai/tools", http.NoBody)
	c.Request = req
	return c, rec
}

func toolErrorFromScratchRecorder(rec *httptest.ResponseRecorder, fallbackCode, fallbackMsg string) *aitool.ToolError {
	status := rec.Code
	if status < 400 {
		status = http.StatusBadRequest
	}
	raw := bytes.TrimSpace(rec.Body.Bytes())
	if len(raw) > 0 {
		var parsed map[string]any
		if json.Unmarshal(raw, &parsed) == nil {
			code, _ := parsed["code"].(string)
			msg, _ := parsed["message"].(string)
			if strings.TrimSpace(code) == "" {
				code = fallbackCode
			}
			if strings.TrimSpace(msg) == "" {
				msg = fallbackMsg
			}
			return aitool.NewError(status, code, msg)
		}
	}
	return aitool.NewError(status, fallbackCode, fallbackMsg)
}

func channelResponseWithProject(resp notificationChannelResponse, projectID string) map[string]any {
	raw, _ := json.Marshal(resp)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if strings.TrimSpace(projectID) != "" {
		out["projectId"] = projectID
	}
	return out
}

func ruleResponseWithProject(rule model.NotificationRule, projectID string) map[string]any {
	raw, _ := json.Marshal(rule)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if strings.TrimSpace(projectID) != "" {
		out["projectId"] = projectID
	}
	return out
}

func (h *Handler) aiOpsListProjectNotifications(tc *aitool.ToolContext, in opsListProjectNotificationsInput) (map[string]any, error) {
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	tc.SetNoStore()

	section := strings.ToLower(notifStringOrDefault(in.Section, "overview"))
	switch section {
	case "overview", "channels", "rules", "deliveries", "presets":
	default:
		return nil, aitool.BadRequest("argument_invalid", "section must be overview, channels, rules, deliveries, or presets")
	}

	includeBindable := notifBoolOrDefault(in.IncludeBindableChannels, false)
	convID := strings.TrimSpace(tc.ConversationProjectID)
	if includeBindable && convID != "" {
		return nil, notifProjectScopeMismatch("includeBindableChannels=true is only allowed in an unbound global conversation")
	}

	projectRef := ""
	if in.Project != nil {
		projectRef = strings.TrimSpace(*in.Project)
	}
	if projectRef == "" && convID != "" {
		projectRef = convID
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	// Cross-project manageable scope="project" channels in global session
	if projectRef == "" {
		if convID != "" || section != "channels" {
			return nil, aitool.BadRequest("project.required", "project is required unless section=channels in an unbound global conversation")
		}
		readableProjectIDs, readErr := h.readableNotificationProjectIDs(tc.Context(), user)
		if readErr != nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", readErr.Error())
		}

		var allProjectChannels []model.NotificationChannel
		query := db.Model(&model.NotificationChannel{}).Where("owner_user_id = '' AND scope = 'project'")
		if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
			chFilter := strings.TrimSpace(*in.Channel)
			query = query.Where("(id = ? OR lower(name) = ?)", chFilter, strings.ToLower(chFilter))
		}
		if err := query.Order("created_at desc, id desc").Find(&allProjectChannels).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if err := h.populateLatestNotificationDeliveries(allProjectChannels, tc.Context()); err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		responses, buildErr := h.buildScopedChannelResponses(tc.Context(), user, allProjectChannels, readableProjectIDs)
		if buildErr != nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", buildErr.Error())
		}
		manageable := make([]notificationChannelResponse, 0, len(responses))
		for _, resp := range responses {
			if resp.CanManage {
				manageable = append(manageable, resp)
			}
		}
		total := int64(len(manageable))
		start := (page - 1) * pageSize
		var paged []notificationChannelResponse
		if start < len(manageable) {
			end := min(start+pageSize, len(manageable))
			paged = manageable[start:end]
		} else {
			paged = []notificationChannelResponse{}
		}
		env := aitool.PageEnvelope(paged, page, pageSize, "createdAt", "desc", total)
		env["section"] = "channels"
		return env, nil
	}

	_, project, err := h.authorizeProjectForAI(tc, projectRef, authz.ActionNotificationRead)
	if err != nil {
		return nil, err
	}

	if section == "presets" {
		return map[string]any{
			"projectId": project.ID,
			"section":   "presets",
			"items":     notification.WebhookPresets(),
		}, nil
	}

	canManage, roleErr := h.host.ProjectRoleActionAllowed(tc.Context(), user, project.ID, authz.ActionNotificationManage)
	if roleErr != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", roleErr.Error())
	}
	isAdmin := authz.IsPlatformAdmin(user.Role)
	readableProjectIDs, readErr := h.readableNotificationProjectIDs(tc.Context(), user)
	if readErr != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", readErr.Error())
	}

	boundSubquery := db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? AND project_id = ?", notification.ScopedResourceChannelType, project.ID)

	if section == "channels" {
		query := db.Model(&model.NotificationChannel{}).
			Where("owner_user_id = '' AND (scope = 'global' OR (scope = 'project' AND id IN (?)))", boundSubquery)
		if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
			chFilter := strings.TrimSpace(*in.Channel)
			query = query.Where("(id = ? OR lower(name) = ?)", chFilter, strings.ToLower(chFilter))
		}
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var channels []model.NotificationChannel
		if err := query.Order("created_at desc, id desc").
			Limit(pageSize).
			Offset((page - 1) * pageSize).
			Find(&channels).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if err := h.populateLatestNotificationDeliveries(channels, tc.Context()); err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		responses, buildErr := h.buildScopedChannelResponses(tc.Context(), user, channels, readableProjectIDs)
		if buildErr != nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", buildErr.Error())
		}

		env := aitool.PageEnvelope(responses, page, pageSize, "createdAt", "desc", total)
		env["projectId"] = project.ID
		env["section"] = "channels"

		if includeBindable {
			var candidates []model.NotificationChannel
			if err := db.Model(&model.NotificationChannel{}).
				Where("owner_user_id = '' AND scope = 'project' AND id NOT IN (?)", boundSubquery).
				Order("created_at desc, id desc").
				Find(&candidates).Error; err == nil {
				candResp, cErr := h.buildScopedChannelResponses(tc.Context(), user, candidates, readableProjectIDs)
				if cErr == nil {
					bindable := make([]notificationChannelResponse, 0)
					for _, item := range candResp {
						if item.CanManage {
							bindable = append(bindable, item)
						}
					}
					env["bindableChannels"] = bindable
				}
			}
		}
		return env, nil
	}

	if section == "rules" {
		query := db.Model(&model.NotificationRule{}).Where("project_id = ?", project.ID)
		if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
			ruleFilter := strings.TrimSpace(*in.Rule)
			query = query.Where("(id = ? OR lower(name) = ?)", ruleFilter, strings.ToLower(ruleFilter))
		}
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var rules []model.NotificationRule
		if err := query.Order("created_at desc, id desc").
			Limit(pageSize).
			Offset((page - 1) * pageSize).
			Find(&rules).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if !canManage && !isAdmin {
			for i := range rules {
				rules[i].ChannelTargetsJSON = redactChannelTargetsJSON(rules[i].ChannelTargetsJSON)
			}
		}
		env := aitool.PageEnvelope(rules, page, pageSize, "createdAt", "desc", total)
		env["projectId"] = project.ID
		env["section"] = "rules"
		return env, nil
	}

	if section == "deliveries" {
		query := db.Model(&model.NotificationDelivery{}).Where("project_id = ? AND recipient_user_id = ''", project.ID)
		if in.Status != nil && strings.TrimSpace(*in.Status) != "" {
			query = query.Where("status = ?", strings.TrimSpace(*in.Status))
		}
		if in.EventType != nil && strings.TrimSpace(*in.EventType) != "" {
			query = query.Where("event_type = ?", strings.TrimSpace(*in.EventType))
		}
		if in.EventID != nil && strings.TrimSpace(*in.EventID) != "" {
			query = query.Where("event_id = ?", strings.TrimSpace(*in.EventID))
		}
		if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
			chRef := strings.TrimSpace(*in.Channel)
			var matchedIDs []string
			_ = db.Model(&model.NotificationChannel{}).
				Where("owner_user_id = '' AND (id = ? OR lower(name) = ?)", chRef, strings.ToLower(chRef)).
				Pluck("id", &matchedIDs).Error
			if len(matchedIDs) > 0 {
				query = query.Where("channel_id IN ?", matchedIDs)
			} else {
				query = query.Where("channel_id = ?", chRef)
			}
		}
		if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
			ruleRef := strings.TrimSpace(*in.Rule)
			var matchedRuleIDs []string
			_ = db.Model(&model.NotificationRule{}).
				Where("project_id = ? AND (id = ? OR lower(name) = ?)", project.ID, ruleRef, strings.ToLower(ruleRef)).
				Pluck("id", &matchedRuleIDs).Error
			if len(matchedRuleIDs) > 0 {
				query = query.Where("rule_id IN ?", matchedRuleIDs)
			} else {
				query = query.Where("rule_id = ?", ruleRef)
			}
		}

		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var deliveries []model.NotificationDelivery
		if err := query.Order("created_at desc, id desc").
			Limit(pageSize).
			Offset((page - 1) * pageSize).
			Find(&deliveries).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		h.enrichNotificationDeliveries(deliveries, canManage || isAdmin, tc.Context())
		env := aitool.PageEnvelope(deliveries, page, pageSize, "createdAt", "desc", total)
		env["projectId"] = project.ID
		env["section"] = "deliveries"
		return env, nil
	}

	// section == "overview"
	var channels []model.NotificationChannel
	if err := db.Model(&model.NotificationChannel{}).
		Where("owner_user_id = '' AND (scope = 'global' OR (scope = 'project' AND id IN (?)))", boundSubquery).
		Order("created_at desc, id desc").
		Limit(20).
		Find(&channels).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	_ = h.populateLatestNotificationDeliveries(channels, tc.Context())
	chResponses, buildErr := h.buildScopedChannelResponses(tc.Context(), user, channels, readableProjectIDs)
	if buildErr != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", buildErr.Error())
	}

	var rules []model.NotificationRule
	if err := db.Model(&model.NotificationRule{}).
		Where("project_id = ?", project.ID).
		Order("created_at desc, id desc").
		Limit(20).
		Find(&rules).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if !canManage && !isAdmin {
		for i := range rules {
			rules[i].ChannelTargetsJSON = redactChannelTargetsJSON(rules[i].ChannelTargetsJSON)
		}
	}

	return map[string]any{
		"projectId": project.ID,
		"section":   "overview",
		"channels":  chResponses,
		"rules":     rules,
	}, nil
}

func (h *Handler) resolveProjectNotificationChannelForAI(tc *aitool.ToolContext, user model.User, projectID, ref string, allowUnboundManageable bool) (model.NotificationChannel, []string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.NotificationChannel{}, nil, aitool.BadRequest("notification.channel_required", "channel is required")
	}
	db := h.aiDB(tc)

	var byID model.NotificationChannel
	if err := db.First(&byID, "id = ? AND owner_user_id = ''", ref).Error; err == nil {
		if notification.NormalizeChannelScope(byID.Scope) != "project" {
			return model.NotificationChannel{}, nil, aitool.BadRequest("notification.channel_scope_not_project", "global notification channels cannot be mutated or unbound via project notification tools")
		}
		boundIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, byID.ID, tc.Context())
		if !allowUnboundManageable && !slices.Contains(boundIDs, projectID) {
			return model.NotificationChannel{}, nil, aitool.NotFound("notification.channel_not_found", "notification channel is not bound to this project space")
		}
		return byID, boundIDs, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.NotificationChannel{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	boundSubquery := db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? AND project_id = ?", notification.ScopedResourceChannelType, projectID)

	var boundMatches []model.NotificationChannel
	if err := db.Where("owner_user_id = '' AND scope = 'project' AND id IN (?) AND lower(name) = ?", boundSubquery, strings.ToLower(ref)).
		Order("created_at asc, id asc").
		Find(&boundMatches).Error; err != nil {
		return model.NotificationChannel{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if len(boundMatches) == 1 {
		boundIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, boundMatches[0].ID, tc.Context())
		return boundMatches[0], boundIDs, nil
	}
	if len(boundMatches) > 1 {
		candidates := make([]aitool.AmbiguousCandidate, len(boundMatches))
		for i, c := range boundMatches {
			candidates[i] = aitool.AmbiguousCandidate{ID: c.ID, Name: c.Name}
		}
		return model.NotificationChannel{}, nil, aitool.AmbiguousResource("notification_channel", ref, candidates)
	}

	if allowUnboundManageable {
		var allProjectMatches []model.NotificationChannel
		if err := db.Where("owner_user_id = '' AND scope = 'project' AND lower(name) = ?", strings.ToLower(ref)).
			Order("created_at asc, id asc").
			Find(&allProjectMatches).Error; err != nil {
			return model.NotificationChannel{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var manageableMatches []model.NotificationChannel
		for _, candidate := range allProjectMatches {
			bIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, candidate.ID, tc.Context())
			if ok, mErr := h.canManageNotificationChannel(tc.Context(), user, candidate, bIDs); mErr == nil && ok {
				manageableMatches = append(manageableMatches, candidate)
			}
		}
		if len(manageableMatches) == 1 {
			boundIDs := h.host.ScopedResourceProjectIDs(notification.ScopedResourceChannelType, manageableMatches[0].ID, tc.Context())
			return manageableMatches[0], boundIDs, nil
		}
		if len(manageableMatches) > 1 {
			candidates := make([]aitool.AmbiguousCandidate, len(manageableMatches))
			for i, c := range manageableMatches {
				candidates[i] = aitool.AmbiguousCandidate{ID: c.ID, Name: c.Name}
			}
			return model.NotificationChannel{}, nil, aitool.AmbiguousResource("notification_channel", ref, candidates)
		}
	}

	// Check if name matched a global channel so R11 returns a clear scope error
	var globalCount int64
	_ = db.Model(&model.NotificationChannel{}).Where("owner_user_id = '' AND scope = 'global' AND lower(name) = ?", strings.ToLower(ref)).Count(&globalCount).Error
	if globalCount > 0 {
		return model.NotificationChannel{}, nil, aitool.BadRequest("notification.channel_scope_not_project", "matched channel has global scope; project notification tools only mutate scope=project channels")
	}
	return model.NotificationChannel{}, nil, aitool.NotFound("notification.channel_not_found", "project notification channel was not found")
}

func hasChannelMutationFields(in opsSaveProjectNotificationChannelInput) bool {
	return (in.Name != nil && strings.TrimSpace(*in.Name) != "") ||
		(in.PresetID != nil && strings.TrimSpace(*in.PresetID) != "") ||
		(in.AdapterKind != nil && strings.TrimSpace(*in.AdapterKind) != "") ||
		len(in.UpsertConfig) > 0 ||
		len(in.RemoveConfigKeys) > 0 ||
		len(in.UpsertSecrets) > 0 ||
		len(in.ClearSecretKeys) > 0 ||
		in.Enabled != nil
}

func buildScopedChannelSaveDraft(
	tc *aitool.ToolContext,
	h *Handler,
	user model.User,
	scope string,
	existing *model.NotificationChannel,
	name *string,
	presetID *string,
	adapterKind *string,
	upsertConfig map[string]any,
	removeConfigKeys []string,
	upsertSecrets map[string]string,
	clearSecretKeys []string,
	enabled *bool,
) (model.NotificationChannel, map[string]string, []string, []string, []aitool.PreviewChange, error) {
	isCreate := existing == nil
	var draft model.NotificationChannel
	if isCreate {
		draft = model.NotificationChannel{
			ID:          id.New("nch"),
			Scope:       scope,
			OwnerUserID: "",
			Enabled:     true,
			CreatedBy:   user.ID,
		}
	} else {
		draft = *existing
		draft.Scope = scope
	}

	effectivePresetID := strings.TrimSpace(draft.PresetID)
	if presetID != nil && strings.TrimSpace(*presetID) != "" {
		reqPreset := strings.TrimSpace(*presetID)
		if !isCreate && reqPreset != effectivePresetID {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.preset_immutable", "presetId cannot be changed on an existing channel")
		}
		effectivePresetID = reqPreset
	}

	if effectivePresetID != "" {
		if len(upsertConfig) > 0 || len(removeConfigKeys) > 0 {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("argument_invalid", "preset channels do not accept upsertConfig or removeConfigKeys")
		}
		preset, found := findWebhookPreset(effectivePresetID)
		if !found {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.preset_not_found", "notification preset not found")
		}
		if adapterKind != nil && strings.TrimSpace(*adapterKind) != "" && strings.TrimSpace(*adapterKind) != preset.AdapterKind {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.adapter_immutable", "preset channels must use webhook adapterKind")
		}
		draft.PresetID = preset.ID
		draft.AdapterKind = preset.AdapterKind
		draft.ConfigJSON = preset.ConfigTemplate
		if name != nil {
			trimmed := strings.TrimSpace(*name)
			if trimmed == "" {
				return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.channel_required", "name cannot be empty")
			}
			draft.Name = trimmed
		} else if isCreate {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.channel_required", "name is required when creating a notification channel")
		}

		toSet, toClear, sensitiveKeys, err := normalizeSecretMutations(draft.AdapterKind, upsertSecrets, clearSecretKeys)
		if err != nil {
			return model.NotificationChannel{}, nil, nil, nil, nil, err
		}
		requiredSet := map[string]bool{}
		allowedSet := map[string]bool{}
		for _, f := range preset.SecretFields {
			requiredSet[f] = true
			allowedSet[f] = true
		}
		for _, f := range preset.OptionalSecretFields {
			allowedSet[f] = true
		}
		for k := range toSet {
			if !allowedSet[k] {
				return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.secret_field_invalid", fmt.Sprintf("secret key %q is not supported by preset %s", k, preset.ID))
			}
		}
		for _, k := range toClear {
			if requiredSet[k] {
				return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.secret_required", fmt.Sprintf("required secret %q cannot be cleared", k))
			}
		}
		existingRefs := decodeStringMap(draft.SecretRefsJSON)
		for _, reqKey := range preset.SecretFields {
			if strings.TrimSpace(toSet[reqKey]) == "" && strings.TrimSpace(existingRefs[reqKey]) == "" {
				return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.secret_required", fmt.Sprintf("notification preset secret %s is required", reqKey))
			}
		}
		if enabled != nil {
			draft.Enabled = *enabled
		}

		// Dry-run validate with overlay
		previewRefs := decodeStringMap(draft.SecretRefsJSON)
		for _, ck := range toClear {
			delete(previewRefs, ck)
		}
		overlay := map[string]string{}
		for k, v := range toSet {
			previewRefs[k] = "preview:" + k
			overlay["preview:"+k] = v
			overlay[k] = v
		}
		draftValidate := draft
		draftValidate.SecretRefsJSON = mustJSON(previewRefs)
		resolver := staticOverlaySecretResolver{overlay: overlay, fallback: h.secrets()}
		if vErr := validateNotificationChannel(tc.Context(), draftValidate, resolver); vErr != nil {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.channel_invalid", vErr.Error())
		}

		var changes []aitool.PreviewChange
		if isCreate {
			changes = append(changes,
				aitool.PreviewChange{Field: "name", After: draft.Name, Action: "create"},
				aitool.PreviewChange{Field: "presetId", After: draft.PresetID, Action: "create"},
				aitool.PreviewChange{Field: "enabled", After: draft.Enabled, Action: "create"},
			)
		} else {
			if draft.Name != existing.Name {
				changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: draft.Name, Action: "update"})
			}
			if draft.Enabled != existing.Enabled {
				changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: draft.Enabled, Action: "update"})
			}
		}
		return draft, toSet, toClear, sensitiveKeys, changes, nil
	}

	// Custom webhook or SMTP channel
	effectiveAdapter := strings.TrimSpace(draft.AdapterKind)
	if adapterKind != nil && strings.TrimSpace(*adapterKind) != "" {
		reqAdapter := strings.TrimSpace(*adapterKind)
		if !isCreate && effectiveAdapter != "" && reqAdapter != effectiveAdapter {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.adapter_immutable", "adapterKind cannot be changed across types on an existing channel")
		}
		effectiveAdapter = reqAdapter
	}
	if effectiveAdapter == "" {
		effectiveAdapter = notification.AdapterKindWebhook
	}
	if effectiveAdapter != notification.AdapterKindWebhook && effectiveAdapter != notification.AdapterKindSMTP {
		return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.adapter_invalid", "adapterKind must be webhook or smtp")
	}
	draft.AdapterKind = effectiveAdapter

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.channel_required", "name cannot be empty")
		}
		draft.Name = trimmed
	} else if isCreate {
		return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.channel_required", "name is required when creating a notification channel")
	}

	toSet, toClear, sensitiveKeys, err := normalizeSecretMutations(draft.AdapterKind, upsertSecrets, clearSecretKeys)
	if err != nil {
		return model.NotificationChannel{}, nil, nil, nil, nil, err
	}
	clearSMTPPass := draft.AdapterKind == notification.AdapterKindSMTP && slices.Contains(toClear, "password")
	mergedConfigJSON, cfgErr := mergeChannelConfigPatch(draft.ConfigJSON, upsertConfig, removeConfigKeys, clearSMTPPass)
	if cfgErr != nil {
		return model.NotificationChannel{}, nil, nil, nil, nil, cfgErr
	}
	draft.ConfigJSON = mergedConfigJSON
	if enabled != nil {
		draft.Enabled = *enabled
	}

	previewRefs := decodeStringMap(draft.SecretRefsJSON)
	for _, ck := range toClear {
		delete(previewRefs, ck)
	}
	overlay := map[string]string{}
	for k, v := range toSet {
		previewRefs[k] = "preview:" + k
		overlay["preview:"+k] = v
		overlay[k] = v
	}
	draftValidate := draft
	draftValidate.ConfigJSON = attachSMTPPasswordRef(draftValidate.AdapterKind, draftValidate.ConfigJSON, previewRefs)
	draftValidate.SecretRefsJSON = mustJSON(previewRefs)
	resolver := staticOverlaySecretResolver{overlay: overlay, fallback: h.secrets()}
	if vErr := validateNotificationChannel(tc.Context(), draftValidate, resolver); vErr != nil {
		return model.NotificationChannel{}, nil, nil, nil, nil, aitool.BadRequest("notification.channel_invalid", vErr.Error())
	}

	var changes []aitool.PreviewChange
	if isCreate {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: draft.Name, Action: "create"},
			aitool.PreviewChange{Field: "adapterKind", After: draft.AdapterKind, Action: "create"},
			aitool.PreviewChange{Field: "config", After: sanitizeNotificationConfig(draft.ConfigJSON), Action: "create"},
			aitool.PreviewChange{Field: "enabled", After: draft.Enabled, Action: "create"},
		)
	} else {
		if draft.Name != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: draft.Name, Action: "update"})
		}
		if draft.ConfigJSON != existing.ConfigJSON {
			changes = append(changes, aitool.PreviewChange{Field: "config", Before: sanitizeNotificationConfig(existing.ConfigJSON), After: sanitizeNotificationConfig(draft.ConfigJSON), Action: "update"})
		}
		if draft.Enabled != existing.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: draft.Enabled, Action: "update"})
		}
	}
	return draft, toSet, toClear, sensitiveKeys, changes, nil
}

func (h *Handler) validateAndPrepareProjectChannelSave(
	tc *aitool.ToolContext,
	in opsSaveProjectNotificationChannelInput,
) (model.User, model.Project, *model.NotificationChannel, []string, model.NotificationChannel, map[string]string, []string, []string, []aitool.PreviewChange, bool, error) {
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, err
	}

	var existing *model.NotificationChannel
	var existingProjectIDs []string
	nextProjectIDs := []string{project.ID}
	pureBindOnly := false

	if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
		found, boundIDs, resolveErr := h.resolveProjectNotificationChannelForAI(tc, user, project.ID, *in.Channel, true)
		if resolveErr != nil {
			return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, resolveErr
		}
		existing = &found
		existingProjectIDs = boundIDs
		nextProjectIDs = normalizeUniqueStrings(append(append([]string(nil), boundIDs...), project.ID))

		canManageAll, mErr := h.canManageNotificationChannel(tc.Context(), user, found, nextProjectIDs)
		if mErr != nil {
			return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, aitool.NewError(http.StatusServiceUnavailable, "auth.project_authorization_unavailable", mErr.Error())
		}
		if !canManageAll {
			return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, aitool.Forbidden("notification.channel_shared_owner_required", "project owner permission is required on all bound project spaces to modify or bind this channel")
		}

		convID := strings.TrimSpace(tc.ConversationProjectID)
		hasExternalProjects := false
		for _, pid := range existingProjectIDs {
			if convID != "" && pid != convID {
				hasExternalProjects = true
				break
			}
		}
		mutating := hasChannelMutationFields(in)
		if convID != "" && hasExternalProjects && mutating {
			return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, notifProjectScopeMismatch("cannot modify configuration of a multi-project shared notification channel from a single-project conversation; only pure union-binding is allowed here")
		}
		if !mutating && !slices.Contains(existingProjectIDs, project.ID) {
			pureBindOnly = true
		}
	}

	if pureBindOnly && existing != nil {
		changes := []aitool.PreviewChange{
			{Field: "projectIds", Before: existingProjectIDs, After: nextProjectIDs, Action: "bind"},
		}
		return user, project, existing, nextProjectIDs, *existing, nil, nil, nil, changes, true, nil
	}

	draft, toSet, toClear, sensitiveKeys, changes, err := buildScopedChannelSaveDraft(
		tc, h, user, "project", existing,
		in.Name, in.PresetID, in.AdapterKind,
		in.UpsertConfig, in.RemoveConfigKeys, in.UpsertSecrets, in.ClearSecretKeys, in.Enabled,
	)
	if err != nil {
		return model.User{}, model.Project{}, nil, nil, model.NotificationChannel{}, nil, nil, nil, nil, false, err
	}
	if existing != nil && !slices.Equal(existingProjectIDs, nextProjectIDs) {
		changes = append(changes, aitool.PreviewChange{Field: "projectIds", Before: existingProjectIDs, After: nextProjectIDs, Action: "bind"})
	}
	return user, project, existing, nextProjectIDs, draft, toSet, toClear, sensitiveKeys, changes, false, nil
}

func (h *Handler) aiPreviewOpsSaveProjectNotificationChannel(tc *aitool.ToolContext, in opsSaveProjectNotificationChannelInput) (*aitool.PreviewResult, error) {
	_, project, existing, _, draft, _, _, sensitiveKeys, changes, pureBindOnly, err := h.validateAndPrepareProjectChannelSave(tc, in)
	if err != nil {
		return nil, err
	}

	resolvedID := ""
	summary := fmt.Sprintf("Create project notification channel %q (%s) in project %s", draft.Name, draft.AdapterKind, project.Identifier)
	if existing != nil {
		resolvedID = existing.ID
		if pureBindOnly {
			summary = fmt.Sprintf("Bind shared notification channel %q (%s) to project %s", existing.Name, existing.ID, project.Identifier)
		} else {
			summary = fmt.Sprintf("Update project notification channel %q (%s) in project %s", draft.Name, existing.ID, project.Identifier)
		}
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "project_notification_channel",
			TargetID:       resolvedID,
			ProjectID:      project.ID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
		LockedPayload: lockedProjectNotifMutation{
			ProjectID:         project.ID,
			ResolvedChannelID: resolvedID,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsSaveProjectNotificationChannel(tc *aitool.ToolContext, in opsSaveProjectNotificationChannelInput) (map[string]any, error) {
	var locked lockedProjectNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedChannelID != "" {
		idCopy := locked.ResolvedChannelID
		in.Channel = &idCopy
	}

	user, project, existing, _, draft, toSet, toClear, _, _, pureBindOnly, err := h.validateAndPrepareProjectChannelSave(tc, in)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	if existing == nil {
		wantEnabled := draft.Enabled
		projectIDs := []string{project.ID}
		txErr := db.Transaction(func(tx *gorm.DB) error {
			secretRefs := map[string]string{}
			refOverlay := make(map[string]string, len(toSet)*2)
			for key, val := range toSet {
				refOverlay[key] = val
				ref, storeErr := h.secrets().StoreContextWithDB(tc.Context(), tx, val, user.ID, "notification_channel:"+draft.ID+":"+key)
				if storeErr != nil {
					return storeErr
				}
				secretRefs[key] = ref
				refOverlay[ref] = val
			}
			draft.ConfigJSON = attachSMTPPasswordRef(draft.AdapterKind, draft.ConfigJSON, secretRefs)
			draft.SecretRefsJSON = mustJSON(secretRefs)
			resolver := staticOverlaySecretResolver{overlay: refOverlay, fallback: h.secrets()}
			if vErr := validateNotificationChannel(tc.Context(), draft, resolver); vErr != nil {
				return errChannelValidation{err: vErr}
			}
			if cErr := tx.Create(&draft).Error; cErr != nil {
				return cErr
			}
			if draft.Enabled != wantEnabled {
				if uErr := tx.Model(&model.NotificationChannel{}).Where("id = ?", draft.ID).Update("enabled", wantEnabled).Error; uErr != nil {
					return uErr
				}
				draft.Enabled = wantEnabled
			}
			return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, draft.ID, projectIDs, nil)
		})
		var vErr errChannelValidation
		if errors.As(txErr, &vErr) {
			return nil, aitool.BadRequest("notification.channel_invalid", vErr.err.Error())
		}
		if txErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "notification.channel_save_failed", txErr.Error())
		}
		draft.ProjectIDs = projectIDs
		h.auditWithContext(user.ID, "notification.channel.create", draft.ID, true, "project", tc.Context())
		resp := notificationChannelResponseForWithPrivacy(draft, true)
		resp.CanManage = true
		return channelResponseWithProject(resp, project.ID), nil
	}

	var savedChannel model.NotificationChannel
	var finalProjects []string
	txErr := db.Transaction(func(tx *gorm.DB) error {
		var lockedRow model.NotificationChannel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRow, "id = ? AND owner_user_id = ''", existing.ID).Error; err != nil {
			return err
		}
		if notification.NormalizeChannelScope(lockedRow.Scope) != "project" {
			return errChannelScopeNotProject
		}
		curProjects, bErr := scopedResourceProjectIDsTx(tx, notification.ScopedResourceChannelType, lockedRow.ID)
		if bErr != nil {
			return bErr
		}
		finalProjects = normalizeUniqueStrings(append(append([]string(nil), curProjects...), project.ID))

		if pureBindOnly {
			savedChannel = lockedRow
			return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, lockedRow.ID, finalProjects, nil)
		}

		secretRefs := decodeStringMap(lockedRow.SecretRefsJSON)
		for _, ck := range toClear {
			if oldRef := strings.TrimSpace(secretRefs[ck]); oldRef != "" {
				resource := "notification_channel:" + lockedRow.ID + ":" + ck
				if dErr := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, oldRef, resource); dErr != nil {
					return dErr
				}
			}
			delete(secretRefs, ck)
		}
		refOverlay := make(map[string]string, len(toSet)*2)
		for key, val := range toSet {
			refOverlay[key] = val
			resource := "notification_channel:" + lockedRow.ID + ":" + key
			newRef, storeErr := h.secrets().StoreContextWithDB(tc.Context(), tx, val, user.ID, resource)
			if storeErr != nil {
				return storeErr
			}
			oldRef := strings.TrimSpace(secretRefs[key])
			secretRefs[key] = newRef
			refOverlay[newRef] = val
			if oldRef != "" && oldRef != newRef {
				if dErr := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, oldRef, resource); dErr != nil {
					return dErr
				}
			}
		}

		lockedRow.Name = draft.Name
		lockedRow.PresetID = draft.PresetID
		lockedRow.AdapterKind = draft.AdapterKind
		lockedRow.Enabled = draft.Enabled
		lockedRow.ConfigJSON = attachSMTPPasswordRef(draft.AdapterKind, draft.ConfigJSON, secretRefs)
		if draft.AdapterKind == notification.AdapterKindSMTP && slices.Contains(toClear, "password") {
			cfgMap := map[string]any{}
			if json.Unmarshal([]byte(lockedRow.ConfigJSON), &cfgMap) == nil {
				delete(cfgMap, "passwordRef")
				lockedRow.ConfigJSON = mustJSON(cfgMap)
			}
		}
		lockedRow.SecretRefsJSON = mustJSON(secretRefs)

		resolver := staticOverlaySecretResolver{overlay: refOverlay, fallback: h.secrets()}
		if vErr := validateNotificationChannel(tc.Context(), lockedRow, resolver); vErr != nil {
			return errChannelValidation{err: vErr}
		}
		if compatErr := ensureChannelUpdateCompatibleWithRules(tx, lockedRow, "project", finalProjects); compatErr != nil {
			return compatErr
		}
		if sErr := tx.Save(&lockedRow).Error; sErr != nil {
			return sErr
		}
		savedChannel = lockedRow
		return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, lockedRow.ID, finalProjects, nil)
	})
	var vErr errChannelValidation
	switch {
	case errors.As(txErr, &vErr):
		return nil, aitool.BadRequest("notification.channel_invalid", vErr.err.Error())
	case errors.Is(txErr, gorm.ErrRecordNotFound):
		return nil, aitool.NotFound("notification.channel_not_found", "notification channel was not found")
	case errors.Is(txErr, errChannelScopeNotProject):
		return nil, aitool.BadRequest("notification.channel_scope_not_project", "only project-scoped notification channels can be modified here")
	case errors.Is(txErr, errChannelInUse):
		return nil, aitool.Conflict("notification.channel_in_use", "remove this channel from notification rules before narrowing its scope")
	case errors.Is(txErr, errChannelSMTPRecipientRequired):
		return nil, aitool.BadRequest("notification.smtp_recipient_required", "smtp channel requires at least one recipient on channel or referencing rules")
	case txErr != nil:
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.channel_save_failed", txErr.Error())
	}

	savedChannel.ProjectIDs = finalProjects
	if pureBindOnly {
		h.auditWithContext(user.ID, "notification.channel.bind_project", savedChannel.ID, true, project.ID, tc.Context())
	} else {
		h.auditWithContext(user.ID, "notification.channel.update", savedChannel.ID, true, "project", tc.Context())
	}
	resp := notificationChannelResponseForWithPrivacy(savedChannel, true)
	resp.CanManage = true
	return channelResponseWithProject(resp, project.ID), nil
}

func (h *Handler) validateUnbindProjectChannel(tc *aitool.ToolContext, in opsUnbindProjectNotificationChannelInput) (model.User, model.Project, model.NotificationChannel, []string, error) {
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return model.User{}, model.Project{}, model.NotificationChannel{}, nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return model.User{}, model.Project{}, model.NotificationChannel{}, nil, err
	}
	channel, boundIDs, err := h.resolveProjectNotificationChannelForAI(tc, user, project.ID, in.Channel, false)
	if err != nil {
		return model.User{}, model.Project{}, model.NotificationChannel{}, nil, err
	}

	remaining := make([]string, 0, len(boundIDs))
	for _, pid := range boundIDs {
		if pid != project.ID {
			remaining = append(remaining, pid)
		}
	}
	if len(remaining) == 0 {
		return model.User{}, model.Project{}, model.NotificationChannel{}, nil, aitool.NewErrorWithHint(
			http.StatusConflict,
			"notification.project_required",
			"cannot unbind the last bound project space from this notification channel",
			"Call ops_delete_project_notification_channel to permanently delete an exclusive single-project channel.",
		)
	}

	var projectRules []model.NotificationRule
	if err := h.aiDB(tc).Where("project_id = ?", project.ID).Find(&projectRules).Error; err != nil {
		return model.User{}, model.Project{}, model.NotificationChannel{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	for _, rule := range projectRules {
		if slices.Contains(decodeStringList(rule.ChannelIDsJSON), channel.ID) {
			return model.User{}, model.Project{}, model.NotificationChannel{}, nil, aitool.Conflict("notification.channel_in_use", "remove this channel from project notification rules before unbinding")
		}
	}
	return user, project, channel, remaining, nil
}

func (h *Handler) aiPreviewOpsUnbindProjectNotificationChannel(tc *aitool.ToolContext, in opsUnbindProjectNotificationChannelInput) (*aitool.PreviewResult, error) {
	_, project, channel, remaining, err := h.validateUnbindProjectChannel(tc, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Unbind shared notification channel %q (%s) from project %s", channel.Name, channel.ID, project.Identifier),
			TargetResource: "project_notification_channel",
			TargetID:       channel.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "projectBinding", Before: project.ID, Action: "unbind"},
			},
			Impact: []string{fmt.Sprintf("Channel itself remains active on %d other bound project(s).", len(remaining))},
		},
		LockedPayload: lockedProjectNotifMutation{
			ProjectID:         project.ID,
			ResolvedChannelID: channel.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsUnbindProjectNotificationChannel(tc *aitool.ToolContext, in opsUnbindProjectNotificationChannelInput) (map[string]any, error) {
	var locked lockedProjectNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedChannelID != "" {
		in.Channel = locked.ResolvedChannelID
	}
	user, project, channel, _, err := h.validateUnbindProjectChannel(tc, in)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	txErr := db.Transaction(func(tx *gorm.DB) error {
		var lockedRow model.NotificationChannel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRow, "id = ? AND owner_user_id = ''", channel.ID).Error; err != nil {
			return err
		}
		if notification.NormalizeChannelScope(lockedRow.Scope) != "project" {
			return errChannelScopeNotProject
		}
		existingIDs, bErr := scopedResourceProjectIDsTx(tx, notification.ScopedResourceChannelType, lockedRow.ID)
		if bErr != nil {
			return bErr
		}
		bound := false
		remaining := make([]string, 0, len(existingIDs))
		for _, pid := range existingIDs {
			if pid == project.ID {
				bound = true
				continue
			}
			remaining = append(remaining, pid)
		}
		if !bound {
			return errChannelNotBound
		}
		if len(remaining) == 0 {
			return errChannelLastProject
		}
		var projectRules []model.NotificationRule
		if err := tx.Where("project_id = ?", project.ID).Find(&projectRules).Error; err != nil {
			return err
		}
		for _, r := range projectRules {
			if slices.Contains(decodeStringList(r.ChannelIDsJSON), lockedRow.ID) {
				return errChannelInUse
			}
		}
		return h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, lockedRow.ID, remaining, nil)
	})
	switch {
	case errors.Is(txErr, gorm.ErrRecordNotFound), errors.Is(txErr, errChannelNotBound):
		return nil, aitool.NotFound("notification.channel_not_found", "notification channel is not bound to this project space")
	case errors.Is(txErr, errChannelScopeNotProject):
		return nil, aitool.BadRequest("notification.channel_scope_not_project", "global notification channels cannot be unbound")
	case errors.Is(txErr, errChannelLastProject):
		return nil, aitool.NewErrorWithHint(http.StatusConflict, "notification.project_required", "cannot unbind the last bound project space", "Call ops_delete_project_notification_channel instead.")
	case errors.Is(txErr, errChannelInUse):
		return nil, aitool.Conflict("notification.channel_in_use", "remove this channel from project notification rules before unbinding")
	case txErr != nil:
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.channel_unbind_failed", txErr.Error())
	}

	h.auditWithContext(user.ID, "notification.channel.unbind_project", channel.ID, true, project.ID, tc.Context())
	return map[string]any{
		"id":        channel.ID,
		"projectId": project.ID,
		"unbound":   true,
	}, nil
}

func (h *Handler) checkDeleteProjectChannelConstraints(tc *aitool.ToolContext, db *gorm.DB, projectID string, channel model.NotificationChannel, boundIDs []string) error {
	if notification.NormalizeChannelScope(channel.Scope) != "project" || strings.TrimSpace(channel.OwnerUserID) != "" {
		return aitool.BadRequest("notification.channel_scope_not_project", "only project-scoped notification channels can be deleted via this tool")
	}
	if !slices.Contains(boundIDs, projectID) {
		return aitool.NotFound("notification.channel_not_found", "notification channel is not bound to this project space")
	}
	if len(boundIDs) > 1 {
		if strings.TrimSpace(tc.ConversationProjectID) != "" {
			err := notifProjectScopeMismatch("cannot delete a notification channel shared across multiple projects from a single-project conversation")
			err.Hint = "Use ops_unbind_project_notification_channel to unbind from this project, or unbind other projects in a global conversation first."
			return err
		}
		return aitool.NewErrorWithHint(
			http.StatusConflict,
			"notification.channel_shared_multiple_projects",
			fmt.Sprintf("notification channel %s is still shared across %d projects", channel.ID, len(boundIDs)),
			"Call ops_unbind_project_notification_channel on the other bound projects until this channel is exclusive to a single project before deleting.",
		)
	}

	var rules []model.NotificationRule
	if err := activeNotificationRulesQuery(db).Select("id", "project_id", "channel_ids_json").Find(&rules).Error; err != nil {
		return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var localRuleIDs []string
	externalCount := 0
	for _, r := range rules {
		if slices.Contains(decodeStringList(r.ChannelIDsJSON), channel.ID) {
			if strings.TrimSpace(r.ProjectID) == projectID {
				localRuleIDs = append(localRuleIDs, r.ID)
			} else {
				externalCount++
			}
		}
	}
	if len(localRuleIDs) > 0 || externalCount > 0 {
		te := aitool.Conflict("notification.channel_in_use", "remove this channel from notification rules before deleting")
		te.Details = map[string]any{
			"projectRuleIds":              localRuleIDs,
			"externalReferencedRuleCount": externalCount,
		}
		return te
	}
	return nil
}

func (h *Handler) aiPreviewOpsDeleteProjectNotificationChannel(tc *aitool.ToolContext, in opsDeleteProjectNotificationChannelInput) (*aitool.PreviewResult, error) {
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	channel, boundIDs, err := h.resolveProjectNotificationChannelForAI(tc, user, project.ID, in.Channel, false)
	if err != nil {
		return nil, err
	}
	if err := h.checkDeleteProjectChannelConstraints(tc, db, project.ID, channel, boundIDs); err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete exclusive project notification channel %q (%s) from project %s", channel.Name, channel.ID, project.Identifier),
			TargetResource: "project_notification_channel",
			TargetID:       channel.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "channel", Before: channel.Name, Action: "delete"},
			},
		},
		LockedPayload: lockedProjectNotifMutation{
			ProjectID:         project.ID,
			ResolvedChannelID: channel.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsDeleteProjectNotificationChannel(tc *aitool.ToolContext, in opsDeleteProjectNotificationChannelInput) (map[string]any, error) {
	var locked lockedProjectNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedChannelID != "" {
		in.Channel = locked.ResolvedChannelID
	}
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	channel, _, err := h.resolveProjectNotificationChannelForAI(tc, user, project.ID, in.Channel, false)
	if err != nil {
		return nil, err
	}

	var innerToolErr error
	txErr := db.Transaction(func(tx *gorm.DB) error {
		var lockedRow model.NotificationChannel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRow, "id = ? AND owner_user_id = ''", channel.ID).Error; err != nil {
			return err
		}
		boundIDs, bErr := scopedResourceProjectIDsTx(tx, notification.ScopedResourceChannelType, lockedRow.ID)
		if bErr != nil {
			return bErr
		}
		if cErr := h.checkDeleteProjectChannelConstraints(tc, tx, project.ID, lockedRow, boundIDs); cErr != nil {
			innerToolErr = cErr
			return cErr
		}
		if bErr := h.host.ReplaceScopedResourceProjectBindings(tx, notification.ScopedResourceChannelType, lockedRow.ID, nil, nil); bErr != nil {
			return bErr
		}
		for key, ref := range decodeStringMap(lockedRow.SecretRefsJSON) {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			resource := "notification_channel:" + lockedRow.ID + ":" + key
			if dErr := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, ref, resource); dErr != nil {
				return dErr
			}
			if presetID := strings.TrimSpace(lockedRow.PresetID); presetID != "" {
				legacy := "notification_channel:preset:" + presetID + ":" + key
				if dErr := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, ref, legacy); dErr != nil {
					return dErr
				}
			}
		}
		return tx.Delete(&lockedRow).Error
	})
	if innerToolErr != nil {
		return nil, innerToolErr
	}
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("notification.channel_not_found", "notification channel was not found")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.channel_delete_failed", txErr.Error())
	}

	h.auditWithContext(user.ID, "notification.channel.delete", channel.ID, true, "project", tc.Context())
	return map[string]any{
		"id":        channel.ID,
		"projectId": project.ID,
		"deleted":   true,
	}, nil
}

func resolveProjectRuleForAI(db *gorm.DB, projectID, ref string) (model.NotificationRule, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.NotificationRule{}, aitool.BadRequest("notification.rule_required", "rule is required")
	}
	var rules []model.NotificationRule
	if err := db.Where("project_id = ?", projectID).Order("created_at asc, id asc").Find(&rules).Error; err != nil {
		return model.NotificationRule{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		rules,
		"notification_rule",
		"notification.rule_not_found",
		ref,
		func(r model.NotificationRule) string { return r.ID },
		func(r model.NotificationRule) string { return r.Name },
	)
}

func resolveEligibleRuleChannelForProject(db *gorm.DB, projectID, ref string) (model.NotificationChannel, error) {
	boundSubquery := db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? AND project_id = ?", notification.ScopedResourceChannelType, projectID)
	var channels []model.NotificationChannel
	if err := db.Where("owner_user_id = '' AND (scope = 'global' OR (scope = 'project' AND id IN (?)))", boundSubquery).
		Order("created_at asc, id asc").
		Find(&channels).Error; err != nil {
		return model.NotificationChannel{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		channels,
		"notification_channel",
		"notification.channel_not_found",
		ref,
		func(c model.NotificationChannel) string { return c.ID },
		func(c model.NotificationChannel) string { return c.Name },
	)
}

func (h *Handler) buildProjectRuleSaveDraft(tc *aitool.ToolContext, user model.User, project model.Project, existing *model.NotificationRule, in opsSaveProjectNotificationRuleInput) (model.NotificationRule, []aitool.PreviewChange, error) {
	db := h.aiDB(tc)
	isCreate := existing == nil
	base := model.NotificationRule{
		ID:        id.New("nrl"),
		ProjectID: project.ID,
		CreatedBy: user.ID,
	}
	if !isCreate {
		base = *existing
	}

	resolveChID := func(ref string) (string, error) {
		ch, err := resolveEligibleRuleChannelForProject(db, project.ID, ref)
		if err != nil {
			return "", err
		}
		return ch.ID, nil
	}

	var addChannelIDs, removeChannelIDs []string
	for _, cRef := range in.AddChannels {
		cid, err := resolveChID(cRef)
		if err != nil {
			return model.NotificationRule{}, nil, err
		}
		addChannelIDs = append(addChannelIDs, cid)
	}
	for _, cRef := range in.RemoveChannels {
		if cid, err := resolveChID(cRef); err == nil {
			removeChannelIDs = append(removeChannelIDs, cid)
		} else {
			removeChannelIDs = append(removeChannelIDs, strings.TrimSpace(cRef))
		}
	}

	nextName := base.Name
	if in.Name != nil {
		nextName = strings.TrimSpace(*in.Name)
	}
	nextEvents := applyStringSetPatch(decodeStringList(base.EventTypesJSON), in.AddEventTypes, in.RemoveEventTypes)
	nextChannels := applyStringSetPatch(decodeStringList(base.ChannelIDsJSON), addChannelIDs, removeChannelIDs)
	targetsRaw, tErr := applyChannelTargetPatches(base.ChannelTargetsJSON, in.UpsertChannelTargets, in.RemoveChannelTargets, resolveChID)
	if tErr != nil {
		return model.NotificationRule{}, nil, tErr
	}

	currentFilter := notification.RuleFilter{
		Scope:      notification.RuleScopeProjects,
		ProjectIDs: []string{project.ID},
	}
	if !isCreate && strings.TrimSpace(base.FilterJSON) != "" {
		if decoded, err := notification.DecodeRuleFilter([]byte(base.FilterJSON)); err == nil {
			currentFilter = decoded
			currentFilter.Scope = notification.RuleScopeProjects
			currentFilter.ProjectIDs = []string{project.ID}
		}
	}

	if in.Filter != nil {
		fp := in.Filter
		if notifBoolOrDefault(fp.ClearFilter, false) {
			currentFilter = notification.RuleFilter{
				Scope:      notification.RuleScopeProjects,
				ProjectIDs: []string{project.ID},
			}
		} else {
			if notifBoolOrDefault(fp.ClearSeverities, false) {
				currentFilter.Severities = nil
			} else if fp.SetSeverities != nil {
				currentFilter.Severities = fp.SetSeverities
			}

			if notifBoolOrDefault(fp.ClearApplications, false) {
				currentFilter.ApplicationIDs = nil
			} else {
				var addApps, rmApps []string
				for _, appRef := range fp.AddApplications {
					app, aErr := aitool.ResolveApplication(db, project.ID, appRef)
					if aErr != nil {
						return model.NotificationRule{}, nil, aErr
					}
					addApps = append(addApps, app.ID)
				}
				for _, appRef := range fp.RemoveApplications {
					if app, aErr := aitool.ResolveApplication(db, project.ID, appRef); aErr == nil {
						rmApps = append(rmApps, app.ID)
					} else {
						rmApps = append(rmApps, strings.TrimSpace(appRef))
					}
				}
				currentFilter.ApplicationIDs = applyStringSetPatch(currentFilter.ApplicationIDs, addApps, rmApps)
			}

			if notifBoolOrDefault(fp.ClearTargets, false) {
				currentFilter.DeploymentTargetIDs = nil
			} else {
				var addTargets, rmTargets []string
				for _, tRef := range fp.AddTargets {
					t, targetErr := resolveRuleFilterTargetInProject(db, project.ID, tRef)
					if targetErr != nil {
						return model.NotificationRule{}, nil, targetErr
					}
					addTargets = append(addTargets, t.ID)
				}
				for _, tRef := range fp.RemoveTargets {
					if t, targetErr := resolveRuleFilterTargetInProject(db, project.ID, tRef); targetErr == nil {
						rmTargets = append(rmTargets, t.ID)
					} else {
						rmTargets = append(rmTargets, strings.TrimSpace(tRef))
					}
				}
				currentFilter.DeploymentTargetIDs = applyStringSetPatch(currentFilter.DeploymentTargetIDs, addTargets, rmTargets)
			}
		}
	}

	encodedFilter, fErr := notification.EncodeRuleFilter(currentFilter)
	if fErr != nil {
		return model.NotificationRule{}, nil, aitool.BadRequest("notification.rule_filter_invalid", fErr.Error())
	}

	nextLocale := base.Locale
	if in.Locale != nil {
		loc := strings.TrimSpace(*in.Locale)
		if loc != "" && loc != "zh-CN" && loc != "en-US" {
			return model.NotificationRule{}, nil, aitool.BadRequest("argument_invalid", "locale must be zh-CN or en-US")
		}
		nextLocale = loc
	}
	nextTZ := base.TimeZone
	if in.TimeZone != nil && strings.TrimSpace(*in.TimeZone) != "" {
		nextTZ = strings.TrimSpace(*in.TimeZone)
	}

	ruleInput := NotificationRuleInput{
		Name:                  nextName,
		EventTypes:            nextEvents,
		Filter:                json.RawMessage(encodedFilter),
		ChannelIDs:            nextChannels,
		ChannelTargets:        targetsRaw,
		TemplateID:            base.TemplateID, // Preserved untouched on project rules
		Locale:                nextLocale,
		TimeZone:              nextTZ,
		RemindIntervalMinutes: in.RemindIntervalMinutes,
		MaxReminders:          in.MaxReminders,
		NotifyOnRecovery:      in.NotifyOnRecovery,
		Enabled:               in.Enabled,
	}

	scratchGin, rec := newScratchGin(tc)
	validated, ok := h.NotificationRuleFromInput(scratchGin, ruleInput, base)
	if !ok {
		return model.NotificationRule{}, nil, toolErrorFromScratchRecorder(rec, "notification.rule_invalid", "invalid project notification rule")
	}
	if !h.validateProjectRuleScopeReferences(scratchGin, project.ID, validated.FilterJSON) {
		return model.NotificationRule{}, nil, toolErrorFromScratchRecorder(rec, "notification.rule_filter_invalid", "invalid project rule filter references")
	}

	var changes []aitool.PreviewChange
	if isCreate {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: validated.Name, Action: "create"},
			aitool.PreviewChange{Field: "eventTypes", After: nextEvents, Action: "create"},
			aitool.PreviewChange{Field: "channelIds", After: nextChannels, Action: "create"},
			aitool.PreviewChange{Field: "enabled", After: validated.Enabled, Action: "create"},
		)
	} else {
		if validated.Name != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: validated.Name, Action: "update"})
		}
		if validated.EventTypesJSON != existing.EventTypesJSON {
			changes = append(changes, aitool.PreviewChange{Field: "eventTypes", Before: decodeStringList(existing.EventTypesJSON), After: nextEvents, Action: "update"})
		}
		if validated.ChannelIDsJSON != existing.ChannelIDsJSON {
			changes = append(changes, aitool.PreviewChange{Field: "channelIds", Before: decodeStringList(existing.ChannelIDsJSON), After: nextChannels, Action: "update"})
		}
		if validated.FilterJSON != existing.FilterJSON {
			changes = append(changes, aitool.PreviewChange{Field: "filterJson", Before: existing.FilterJSON, After: validated.FilterJSON, Action: "update"})
		}
		if validated.Enabled != existing.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: validated.Enabled, Action: "update"})
		}
	}
	return validated, changes, nil
}

func (h *Handler) aiPreviewOpsSaveProjectNotificationRule(tc *aitool.ToolContext, in opsSaveProjectNotificationRuleInput) (*aitool.PreviewResult, error) {
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationRule
	resolvedID := ""
	if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
		found, rErr := resolveProjectRuleForAI(db, project.ID, *in.Rule)
		if rErr != nil {
			return nil, rErr
		}
		existing = &found
		resolvedID = found.ID
	}

	draft, changes, err := h.buildProjectRuleSaveDraft(tc, user, project, existing, in)
	if err != nil {
		return nil, err
	}

	summary := fmt.Sprintf("Create project notification rule %q in project %s", draft.Name, project.Identifier)
	if existing != nil {
		summary = fmt.Sprintf("Update project notification rule %q (%s) in project %s", draft.Name, existing.ID, project.Identifier)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "project_notification_rule",
			TargetID:       resolvedID,
			ProjectID:      project.ID,
			Changes:        changes,
		},
		LockedPayload: lockedProjectNotifMutation{
			ProjectID:      project.ID,
			ResolvedRuleID: resolvedID,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsSaveProjectNotificationRule(tc *aitool.ToolContext, in opsSaveProjectNotificationRuleInput) (map[string]any, error) {
	var locked lockedProjectNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedRuleID != "" {
		idCopy := locked.ResolvedRuleID
		in.Rule = &idCopy
	}
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationRule
	if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
		found, rErr := resolveProjectRuleForAI(db, project.ID, *in.Rule)
		if rErr != nil {
			return nil, rErr
		}
		existing = &found
	}

	draft, _, err := h.buildProjectRuleSaveDraft(tc, user, project, existing, in)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		if err := createNotificationRule(db, &draft); err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "notification.rule_save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.project_rule.create", draft.ID, true, project.ID, tc.Context())
	} else {
		if err := db.Save(&draft).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "notification.rule_save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.project_rule.update", draft.ID, true, project.ID, tc.Context())
	}

	return ruleResponseWithProject(draft, project.ID), nil
}

func (h *Handler) aiPreviewOpsDeleteProjectNotificationRule(tc *aitool.ToolContext, in opsDeleteProjectNotificationRuleInput) (*aitool.PreviewResult, error) {
	_, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	rule, err := resolveProjectRuleForAI(h.aiDB(tc), project.ID, in.Rule)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete project notification rule %q (%s) from project %s", rule.Name, rule.ID, project.Identifier),
			TargetResource: "project_notification_rule",
			TargetID:       rule.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "rule", Before: rule.Name, Action: "delete"},
			},
		},
		LockedPayload: lockedProjectNotifMutation{
			ProjectID:      project.ID,
			ResolvedRuleID: rule.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsDeleteProjectNotificationRule(tc *aitool.ToolContext, in opsDeleteProjectNotificationRuleInput) (map[string]any, error) {
	var locked lockedProjectNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedRuleID != "" {
		in.Rule = locked.ResolvedRuleID
	}
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	rule, err := resolveProjectRuleForAI(db, project.ID, in.Rule)
	if err != nil {
		return nil, err
	}

	res := db.Where("id = ? AND project_id = ?", rule.ID, project.ID).Delete(&model.NotificationRule{})
	if res.Error != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.rule_delete_failed", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return nil, aitool.NotFound("notification.rule_not_found", "notification rule was not found")
	}
	h.auditWithContext(user.ID, "notification.project_rule.delete", rule.ID, true, project.ID, tc.Context())

	return map[string]any{
		"id":        rule.ID,
		"projectId": project.ID,
		"deleted":   true,
	}, nil
}

func (h *Handler) aiPreviewOpsTestOrRetryProjectNotification(tc *aitool.ToolContext, in opsTestOrRetryProjectNotificationInput) (*aitool.PreviewResult, error) {
	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	chRef, deliveryID, err := validateTestOrRetryChoice(in.Channel, in.RetryDeliveryID)
	if err != nil {
		return nil, err
	}

	if chRef != "" {
		if _, rErr := parseOptionalTestRecipient(in.TestRecipient, user.Email); rErr != nil {
			return nil, rErr
		}
		channel, _, resolveErr := h.resolveProjectNotificationChannelForAI(tc, user, project.ID, chRef, false)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Send test notification via project channel %q (%s) in project %s", channel.Name, channel.ID, project.Identifier),
				TargetResource: "project_notification_channel",
				TargetID:       channel.ID,
				ProjectID:      project.ID,
				Impact:         []string{"Dispatches a real external test message via the selected project notification channel."},
			},
			LockedPayload: lockedProjectNotifMutation{
				ProjectID:         project.ID,
				ResolvedChannelID: channel.ID,
			},
		}, nil
	}

	var delivery model.NotificationDelivery
	if err := db.First(&delivery, "id = ? AND project_id = ? AND recipient_user_id = ''", deliveryID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("notification.delivery_not_found", "project notification delivery was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if err := ensureDeliveryRetryable(delivery, time.Now()); err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Retry failed project notification delivery %s in project %s", delivery.ID, project.Identifier),
			TargetResource: "notification_delivery",
			TargetID:       delivery.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "status", Before: delivery.Status, After: "pending", Action: "retry"},
			},
		},
		LockedPayload: lockedProjectNotifMutation{
			ProjectID:  project.ID,
			DeliveryID: delivery.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteOpsTestOrRetryProjectNotification(tc *aitool.ToolContext, in opsTestOrRetryProjectNotificationInput) (map[string]any, error) {
	var locked lockedProjectNotifMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	user, project, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionNotificationManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectMutableForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	chRef, deliveryID, err := validateTestOrRetryChoice(in.Channel, in.RetryDeliveryID)
	if err != nil {
		return nil, err
	}

	if chRef != "" {
		if locked.ResolvedChannelID != "" {
			chRef = locked.ResolvedChannelID
		}
		recipient, rErr := parseOptionalTestRecipient(in.TestRecipient, user.Email)
		if rErr != nil {
			return nil, rErr
		}
		channel, _, resolveErr := h.resolveProjectNotificationChannelForAI(tc, user, project.ID, chRef, false)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if !h.allowPersonalNotificationTest(tc.Gin, user.ID) {
			return nil, tc.AbortedError("notification.test_rate_limited", "notification test rate limit exceeded")
		}
		adapter, aErr := notification.DefaultRegistry().Adapter(channel.AdapterKind)
		if aErr != nil {
			return nil, aitool.BadRequest("notification.adapter_invalid", aErr.Error())
		}
		testCtx := notification.WithEgressPolicy(tc.Context(), notification.EgressPolicyForScope("project"))
		if recipient != "" {
			testCtx = notification.WithTestRecipient(testCtx, recipient)
		}
		if tErr := adapter.Test(testCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), h.secrets()); tErr != nil {
			h.auditWithContext(user.ID, "notification.channel.test", channel.ID, false, tErr.Error(), tc.Context())
			return nil, aitool.NewError(http.StatusBadGateway, "notification.channel_test_failed", tErr.Error())
		}
		h.auditWithContext(user.ID, "notification.channel.test", channel.ID, true, "", tc.Context())
		return map[string]any{
			"projectId": project.ID,
			"mode":      "test",
			"channelId": channel.ID,
			"status":    "ok",
		}, nil
	}

	targetDeliveryID := firstNonEmpty(locked.DeliveryID, deliveryID)
	var existing model.NotificationDelivery
	if err := db.First(&existing, "id = ? AND project_id = ? AND recipient_user_id = ''", targetDeliveryID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("notification.delivery_not_found", "project notification delivery was not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if err := ensureDeliveryRetryable(existing, time.Now()); err != nil {
		return nil, err
	}

	svc := notification.Service{DB: db, Enqueuer: h.host.DeliveryEnqueuer()}
	retried, retryErr := svc.RetryDelivery(tc.Context(), existing.ID)
	if errors.Is(retryErr, notification.ErrDeliveryActive) {
		return nil, aitool.Conflict("notification.delivery_not_retryable", retryErr.Error())
	}
	if retryErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "notification.delivery_retry_failed", retryErr.Error())
	}
	items := []model.NotificationDelivery{retried}
	h.enrichNotificationDeliveries(items, true, tc.Context())
	h.auditWithContext(user.ID, "notification.project_delivery.retry", retried.ID, true, project.ID, tc.Context())

	return map[string]any{
		"projectId":  project.ID,
		"mode":       "retry",
		"deliveryId": items[0].ID,
		"status":     items[0].Status,
		"delivery":   items[0],
	}, nil
}
