package billingapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const billingUsageMeterEnum = "build.cpu_vcpu_minute|build.memory_gib_minute|build.job|runtime.cpu_vcpu_hour|runtime.memory_gib_hour|storage.gib_day|storage.transfer_gib|gateway.egress_gib|gateway.requests_1000|ai.prompt_tokens_1m|ai.completion_tokens_1m|ai.cached_prompt_tokens_1m|ai.cache_write_prompt_tokens_1m"
const billingRateRuleMeterEnum = "build.cpu_vcpu_minute|build.memory_gib_minute|runtime.cpu_vcpu_hour|runtime.memory_gib_hour|storage.gib_day|storage.transfer_gib|gateway.egress_gib|gateway.requests_1000"

type opsGetBillingSummaryInput struct {
	Project                *string `json:"project,omitempty" desc:"可选限定查看本人作为 billingOwner 的项目费用归属（查个人总余额或 ai.* 费用请省略）"`
	PeriodStart            *string `json:"periodStart,omitempty" desc:"RFC3339 起始时间（与 periodEnd 成对传入，半开区间 [periodStart, periodEnd)）"`
	PeriodEnd              *string `json:"periodEnd,omitempty" desc:"RFC3339 结束时间（须晚于 periodStart）"`
	TimeZone               *string `json:"timeZone,omitempty" default:"UTC" desc:"IANA 时区名（如 Asia/Shanghai，默认 UTC），决定今日/本月边界"`
	IncludeDeploymentSpend *bool   `json:"includeDeploymentSpend,omitempty" default:"true" desc:"是否附带按应用/环境的消费排行（默认 true）"`
	IncludeRateRules       *bool   `json:"includeRateRules,omitempty" default:"false" desc:"是否附带平台计费单价规则表（默认 false）"`
}

type opsListBillingRecordsInput struct {
	RecordType  string  `json:"recordType" required:"true" enum:"ledger|usage" desc:"查询记录类型：ledger（钱包流水）或 usage（资源计量明细）"`
	Project     *string `json:"project,omitempty" desc:"限定本人作为 billingOwner 的项目 ID 或 identifier（ai.* 费用不归属项目，查 ai.* 时请勿传入）"`
	Meter       *string `json:"meter,omitempty" enum:"build.cpu_vcpu_minute|build.memory_gib_minute|build.job|runtime.cpu_vcpu_hour|runtime.memory_gib_hour|storage.gib_day|storage.transfer_gib|gateway.egress_gib|gateway.requests_1000|ai.prompt_tokens_1m|ai.completion_tokens_1m|ai.cached_prompt_tokens_1m|ai.cache_write_prompt_tokens_1m" desc:"recordType=usage 时按计量项枚举过滤"`
	LedgerType  *string `json:"ledgerType,omitempty" enum:"debit|credit|adjustment" desc:"recordType=ledger 时按流水类型过滤"`
	PeriodStart *string `json:"periodStart,omitempty" desc:"RFC3339 起始时间（与 periodEnd 成对传入）"`
	PeriodEnd   *string `json:"periodEnd,omitempty" desc:"RFC3339 结束时间（须晚于 periodStart）"`
	Page        *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码（>=1）"`
	PageSize    *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（默认 20，上限 50）"`
}

type adminInspectBillingInput struct {
	View          string   `json:"view" required:"true" enum:"platform_summary|user_summary|ledger|usage|deployment_spend|rate_rules" desc:"财务检查视图"`
	User          *string  `json:"user,omitempty" desc:"目标用户 ID 或唯一邮箱（view=user_summary 时必填；ledger/usage/deployment_spend 传入时限定该用户）"`
	Projects      []string `json:"projects,omitempty" desc:"按项目 ID 或 identifier 列表过滤（ai.* 用量不归属项目）"`
	LedgerEntryID *string  `json:"ledgerEntryId,omitempty" desc:"view=ledger 时按流水 ID（bled_xxx）精确回读单条记录"`
	LedgerType    *string  `json:"ledgerType,omitempty" enum:"debit|credit|adjustment" desc:"view=ledger 时按流水类型过滤"`
	Meter         *string  `json:"meter,omitempty" enum:"build.cpu_vcpu_minute|build.memory_gib_minute|build.job|runtime.cpu_vcpu_hour|runtime.memory_gib_hour|storage.gib_day|storage.transfer_gib|gateway.egress_gib|gateway.requests_1000|ai.prompt_tokens_1m|ai.completion_tokens_1m|ai.cached_prompt_tokens_1m|ai.cache_write_prompt_tokens_1m" desc:"view=usage 时按计量项过滤"`
	PeriodStart   *string  `json:"periodStart,omitempty" desc:"RFC3339 起始时间（与 periodEnd 成对传入）"`
	PeriodEnd     *string  `json:"periodEnd,omitempty" desc:"RFC3339 结束时间"`
	TimeZone      *string  `json:"timeZone,omitempty" default:"UTC" desc:"IANA 时区名，默认 UTC（仅 platform_summary / user_summary 使用）"`
	Page          *int     `json:"page,omitempty" min:"1" default:"1" desc:"页码（>=1）"`
	PageSize      *int     `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（默认 20，上限 50）"`
}

type adminBillingRateRulePatch struct {
	Meter          string  `json:"meter" required:"true" enum:"build.cpu_vcpu_minute|build.memory_gib_minute|runtime.cpu_vcpu_hour|runtime.memory_gib_hour|storage.gib_day|storage.transfer_gib|gateway.egress_gib|gateway.requests_1000" desc:"计费计量项标识"`
	CreditsPerUnit *string `json:"creditsPerUnit,omitempty" desc:"非负十进制单位单价；省略保持存量单价"`
	Enabled        *bool   `json:"enabled,omitempty" desc:"是否启用该计费项；省略保持存量启停状态"`
}

type adminUpdateBillingRateRulesInput struct {
	Rules []adminBillingRateRulePatch `json:"rules" required:"true" min:"1" desc:"待增量修改的计费单价规则列表"`
}

