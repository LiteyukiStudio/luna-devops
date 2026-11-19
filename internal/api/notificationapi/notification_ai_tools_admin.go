package notificationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/platformmail"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type adminListPlatformNotificationsInput struct {
	Section      string  `json:"section" enum:"channels|templates|rules|deliveries|presets" desc:"查询子视图"`
	ChannelScope *string `json:"channelScope,omitempty" enum:"global|project|all" default:"global" desc:"仅 section=channels 有效，过滤渠道作用域"`
	Search       *string `json:"search,omitempty" desc:"channels/templates/rules 按名称或关键字模糊搜索"`
	AdapterKind  *string `json:"adapterKind,omitempty" enum:"webhook|smtp" desc:"channels/templates 按适配器类型过滤"`
	Status       *string `json:"status,omitempty" desc:"deliveries 按投递状态过滤"`
	EventType    *string `json:"eventType,omitempty" desc:"deliveries/templates 按事件类型过滤"`
	Channel      *string `json:"channel,omitempty" desc:"channels/deliveries 按渠道 ID 或名称精确过滤"`
	Template     *string `json:"template,omitempty" desc:"templates/rules 按通知消息模板 ID 或名称精确过滤"`
	Rule         *string `json:"rule,omitempty" desc:"rules/deliveries 按规则 ID 或名称精确过滤"`
	Project      *string `json:"project,omitempty" desc:"deliveries 按事件发生项目过滤，或 section=channels&channelScope!=global 时按绑定项目过滤"`
	Page         *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize     *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type adminConfigurePlatformMailInput struct {
	Host                         *string `json:"host,omitempty" desc:"SMTP 服务器主机名"`
	Port                         *int    `json:"port,omitempty" min:"1" max:"65535" desc:"SMTP 端口"`
	Security                     *string `json:"security,omitempty" enum:"none|starttls|tls" desc:"传输加密模式（默认 starttls）"`
	Username                     *string `json:"username,omitempty" desc:"SMTP 认证用户名（省略保持原值）"`
	ClearUsername                *bool   `json:"clearUsername,omitempty" desc:"显式清空 SMTP 用户名"`
	Password                     *string `json:"password,omitempty" sensitive:"true" desc:"SMTP 认证密码（省略保留原密码）"`
	ClearPassword                *bool   `json:"clearPassword,omitempty" desc:"显式清除已保存的 SMTP 密码"`
	FromAddress                  *string `json:"fromAddress,omitempty" desc:"发件人邮箱地址"`
	FromName                     *string `json:"fromName,omitempty" desc:"发件人显示名称"`
	ClearFromName                *bool   `json:"clearFromName,omitempty" desc:"显式重置发件人显示名称为默认"`
	PersonalEmailCooldownSeconds *int    `json:"personalEmailCooldownSeconds,omitempty" min:"0" max:"3600" desc:"个人邮件通知聚合冷却秒数（0..3600）"`
	SendTestTo                   *string `json:"sendTestTo,omitempty" desc:"保存后（或仅传该字段时用已存配置）向指定邮箱发送连通性测试邮件"`
}

type adminSaveGlobalNotificationChannelInput struct {
	Channel          *string           `json:"channel,omitempty" desc:"省略为新建；传入全局渠道 ID 或唯一名称为存量基底增量修改"`
	Name             *string           `json:"name,omitempty" desc:"渠道名称（新建必填）"`
	PresetID         *string           `json:"presetId,omitempty" enum:"feishu-bot|lark-bot|wecom-bot|gotify|dingtalk-bot|slack-webhook|discord-webhook" desc:"7 类 Webhook 预设之一（自定义 Webhook/SMTP 省略）"`
	AdapterKind      *string           `json:"adapterKind,omitempty" enum:"webhook|smtp" desc:"适配器类型（默认 webhook）"`
	UpsertConfig     map[string]any    `json:"upsertConfig,omitempty" desc:"增量合并非敏感渠道配置（禁止明文密码或敏感鉴权头字面量）"`
	RemoveConfigKeys []string          `json:"removeConfigKeys,omitempty" desc:"移除指定可选配置键"`
	UpsertSecrets    map[string]string `json:"upsertSecrets,omitempty" sensitive:"true" desc:"增量写入签名密钥、Token 或 SMTP password"`
	ClearSecretKeys  []string          `json:"clearSecretKeys,omitempty" desc:"显式清除指定密钥键"`
	Enabled          *bool             `json:"enabled,omitempty" desc:"是否启用（新建默认 true）"`
}

type adminSaveNotificationTemplateInput struct {
	Template              *string `json:"template,omitempty" desc:"省略为新建；传入消息模板 ID 或唯一名称为增量修改"`
	Name                  *string `json:"name,omitempty" desc:"消息模板名称（新建必填）"`
	EventType             *string `json:"eventType,omitempty" desc:"匹配事件类型（新建必填）"`
	AdapterKind           *string `json:"adapterKind,omitempty" enum:"webhook|smtp" desc:"适配器类型（新建必填）"`
	Locale                *string `json:"locale,omitempty" enum:"zh-CN|en-US" desc:"语言区域（默认 zh-CN）"`
	SubjectTemplate       *string `json:"subjectTemplate,omitempty" desc:"邮件标题模板"`
	ClearSubjectTemplate  *bool   `json:"clearSubjectTemplate,omitempty" desc:"显式清空自定义标题模板"`
	BodyTemplate          *string `json:"bodyTemplate,omitempty" desc:"文本正文模板"`
	ClearBodyTemplate     *bool   `json:"clearBodyTemplate,omitempty" desc:"显式清空文本正文模板"`
	JSONBodyTemplate      *string `json:"jsonBodyTemplate,omitempty" desc:"Webhook JSON 正文模板"`
	ClearJSONBodyTemplate *bool   `json:"clearJsonBodyTemplate,omitempty" desc:"显式清空 JSON 正文模板"`
	Enabled               *bool   `json:"enabled,omitempty" desc:"是否启用（新建默认 true）"`
}

type globalRuleFilterPatchInput struct {
	Scope              *string  `json:"scope,omitempty" enum:"all|projects" desc:"全局规则匹配范围：all（全平台）或 projects（指定项目集）"`
	AddProjects        []string `json:"addProjects,omitempty" desc:"scope=projects 时增量添加项目 ID 或 identifier"`
	RemoveProjects     []string `json:"removeProjects,omitempty" desc:"增量移除项目"`
	AddApplications    []string `json:"addApplications,omitempty" desc:"增量添加应用过滤"`
	RemoveApplications []string `json:"removeApplications,omitempty" desc:"增量移除应用过滤"`
	ClearApplications  *bool    `json:"clearApplications,omitempty" desc:"清空应用过滤条件"`
	AddTargets         []string `json:"addTargets,omitempty" desc:"增量添加部署目标过滤"`
	RemoveTargets      []string `json:"removeTargets,omitempty" desc:"增量移除部署目标过滤"`
	ClearTargets       *bool    `json:"clearTargets,omitempty" desc:"清空部署目标过滤条件"`
	SetSeverities      []string `json:"setSeverities,omitempty" enum:"info|warning|error" desc:"设置事件严重级别过滤"`
	ClearSeverities    *bool    `json:"clearSeverities,omitempty" desc:"清空严重级别过滤条件"`
	ClearFilter        *bool    `json:"clearFilter,omitempty" desc:"重置为全平台 {scope: \"all\"} 过滤"`
}

