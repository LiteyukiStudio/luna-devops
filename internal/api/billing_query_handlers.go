package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListBillingLedgerEntries(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, ok := h.billingScopeForUser(ctx, user)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	period, ok := billingPeriodFromQuery(ctx)
	if !ok {
		return
	}
	query := h.dbFor(ctx).Table("billing_ledger_entries as ledger").Where("ledger.user_id in ?", scope.UserIDs)
	if scope.FilterProjectIDs {
		query = query.Where("ledger.project_id in ?", scope.ProjectIDs)
	}
	if entryType := strings.TrimSpace(ctx.Query("type")); entryType != "" {
		query = query.Where("ledger.type = ?", entryType)
	}
	query = applyBillingCreatedPeriod(query, "ledger.created_at", period)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var entries []billingLedgerEntryItem
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":     "ledger.created_at",
		"amountCredits": "ledger.amount_credits",
		"reason":        "ledger.reason",
	}, "ledger.created_at")
	if err := query.Select(`
			ledger.id,
			ledger.user_id,
			ledger.project_id,
			ledger.type,
			ledger.amount_credits,
			ledger.balance_after_credits,
			ledger.reason,
			ledger.meter,
			ledger.usage_record_id,
			ledger.resource_type,
			ledger.resource_id,
			ledger.description,
			ledger.created_by,
			ledger.created_at,
			COALESCE(usage.application_id, '') AS application_id,
			COALESCE(applications.name, '') AS application_name,
			COALESCE(applications.identifier, '') AS application_identifier
		`).
		Joins("LEFT JOIN billing_usage_records AS usage ON usage.id = ledger.usage_record_id").
		Joins("LEFT JOIN applications ON applications.id = usage.application_id").
		Order(orderBy).
		Limit(pagination.PageSize).
		Offset(pagination.Offset()).
		Find(&entries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(entries, total, pagination))
}

func (h *Handlers) ListBillingUsageRecords(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, ok := h.billingScopeForUser(ctx, user)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	period, ok := billingPeriodFromQuery(ctx)
	if !ok {
		return
	}
	query := h.dbFor(ctx).Table("billing_usage_records as usage").Where("usage.billed_user_id in ?", scope.UserIDs)
	if scope.FilterProjectIDs {
		query = query.Where("usage.project_id in ?", scope.ProjectIDs)
	}
	if meter := strings.TrimSpace(ctx.Query("meter")); meter != "" {
		query = query.Where("usage.meter = ?", meter)
	}
	query = applyBillingUsagePeriod(query, period)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var records []billingUsageRecordItem
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":     "usage.created_at",
		"amountCredits": "usage.amount_credits",
		"meter":         "usage.meter",
	}, "usage.created_at")
	if err := query.Select(`
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
		Order(orderBy).
		Limit(pagination.PageSize).
		Offset(pagination.Offset()).
		Find(&records).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(records, total, pagination))
}

func (h *Handlers) ListBillingDeploymentSpend(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, ok := h.billingScopeForUser(ctx, user)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	period, ok := billingPeriodFromQuery(ctx)
	if !ok {
		return
	}
	if len(scope.UserIDs) == 0 {
		ctx.JSON(http.StatusOK, paginatedResponse([]billingDeploymentSpendItem{}, 0, pagination))
		return
	}

	deploymentTargetIDSQL := billingDeploymentTargetIDSQL()
	grouped := h.dbFor(ctx).Table("billing_usage_records as usage").
		Select("usage.project_id, usage.application_id, "+deploymentTargetIDSQL+" AS deployment_target_id").
		Joins("LEFT JOIN build_runs ON build_runs.id = usage.resource_id AND usage.resource_type = ?", billing.ResourceTypeBuildRun).
		Joins("LEFT JOIN gateway_routes ON gateway_routes.id = split_part(usage.resource_id, ':', 1) AND usage.resource_type = ?", billing.ResourceTypeGateway).
		Where("usage.billed_user_id in ? AND usage.status = ?", scope.UserIDs, "settled").
		Group("usage.project_id, usage.application_id, " + deploymentTargetIDSQL)
	if scope.FilterProjectIDs {
		grouped = grouped.Where("usage.project_id in ?", scope.ProjectIDs)
	}
	grouped = applyBillingUsagePeriod(grouped, period)
	var total int64
	if err := h.dbFor(ctx).Table("(?) as grouped", grouped).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	var items []billingDeploymentSpendItem
	query := h.dbFor(ctx).Table("billing_usage_records as usage").
		Select(`
			usage.project_id,
			projects.name AS project_name,
			projects.identifier AS project_identifier,
			usage.application_id,
			COALESCE(applications.name, '') AS application_name,
			COALESCE(applications.identifier, '') AS application_identifier,
			`+deploymentTargetIDSQL+` AS deployment_target_id,
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
		Where("usage.billed_user_id in ? AND usage.status = ?", scope.UserIDs, "settled").
		Group("usage.project_id, projects.name, projects.identifier, usage.application_id, applications.name, applications.identifier, " + deploymentTargetIDSQL + ", deployment_targets.name, deployment_targets.stage")
	if scope.FilterProjectIDs {
		query = query.Where("usage.project_id in ?", scope.ProjectIDs)
	}
	query = applyBillingUsagePeriod(query, period)
	orderBy := orderByClause(pagination, map[string]string{
		"amountCredits":        "amount_credits",
		"projectName":          "project_name",
		"applicationName":      "application_name",
		"deploymentTargetName": "deployment_target_name",
	}, "amount_credits")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&items).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
}
