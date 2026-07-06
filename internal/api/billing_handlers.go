package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func (h *Handlers) GetBillingSummary(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, ok := h.billingScopeForUser(ctx, user)
	if !ok {
		return
	}
	if strings.TrimSpace(ctx.Query("accountScope")) == "current" {
		scope.UserIDs = []string{user.ID}
		scope.ProjectIDs = nil
		scope.FilterProjectIDs = false
	}
	lowBalanceLimit := decimal.RequireFromString("100")
	if configuredLimit, err := decimal.NewFromString(strings.TrimSpace(h.configs.get([]string{"billing.lowBalanceThresholdCredits"})["billing.lowBalanceThresholdCredits"])); err == nil && !configuredLimit.IsNegative() {
		lowBalanceLimit = configuredLimit
	}
	period, ok := billingPeriodFromQuery(ctx)
	if !ok {
		return
	}
	summary, err := (billing.Service{DB: h.db}).Summary(scope.UserIDs, scope.ProjectIDs, time.Now(), lowBalanceLimit, period.Start, period.End)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, summary)
}

type gatewayTrafficStatusResponse struct {
	Available             bool       `json:"available"`
	Installed             bool       `json:"installed"`
	Status                string     `json:"status"`
	ComponentID           string     `json:"componentId"`
	InstallableTemplateID string     `json:"installableTemplateId"`
	LastHeartbeatAt       *time.Time `json:"lastHeartbeatAt"`
	LastReportedAt        *time.Time `json:"lastReportedAt"`
	LastWindowStart       *time.Time `json:"lastWindowStart"`
	LastWindowEnd         *time.Time `json:"lastWindowEnd"`
	LastError             string     `json:"lastError"`
}