type adminSaveGlobalNotificationRuleInput struct {
	Rule                  *string                       `json:"rule,omitempty" desc:"省略为新建；传入全局规则 ID 或唯一名称为增量更新"`
	Name                  *string                       `json:"name,omitempty" desc:"规则名称（新建必填）"`
	AddEventTypes         []string                      `json:"addEventTypes,omitempty" desc:"增量添加事件类型"`
	RemoveEventTypes      []string                      `json:"removeEventTypes,omitempty" desc:"增量移除事件类型"`
	AddChannels           []string                      `json:"addChannels,omitempty" desc:"增量添加全局通知渠道 ID 或名称"`
	RemoveChannels        []string                      `json:"removeChannels,omitempty" desc:"增量移除全局通知渠道"`
	UpsertChannelTargets  []ruleChannelTargetPatchInput `json:"upsertChannelTargets,omitempty" desc:"配置 SMTP 渠道收件人覆盖"`
	RemoveChannelTargets  []string                      `json:"removeChannelTargets,omitempty" desc:"按渠道移除收件人覆盖"`
	Template              *string                       `json:"template,omitempty" desc:"绑定自定义通知消息模板 ID 或名称"`
	ClearTemplate         *bool                         `json:"clearTemplate,omitempty" desc:"解绑自定义通知消息模板"`
	Filter                *globalRuleFilterPatchInput   `json:"filter,omitempty" desc:"纯增量全局规则过滤条件"`
	Locale                *string                       `json:"locale,omitempty" enum:"zh-CN|en-US" desc:"通知语言"`
	TimeZone              *string                       `json:"timeZone,omitempty" desc:"IANA 时区名"`
	RemindIntervalMinutes *int                          `json:"remindIntervalMinutes,omitempty" min:"0" max:"10080" desc:"重复提醒间隔分钟数（0..10080）"`
	MaxReminders          *int                          `json:"maxReminders,omitempty" min:"0" max:"100" desc:"最大提醒次数（0..100）"`
	NotifyOnRecovery      *bool                         `json:"notifyOnRecovery,omitempty" desc:"恢复时是否发送通知"`
	Enabled               *bool                         `json:"enabled,omitempty" desc:"是否启用"`
}

type adminTestOrRetryGlobalNotificationInput struct {
	Channel         *string `json:"channel,omitempty" desc:"测试指定平台全局通知渠道 ID 或唯一名称（与 retryDeliveryId 互斥）"`
	TestRecipient   *string `json:"testRecipient,omitempty" desc:"全局 SMTP 渠道测试收件邮箱"`
	RetryDeliveryID *string `json:"retryDeliveryId,omitempty" desc:"重试归属平台全局规则触发的失败通知投递记录 ID（与 channel 互斥）"`
}

type adminDeletePlatformNotificationResourceInput struct {
	ResourceType string `json:"resourceType" enum:"channel|template|rule" desc:"待删除的全局通知资源类型"`
	Resource     string `json:"resource" desc:"目标全局通知渠道、消息模板或全局规则的 ID 或唯一名称"`
}

type lockedAdminNotifMutation struct {
	ResourceType string `json:"resourceType,omitempty"`
	ResolvedID   string `json:"resolvedId,omitempty"`
	DeliveryID   string `json:"deliveryId,omitempty"`
}