type adminAdjustUserWalletInput struct {
	User          string `json:"user" required:"true" min:"1" desc:"目标用户 ID 或唯一邮箱"`
	Type          string `json:"type" required:"true" enum:"credit|adjustment" desc:"调账类型：credit（充值，金额须为正）或 adjustment（正负调账）"`
	AmountCredits string `json:"amountCredits" required:"true" min:"1" desc:"非零高精度十进制金额字符串"`
	Description   string `json:"description" required:"true" min:"1" desc:"财务调账审计原因说明"`
}

type walletAdjustLockedPayload struct {
	TargetUserID  string `json:"targetUserId"`
	Type          string `json:"type"`
	AmountCredits string `json:"amountCredits"`
}

type gatewayTrafficSummaryView struct {
	Available       bool       `json:"available"`
	Installed       bool       `json:"installed"`
	Status          string     `json:"status"`
	ObservationCode string     `json:"observationCode"`
	ObservedAt      *time.Time `json:"observedAt,omitempty"`
	LastReportedAt  *time.Time `json:"lastReportedAt,omitempty"`
	LastWindowStart *time.Time `json:"lastWindowStart,omitempty"`
	LastWindowEnd   *time.Time `json:"lastWindowEnd,omitempty"`
}

func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsGetBillingSummaryInput, map[string]any]{
		Name:                     "ops_get_billing_summary",
		Domain:                   aitool.DomainOps,
		Title:                    "查看本人账单与消费汇总",
		Summary:                  "查询当前登录用户本人的钱包余额、今日/本月消费汇总、应用/环境消费排行、网关流量探针鲜活度及可选计费单价表",
		AliasesZH:                []string{"个人账单", "钱包余额", "消费汇总", "费用排行", "计费单价"},
		AliasesEN:                []string{"my billing summary", "wallet balance", "spend summary"},
		AllowPersonalProjectOmit: true,
		ApprovalTier:             aitool.ApprovalTierL0,
		Effect:                   aitool.EffectRead,
		RequiredScopes:           []string{string(authz.ActionBillingRead)},
		Execute:                  h.executeOpsGetBillingSummary,
	})

	aitool.Register(reg, aitool.Spec[opsListBillingRecordsInput, map[string]any]{
		Name:                     "ops_list_billing_records",
		Domain:                   aitool.DomainOps,
		Title:                    "查询本人账单流水与计量明细",
		Summary:                  "分页查询当前登录用户本人的钱包流水（ledger）或资源用量明细（usage）",
		AliasesZH:                []string{"账单流水", "扣费明细", "用量记录", "消费记录"},
		AliasesEN:                []string{"my billing records", "wallet ledger", "usage records"},
		AllowPersonalProjectOmit: true,
		ApprovalTier:             aitool.ApprovalTierL0,
		Effect:                   aitool.EffectRead,
		RequiredScopes:           []string{string(authz.ActionBillingRead)},
		Execute:                  h.executeOpsListBillingRecords,
	})

	aitool.Register(reg, aitool.Spec[adminInspectBillingInput, map[string]any]{
		Name:           "admin_inspect_billing",
		Domain:         aitool.DomainAdmin,
		Title:          "全局或跨用户财务账单检查",
		Summary:        "查询全平台或指定用户的账单汇总、钱包流水、资源计量明细、跨项目消费排行或计费单价规则",
		AliasesZH:      []string{"全平台账单", "财务审计", "查看用户余额", "计费规则查询"},
		AliasesEN:      []string{"inspect platform billing", "admin billing", "rate rules"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionBillingRead)},
		Execute:        h.executeAdminInspectBilling,
	})

	aitool.Register(reg, aitool.Spec[adminUpdateBillingRateRulesInput, map[string]any]{
		Name:           "admin_update_billing_rate_rules",
		Domain:         aitool.DomainAdmin,
		Title:          "增量修改平台计费单价规则",
		Summary:        "按计量项增量修改平台全局资源计费单价与启停状态，未提及字段保持存量不变",
		AliasesZH:      []string{"修改计费单价", "计费费率设置", "资源定价"},
		AliasesEN:      []string{"update billing rate rules", "update pricing"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionBillingAdjust)},
		Verification: []aitool.OperationVerification{{
			OperationID:     "admin_inspect_billing",
			Family:          "billing_rate_rules",
			StaticArguments: map[string]any{"view": "rate_rules"},
		}},
		Preview: h.previewAdminUpdateBillingRateRules,
		Execute: h.executeAdminUpdateBillingRateRules,
	})

	aitool.Register(reg, aitool.Spec[adminAdjustUserWalletInput, map[string]any]{
		Name:           "admin_adjust_user_wallet",
		Domain:         aitool.DomainAdmin,
		Title:          "用户钱包充值或调账",
		Summary:        "为指定平台用户钱包执行充值（credit）或正负调账（adjustment），绑定幂等键防重",
		AliasesZH:      []string{"钱包充值", "手工调账", "余额扣减", "增加额度"},
		AliasesEN:      []string{"adjust user wallet", "credit user wallet"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionBillingAdjust)},
		Verification: []aitool.OperationVerification{{
			OperationID:     "admin_inspect_billing",
			Family:          "resource",
			IDFrom:          "result.id",
			ReadIDArgument:  "ledgerEntryId",
			Collection:      true,
			CollectionPath:  "items",
			StaticArguments: map[string]any{"view": "ledger"},
		}},
		Preview: h.previewAdminAdjustUserWallet,
		Execute: h.executeAdminAdjustUserWallet,
	})
}