func (h *Handlers) GetGatewayTrafficStatus(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	state, ok, err := h.gatewayTrafficRuntimeStore().Summary(ctx.Request.Context())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		ctx.JSON(http.StatusOK, gatewayTrafficStatusResponse{
			Available:             false,
			Installed:             false,
			Status:                "not_installed",
			ComponentID:           systemComponentGatewayTrafficProbe,
			InstallableTemplateID: "liteyuki-gateway-traffic-probe",
		})
		return
	}
	ctx.JSON(http.StatusOK, gatewayTrafficStatusResponse{
		Available:             state.Status == "ready",
		Installed:             true,
		Status:                state.Status,
		ComponentID:           systemComponentGatewayTrafficProbe,
		InstallableTemplateID: "liteyuki-gateway-traffic-probe",
		LastHeartbeatAt:       &state.LastHeartbeatAt,
		LastReportedAt:        state.LastReportedAt,
		LastWindowStart:       state.LastWindowStart,
		LastWindowEnd:         state.LastWindowEnd,
		LastError:             state.LastError,
	})
}

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
	query := h.db.Table("billing_ledger_entries as ledger").Where("ledger.user_id in ?", scope.UserIDs)
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
			COALESCE(applications.slug, '') AS application_slug
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
	query := h.db.Table("billing_usage_records as usage").Where("usage.billed_user_id in ?", scope.UserIDs)
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
			COALESCE(applications.slug, '') AS application_slug,
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
	grouped := h.db.Table("billing_usage_records as usage").
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
	if err := h.db.Table("(?) as grouped", grouped).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	var items []billingDeploymentSpendItem
	query := h.db.Table("billing_usage_records as usage").
		Select(`
			usage.project_id,
			projects.name AS project_name,
			projects.slug AS project_slug,
			usage.application_id,
			COALESCE(applications.name, '') AS application_name,
			COALESCE(applications.slug, '') AS application_slug,
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
		Group("usage.project_id, projects.name, projects.slug, usage.application_id, applications.name, applications.slug, " + deploymentTargetIDSQL + ", deployment_targets.name, deployment_targets.stage")
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

type billingPeriodQuery struct {
	Start *time.Time
	End   *time.Time
}

func billingPeriodFromQuery(ctx *gin.Context) (billingPeriodQuery, bool) {
	rawStart := strings.TrimSpace(ctx.Query("periodStart"))
	rawEnd := strings.TrimSpace(ctx.Query("periodEnd"))
	if rawStart == "" && rawEnd == "" {
		return billingPeriodQuery{}, true
	}
	if rawStart == "" || rawEnd == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_required", "periodStart and periodEnd must be provided together")
		return billingPeriodQuery{}, false
	}
	start, err := time.Parse(time.RFC3339, rawStart)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_start_invalid", "periodStart must be RFC3339 time")
		return billingPeriodQuery{}, false
	}
	end, err := time.Parse(time.RFC3339, rawEnd)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_end_invalid", "periodEnd must be RFC3339 time after periodStart")
		return billingPeriodQuery{}, false
	}
	if !end.After(start) {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_end_invalid", "periodEnd must be RFC3339 time after periodStart")
		return billingPeriodQuery{}, false
	}
	return billingPeriodQuery{Start: &start, End: &end}, true
}

func applyBillingCreatedPeriod(query *gorm.DB, column string, period billingPeriodQuery) *gorm.DB {
	if period.Start != nil {
		query = query.Where(column+" >= ?", *period.Start)
	}
	if period.End != nil {
		query = query.Where(column+" < ?", *period.End)
	}
	return query
}

func applyBillingUsagePeriod(query *gorm.DB, period billingPeriodQuery) *gorm.DB {
	if period.Start != nil {
		query = query.Where("usage.period_end > ?", *period.Start)
	}
	if period.End != nil {
		query = query.Where("usage.period_start < ?", *period.End)
	}
	return query
}

func billingDeploymentTargetIDSQL() string {
	return `CASE
		WHEN usage.resource_type = 'build_run' THEN COALESCE(build_runs.deployment_target_id, '')
		WHEN usage.resource_type IN ('runtime_target', 'storage_volume') THEN split_part(usage.resource_id, ':', 1)
		WHEN usage.resource_type = 'gateway_route' THEN COALESCE(gateway_routes.deployment_target_id, '')
		ELSE ''
	END`
}

func (h *Handlers) ListBillingRateRules(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	rules, err := (billing.Service{DB: h.db}).ListRateRules()
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, rules)
}

func (h *Handlers) UpdateBillingRateRules(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}

	var input updateBillingRateRulesInput
	if !bindJSON(ctx, &input) {
		return
	}
	updates := make([]billing.RateRuleUpdate, 0, len(input.Rules))
	for _, rule := range input.Rules {
		meter := strings.TrimSpace(rule.Meter)
		if meter == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "billing.rate_rule_meter_required", "billing rate rule meter is required")
			return
		}
		creditsPerUnit, err := decimal.NewFromString(strings.TrimSpace(rule.CreditsPerUnit))
		if err != nil || creditsPerUnit.IsNegative() {
			writeErrorCode(ctx, http.StatusBadRequest, "billing.rate_rule_invalid_price", "billing rate rule price must be a non-negative decimal")
			return
		}
		updates = append(updates, billing.RateRuleUpdate{
			Meter:          meter,
			CreditsPerUnit: creditsPerUnit,
			Enabled:        rule.Enabled,
		})
	}
	rules, err := (billing.Service{DB: h.db}).UpdateRateRules(updates)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.rate_rule_unknown", "unknown billing rate rule meter")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, rules)
}

func (h *Handlers) CreateBillingWalletTransaction(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input billingWalletTransactionInput
	if !bindJSON(ctx, &input) {
		return
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.user_required", "user is required")
		return
	}
	var targetUser model.User
	if err := h.db.First(&targetUser, "id = ?", userID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "user not found")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(input.AmountCredits))
	if err != nil || amount.IsZero() {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.transaction_invalid_amount", "billing transaction amount must be a non-zero decimal")
		return
	}
	transactionType := strings.TrimSpace(input.Type)
	if transactionType == "" {
		transactionType = "credit"
	}
	entry, err := (billing.Service{DB: h.db}).ApplyWalletTransaction(billing.WalletTransactionInput{
		UserID:        targetUser.ID,
		AmountCredits: amount,
		Type:          transactionType,
		Description:   strings.TrimSpace(input.Description),
		ActorID:       user.ID,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.transaction_invalid", err.Error())
		return
	}
	h.audit(user.ID, "billing.wallet_transaction", entry.ID, true, "")
	ctx.JSON(http.StatusCreated, entry)
}

func (h *Handlers) CreateExternalBillingTransaction(ctx *gin.Context) {
	if !strings.HasPrefix(strings.ToLower(ctx.GetHeader("Authorization")), "bearer ") {
		writeErrorCode(ctx, http.StatusUnauthorized, "billing.bearer_token_required", "external billing API requires a bearer access token")
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input externalBillingTransactionInput
	if !bindJSON(ctx, &input) {
		return
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.user_required", "user is required")
		return
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 160 {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.idempotency_key_invalid", "idempotency key must be 8 to 160 characters")
		return
	}
	var targetUser model.User
	if err := h.db.First(&targetUser, "id = ?", userID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "user not found")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(input.AmountCredits))
	if err != nil || amount.IsZero() {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.transaction_invalid_amount", "billing transaction amount must be a non-zero decimal")
		return
	}
	transactionType := strings.TrimSpace(input.Type)
	if transactionType == "" {
		transactionType = "credit"
	}
	reason := billing.ReasonExternalRecharge
	if transactionType == "adjustment" {
		reason = billing.ReasonExternalAdjust
	}
	entry, err := (billing.Service{DB: h.db}).ApplyWalletTransaction(billing.WalletTransactionInput{
		UserID:         targetUser.ID,
		AmountCredits:  amount,
		Type:           transactionType,
		Reason:         reason,
		Description:    strings.TrimSpace(input.Description),
		IdempotencyKey: idempotencyKey,
		ActorID:        user.ID,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.transaction_invalid", err.Error())
		return
	}
	h.audit(user.ID, "billing.external_transaction", entry.ID, true, idempotencyKey)
	ctx.JSON(http.StatusOK, entry)
}

func (h *Handlers) CreateGatewayTrafficUsage(ctx *gin.Context) {
	actorID := ""
	var component model.SystemComponentInstallation
	componentAuthenticated := false
	if token := bearerTokenFromHeader(ctx.GetHeader("Authorization")); token != "" {
		if item, ok := h.systemComponentForBearerToken(token, systemComponentGatewayTrafficProbe); ok {
			component = item
			componentAuthenticated = true
			actorID = item.ID
		}
	}
	if !componentAuthenticated {
		user, ok := h.currentUser(ctx)
		if !ok {
			return
		}
		if user.Role != "platform_admin" {
			writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
			return
		}
		actorID = user.ID
	}
	var input gatewayTrafficUsageInput
	if !bindJSON(ctx, &input) {
		return
	}
	routeID := strings.TrimSpace(input.RouteID)
	if routeID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.gateway_route_required", "gateway route is required")
		return
	}
	if input.ResponseBytes <= 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.gateway_response_bytes_invalid", "gateway response bytes must be positive")
		return
	}
	periodStart, err := time.Parse(time.RFC3339, strings.TrimSpace(input.PeriodStart))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_start_invalid", "periodStart must be RFC3339 time")
		return
	}
	periodEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(input.PeriodEnd))
	if err != nil || !periodEnd.After(periodStart) {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_end_invalid", "periodEnd must be RFC3339 time after periodStart")
		return
	}
	var route model.GatewayRoute
	if err := h.db.First(&route, "id = ? and delete_status = ?", routeID, "active").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "gateway route not found")
		return
	}
	if componentAuthenticated && !h.gatewayRouteBelongsToRuntimeCluster(route, component.RuntimeClusterID) {
		writeErrorCode(ctx, http.StatusForbidden, "billing.gateway_route_cluster_forbidden", "gateway route does not belong to the probe runtime cluster")
		return
	}
	err = (billing.Service{DB: h.db}).SettleGatewayTrafficWindow(billing.GatewayTrafficUsageInput{
		Route:         route,
		ResponseBytes: input.ResponseBytes,
		RequestCount:  input.RequestCount,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		ActorID:       actorID,
	})
	if errors.Is(err, billing.ErrAlreadySettled) {
		if componentAuthenticated {
			h.markGatewayTrafficReported(ctx, component.RuntimeClusterID, periodStart, periodEnd)
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "already_settled"})
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if componentAuthenticated {
		h.markGatewayTrafficReported(ctx, component.RuntimeClusterID, periodStart, periodEnd)
	}
	h.audit(actorID, "billing.gateway_traffic", route.ID, true, "")
	ctx.JSON(http.StatusCreated, gin.H{"status": "settled"})
}

func (h *Handlers) CreateGatewayTrafficProbeHello(ctx *gin.Context) {
	token := bearerTokenFromHeader(ctx.GetHeader("Authorization"))
	component, ok := h.systemComponentForBearerToken(token, systemComponentGatewayTrafficProbe)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "gateway traffic probe token is invalid")
		return
	}
	if err := h.gatewayTrafficRuntimeStore().MarkHello(ctx.Request.Context(), component.RuntimeClusterID); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handlers) markGatewayTrafficReported(ctx *gin.Context, runtimeClusterID string, periodStart time.Time, periodEnd time.Time) {
	if err := h.gatewayTrafficRuntimeStore().MarkReport(ctx.Request.Context(), runtimeClusterID, periodStart, periodEnd); err != nil {
		h.audit("", "billing.gateway_traffic_status", runtimeClusterID, false, err.Error())
	}
}

func bearerTokenFromHeader(header string) string {
	header = strings.TrimSpace(header)
	if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}

func (h *Handlers) gatewayRouteBelongsToRuntimeCluster(route model.GatewayRoute, clusterID string) bool {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return false
	}
	var target model.DeploymentTarget
	if err := h.db.Select("id", "cluster_id").First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		return false
	}
	targetClusterID := strings.TrimSpace(target.ClusterID)
	if targetClusterID == "" {
		targetClusterID = h.defaultRuntimeClusterID()
	}
	return targetClusterID == clusterID
}

func (h *Handlers) billingScopeForUser(ctx *gin.Context, user model.User) (billingScope, bool) {
	scope := billingScope{}
	requested := make([]string, 0)
	for _, rawProjectIDs := range ctx.QueryArray("projectIds") {
		requested = append(requested, strings.Split(rawProjectIDs, ",")...)
	}
	requested = normalizeStringList(requested)
	if user.Role == "platform_admin" {
		scope.ProjectIDs = requested
		scope.FilterProjectIDs = len(requested) > 0
		var wallets []model.UserWallet
		if err := h.db.Select("user_id").Find(&wallets).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return scope, false
		}
		for _, wallet := range wallets {
			if strings.TrimSpace(wallet.UserID) != "" {
				scope.UserIDs = append(scope.UserIDs, wallet.UserID)
			}
		}
		if len(scope.UserIDs) == 0 {
			var users []model.User
			if err := h.db.Select("id").Find(&users).Error; err != nil {
				writeError(ctx, http.StatusInternalServerError, err.Error())
				return scope, false
			}
			for _, item := range users {
				scope.UserIDs = append(scope.UserIDs, item.ID)
			}
		}
		if scope.FilterProjectIDs && !h.ensureBillingProjectsExist(ctx, requested) {
			return scope, false
		}
		return scope, true
	}
	scope.UserIDs = []string{user.ID}
	if len(requested) == 0 {
		return scope, true
	}
	scope.FilterProjectIDs = true
	scope.ProjectIDs = requested
	for _, projectID := range requested {
		var count int64
		if err := h.db.Model(&model.Project{}).Where("id = ? and billing_owner_user_id = ?", projectID, user.ID).Count(&count).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return scope, false
		}
		if count == 0 {
			writeErrorCode(ctx, http.StatusForbidden, "billing.project_forbidden", "current user cannot access the requested project billing")
			return scope, false
		}
	}
	return scope, true
}

func (h *Handlers) ensureBillingProjectsExist(ctx *gin.Context, projectIDs []string) bool {
	if len(projectIDs) == 0 {
		return true
	}
	var count int64
	if err := h.db.Model(&model.Project{}).Where("id in ?", projectIDs).Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count != int64(len(projectIDs)) {
		writeErrorCode(ctx, http.StatusNotFound, "billing.project_not_found", "project not found")
		return false
	}
	return true
}

type billingScope struct {
	UserIDs          []string
	ProjectIDs       []string
	FilterProjectIDs bool
}

type updateBillingRateRulesInput struct {
	Rules []updateBillingRateRuleInput `json:"rules"`
}

type updateBillingRateRuleInput struct {
	Meter          string `json:"meter"`
	CreditsPerUnit string `json:"creditsPerUnit"`
	Enabled        bool   `json:"enabled"`
}

type billingLedgerEntryItem struct {
	ID                  string          `json:"id"`
	UserID              string          `json:"userId"`
	ProjectID           string          `json:"projectId"`
	ApplicationID       string          `json:"applicationId"`
	ApplicationName     string          `json:"applicationName"`
	ApplicationSlug     string          `json:"applicationSlug"`
	Type                string          `json:"type"`
	AmountCredits       decimal.Decimal `json:"amountCredits"`
	BalanceAfterCredits decimal.Decimal `json:"balanceAfterCredits"`
	Reason              string          `json:"reason"`
	Meter               string          `json:"meter"`
	UsageRecordID       string          `json:"usageRecordId"`
	ResourceType        string          `json:"resourceType"`
	ResourceID          string          `json:"resourceId"`
	Description         string          `json:"description"`
	CreatedBy           string          `json:"createdBy"`
	CreatedAt           time.Time       `json:"createdAt"`
}

type billingUsageRecordItem struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"projectId"`
	BilledUserID    string          `json:"billedUserId"`
	ApplicationID   string          `json:"applicationId"`
	ApplicationName string          `json:"applicationName"`
	ApplicationSlug string          `json:"applicationSlug"`
	Meter           string          `json:"meter"`
	Quantity        decimal.Decimal `json:"quantity"`
	Unit            string          `json:"unit"`
	AmountCredits   decimal.Decimal `json:"amountCredits"`
	ResourceType    string          `json:"resourceType"`
	ResourceID      string          `json:"resourceId"`
	PeriodStart     time.Time       `json:"periodStart"`
	PeriodEnd       time.Time       `json:"periodEnd"`
	Status          string          `json:"status"`
	Metadata        string          `json:"metadata"`
	SettledAt       *time.Time      `json:"settledAt"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type billingDeploymentSpendItem struct {
	ProjectID             string          `json:"projectId"`
	ProjectName           string          `json:"projectName"`
	ProjectSlug           string          `json:"projectSlug"`
	ApplicationID         string          `json:"applicationId"`
	ApplicationName       string          `json:"applicationName"`
	ApplicationSlug       string          `json:"applicationSlug"`
	DeploymentTargetID    string          `json:"deploymentTargetId"`
	DeploymentTargetName  string          `json:"deploymentTargetName"`
	DeploymentTargetStage string          `json:"deploymentTargetStage"`
	AmountCredits         decimal.Decimal `json:"amountCredits"`
	BuildCredits          decimal.Decimal `json:"buildCredits"`
	RuntimeCredits        decimal.Decimal `json:"runtimeCredits"`
	StorageCredits        decimal.Decimal `json:"storageCredits"`
	GatewayCredits        decimal.Decimal `json:"gatewayCredits"`
	OtherCredits          decimal.Decimal `json:"otherCredits"`
}

type billingWalletTransactionInput struct {
	AmountCredits string `json:"amountCredits"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	UserID        string `json:"userId"`
}

type externalBillingTransactionInput struct {
	AmountCredits  string `json:"amountCredits"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotencyKey"`
	UserID         string `json:"userId"`
}

type gatewayTrafficUsageInput struct {
	RouteID       string `json:"routeId"`
	ResponseBytes int64  `json:"responseBytes"`
	RequestCount  int64  `json:"requestCount"`
	PeriodStart   string `json:"periodStart"`
	PeriodEnd     string `json:"periodEnd"`
}