func (h *Handler) registerAdminNotificationAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[adminListPlatformNotificationsInput, map[string]any]{
		Name:           "admin_list_platform_notifications",
		Domain:         aitool.DomainAdmin,
		Title:          "查看平台全局通知配置与投递记录",
		Summary:        "按明确子视图分页查看平台全局/项目通知渠道、消息模板、全局通知规则、预设或全局规则投递记录",
		Tags:           []string{"notifications", "admin"},
		AliasesZH:      []string{"查看全局通知渠道", "查看通知消息模板", "查看全局通知规则", "查看全局通知投递"},
		AliasesEN:      []string{"list platform notifications", "global notification channels", "notification templates"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionNotificationRead)},
		Execute:        h.aiAdminListPlatformNotifications,
	})

	aitool.Register(reg, aitool.Spec[adminConfigurePlatformMailInput, map[string]any]{
		Name:           "admin_configure_platform_mail",
		Domain:         aitool.DomainAdmin,
		Title:          "配置平台系统发信 SMTP 或发送测试邮件",
		Summary:        "增量更新平台系统发信 SMTP 配置或发送连通性测试邮件",
		Tags:           []string{"mail", "smtp", "admin"},
		AliasesZH:      []string{"配置全局邮件服务", "修改平台 SMTP", "发送系统测试邮件"},
		AliasesEN:      []string{"configure platform mail", "update smtp settings", "test platform mail"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionConfigWrite)},
		Preview:        h.aiPreviewAdminConfigurePlatformMail,
		Execute:        h.aiExecuteAdminConfigurePlatformMail,
	})

	aitool.Register(reg, aitool.Spec[adminSaveGlobalNotificationChannelInput, notificationChannelResponse]{
		Name:           "admin_save_global_notification_channel",
		Domain:         aitool.DomainAdmin,
		Title:          "保存平台全局通知渠道",
		Summary:        "创建或纯增量修改平台全局通知渠道（支持预设创建）",
		Tags:           []string{"notifications", "admin"},
		AliasesZH:      []string{"创建全局通知渠道", "修改全局通知渠道"},
		AliasesEN:      []string{"save global notification channel", "create global notification channel"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "admin_list_platform_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "channel",
				StaticArguments: map[string]any{"section": "channels", "channelScope": "global"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewAdminSaveGlobalNotificationChannel,
		Execute: h.aiExecuteAdminSaveGlobalNotificationChannel,
	})

	aitool.Register(reg, aitool.Spec[adminSaveNotificationTemplateInput, model.NotificationTemplate]{
		Name:           "admin_save_notification_template",
		Domain:         aitool.DomainAdmin,
		Title:          "保存平台通知消息模板",
		Summary:        "创建或纯增量修改平台通知消息模板",
		Tags:           []string{"notifications", "templates", "admin"},
		AliasesZH:      []string{"创建通知消息模板", "修改通知模板"},
		AliasesEN:      []string{"save notification template", "create notification template"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "admin_list_platform_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "template",
				StaticArguments: map[string]any{"section": "templates"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewAdminSaveNotificationTemplate,
		Execute: h.aiExecuteAdminSaveNotificationTemplate,
	})

	aitool.Register(reg, aitool.Spec[adminSaveGlobalNotificationRuleInput, model.NotificationRule]{
		Name:           "admin_save_global_notification_rule",
		Domain:         aitool.DomainAdmin,
		Title:          "保存平台全局通知规则",
		Summary:        "创建或纯增量修改平台全局通知规则",
		Tags:           []string{"notifications", "rules", "admin"},
		AliasesZH:      []string{"创建全局通知规则", "修改全局通知规则"},
		AliasesEN:      []string{"save global notification rule", "create global notification rule"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "admin_list_platform_notifications",
				Family:          "resource",
				IDFrom:          "result.id",
				ReadIDArgument:  "rule",
				StaticArguments: map[string]any{"section": "rules"},
				Collection:      true,
				CollectionPath:  "items",
				Expect:          "present",
			},
		},
		Preview: h.aiPreviewAdminSaveGlobalNotificationRule,
		Execute: h.aiExecuteAdminSaveGlobalNotificationRule,
	})

	aitool.Register(reg, aitool.Spec[adminTestOrRetryGlobalNotificationInput, map[string]any]{
		Name:           "admin_test_or_retry_global_notification",
		Domain:         aitool.DomainAdmin,
		Title:          "测试全局通知渠道或重试全局规则投递",
		Summary:        "发送平台全局通知渠道测试消息或重试失败的全局规则通知投递记录",
		Tags:           []string{"notifications", "admin"},
		AliasesZH:      []string{"测试全局通知渠道", "重试全局通知投递"},
		AliasesEN:      []string{"test global notification channel", "retry global notification delivery"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Preview:        h.aiPreviewAdminTestOrRetryGlobalNotification,
		Execute:        h.aiExecuteAdminTestOrRetryGlobalNotification,
	})

	aitool.Register(reg, aitool.Spec[adminDeletePlatformNotificationResourceInput, map[string]any]{
		Name:           "admin_delete_platform_notification_resource",
		Domain:         aitool.DomainAdmin,
		Title:          "删除平台全局通知资源",
		Summary:        "删除平台全局通知渠道、通知消息模板或全局通知规则",
		Tags:           []string{"notifications", "admin"},
		AliasesZH:      []string{"删除全局通知渠道", "删除通知消息模板", "删除全局通知规则"},
		AliasesEN:      []string{"delete global notification channel", "delete notification template", "delete global notification rule"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionNotificationManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:        "admin_list_platform_notifications",
				Family:             "resource",
				WhenArgumentEquals: map[string]any{"resourceType": "channel"},
				IDFrom:             "result.id",
				ReadIDArgument:     "channel",
				StaticArguments:    map[string]any{"section": "channels", "channelScope": "global"},
				Collection:         true,
				CollectionPath:     "items",
				Expect:             "absent",
			},
			{
				OperationID:        "admin_list_platform_notifications",
				Family:             "resource",
				WhenArgumentEquals: map[string]any{"resourceType": "template"},
				IDFrom:             "result.id",
				ReadIDArgument:     "template",
				StaticArguments:    map[string]any{"section": "templates"},
				Collection:         true,
				CollectionPath:     "items",
				Expect:             "absent",
			},
			{
				OperationID:        "admin_list_platform_notifications",
				Family:             "resource",
				WhenArgumentEquals: map[string]any{"resourceType": "rule"},
				IDFrom:             "result.id",
				ReadIDArgument:     "rule",
				StaticArguments:    map[string]any{"section": "rules"},
				Collection:         true,
				CollectionPath:     "items",
				Expect:             "absent",
			},
		},
		Preview: h.aiPreviewAdminDeletePlatformNotificationResource,
		Execute: h.aiExecuteAdminDeletePlatformNotificationResource,
	})
}

func resolveGlobalChannelForAI(db *gorm.DB, ref string) (model.NotificationChannel, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.NotificationChannel{}, aitool.BadRequest("notification.channel_required", "channel is required")
	}
	var byID model.NotificationChannel
	if err := db.First(&byID, "id = ? AND owner_user_id = ''", ref).Error; err == nil {
		if notification.NormalizeChannelScope(byID.Scope) != "global" {
			return model.NotificationChannel{}, aitool.BadRequest("notification.channel_scope_not_global", "channel is project-scoped; global notification tools only operate on scope=global channels")
		}
		return byID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.NotificationChannel{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var channels []model.NotificationChannel
	if err := db.Where("owner_user_id = '' AND scope = 'global'").Order("created_at asc, id asc").Find(&channels).Error; err != nil {
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

func resolveGlobalTemplateForAI(db *gorm.DB, ref string) (model.NotificationTemplate, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.NotificationTemplate{}, aitool.BadRequest("notification.template_required", "template is required")
	}
	var templates []model.NotificationTemplate
	if err := db.Where("project_id = ''").Order("created_at asc, id asc").Find(&templates).Error; err != nil {
		return model.NotificationTemplate{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		templates,
		"notification_template",
		"notification.template_not_found",
		ref,
		func(t model.NotificationTemplate) string { return t.ID },
		func(t model.NotificationTemplate) string { return t.Name },
	)
}

func resolveGlobalRuleForAI(db *gorm.DB, ref string) (model.NotificationRule, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.NotificationRule{}, aitool.BadRequest("notification.rule_required", "rule is required")
	}
	var byID model.NotificationRule
	if err := db.First(&byID, "id = ?", ref).Error; err == nil {
		if strings.TrimSpace(byID.ProjectID) != "" {
			return model.NotificationRule{}, aitool.BadRequest("notification.rule_scope_not_global", "rule belongs to a project; global notification tools only operate on global rules")
		}
		return byID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.NotificationRule{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var rules []model.NotificationRule
	if err := db.Where("project_id = ''").Order("created_at asc, id asc").Find(&rules).Error; err != nil {
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

func (h *Handler) aiAdminListPlatformNotifications(tc *aitool.ToolContext, in adminListPlatformNotificationsInput) (map[string]any, error) {
	if _, err := h.aiRequireAdmin(tc); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	tc.SetNoStore()

	section := strings.ToLower(strings.TrimSpace(in.Section))
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	switch section {
	case "presets":
		return map[string]any{
			"section": "presets",
			"items":   notification.WebhookPresets(),
		}, nil

	case "channels":
		chScope := strings.ToLower(notifStringOrDefault(in.ChannelScope, "global"))
		query := db.Model(&model.NotificationChannel{}).Where("owner_user_id = ''")
		switch chScope {
		case "global":
			query = query.Where("scope = 'global'")
		case "project":
			query = query.Where("scope = 'project'")
		case "all":
			query = query.Where("scope IN ?", []string{"global", "project"})
		default:
			return nil, aitool.BadRequest("argument_invalid", "channelScope must be global, project, or all")
		}

		if in.Project != nil && strings.TrimSpace(*in.Project) != "" && chScope != "global" {
			proj, pErr := aitool.ResolveProject(db, *in.Project)
			if pErr != nil {
				return nil, pErr
			}
			sub := db.Model(&model.ScopedResourceProjectBinding{}).
				Select("resource_id").
				Where("resource_type = ? AND project_id = ?", notification.ScopedResourceChannelType, proj.ID)
			query = query.Where("scope = 'project' AND id IN (?)", sub)
		}
		if in.AdapterKind != nil && strings.TrimSpace(*in.AdapterKind) != "" {
			query = query.Where("adapter_kind = ?", strings.TrimSpace(*in.AdapterKind))
		}
		if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
			like := "%" + strings.TrimSpace(*in.Search) + "%"
			query = query.Where("(name ILIKE ? OR adapter_kind ILIKE ? OR preset_id ILIKE ?)", like, like, like)
		}
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
		h.populateChannelProjectIDs(channels, tc.Context())
		env := aitool.PageEnvelope(notificationChannelResponsesWithManage(channels, true, true), page, pageSize, "createdAt", "desc", total)
		env["section"] = "channels"
		env["channelScope"] = chScope
		return env, nil

	case "templates":
		query := db.Model(&model.NotificationTemplate{}).Where("project_id = ''")
		if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
			like := "%" + strings.TrimSpace(*in.Search) + "%"
			query = query.Where("(name ILIKE ? OR event_type ILIKE ?)", like, like)
		}
		if in.AdapterKind != nil && strings.TrimSpace(*in.AdapterKind) != "" {
			query = query.Where("adapter_kind = ?", strings.TrimSpace(*in.AdapterKind))
		}
		if in.EventType != nil && strings.TrimSpace(*in.EventType) != "" {
			query = query.Where("event_type = ?", strings.TrimSpace(*in.EventType))
		}
		if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
			tFilter := strings.TrimSpace(*in.Template)
			query = query.Where("(id = ? OR lower(name) = ?)", tFilter, strings.ToLower(tFilter))
		}

		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var templates []model.NotificationTemplate
		if err := query.Order("created_at desc, id desc").
			Limit(pageSize).
			Offset((page - 1) * pageSize).
			Find(&templates).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		env := aitool.PageEnvelope(templates, page, pageSize, "createdAt", "desc", total)
		env["section"] = "templates"
		return env, nil

	case "rules":
		query := db.Model(&model.NotificationRule{}).Where("project_id = ''")
		if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
			like := "%" + strings.TrimSpace(*in.Search) + "%"
			query = query.Where("name ILIKE ?", like)
		}
		if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
			tRef := strings.TrimSpace(*in.Template)
			var matchedTplIDs []string
			_ = db.Model(&model.NotificationTemplate{}).
				Where("project_id = '' AND (id = ? OR lower(name) = ?)", tRef, strings.ToLower(tRef)).
				Pluck("id", &matchedTplIDs).Error
			if len(matchedTplIDs) > 0 {
				query = query.Where("template_id IN ?", matchedTplIDs)
			} else {
				query = query.Where("template_id = ?", tRef)
			}
		}
		if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
			rFilter := strings.TrimSpace(*in.Rule)
			query = query.Where("(id = ? OR lower(name) = ?)", rFilter, strings.ToLower(rFilter))
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
		env := aitool.PageEnvelope(rules, page, pageSize, "createdAt", "desc", total)
		env["section"] = "rules"
		return env, nil

	case "deliveries":
		query := db.Model(&model.NotificationDelivery{}).
			Where("recipient_user_id = '' AND (rule_id = '' OR EXISTS (SELECT 1 FROM notification_rules r WHERE r.id = notification_deliveries.rule_id AND r.project_id = ''))")
		if in.Status != nil && strings.TrimSpace(*in.Status) != "" {
			query = query.Where("status = ?", strings.TrimSpace(*in.Status))
		}
		if in.EventType != nil && strings.TrimSpace(*in.EventType) != "" {
			query = query.Where("event_type = ?", strings.TrimSpace(*in.EventType))
		}
		if in.Project != nil && strings.TrimSpace(*in.Project) != "" {
			proj, pErr := aitool.ResolveProject(db, *in.Project)
			if pErr != nil {
				return nil, pErr
			}
			query = query.Where("project_id = ?", proj.ID)
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
			rRef := strings.TrimSpace(*in.Rule)
			var matchedRuleIDs []string
			_ = db.Model(&model.NotificationRule{}).
				Where("project_id = '' AND (id = ? OR lower(name) = ?)", rRef, strings.ToLower(rRef)).
				Pluck("id", &matchedRuleIDs).Error
			if len(matchedRuleIDs) > 0 {
				query = query.Where("rule_id IN ?", matchedRuleIDs)
			} else {
				query = query.Where("rule_id = ?", rRef)
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
		h.enrichNotificationDeliveries(deliveries, true, tc.Context())
		env := aitool.PageEnvelope(deliveries, page, pageSize, "createdAt", "desc", total)
		env["section"] = "deliveries"
		return env, nil

	default:
		return nil, aitool.BadRequest("argument_invalid", "section must be channels, templates, rules, deliveries, or presets")
	}
}

func preparePlatformMailPatch(existing model.PlatformMailSettings, in adminConfigurePlatformMailInput) (model.PlatformMailSettings, string, bool, bool, []string, []aitool.PreviewChange, error) {
	clearUser := notifBoolOrDefault(in.ClearUsername, false)
	clearPass := notifBoolOrDefault(in.ClearPassword, false)
	clearFromName := notifBoolOrDefault(in.ClearFromName, false)

	if clearUser && in.Username != nil && strings.TrimSpace(*in.Username) != "" {
		return model.PlatformMailSettings{}, "", false, false, nil, nil, aitool.BadRequest("argument_invalid", "username and clearUsername=true cannot both be provided")
	}
	if clearPass && in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		return model.PlatformMailSettings{}, "", false, false, nil, nil, aitool.BadRequest("argument_invalid", "password and clearPassword=true cannot both be provided")
	}
	if clearFromName && in.FromName != nil && strings.TrimSpace(*in.FromName) != "" {
		return model.PlatformMailSettings{}, "", false, false, nil, nil, aitool.BadRequest("argument_invalid", "fromName and clearFromName=true cannot both be provided")
	}

	mutated := false
	next := existing
	if in.Host != nil {
		next.Host = strings.TrimSpace(*in.Host)
		mutated = true
	}
	if in.Port != nil {
		next.Port = *in.Port
		mutated = true
	}
	if in.Security != nil && strings.TrimSpace(*in.Security) != "" {
		next.Security = strings.ToLower(strings.TrimSpace(*in.Security))
		mutated = true
	}
	if clearUser {
		next.Username = ""
		mutated = true
	} else if in.Username != nil {
		next.Username = strings.TrimSpace(*in.Username)
		mutated = true
	}

	newPassword := ""
	var sensitiveKeys []string
	if clearPass {
		next.PasswordRef = ""
		sensitiveKeys = append(sensitiveKeys, "password")
		mutated = true
	} else if in.Password != nil && strings.TrimSpace(*in.Password) != "" {
		newPassword = strings.TrimSpace(*in.Password)
		sensitiveKeys = append(sensitiveKeys, "password")
		mutated = true
	}

	if in.FromAddress != nil {
		next.FromAddress = strings.TrimSpace(*in.FromAddress)
		mutated = true
	}
	if clearFromName {
		next.FromName = ""
		mutated = true
	} else if in.FromName != nil {
		next.FromName = strings.TrimSpace(*in.FromName)
		mutated = true
	}
	if in.PersonalEmailCooldownSeconds != nil {
		next.PersonalEmailCooldownSeconds = *in.PersonalEmailCooldownSeconds
		mutated = true
	}

	sendTest := in.SendTestTo != nil && strings.TrimSpace(*in.SendTestTo) != ""
	if !mutated && !sendTest {
		return model.PlatformMailSettings{}, "", false, false, nil, nil, aitool.BadRequest("argument_invalid", "at least one SMTP setting field or sendTestTo must be provided")
	}

	normalized := platformmail.Normalize(next)
	if mutated {
		if err := platformmail.Validate(normalized, newPassword != ""); err != nil {
			return model.PlatformMailSettings{}, "", false, false, nil, nil, aitool.BadRequest("mail.settings_invalid", err.Error())
		}
	} else if sendTest {
		if err := platformmail.Validate(existing, false); err != nil {
			return model.PlatformMailSettings{}, "", false, false, nil, nil, aitool.BadRequest("mail.settings_invalid", err.Error())
		}
	}

	var changes []aitool.PreviewChange
	if normalized.Host != existing.Host {
		changes = append(changes, aitool.PreviewChange{Field: "host", Before: existing.Host, After: normalized.Host, Action: "update"})
	}
	if normalized.Port != existing.Port {
		changes = append(changes, aitool.PreviewChange{Field: "port", Before: existing.Port, After: normalized.Port, Action: "update"})
	}
	if normalized.Security != existing.Security {
		changes = append(changes, aitool.PreviewChange{Field: "security", Before: existing.Security, After: normalized.Security, Action: "update"})
	}
	if normalized.Username != existing.Username {
		changes = append(changes, aitool.PreviewChange{Field: "username", Before: existing.Username, After: normalized.Username, Action: "update"})
	}
	if normalized.FromAddress != existing.FromAddress {
		changes = append(changes, aitool.PreviewChange{Field: "fromAddress", Before: existing.FromAddress, After: normalized.FromAddress, Action: "update"})
	}
	if normalized.FromName != existing.FromName {
		changes = append(changes, aitool.PreviewChange{Field: "fromName", Before: existing.FromName, After: normalized.FromName, Action: "update"})
	}
	if normalized.PersonalEmailCooldownSeconds != existing.PersonalEmailCooldownSeconds {
		changes = append(changes, aitool.PreviewChange{Field: "personalEmailCooldownSeconds", Before: existing.PersonalEmailCooldownSeconds, After: normalized.PersonalEmailCooldownSeconds, Action: "update"})
	}
	sort.Strings(sensitiveKeys)
	return normalized, newPassword, clearPass, mutated, sensitiveKeys, changes, nil
}

func (h *Handler) aiPreviewAdminConfigurePlatformMail(tc *aitool.ToolContext, in adminConfigurePlatformMailInput) (*aitool.PreviewResult, error) {
	if _, err := h.aiRequireAdmin(tc); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	existing, err := platformmail.Get(tc.Context(), db)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "mail.settings_load_failed", err.Error())
	}

	next, _, _, mutated, sensitiveKeys, changes, err := preparePlatformMailPatch(existing, in)
	if err != nil {
		return nil, err
	}

	var impact []string
	if in.SendTestTo != nil && strings.TrimSpace(*in.SendTestTo) != "" {
		recipient, rErr := h.normalizedRegistrationEmail(*in.SendTestTo)
		if rErr != nil {
			return nil, aitool.BadRequest("mail.recipient_invalid", rErr.Error())
		}
		impact = append(impact, fmt.Sprintf("Sends a live test email to %s via platform SMTP (%s:%d).", recipient, next.Host, next.Port))
	}

	summary := "Send platform SMTP connectivity test email"
	if mutated {
		summary = fmt.Sprintf("Update platform SMTP settings (%s:%d)", next.Host, next.Port)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "platform_mail_settings",
			TargetID:       platformmail.SettingsID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
			Impact:         impact,
		},
	}, nil
}

func (h *Handler) aiExecuteAdminConfigurePlatformMail(tc *aitool.ToolContext, in adminConfigurePlatformMailInput) (map[string]any, error) {
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	existing, err := platformmail.Get(tc.Context(), db)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "mail.settings_load_failed", err.Error())
	}

	next, newPassword, clearPass, mutated, _, _, err := preparePlatformMailPatch(existing, in)
	if err != nil {
		return nil, err
	}

	saved := existing
	if mutated {
		const secretResource = "platform_mail_settings:" + platformmail.SettingsID + ":password"
		txErr := db.Transaction(func(tx *gorm.DB) error {
			oldRef := strings.TrimSpace(existing.PasswordRef)
			if clearPass {
				next.PasswordRef = ""
				if oldRef != "" {
					if dErr := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, oldRef, secretResource); dErr != nil {
						return dErr
					}
				}
			} else if newPassword != "" {
				newRef, sErr := h.secrets().StoreContextWithDB(tc.Context(), tx, newPassword, user.ID, secretResource)
				if sErr != nil {
					return sErr
				}
				next.PasswordRef = newRef
				if oldRef != "" && oldRef != newRef {
					if dErr := h.secrets().DeleteRefContextWithDB(tc.Context(), tx, oldRef, secretResource); dErr != nil {
						return dErr
					}
				}
			}
			var saveErr error
			saved, saveErr = platformmail.Save(tc.Context(), tx, next)
			return saveErr
		})
		if txErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "mail.settings_update_failed", txErr.Error())
		}
		h.auditWithContext(user.ID, "mail.settings.update", saved.ID, true, "mail settings updated", tc.Context())
	}

	resp := PlatformMailSettingsResponseFor(saved)
	out := map[string]any{
		"host":                         resp.Host,
		"port":                         resp.Port,
		"security":                     resp.Security,
		"username":                     resp.Username,
		"passwordSet":                  resp.PasswordSet,
		"fromAddress":                  resp.FromAddress,
		"fromName":                     resp.FromName,
		"personalEmailCooldownSeconds": resp.PersonalEmailCooldownSeconds,
		"testSent":                     false,
	}

	if in.SendTestTo != nil && strings.TrimSpace(*in.SendTestTo) != "" {
		recipient, rErr := h.normalizedRegistrationEmail(*in.SendTestTo)
		if rErr != nil {
			return nil, aitool.BadRequest("mail.recipient_invalid", rErr.Error())
		}
		msg, renderErr := notification.RenderEventEmail(notification.TestEvent(time.Now()), user.Language, notification.DefaultTimeZone)
		if renderErr != nil {
			msg = notification.RenderedMessage{
				Subject: "Luna DevOps test email",
				Body:    "This test email confirms that the Luna DevOps platform mail service is configured correctly.",
			}
		}
		if _, sendErr := platformmail.Send(tc.Context(), db, h.secrets(), recipient, msg); sendErr != nil {
			h.auditWithContext(user.ID, "mail.settings.test", platformmail.SettingsID, false, sendErr.Error(), tc.Context())
			return nil, aitool.NewError(http.StatusBadGateway, "mail.test_failed", sendErr.Error())
		}
		h.auditWithContext(user.ID, "mail.settings.test", platformmail.SettingsID, true, "", tc.Context())
		out["testSent"] = true
		out["testRecipient"] = recipient
	}
	return out, nil
}