func (h *Handler) executeOpsGetBillingSummary(tc *aitool.ToolContext, in opsGetBillingSummaryInput) (map[string]any, error) {
	tc.SetNoStore()
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	var projectIDs []string
	resolvedProjectID := ""
	if projRef := strings.TrimSpace(derefBillingStr(in.Project)); projRef != "" {
		proj, err := h.resolvePersonalBillingProject(tc, projRef)
		if err != nil {
			return nil, err
		}
		resolvedProjectID = proj.ID
		projectIDs = []string{proj.ID}
	}

	period, err := parseBillingToolPeriod(in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return nil, err
	}
	loc, err := parseBillingToolTimeZone(in.TimeZone)
	if err != nil {
		return nil, err
	}

	summary, err := (billing.Service{DB: tc.DB}).Summary(billing.SummaryInput{
		UserIDs:         []string{tc.User.ID},
		ProjectIDs:      projectIDs,
		Now:             time.Now().In(loc),
		LowBalanceLimit: h.configuredLowBalanceLimit(),
		PeriodStart:     period.Start,
		PeriodEnd:       period.End,
		PlatformTotal:   false,
	})
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "billing.summary_failed", err.Error())
	}

	trafficStatus, err := h.buildGatewayTrafficSummary(tc)
	if err != nil {
		return nil, err
	}

	out := summaryToMap(summary)
	out["gatewayTrafficStatus"] = trafficStatus
	if resolvedProjectID != "" {
		out["projectId"] = resolvedProjectID
	}

	includeSpend := in.IncludeDeploymentSpend == nil || *in.IncludeDeploymentSpend
	if includeSpend {
		spendItems, _, spendErr := queryDeploymentSpend(tc.DB, []string{tc.User.ID}, projectIDs, len(projectIDs) > 0, period, 1, 50)
		if spendErr != nil {
			return nil, spendErr
		}
		out["deploymentSpend"] = spendItems
	}

	if in.IncludeRateRules != nil && *in.IncludeRateRules {
		rules, ruleErr := (billing.Service{DB: tc.DB}).ListRateRules()
		if ruleErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "billing.rate_rules_failed", ruleErr.Error())
		}
		out["rateRules"] = rules
	}

	return out, nil
}