func (h *Handler) aiPreviewAdminSaveGlobalNotificationChannel(tc *aitool.ToolContext, in adminSaveGlobalNotificationChannelInput) (*aitool.PreviewResult, error) {
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationChannel
	resolvedID := ""
	if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
		found, resolveErr := resolveGlobalChannelForAI(db, *in.Channel)
		if resolveErr != nil {
			return nil, resolveErr
		}
		existing = &found
		resolvedID = found.ID
	}

	draft, _, _, sensitiveKeys, changes, err := buildScopedChannelSaveDraft(
		tc, h, user, "global", existing,
		in.Name, in.PresetID, in.AdapterKind,
		in.UpsertConfig, in.RemoveConfigKeys, in.UpsertSecrets, in.ClearSecretKeys, in.Enabled,
	)
	if err != nil {
		return nil, err
	}

	summary := fmt.Sprintf("Create global notification channel %q (%s)", draft.Name, draft.AdapterKind)
	if existing != nil {
		summary = fmt.Sprintf("Update global notification channel %q (%s)", draft.Name, existing.ID)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "global_notification_channel",
			TargetID:       resolvedID,
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
		LockedPayload: lockedAdminNotifMutation{
			ResourceType: "channel",
			ResolvedID:   resolvedID,
		},
	}, nil
}