func (h *Handler) executeOpsListBillingRecords(tc *aitool.ToolContext, in opsListBillingRecordsInput) (map[string]any, error) {
	recordType := strings.ToLower(strings.TrimSpace(in.RecordType))
	if recordType != "ledger" && recordType != "usage" {
		return nil, aitool.BadRequest("billing.record_type_invalid", "recordType must be ledger or usage")
	}

	meter := strings.TrimSpace(derefBillingStr(in.Meter))
	ledgerType := strings.TrimSpace(derefBillingStr(in.LedgerType))
	if recordType == "ledger" && meter != "" {
		return nil, aitool.BadRequest("billing.argument_invalid", "meter filter is only supported when recordType=usage")
	}
	if recordType == "usage" && ledgerType != "" {
		return nil, aitool.BadRequest("billing.argument_invalid", "ledgerType filter is only supported when recordType=ledger")
	}
	if meter != "" && !isAllowedMeterValue(meter, billingUsageMeterEnum) {
		return nil, aitool.BadRequest("billing.meter_invalid", "unsupported billing meter")
	}
	if ledgerType != "" && ledgerType != "debit" && ledgerType != "credit" && ledgerType != "adjustment" {
		return nil, aitool.BadRequest("billing.ledger_type_invalid", "ledgerType must be debit, credit, or adjustment")
	}

	projRef := strings.TrimSpace(derefBillingStr(in.Project))
	if projRef != "" && strings.HasPrefix(meter, "ai.") {
		return nil, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"billing.ai_meter_not_project_scoped",
			"ai.* usage is billed directly to the personal wallet and never belongs to a project",
			"Omit the project parameter when querying ai.* meters.",
		)
	}

	period, err := parseBillingToolPeriod(in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return nil, err
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	var projectIDs []string
	resolvedProjectID := ""
	if projRef != "" {
		proj, projErr := h.resolvePersonalBillingProject(tc, projRef)
		if projErr != nil {
			return nil, projErr
		}
		resolvedProjectID = proj.ID
		projectIDs = []string{proj.ID}
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	if recordType == "ledger" {
		entries, total, qErr := queryBillingLedger(tc.DB, []string{tc.User.ID}, projectIDs, len(projectIDs) > 0, "", ledgerType, period, page, pageSize)
		if qErr != nil {
			return nil, qErr
		}
		env := aitool.PageEnvelope(entries, page, pageSize, "createdAt", "desc", total)
		env["recordType"] = "ledger"
		if resolvedProjectID != "" {
			env["projectId"] = resolvedProjectID
		}
		return env, nil
	}

	records, total, qErr := queryBillingUsage(tc.DB, []string{tc.User.ID}, projectIDs, len(projectIDs) > 0, meter, period, page, pageSize)
	if qErr != nil {
		return nil, qErr
	}
	env := aitool.PageEnvelope(records, page, pageSize, "createdAt", "desc", total)
	env["recordType"] = "usage"
	if resolvedProjectID != "" {
		env["projectId"] = resolvedProjectID
	}
	return env, nil
}

func (h *Handler) executeAdminInspectBilling(tc *aitool.ToolContext, in adminInspectBillingInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	view := strings.ToLower(strings.TrimSpace(in.View))
	switch view {
	case "platform_summary", "user_summary", "ledger", "usage", "deployment_spend", "rate_rules":
	default:
		return nil, aitool.BadRequest("billing.view_invalid", "unsupported billing inspection view")
	}

	userRef := strings.TrimSpace(derefBillingStr(in.User))
	if view == "user_summary" && userRef == "" {
		return nil, aitool.BadRequest("billing.user_required", "user is required when view=user_summary")
	}
	if view == "platform_summary" && userRef != "" {
		return nil, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"billing.argument_invalid",
			"user cannot be passed when view=platform_summary",
			"Switch to view=user_summary when inspecting a specific user's billing summary.",
		)
	}

	meter := strings.TrimSpace(derefBillingStr(in.Meter))
	if meter != "" {
		if view != "usage" {
			return nil, aitool.BadRequest("billing.argument_invalid", "meter filter only applies when view=usage")
		}
		if !isAllowedMeterValue(meter, billingUsageMeterEnum) {
			return nil, aitool.BadRequest("billing.meter_invalid", "unsupported billing meter")
		}
	}
	if len(in.Projects) > 0 && strings.HasPrefix(meter, "ai.") {
		return nil, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"billing.ai_meter_not_project_scoped",
			"ai.* usage never belongs to a project",
			"Omit projects when filtering ai.* meters.",
		)
	}

	ledgerType := strings.TrimSpace(derefBillingStr(in.LedgerType))
	if ledgerType != "" {
		if view != "ledger" {
			return nil, aitool.BadRequest("billing.argument_invalid", "ledgerType filter only applies when view=ledger")
		}
		if ledgerType != "debit" && ledgerType != "credit" && ledgerType != "adjustment" {
			return nil, aitool.BadRequest("billing.ledger_type_invalid", "ledgerType must be debit, credit, or adjustment")
		}
	}
	ledgerEntryID := strings.TrimSpace(derefBillingStr(in.LedgerEntryID))
	if ledgerEntryID != "" && view != "ledger" {
		return nil, aitool.BadRequest("billing.argument_invalid", "ledgerEntryId only applies when view=ledger")
	}

	period, err := parseBillingToolPeriod(in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return nil, err
	}
	loc, err := parseBillingToolTimeZone(in.TimeZone)
	if err != nil {
		return nil, err
	}

	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	if view == "rate_rules" {
		rules, rErr := (billing.Service{DB: tc.DB}).ListRateRules()
		if rErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "billing.rate_rules_failed", rErr.Error())
		}
		return map[string]any{
			"view":  "rate_rules",
			"rules": rules,
			"items": rules,
		}, nil
	}

	var targetUser *model.User
	if userRef != "" {
		u, uErr := resolveBillingUser(tc.DB, userRef)
		if uErr != nil {
			return nil, uErr
		}
		targetUser = &u
	}

	var projectIDs []string
	if len(in.Projects) > 0 {
		seen := make(map[string]bool, len(in.Projects))
		for _, pRef := range in.Projects {
			proj, pErr := aitool.ResolveProject(tc.DB, pRef)
			if pErr != nil {
				return nil, pErr
			}
			if !seen[proj.ID] {
				seen[proj.ID] = true
				projectIDs = append(projectIDs, proj.ID)
			}
		}
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	switch view {
	case "platform_summary":
		allUserIDs, uErr := allPlatformBillingUserIDs(tc.DB)
		if uErr != nil {
			return nil, uErr
		}
		summary, sErr := (billing.Service{DB: tc.DB}).Summary(billing.SummaryInput{
			UserIDs:         allUserIDs,
			ProjectIDs:      projectIDs,
			Now:             time.Now().In(loc),
			LowBalanceLimit: h.configuredLowBalanceLimit(),
			PeriodStart:     period.Start,
			PeriodEnd:       period.End,
			PlatformTotal:   true,
		})
		if sErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "billing.summary_failed", sErr.Error())
		}
		out := summaryToMap(summary)
		out["view"] = "platform_summary"
		if len(projectIDs) > 0 {
			out["projectIds"] = projectIDs
		}
		return out, nil

	case "user_summary":
		summary, sErr := (billing.Service{DB: tc.DB}).Summary(billing.SummaryInput{
			UserIDs:         []string{targetUser.ID},
			ProjectIDs:      projectIDs,
			Now:             time.Now().In(loc),
			LowBalanceLimit: h.configuredLowBalanceLimit(),
			PeriodStart:     period.Start,
			PeriodEnd:       period.End,
			PlatformTotal:   false,
		})
		if sErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "billing.summary_failed", sErr.Error())
		}
		out := summaryToMap(summary)
		out["view"] = "user_summary"
		out["userId"] = targetUser.ID
		out["userEmail"] = targetUser.Email
		if len(projectIDs) > 0 {
			out["projectIds"] = projectIDs
		}
		return out, nil

	case "ledger":
		var userIDs []string
		if targetUser != nil {
			userIDs = []string{targetUser.ID}
		}
		entries, total, qErr := queryBillingLedger(tc.DB, userIDs, projectIDs, len(projectIDs) > 0, ledgerEntryID, ledgerType, period, page, pageSize)
		if qErr != nil {
			return nil, qErr
		}
		env := aitool.PageEnvelope(entries, page, pageSize, "createdAt", "desc", total)
		env["view"] = "ledger"
		if targetUser != nil {
			env["userId"] = targetUser.ID
		}
		if len(projectIDs) > 0 {
			env["projectIds"] = projectIDs
		}
		if ledgerEntryID != "" && len(entries) == 1 {
			env["entry"] = entries[0]
		}
		return env, nil

	case "usage":
		var userIDs []string
		if targetUser != nil {
			userIDs = []string{targetUser.ID}
		}
		records, total, qErr := queryBillingUsage(tc.DB, userIDs, projectIDs, len(projectIDs) > 0, meter, period, page, pageSize)
		if qErr != nil {
			return nil, qErr
		}
		env := aitool.PageEnvelope(records, page, pageSize, "createdAt", "desc", total)
		env["view"] = "usage"
		if targetUser != nil {
			env["userId"] = targetUser.ID
		}
		if len(projectIDs) > 0 {
			env["projectIds"] = projectIDs
		}
		return env, nil

	case "deployment_spend":
		var userIDs []string
		if targetUser != nil {
			userIDs = []string{targetUser.ID}
		} else {
			var uErr error
			userIDs, uErr = allPlatformBillingUserIDs(tc.DB)
			if uErr != nil {
				return nil, uErr
			}
		}
		items, total, qErr := queryDeploymentSpend(tc.DB, userIDs, projectIDs, len(projectIDs) > 0, period, page, pageSize)
		if qErr != nil {
			return nil, qErr
		}
		env := aitool.PageEnvelope(items, page, pageSize, "amountCredits", "desc", total)
		env["view"] = "deployment_spend"
		if targetUser != nil {
			env["userId"] = targetUser.ID
		}
		if len(projectIDs) > 0 {
			env["projectIds"] = projectIDs
		}
		return env, nil
	}

	return nil, aitool.BadRequest("billing.view_invalid", "unsupported billing inspection view")
}

func (h *Handler) previewAdminUpdateBillingRateRules(tc *aitool.ToolContext, in adminUpdateBillingRateRulesInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	updates, changes, err := h.prepareBillingRateRuleUpdates(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update %d billing rate rule(s)", len(updates)),
			TargetResource: "billing_rate_rules",
			Changes:        changes,
		},
	}, nil
}

func (h *Handler) executeAdminUpdateBillingRateRules(tc *aitool.ToolContext, in adminUpdateBillingRateRulesInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	updates, _, err := h.prepareBillingRateRuleUpdates(tc, in)
	if err != nil {
		return nil, err
	}
	rules, err := (billing.Service{DB: tc.DB}).UpdateRateRules(updates)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, aitool.BadRequest("billing.rate_rule_unknown", "unknown billing rate rule meter")
	}
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "billing.rate_rules_update_failed", err.Error())
	}

	meters := make([]string, 0, len(updates))
	for _, u := range updates {
		meters = append(meters, u.Meter)
	}
	h.auditWithContext(tc.User.ID, "billing.rate_rules.update", "billing.rate_rules", true, "updated meters: "+strings.Join(meters, ","), tc.Context())
	return map[string]any{
		"rules": rules,
		"items": rules,
	}, nil
}