func (h *Handler) aiExecuteAdminSaveGlobalNotificationChannel(tc *aitool.ToolContext, in adminSaveGlobalNotificationChannelInput) (notificationChannelResponse, error) {
	var locked lockedAdminNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedID != "" {
		idCopy := locked.ResolvedID
		in.Channel = &idCopy
	}
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return notificationChannelResponse{}, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationChannel
	if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
		found, resolveErr := resolveGlobalChannelForAI(db, *in.Channel)
		if resolveErr != nil {
			return notificationChannelResponse{}, resolveErr
		}
		existing = &found
	}

	draft, toSet, toClear, _, _, err := buildScopedChannelSaveDraft(
		tc, h, user, "global", existing,
		in.Name, in.PresetID, in.AdapterKind,
		in.UpsertConfig, in.RemoveConfigKeys, in.UpsertSecrets, in.ClearSecretKeys, in.Enabled,
	)
	if err != nil {
		return notificationChannelResponse{}, err
	}

	if existing == nil {
		wantEnabled := draft.Enabled
		txErr := db.Transaction(func(tx *gorm.DB) error {
			secretRefs := map[string]string{}
			refOverlay := make(map[string]string, len(toSet)*2)
			for key, val := range toSet {
				refOverlay[key] = val
				ref, sErr := h.secrets().StoreContextWithDB(tc.Context(), tx, val, user.ID, "notification_channel:"+draft.ID+":"+key)
				if sErr != nil {
					return sErr
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
			return nil
		})
		var vErr errChannelValidation
		if errors.As(txErr, &vErr) {
			return notificationChannelResponse{}, aitool.BadRequest("notification.channel_invalid", vErr.err.Error())
		}
		if txErr != nil {
			return notificationChannelResponse{}, aitool.NewError(http.StatusInternalServerError, "notification.channel_save_failed", txErr.Error())
		}
		h.auditWithContext(user.ID, "notification.channel.create", draft.ID, true, "global", tc.Context())
		resp := notificationChannelResponseForWithPrivacy(draft, true)
		resp.CanManage = true
		return resp, nil
	}

	var saved model.NotificationChannel
	txErr := db.Transaction(func(tx *gorm.DB) error {
		var lockedRow model.NotificationChannel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRow, "id = ? AND owner_user_id = '' AND scope = 'global'", existing.ID).Error; err != nil {
			return err
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
			newRef, sErr := h.secrets().StoreContextWithDB(tc.Context(), tx, val, user.ID, resource)
			if sErr != nil {
				return sErr
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
		if draft.AdapterKind == notification.AdapterKindSMTP && notifContainsString(toClear, "password") {
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
		if compatErr := ensureChannelUpdateCompatibleWithRules(tx, lockedRow, "global", nil); compatErr != nil {
			return compatErr
		}
		if sErr := tx.Save(&lockedRow).Error; sErr != nil {
			return sErr
		}
		saved = lockedRow
		return nil
	})
	var vErr errChannelValidation
	switch {
	case errors.As(txErr, &vErr):
		return notificationChannelResponse{}, aitool.BadRequest("notification.channel_invalid", vErr.err.Error())
	case errors.Is(txErr, gorm.ErrRecordNotFound):
		return notificationChannelResponse{}, aitool.NotFound("notification.channel_not_found", "global notification channel was not found")
	case errors.Is(txErr, errChannelInUse):
		return notificationChannelResponse{}, aitool.Conflict("notification.channel_in_use", "channel is referenced by notification rules")
	case errors.Is(txErr, errChannelSMTPRecipientRequired):
		return notificationChannelResponse{}, aitool.BadRequest("notification.smtp_recipient_required", "smtp channel requires at least one recipient on channel or referencing rules")
	case txErr != nil:
		return notificationChannelResponse{}, aitool.NewError(http.StatusInternalServerError, "notification.channel_save_failed", txErr.Error())
	}

	h.auditWithContext(user.ID, "notification.channel.update", saved.ID, true, "global", tc.Context())
	resp := notificationChannelResponseForWithPrivacy(saved, true)
	resp.CanManage = true
	return resp, nil
}

func notifContainsString(items []string, target string) bool {
	for _, s := range items {
		if s == target {
			return true
		}
	}
	return false
}

func prepareNotificationTemplateDraft(user model.User, existing *model.NotificationTemplate, in adminSaveNotificationTemplateInput) (model.NotificationTemplate, []aitool.PreviewChange, error) {
	if notifBoolOrDefault(in.ClearSubjectTemplate, false) && in.SubjectTemplate != nil && strings.TrimSpace(*in.SubjectTemplate) != "" {
		return model.NotificationTemplate{}, nil, aitool.BadRequest("argument_invalid", "subjectTemplate and clearSubjectTemplate=true are mutually exclusive")
	}
	if notifBoolOrDefault(in.ClearBodyTemplate, false) && in.BodyTemplate != nil && strings.TrimSpace(*in.BodyTemplate) != "" {
		return model.NotificationTemplate{}, nil, aitool.BadRequest("argument_invalid", "bodyTemplate and clearBodyTemplate=true are mutually exclusive")
	}
	if notifBoolOrDefault(in.ClearJSONBodyTemplate, false) && in.JSONBodyTemplate != nil && strings.TrimSpace(*in.JSONBodyTemplate) != "" {
		return model.NotificationTemplate{}, nil, aitool.BadRequest("argument_invalid", "jsonBodyTemplate and clearJsonBodyTemplate=true are mutually exclusive")
	}

	isCreate := existing == nil
	draft := model.NotificationTemplate{
		ID:        id.New("ntp"),
		Locale:    "zh-CN",
		Enabled:   true,
		CreatedBy: user.ID,
	}
	if !isCreate {
		draft = *existing
	}

	if in.Name != nil {
		draft.Name = strings.TrimSpace(*in.Name)
	}
	if in.EventType != nil {
		draft.EventType = strings.TrimSpace(*in.EventType)
	}
	if in.AdapterKind != nil {
		draft.AdapterKind = strings.TrimSpace(*in.AdapterKind)
	}
	if in.Locale != nil && strings.TrimSpace(*in.Locale) != "" {
		loc := strings.TrimSpace(*in.Locale)
		if loc != "zh-CN" && loc != "en-US" {
			return model.NotificationTemplate{}, nil, aitool.BadRequest("argument_invalid", "locale must be zh-CN or en-US")
		}
		draft.Locale = loc
	}
	if notifBoolOrDefault(in.ClearSubjectTemplate, false) {
		draft.SubjectTemplate = ""
	} else if in.SubjectTemplate != nil {
		draft.SubjectTemplate = *in.SubjectTemplate
	}
	if notifBoolOrDefault(in.ClearBodyTemplate, false) {
		draft.BodyTemplate = ""
	} else if in.BodyTemplate != nil {
		draft.BodyTemplate = *in.BodyTemplate
	}
	if notifBoolOrDefault(in.ClearJSONBodyTemplate, false) {
		draft.JSONBodyTemplate = ""
	} else if in.JSONBodyTemplate != nil {
		draft.JSONBodyTemplate = *in.JSONBodyTemplate
	}
	if in.Enabled != nil {
		draft.Enabled = *in.Enabled
	}

	if draft.Name == "" || draft.EventType == "" || draft.AdapterKind == "" {
		return model.NotificationTemplate{}, nil, aitool.BadRequest("notification.template_required", "name, eventType, and adapterKind are required")
	}
	if draft.AdapterKind != notification.AdapterKindWebhook && draft.AdapterKind != notification.AdapterKindSMTP {
		return model.NotificationTemplate{}, nil, aitool.BadRequest("notification.adapter_invalid", "adapterKind must be webhook or smtp")
	}

	var changes []aitool.PreviewChange
	if isCreate {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: draft.Name, Action: "create"},
			aitool.PreviewChange{Field: "eventType", After: draft.EventType, Action: "create"},
			aitool.PreviewChange{Field: "adapterKind", After: draft.AdapterKind, Action: "create"},
			aitool.PreviewChange{Field: "enabled", After: draft.Enabled, Action: "create"},
		)
	} else {
		if draft.Name != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: draft.Name, Action: "update"})
		}
		if draft.EventType != existing.EventType {
			changes = append(changes, aitool.PreviewChange{Field: "eventType", Before: existing.EventType, After: draft.EventType, Action: "update"})
		}
		if draft.AdapterKind != existing.AdapterKind {
			changes = append(changes, aitool.PreviewChange{Field: "adapterKind", Before: existing.AdapterKind, After: draft.AdapterKind, Action: "update"})
		}
		if draft.Enabled != existing.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: draft.Enabled, Action: "update"})
		}
	}
	return draft, changes, nil
}

func (h *Handler) aiPreviewAdminSaveNotificationTemplate(tc *aitool.ToolContext, in adminSaveNotificationTemplateInput) (*aitool.PreviewResult, error) {
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationTemplate
	resolvedID := ""
	if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
		found, rErr := resolveGlobalTemplateForAI(db, *in.Template)
		if rErr != nil {
			return nil, rErr
		}
		existing = &found
		resolvedID = found.ID
	}

	draft, changes, err := prepareNotificationTemplateDraft(user, existing, in)
	if err != nil {
		return nil, err
	}

	summary := fmt.Sprintf("Create notification template %q (%s)", draft.Name, draft.EventType)
	if existing != nil {
		summary = fmt.Sprintf("Update notification template %q (%s)", draft.Name, existing.ID)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "notification_template",
			TargetID:       resolvedID,
			Changes:        changes,
		},
		LockedPayload: lockedAdminNotifMutation{
			ResourceType: "template",
			ResolvedID:   resolvedID,
		},
	}, nil
}

func (h *Handler) aiExecuteAdminSaveNotificationTemplate(tc *aitool.ToolContext, in adminSaveNotificationTemplateInput) (model.NotificationTemplate, error) {
	var locked lockedAdminNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedID != "" {
		idCopy := locked.ResolvedID
		in.Template = &idCopy
	}
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return model.NotificationTemplate{}, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationTemplate
	if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
		found, rErr := resolveGlobalTemplateForAI(db, *in.Template)
		if rErr != nil {
			return model.NotificationTemplate{}, rErr
		}
		existing = &found
	}

	draft, _, err := prepareNotificationTemplateDraft(user, existing, in)
	if err != nil {
		return model.NotificationTemplate{}, err
	}

	if existing == nil {
		wantEnabled := draft.Enabled
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&draft).Error; err != nil {
				return err
			}
			if draft.Enabled != wantEnabled {
				if err := tx.Model(&model.NotificationTemplate{}).Where("id = ?", draft.ID).Update("enabled", wantEnabled).Error; err != nil {
					return err
				}
				draft.Enabled = wantEnabled
			}
			return nil
		}); err != nil {
			return model.NotificationTemplate{}, aitool.NewError(http.StatusInternalServerError, "notification.template_save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.template.create", draft.ID, true, "", tc.Context())
	} else {
		if err := db.Save(&draft).Error; err != nil {
			return model.NotificationTemplate{}, aitool.NewError(http.StatusInternalServerError, "notification.template_save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.template.update", draft.ID, true, "", tc.Context())
	}
	return draft, nil
}