func (h *Handler) prepareBillingRateRuleUpdates(tc *aitool.ToolContext, in adminUpdateBillingRateRulesInput) ([]billing.RateRuleUpdate, []aitool.PreviewChange, error) {
	if len(in.Rules) == 0 {
		return nil, nil, aitool.BadRequest("billing.rate_rules_required", "at least one rate rule patch is required")
	}
	for _, item := range in.Rules {
		meter := strings.TrimSpace(item.Meter)
		if !isAllowedMeterValue(meter, billingRateRuleMeterEnum) {
			return nil, nil, aitool.BadRequest("billing.rate_rule_unknown", fmt.Sprintf("unknown billing rate rule meter %q", meter))
		}
		if item.CreditsPerUnit == nil && item.Enabled == nil {
			return nil, nil, aitool.BadRequest("billing.rate_rule_noop", fmt.Sprintf("meter %q must specify creditsPerUnit or enabled", meter))
		}
		if item.CreditsPerUnit != nil {
			price, err := decimal.NewFromString(strings.TrimSpace(*item.CreditsPerUnit))
			if err != nil || price.IsNegative() {
				return nil, nil, aitool.BadRequest("billing.rate_rule_invalid_price", "creditsPerUnit must be a non-negative decimal")
			}
		}
	}

	if tc.DB == nil {
		return nil, nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	existingRules, err := (billing.Service{DB: tc.DB}).ListRateRules()
	if err != nil {
		return nil, nil, aitool.NewError(http.StatusInternalServerError, "billing.rate_rules_failed", err.Error())
	}
	byMeter := make(map[string]model.BillingRateRule, len(existingRules))
	for _, rule := range existingRules {
		byMeter[rule.Meter] = rule
	}

	seen := make(map[string]bool, len(in.Rules))
	updates := make([]billing.RateRuleUpdate, 0, len(in.Rules))
	changes := make([]aitool.PreviewChange, 0, len(in.Rules)*2)

	for _, patch := range in.Rules {
		meter := strings.TrimSpace(patch.Meter)
		if seen[meter] {
			return nil, nil, aitool.BadRequest("billing.rate_rule_duplicate_meter", fmt.Sprintf("duplicate meter %q in rules", meter))
		}
		seen[meter] = true

		current, exists := byMeter[meter]
		if !exists {
			return nil, nil, aitool.BadRequest("billing.rate_rule_unknown", fmt.Sprintf("unknown billing rate rule meter %q", meter))
		}

		nextPrice := current.CreditsPerUnit
		if patch.CreditsPerUnit != nil {
			parsed, _ := decimal.NewFromString(strings.TrimSpace(*patch.CreditsPerUnit))
			nextPrice = parsed
			changes = append(changes, aitool.PreviewChange{
				Field:  meter + ".creditsPerUnit",
				Before: current.CreditsPerUnit.String(),
				After:  nextPrice.String(),
				Action: "update",
			})
		}

		nextEnabled := current.Enabled
		if patch.Enabled != nil {
			nextEnabled = *patch.Enabled
			changes = append(changes, aitool.PreviewChange{
				Field:  meter + ".enabled",
				Before: current.Enabled,
				After:  nextEnabled,
				Action: "update",
			})
		}

		updates = append(updates, billing.RateRuleUpdate{
			Meter:          meter,
			CreditsPerUnit: nextPrice,
			Enabled:        nextEnabled,
		})
	}

	return updates, changes, nil
}

func (h *Handler) previewAdminAdjustUserWallet(tc *aitool.ToolContext, in adminAdjustUserWalletInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	txType, amount, desc, err := validateWalletAdjustParams(in)
	if err != nil {
		return nil, err
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	targetUser, err := resolveBillingUser(tc.DB, in.User)
	if err != nil {
		return nil, err
	}

	var wallet model.UserWallet
	currentBalance := decimal.Zero
	if wErr := tc.DB.First(&wallet, "user_id = ?", targetUser.ID).Error; wErr == nil {
		currentBalance = wallet.BalanceCredits
	} else if !errors.Is(wErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", wErr.Error())
	}

	nextBalance := currentBalance.Add(amount)
	if amount.IsNegative() {
		var reserved struct {
			Credits decimal.Decimal `gorm:"column:credits"`
		}
		if rErr := tc.DB.Table("ai.model_usages").
			Where("owner_user_id = ? AND status = ? AND settlement_status = ?", targetUser.ID, "reported", "pending").
			Select(`COALESCE(SUM((
				((prompt_tokens - COALESCE(cached_prompt_tokens, 0) - COALESCE(cache_write_prompt_tokens, 0))::numeric * input_credits_per_million)
				+ ((COALESCE(cached_prompt_tokens, 0) + COALESCE(cache_write_prompt_tokens, 0))::numeric * cached_input_credits_per_million)
				+ (completion_tokens::numeric * output_credits_per_million)
			) / 1000000), 0) AS credits`).
			Scan(&reserved).Error; rErr == nil && reserved.Credits.IsPositive() && nextBalance.LessThan(reserved.Credits) {
			return nil, aitool.Conflict("billing.reserved_balance", billing.ErrReservedBalance.Error())
		}
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Wallet %s of %s credits for %s (%s): %s", txType, amount.String(), targetUser.Email, targetUser.ID, desc),
			TargetResource: "user_wallet",
			TargetID:       targetUser.ID,
			Changes: []aitool.PreviewChange{
				{
					Field:  "balanceCredits",
					Before: currentBalance.String(),
					After:  nextBalance.String(),
					Action: txType,
				},
			},
		},
		LockedPayload: walletAdjustLockedPayload{
			TargetUserID:  targetUser.ID,
			Type:          txType,
			AmountCredits: amount.String(),
		},
	}, nil
}

func (h *Handler) executeAdminAdjustUserWallet(tc *aitool.ToolContext, in adminAdjustUserWalletInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	txType, amount, desc, err := validateWalletAdjustParams(in)
	if err != nil {
		return nil, err
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	targetUser, err := resolveBillingUser(tc.DB, in.User)
	if err != nil {
		return nil, err
	}
	if len(tc.LockedPayload) > 0 {
		var locked walletAdjustLockedPayload
		if lockErr := tc.UnmarshalLockedPayload(&locked); lockErr != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", lockErr.Error())
		}
		if locked.TargetUserID != targetUser.ID || locked.Type != txType {
			return nil, aitool.Conflict("ai.preview_lock_invalid", "target user changed since preview")
		}
	}

	idemKey := strings.TrimSpace(tc.IdempotencyKey)
	if idemKey == "" && strings.TrimSpace(tc.ToolCallID) != "" {
		idemKey = "aitool:" + strings.TrimSpace(tc.ToolCallID)
	}

	entry, err := (billing.Service{DB: tc.DB}).ApplyWalletTransaction(billing.WalletTransactionInput{
		UserID:         targetUser.ID,
		AmountCredits:  amount,
		Type:           txType,
		Description:    desc,
		IdempotencyKey: idemKey,
		ActorID:        tc.User.ID,
	})
	if err != nil {
		if errors.Is(err, billing.ErrReservedBalance) {
			return nil, aitool.Conflict("billing.reserved_balance", err.Error())
		}
		return nil, aitool.BadRequest("billing.transaction_invalid", err.Error())
	}

	h.auditWithContext(tc.User.ID, "billing.wallet_transaction", entry.ID, true, desc, tc.Context())
	return map[string]any{
		"id":                  entry.ID,
		"userId":              entry.UserID,
		"userEmail":           targetUser.Email,
		"type":                entry.Type,
		"amountCredits":       entry.AmountCredits,
		"balanceAfterCredits": entry.BalanceAfterCredits,
		"reason":              entry.Reason,
		"description":         entry.Description,
		"createdBy":           entry.CreatedBy,
		"createdAt":           entry.CreatedAt,
	}, nil
}

func validateWalletAdjustParams(in adminAdjustUserWalletInput) (string, decimal.Decimal, string, error) {
	if strings.TrimSpace(in.User) == "" {
		return "", decimal.Zero, "", aitool.BadRequest("billing.user_required", "user is required")
	}
	txType := strings.ToLower(strings.TrimSpace(in.Type))
	if txType != "credit" && txType != "adjustment" {
		return "", decimal.Zero, "", aitool.BadRequest("billing.transaction_invalid_type", "type must be credit or adjustment")
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(in.AmountCredits))
	if err != nil || amount.IsZero() {
		return "", decimal.Zero, "", aitool.BadRequest("billing.transaction_invalid_amount", "amountCredits must be a non-zero decimal")
	}
	if txType == "credit" && amount.IsNegative() {
		return "", decimal.Zero, "", aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"billing.transaction_invalid_amount",
			"credit amountCredits must be positive",
			"Use type=adjustment with a negative amountCredits when deducting from a user wallet.",
		)
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		return "", decimal.Zero, "", aitool.BadRequest("billing.description_required", "description is required")
	}
	return txType, amount, desc, nil
}