func (h *Handler) buildGlobalRuleSaveDraft(tc *aitool.ToolContext, user model.User, existing *model.NotificationRule, in adminSaveGlobalNotificationRuleInput) (model.NotificationRule, []aitool.PreviewChange, error) {
	db := h.aiDB(tc)
	if notifBoolOrDefault(in.ClearTemplate, false) && in.Template != nil && strings.TrimSpace(*in.Template) != "" {
		return model.NotificationRule{}, nil, aitool.BadRequest("argument_invalid", "template and clearTemplate=true are mutually exclusive")
	}

	isCreate := existing == nil
	base := model.NotificationRule{
		ID:        id.New("nrl"),
		ProjectID: "",
		CreatedBy: user.ID,
	}
	if !isCreate {
		base = *existing
	}

	resolveChID := func(ref string) (string, error) {
		ch, err := resolveGlobalChannelForAI(db, ref)
		if err != nil {
			return "", err
		}
		return ch.ID, nil
	}

	var addChIDs, rmChIDs []string
	for _, ref := range in.AddChannels {
		cid, err := resolveChID(ref)
		if err != nil {
			return model.NotificationRule{}, nil, err
		}
		addChIDs = append(addChIDs, cid)
	}
	for _, ref := range in.RemoveChannels {
		if cid, err := resolveChID(ref); err == nil {
			rmChIDs = append(rmChIDs, cid)
		} else {
			rmChIDs = append(rmChIDs, strings.TrimSpace(ref))
		}
	}

	nextName := base.Name
	if in.Name != nil {
		nextName = strings.TrimSpace(*in.Name)
	}
	nextEvents := applyStringSetPatch(decodeStringList(base.EventTypesJSON), in.AddEventTypes, in.RemoveEventTypes)
	nextChannels := applyStringSetPatch(decodeStringList(base.ChannelIDsJSON), addChIDs, rmChIDs)
	targetsRaw, tErr := applyChannelTargetPatches(base.ChannelTargetsJSON, in.UpsertChannelTargets, in.RemoveChannelTargets, resolveChID)
	if tErr != nil {
		return model.NotificationRule{}, nil, tErr
	}

	nextTemplateID := base.TemplateID
	if notifBoolOrDefault(in.ClearTemplate, false) {
		nextTemplateID = ""
	} else if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
		tpl, tplErr := resolveGlobalTemplateForAI(db, *in.Template)
		if tplErr != nil {
			return model.NotificationRule{}, nil, tplErr
		}
		nextTemplateID = tpl.ID
	}

	currentFilter := notification.RuleFilter{Scope: notification.RuleScopeAll}
	if !isCreate && strings.TrimSpace(base.FilterJSON) != "" {
		if decoded, err := notification.DecodeRuleFilter([]byte(base.FilterJSON)); err == nil {
			currentFilter = decoded
		}
	}

	if in.Filter != nil {
		fp := in.Filter
		if notifBoolOrDefault(fp.ClearFilter, false) {
			currentFilter = notification.RuleFilter{Scope: notification.RuleScopeAll}
		} else {
			var addProjIDs, rmProjIDs []string
			for _, pRef := range fp.AddProjects {
				proj, pErr := aitool.ResolveProject(db, pRef)
				if pErr != nil {
					return model.NotificationRule{}, nil, pErr
				}
				addProjIDs = append(addProjIDs, proj.ID)
			}
			for _, pRef := range fp.RemoveProjects {
				if proj, pErr := aitool.ResolveProject(db, pRef); pErr == nil {
					rmProjIDs = append(rmProjIDs, proj.ID)
				} else {
					rmProjIDs = append(rmProjIDs, strings.TrimSpace(pRef))
				}
			}
			if fp.Scope != nil && strings.TrimSpace(*fp.Scope) != "" {
				currentFilter.Scope = strings.TrimSpace(*fp.Scope)
			} else if len(addProjIDs) > 0 && currentFilter.Scope == "" {
				currentFilter.Scope = notification.RuleScopeProjects
			}
			if currentFilter.Scope == notification.RuleScopeAll {
				if len(addProjIDs) > 0 {
					currentFilter.Scope = notification.RuleScopeProjects
					currentFilter.ProjectIDs = applyStringSetPatch(currentFilter.ProjectIDs, addProjIDs, rmProjIDs)
				} else {
					currentFilter.ProjectIDs = nil
				}
			} else {
				currentFilter.ProjectIDs = applyStringSetPatch(currentFilter.ProjectIDs, addProjIDs, rmProjIDs)
			}

			if notifBoolOrDefault(fp.ClearSeverities, false) {
				currentFilter.Severities = nil
			} else if fp.SetSeverities != nil {
				currentFilter.Severities = fp.SetSeverities
			}

			if notifBoolOrDefault(fp.ClearApplications, false) {
				currentFilter.ApplicationIDs = nil
			} else {
				currentFilter.ApplicationIDs = applyStringSetPatch(currentFilter.ApplicationIDs, fp.AddApplications, fp.RemoveApplications)
			}

			if notifBoolOrDefault(fp.ClearTargets, false) {
				currentFilter.DeploymentTargetIDs = nil
			} else {
				currentFilter.DeploymentTargetIDs = applyStringSetPatch(currentFilter.DeploymentTargetIDs, fp.AddTargets, fp.RemoveTargets)
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
		TemplateID:            nextTemplateID,
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
		return model.NotificationRule{}, nil, toolErrorFromScratchRecorder(rec, "notification.rule_invalid", "invalid global notification rule")
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
		if validated.TemplateID != existing.TemplateID {
			changes = append(changes, aitool.PreviewChange{Field: "templateId", Before: existing.TemplateID, After: validated.TemplateID, Action: "update"})
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

func (h *Handler) aiPreviewAdminSaveGlobalNotificationRule(tc *aitool.ToolContext, in adminSaveGlobalNotificationRuleInput) (*aitool.PreviewResult, error) {
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationRule
	resolvedID := ""
	if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
		found, rErr := resolveGlobalRuleForAI(db, *in.Rule)
		if rErr != nil {
			return nil, rErr
		}
		existing = &found
		resolvedID = found.ID
	}

	draft, changes, err := h.buildGlobalRuleSaveDraft(tc, user, existing, in)
	if err != nil {
		return nil, err
	}

	summary := fmt.Sprintf("Create global notification rule %q", draft.Name)
	if existing != nil {
		summary = fmt.Sprintf("Update global notification rule %q (%s)", draft.Name, existing.ID)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "global_notification_rule",
			TargetID:       resolvedID,
			Changes:        changes,
		},
		LockedPayload: lockedAdminNotifMutation{
			ResourceType: "rule",
			ResolvedID:   resolvedID,
		},
	}, nil
}

func (h *Handler) aiExecuteAdminSaveGlobalNotificationRule(tc *aitool.ToolContext, in adminSaveGlobalNotificationRuleInput) (model.NotificationRule, error) {
	var locked lockedAdminNotifMutation
	if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr == nil && locked.ResolvedID != "" {
		idCopy := locked.ResolvedID
		in.Rule = &idCopy
	}
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return model.NotificationRule{}, err
	}
	db := h.aiDB(tc)

	var existing *model.NotificationRule
	if in.Rule != nil && strings.TrimSpace(*in.Rule) != "" {
		found, rErr := resolveGlobalRuleForAI(db, *in.Rule)
		if rErr != nil {
			return model.NotificationRule{}, rErr
		}
		existing = &found
	}

	draft, _, err := h.buildGlobalRuleSaveDraft(tc, user, existing, in)
	if err != nil {
		return model.NotificationRule{}, err
	}

	if existing == nil {
		if err := createNotificationRule(db, &draft); err != nil {
			return model.NotificationRule{}, aitool.NewError(http.StatusInternalServerError, "notification.rule_save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.rule.create", draft.ID, true, "", tc.Context())
	} else {
		if err := db.Save(&draft).Error; err != nil {
			return model.NotificationRule{}, aitool.NewError(http.StatusInternalServerError, "notification.rule_save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "notification.rule.update", draft.ID, true, "", tc.Context())
	}
	return draft, nil
}

func loadGlobalRuleDeliveryForRetry(db *gorm.DB, deliveryID string) (model.NotificationDelivery, error) {
	var delivery model.NotificationDelivery
	if err := db.First(&delivery, "id = ? AND recipient_user_id = ''", strings.TrimSpace(deliveryID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.NotificationDelivery{}, aitool.NotFound("notification.delivery_not_found", "global notification delivery was not found")
		}
		return model.NotificationDelivery{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if ruleID := strings.TrimSpace(delivery.RuleID); ruleID != "" {
		var rule model.NotificationRule
		if err := db.Unscoped().First(&rule, "id = ?", ruleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.NotificationDelivery{}, aitool.NotFound("notification.delivery_not_found", "associated notification rule was not found")
			}
			return model.NotificationDelivery{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if strings.TrimSpace(rule.ProjectID) != "" {
			return model.NotificationDelivery{}, aitool.BadRequest("notification.delivery_scope_not_global", "delivery belongs to a project notification rule; use ops_test_or_retry_project_notification instead")
		}
	}
	return delivery, nil
}

func (h *Handler) aiPreviewAdminTestOrRetryGlobalNotification(tc *aitool.ToolContext, in adminTestOrRetryGlobalNotificationInput) (*aitool.PreviewResult, error) {
	user, err := h.aiRequireAdmin(tc)
	if err != nil {
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
		channel, resolveErr := resolveGlobalChannelForAI(db, chRef)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Send test notification via global channel %q (%s)", channel.Name, channel.ID),
				TargetResource: "global_notification_channel",
				TargetID:       channel.ID,
				Impact:         []string{"Dispatches a live external test notification via the selected global channel."},
			},
			LockedPayload: lockedAdminNotifMutation{
				ResourceType: "channel",
				ResolvedID:   channel.ID,
			},
		}, nil
	}

	delivery, err := loadGlobalRuleDeliveryForRetry(db, deliveryID)
	if err != nil {
		return nil, err
	}
	if err := ensureDeliveryRetryable(delivery, time.Now()); err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Retry failed global notification delivery %s (event=%s)", delivery.ID, delivery.EventType),
			TargetResource: "notification_delivery",
			TargetID:       delivery.ID,
			Changes: []aitool.PreviewChange{
				{Field: "status", Before: delivery.Status, After: "pending", Action: "retry"},
			},
		},
		LockedPayload: lockedAdminNotifMutation{
			DeliveryID: delivery.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteAdminTestOrRetryGlobalNotification(tc *aitool.ToolContext, in adminTestOrRetryGlobalNotificationInput) (map[string]any, error) {
	var locked lockedAdminNotifMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	chRef, deliveryID, err := validateTestOrRetryChoice(in.Channel, in.RetryDeliveryID)
	if err != nil {
		return nil, err
	}

	if chRef != "" {
		if locked.ResolvedID != "" {
			chRef = locked.ResolvedID
		}
		recipient, rErr := parseOptionalTestRecipient(in.TestRecipient, user.Email)
		if rErr != nil {
			return nil, rErr
		}
		channel, resolveErr := resolveGlobalChannelForAI(db, chRef)
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
		testCtx := notification.WithEgressPolicy(tc.Context(), notification.EgressPolicyForScope("global"))
		if recipient != "" {
			testCtx = notification.WithTestRecipient(testCtx, recipient)
		}
		if tErr := adapter.Test(testCtx, json.RawMessage(channel.ConfigJSON), json.RawMessage(channel.SecretRefsJSON), h.secrets()); tErr != nil {
			h.auditWithContext(user.ID, "notification.channel.test", channel.ID, false, tErr.Error(), tc.Context())
			return nil, aitool.NewError(http.StatusBadGateway, "notification.channel_test_failed", tErr.Error())
		}
		h.auditWithContext(user.ID, "notification.channel.test", channel.ID, true, "", tc.Context())
		return map[string]any{
			"mode":      "test",
			"channelId": channel.ID,
			"status":    "ok",
		}, nil
	}

	targetDeliveryID := firstNonEmpty(locked.DeliveryID, deliveryID)
	existing, err := loadGlobalRuleDeliveryForRetry(db, targetDeliveryID)
	if err != nil {
		return nil, err
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
	h.auditWithContext(user.ID, "notification.delivery.retry", retried.ID, true, "", tc.Context())

	return map[string]any{
		"mode":       "retry",
		"deliveryId": items[0].ID,
		"status":     items[0].Status,
		"delivery":   items[0],
	}, nil
}

func (h *Handler) aiPreviewAdminDeletePlatformNotificationResource(tc *aitool.ToolContext, in adminDeletePlatformNotificationResourceInput) (*aitool.PreviewResult, error) {
	if _, err := h.aiRequireAdmin(tc); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	resType := strings.ToLower(strings.TrimSpace(in.ResourceType))

	switch resType {
	case "channel":
		ch, err := resolveGlobalChannelForAI(db, in.Resource)
		if err != nil {
			return nil, err
		}
		referenced, rErr := channelReferencedByAnyRule(db, ch.ID)
		if rErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", rErr.Error())
		}
		if referenced {
			return nil, aitool.Conflict("notification.channel_in_use", "remove this global channel from all notification rules before deleting")
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Delete global notification channel %q (%s)", ch.Name, ch.ID),
				TargetResource: "global_notification_channel",
				TargetID:       ch.ID,
				Changes: []aitool.PreviewChange{
					{Field: "channel", Before: ch.Name, Action: "delete"},
				},
			},
			LockedPayload: lockedAdminNotifMutation{ResourceType: "channel", ResolvedID: ch.ID},
		}, nil

	case "template":
		tpl, err := resolveGlobalTemplateForAI(db, in.Resource)
		if err != nil {
			return nil, err
		}
		var refRules int64
		_ = db.Model(&model.NotificationRule{}).Where("template_id = ?", tpl.ID).Count(&refRules).Error
		var impact []string
		if refRules > 0 {
			impact = append(impact, fmt.Sprintf("Detaches custom template from %d referencing notification rule(s), reverting them to built-in default templates.", refRules))
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Delete notification template %q (%s)", tpl.Name, tpl.ID),
				TargetResource: "notification_template",
				TargetID:       tpl.ID,
				Changes: []aitool.PreviewChange{
					{Field: "template", Before: tpl.Name, Action: "delete"},
				},
				Impact: impact,
			},
			LockedPayload: lockedAdminNotifMutation{ResourceType: "template", ResolvedID: tpl.ID},
		}, nil

	case "rule":
		rule, err := resolveGlobalRuleForAI(db, in.Resource)
		if err != nil {
			return nil, err
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Delete global notification rule %q (%s)", rule.Name, rule.ID),
				TargetResource: "global_notification_rule",
				TargetID:       rule.ID,
				Changes: []aitool.PreviewChange{
					{Field: "rule", Before: rule.Name, Action: "delete"},
				},
			},
			LockedPayload: lockedAdminNotifMutation{ResourceType: "rule", ResolvedID: rule.ID},
		}, nil

	default:
		return nil, aitool.BadRequest("argument_invalid", "resourceType must be channel, template, or rule")
	}
}

func (h *Handler) aiExecuteAdminDeletePlatformNotificationResource(tc *aitool.ToolContext, in adminDeletePlatformNotificationResourceInput) (map[string]any, error) {
	var locked lockedAdminNotifMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	user, err := h.aiRequireAdmin(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)
	resType := strings.ToLower(strings.TrimSpace(in.ResourceType))
	targetRef := firstNonEmpty(locked.ResolvedID, in.Resource)

	switch resType {
	case "channel":
		ch, err := resolveGlobalChannelForAI(db, targetRef)
		if err != nil {
			return nil, err
		}
		txErr := db.Transaction(func(tx *gorm.DB) error {
			var lockedRow model.NotificationChannel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRow, "id = ? AND owner_user_id = '' AND scope = 'global'", ch.ID).Error; err != nil {
				return err
			}
			referenced, rErr := channelReferencedByAnyRule(tx, lockedRow.ID)
			if rErr != nil {
				return rErr
			}
			if referenced {
				return errChannelInUse
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
			res := tx.Delete(&lockedRow)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
		switch {
		case errors.Is(txErr, gorm.ErrRecordNotFound):
			return nil, aitool.NotFound("notification.channel_not_found", "global notification channel was not found")
		case errors.Is(txErr, errChannelInUse):
			return nil, aitool.Conflict("notification.channel_in_use", "remove this global channel from all notification rules before deleting")
		case txErr != nil:
			return nil, aitool.NewError(http.StatusInternalServerError, "notification.channel_delete_failed", txErr.Error())
		}
		h.auditWithContext(user.ID, "notification.channel.delete", ch.ID, true, "global", tc.Context())
		return map[string]any{
			"id":           ch.ID,
			"resourceType": "channel",
			"section":      "channels",
			"deleted":      true,
		}, nil

	case "template":
		tpl, err := resolveGlobalTemplateForAI(db, targetRef)
		if err != nil {
			return nil, err
		}
		txErr := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.NotificationRule{}).Where("template_id = ?", tpl.ID).Update("template_id", "").Error; err != nil {
				return err
			}
			res := tx.Where("id = ? AND project_id = ''", tpl.ID).Delete(&model.NotificationTemplate{})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("notification.template_not_found", "notification template was not found")
		}
		if txErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "notification.template_delete_failed", txErr.Error())
		}
		h.auditWithContext(user.ID, "notification.template.delete", tpl.ID, true, "", tc.Context())
		return map[string]any{
			"id":           tpl.ID,
			"resourceType": "template",
			"section":      "templates",
			"deleted":      true,
		}, nil

	case "rule":
		rule, err := resolveGlobalRuleForAI(db, targetRef)
		if err != nil {
			return nil, err
		}
		res := db.Where("id = ? AND project_id = ''", rule.ID).Delete(&model.NotificationRule{})
		if res.Error != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "notification.rule_delete_failed", res.Error.Error())
		}
		if res.RowsAffected == 0 {
			return nil, aitool.NotFound("notification.rule_not_found", "global notification rule was not found")
		}
		h.auditWithContext(user.ID, "notification.rule.delete", rule.ID, true, "", tc.Context())
		return map[string]any{
			"id":           rule.ID,
			"resourceType": "rule",
			"section":      "rules",
			"deleted":      true,
		}, nil

	default:
		return nil, aitool.BadRequest("argument_invalid", "resourceType must be channel, template, or rule")
	}
}