func (h *Handler) resolvePersonalBillingProject(tc *aitool.ToolContext, projRef string) (model.Project, error) {
	proj, err := aitool.ResolveProject(tc.DB, projRef)
	if err != nil {
		return model.Project{}, err
	}
	if convProjID := strings.TrimSpace(tc.ConversationProjectID); convProjID != "" && proj.ID != convProjID {
		mismatch := aitool.Forbidden("ai.project_scope_mismatch", "requested project does not match bound conversation project")
		mismatch.Hint = "Switch to an unbound global conversation when inspecting another project."
		return model.Project{}, mismatch
	}
	if strings.TrimSpace(proj.BillingOwnerUserID) != tc.User.ID {
		forbidden := aitool.Forbidden("billing.project_forbidden", "current user is not the billingOwner of the requested project")
		if authz.IsPlatformAdmin(tc.User.Role) {
			forbidden.Hint = "Use admin_inspect_billing to inspect billing for projects owned by another user."
		}
		return model.Project{}, forbidden
	}
	return proj, nil
}

func (h *Handler) buildGatewayTrafficSummary(tc *aitool.ToolContext) (gatewayTrafficSummaryView, error) {
	var installations []model.SystemComponentInstallation
	if err := tc.DB.Where("component_id = ?", systemComponentGatewayTrafficProbe).Order("created_at desc").Find(&installations).Error; err != nil {
		return gatewayTrafficSummaryView{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if len(installations) == 0 {
		observedAt := time.Now().UTC()
		return gatewayTrafficSummaryView{
			Available:       false,
			Installed:       false,
			Status:          observation.StatusNotConfigured,
			ObservationCode: "billing.gateway_traffic_probe_not_configured",
			ObservedAt:      &observedAt,
		}, nil
	}

	h.observeSystemComponentInstallations(tc.Context(), installations)
	runtimeStatus, observationCode, observedAt := summarizeGatewayTrafficProbeObservations(installations)

	var latestUsage model.BillingUsageRecord
	usageResult := tc.DB.
		Where("resource_type = ? and meter = ? and status = ?", billing.ResourceTypeGateway, "gateway.egress_gib", "settled").
		Order("period_end desc").
		First(&latestUsage)
	if usageResult.Error != nil && !errors.Is(usageResult.Error, gorm.ErrRecordNotFound) {
		return gatewayTrafficSummaryView{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", usageResult.Error.Error())
	}
	hasReport := usageResult.Error == nil
	var lastReportedAt, lastWindowStart, lastWindowEnd *time.Time
	reportFresh := false
	if hasReport {
		reportedAt := latestUsage.CreatedAt.UTC()
		if latestUsage.SettledAt != nil {
			reportedAt = latestUsage.SettledAt.UTC()
		}
		ws := latestUsage.PeriodStart.UTC()
		we := latestUsage.PeriodEnd.UTC()
		lastReportedAt = &reportedAt
		lastWindowStart = &ws
		lastWindowEnd = &we
		reportFresh = we.After(time.Now().UTC().Add(-gatewayTrafficReportFreshness))
	}
	if runtimeStatus == observation.StatusReady {
		switch {
		case !hasReport:
			observationCode = "billing.gateway_traffic_waiting_report"
		case !reportFresh:
			observationCode = "billing.gateway_traffic_report_stale"
		}
	}

	return gatewayTrafficSummaryView{
		Available:       runtimeStatus == observation.StatusReady && reportFresh,
		Installed:       true,
		Status:          runtimeStatus,
		ObservationCode: observationCode,
		ObservedAt:      observedAt,
		LastReportedAt:  lastReportedAt,
		LastWindowStart: lastWindowStart,
		LastWindowEnd:   lastWindowEnd,
	}, nil
}

func (h *Handler) configuredLowBalanceLimit() decimal.Decimal {
	limit := decimal.RequireFromString("100")
	raw := strings.TrimSpace(h.configValues([]string{"billing.lowBalanceThresholdCredits"})["billing.lowBalanceThresholdCredits"])
	if parsed, err := decimal.NewFromString(raw); err == nil && !parsed.IsNegative() {
		limit = parsed
	}
	return limit
}

func parseBillingToolPeriod(rawStartPtr, rawEndPtr *string) (billingPeriodQuery, error) {
	rawStart := strings.TrimSpace(derefBillingStr(rawStartPtr))
	rawEnd := strings.TrimSpace(derefBillingStr(rawEndPtr))
	if rawStart == "" && rawEnd == "" {
		return billingPeriodQuery{}, nil
	}
	if rawStart == "" || rawEnd == "" {
		return billingPeriodQuery{}, aitool.BadRequest("billing.period_required", "periodStart and periodEnd must be provided together")
	}
	start, err := time.Parse(time.RFC3339, rawStart)
	if err != nil {
		return billingPeriodQuery{}, aitool.BadRequest("billing.period_start_invalid", "periodStart must be RFC3339 time")
	}
	end, err := time.Parse(time.RFC3339, rawEnd)
	if err != nil || !end.After(start) {
		return billingPeriodQuery{}, aitool.BadRequest("billing.period_end_invalid", "periodEnd must be RFC3339 time after periodStart")
	}
	return billingPeriodQuery{Start: &start, End: &end}, nil
}

func parseBillingToolTimeZone(rawPtr *string) (*time.Location, error) {
	name := strings.TrimSpace(derefBillingStr(rawPtr))
	if name == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil || name == "Local" {
		return nil, aitool.BadRequest("billing.time_zone_invalid", "timeZone must be a valid IANA time zone name")
	}
	return loc, nil
}

func resolveBillingUser(db *gorm.DB, ref string) (model.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.User{}, aitool.BadRequest("billing.user_required", "user is required")
	}
	var user model.User
	if err := db.Where("id = ? OR LOWER(email) = LOWER(?)", ref, ref).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, aitool.NotFound("billing.user_not_found", "billing user was not found")
		}
		return model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return user, nil
}

func allPlatformBillingUserIDs(db *gorm.DB) ([]string, error) {
	var wallets []model.UserWallet
	if err := db.Select("user_id").Find(&wallets).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	ids := make([]string, 0, len(wallets))
	for _, w := range wallets {
		if id := strings.TrimSpace(w.UserID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		return ids, nil
	}
	var users []model.User
	if err := db.Select("id").Find(&users).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return ids, nil
}

func queryBillingLedger(
	db *gorm.DB,
	userIDs []string,
	projectIDs []string,
	filterProjects bool,
	ledgerEntryID string,
	ledgerType string,
	period billingPeriodQuery,
	page, pageSize int,
) ([]billingLedgerEntryItem, int64, error) {
	query := db.Table("billing_ledger_entries as ledger").
		Joins("LEFT JOIN billing_usage_records AS usage ON usage.id = ledger.usage_record_id")
	if len(userIDs) > 0 {
		query = query.Where("ledger.user_id in ?", userIDs)
	}
	if filterProjects {
		query = query.Where("ledger.project_id in ?", projectIDs)
	}
	if ledgerEntryID != "" {
		query = query.Where("ledger.id = ?", ledgerEntryID)
	}
	if ledgerType != "" {
		query = query.Where("ledger.type = ?", ledgerType)
	}
	query = applyBillingOccurredPeriod(query, billing.OccurredAtSQL, period)

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var entries []billingLedgerEntryItem
	if err := query.Session(&gorm.Session{}).Select(`
			ledger.id,
			ledger.user_id,
			ledger.project_id,
			ledger.type,
			ledger.amount_credits,
			ledger.balance_after_credits,
			ledger.reason,
			ledger.meter,
			ledger.usage_record_id,
			usage.quantity AS usage_quantity,
			COALESCE(usage.unit, '') AS usage_unit,
			ledger.resource_type,
			ledger.resource_id,
			ledger.description,
			ledger.created_by,
			ledger.created_at,
			` + billing.OccurredAtSQL + ` AS occurred_at,
			COALESCE(usage.application_id, '') AS application_id,
			COALESCE(applications.name, '') AS application_name,
			COALESCE(applications.identifier, '') AS application_identifier
		`).
		Joins("LEFT JOIN applications ON applications.id = usage.application_id").
		Order("ledger.created_at desc, ledger.id desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&entries).Error; err != nil {
		return nil, 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if entries == nil {
		entries = []billingLedgerEntryItem{}
	}
	return entries, total, nil
}

func queryBillingUsage(
	db *gorm.DB,
	userIDs []string,
	projectIDs []string,
	filterProjects bool,
	meter string,
	period billingPeriodQuery,
	page, pageSize int,
) ([]billingUsageRecordItem, int64, error) {
	query := db.Table("billing_usage_records as usage")
	if len(userIDs) > 0 {
		query = query.Where("usage.billed_user_id in ?", userIDs)
	}
	if filterProjects {
		query = query.Where("usage.project_id in ?", projectIDs)
	}
	if meter != "" {
		query = query.Where("usage.meter = ?", meter)
	}
	query = applyBillingOccurredPeriod(query, "usage.period_start", period)

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var records []billingUsageRecordItem
	if err := query.Session(&gorm.Session{}).Select(`
			usage.id,
			usage.project_id,
			usage.billed_user_id,
			usage.application_id,
			COALESCE(applications.name, '') AS application_name,
			COALESCE(applications.identifier, '') AS application_identifier,
			usage.meter,
			usage.quantity,
			usage.unit,
			usage.amount_credits,
			usage.resource_type,
			usage.resource_id,
			usage.period_start,
			usage.period_end,
			usage.status,
			usage.metadata,
			usage.settled_at,
			usage.created_at,
			usage.updated_at
		`).
		Joins("LEFT JOIN applications ON applications.id = usage.application_id").
		Order("usage.created_at desc, usage.id desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&records).Error; err != nil {
		return nil, 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if records == nil {
		records = []billingUsageRecordItem{}
	}
	return records, total, nil
}

func queryDeploymentSpend(
	db *gorm.DB,
	userIDs []string,
	projectIDs []string,
	filterProjects bool,
	period billingPeriodQuery,
	page, pageSize int,
) ([]billingDeploymentSpendItem, int64, error) {
	if len(userIDs) == 0 {
		return []billingDeploymentSpendItem{}, 0, nil
	}

	deploymentTargetIDSQL := billingDeploymentTargetIDSQL()
	spendGroupSQL := billingSpendGroupSQL()
	grouped := db.Table("billing_usage_records as usage").
		Select("usage.project_id, usage.application_id, "+deploymentTargetIDSQL+" AS deployment_target_id, "+spendGroupSQL+" AS spend_group").
		Joins("LEFT JOIN build_runs ON build_runs.id = usage.resource_id AND usage.resource_type = ?", billing.ResourceTypeBuildRun).
		Joins("LEFT JOIN gateway_routes ON gateway_routes.id = split_part(usage.resource_id, ':', 1) AND usage.resource_type = ?", billing.ResourceTypeGateway).
		Where("usage.billed_user_id in ? AND usage.status = ?", userIDs, "settled").
		Group("usage.project_id, usage.application_id, " + deploymentTargetIDSQL + ", " + spendGroupSQL)
	if filterProjects {
		grouped = grouped.Where("usage.project_id in ?", projectIDs)
	}
	grouped = applyBillingOccurredPeriod(grouped, "usage.period_start", period)

	var total int64
	if err := db.Table("(?) as grouped", grouped).Count(&total).Error; err != nil {
		return nil, 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var items []billingDeploymentSpendItem
	query := db.Table("billing_usage_records as usage").
		Select(`
			usage.project_id,
			projects.name AS project_name,
			projects.identifier AS project_identifier,
			usage.application_id,
			COALESCE(applications.name, '') AS application_name,
			COALESCE(applications.identifier, '') AS application_identifier,
			`+deploymentTargetIDSQL+` AS deployment_target_id,
			`+spendGroupSQL+` AS spend_group,
			COALESCE(deployment_targets.name, '') AS deployment_target_name,
			COALESCE(deployment_targets.stage, '') AS deployment_target_stage,
			COALESCE(SUM(usage.amount_credits), 0) AS amount_credits,
			COALESCE(SUM(CASE WHEN usage.meter LIKE 'build.%' THEN usage.amount_credits ELSE 0 END), 0) AS build_credits,
			COALESCE(SUM(CASE WHEN usage.meter LIKE 'runtime.%' THEN usage.amount_credits ELSE 0 END), 0) AS runtime_credits,
			COALESCE(SUM(CASE WHEN usage.meter LIKE 'storage.%' THEN usage.amount_credits ELSE 0 END), 0) AS storage_credits,
			COALESCE(SUM(CASE WHEN usage.meter LIKE 'gateway.%' THEN usage.amount_credits ELSE 0 END), 0) AS gateway_credits,
			COALESCE(SUM(CASE WHEN usage.meter NOT LIKE 'build.%' AND usage.meter NOT LIKE 'runtime.%' AND usage.meter NOT LIKE 'storage.%' AND usage.meter NOT LIKE 'gateway.%' THEN usage.amount_credits ELSE 0 END), 0) AS other_credits
		`).
		Joins("JOIN projects ON projects.id = usage.project_id").
		Joins("LEFT JOIN applications ON applications.id = usage.application_id").
		Joins("LEFT JOIN build_runs ON build_runs.id = usage.resource_id AND usage.resource_type = ?", billing.ResourceTypeBuildRun).
		Joins("LEFT JOIN gateway_routes ON gateway_routes.id = split_part(usage.resource_id, ':', 1) AND usage.resource_type = ?", billing.ResourceTypeGateway).
		Joins("LEFT JOIN deployment_targets ON deployment_targets.id = "+deploymentTargetIDSQL).
		Where("usage.billed_user_id in ? AND usage.status = ?", userIDs, "settled").
		Group("usage.project_id, projects.name, projects.identifier, usage.application_id, applications.name, applications.identifier, " + deploymentTargetIDSQL + ", " + spendGroupSQL + ", deployment_targets.name, deployment_targets.stage")
	if filterProjects {
		query = query.Where("usage.project_id in ?", projectIDs)
	}
	query = applyBillingOccurredPeriod(query, "usage.period_start", period)
	if err := query.Order("amount_credits desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		return nil, 0, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if items == nil {
		items = []billingDeploymentSpendItem{}
	}
	return items, total, nil
}

func summaryToMap(s billing.ProjectBillingSummary) map[string]any {
	monthly := s.MonthlyCategories
	if monthly == nil {
		monthly = []billing.BillingSpendCategory{}
	}
	period := s.PeriodCategories
	if period == nil {
		period = []billing.BillingSpendCategory{}
	}
	out := map[string]any{
		"balanceScope":      s.BalanceScope,
		"balanceCredits":    s.BalanceCredits,
		"todaySpend":        s.TodaySpend,
		"monthSpend":        s.MonthSpend,
		"periodSpend":       s.PeriodSpend,
		"pendingSpend":      s.PendingSpend,
		"availableCredits":  s.AvailableCredits,
		"lowBalanceLimit":   s.LowBalanceLimit,
		"monthlyCategories": monthly,
		"periodCategories":  period,
	}
	if s.BalanceStatus != "" {
		out["balanceStatus"] = s.BalanceStatus
	}
	return out
}

func isAllowedMeterValue(meter, pipeSeparatedEnum string) bool {
	for _, candidate := range strings.Split(pipeSeparatedEnum, "|") {
		if strings.TrimSpace(candidate) == meter {
			return true
		}
	}
	return false
}

func derefBillingStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
