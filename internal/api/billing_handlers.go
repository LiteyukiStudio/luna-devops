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
	projectIDs, ok := h.billingProjectIDsForUser(ctx, user)
	if !ok {
		return
	}
	lowBalanceLimit := decimal.RequireFromString("100")
	if configuredLimit, err := decimal.NewFromString(strings.TrimSpace(h.configs.get([]string{"billing.lowBalanceThresholdCredits"})["billing.lowBalanceThresholdCredits"])); err == nil && !configuredLimit.IsNegative() {
		lowBalanceLimit = configuredLimit
	}
	summary, err := (billing.Service{DB: h.db}).Summary(projectIDs, time.Now(), lowBalanceLimit)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, summary)
}

func (h *Handlers) ListBillingLedgerEntries(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectIDs, ok := h.billingProjectIDsForUser(ctx, user)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.db.Where("project_id in ?", projectIDs)
	if entryType := strings.TrimSpace(ctx.Query("type")); entryType != "" {
		query = query.Where("type = ?", entryType)
	}
	var total int64
	if err := query.Model(&model.BillingLedgerEntry{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var entries []model.BillingLedgerEntry
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":     "created_at",
		"amountCredits": "amount_credits",
		"reason":        "reason",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&entries).Error; err != nil {
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
	projectIDs, ok := h.billingProjectIDsForUser(ctx, user)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.db.Where("project_id in ?", projectIDs)
	if meter := strings.TrimSpace(ctx.Query("meter")); meter != "" {
		query = query.Where("meter = ?", meter)
	}
	var total int64
	if err := query.Model(&model.BillingUsageRecord{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var records []model.BillingUsageRecord
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":     "created_at",
		"amountCredits": "amount_credits",
		"meter":         "meter",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&records).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(records, total, pagination))
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
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.project_required", "project is required")
		return
	}
	var project model.Project
	if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
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
		ProjectID:     project.ID,
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
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.project_required", "project is required")
		return
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 160 {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.idempotency_key_invalid", "idempotency key must be 8 to 160 characters")
		return
	}
	var project model.Project
	if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
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
		ProjectID:      project.ID,
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

func (h *Handlers) billingProjectIDsForUser(ctx *gin.Context, user model.User) ([]string, bool) {
	var memberships []model.ProjectMember
	if err := h.db.Find(&memberships, "user_id = ?", user.ID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	allowed := map[string]bool{}
	for _, membership := range memberships {
		allowed[membership.ProjectID] = true
	}
	requested := make([]string, 0)
	for _, rawProjectIDs := range ctx.QueryArray("projectIds") {
		requested = append(requested, strings.Split(rawProjectIDs, ",")...)
	}
	requested = normalizeStringList(requested)
	if len(requested) == 0 {
		ids := make([]string, 0, len(allowed))
		for projectID := range allowed {
			ids = append(ids, projectID)
		}
		return ids, true
	}
	ids := make([]string, 0, len(requested))
	for _, projectID := range requested {
		if !allowed[projectID] {
			writeErrorCode(ctx, http.StatusForbidden, "billing.project_forbidden", "current user cannot access the requested project billing")
			return nil, false
		}
		ids = append(ids, projectID)
	}
	return ids, true
}

type updateBillingRateRulesInput struct {
	Rules []updateBillingRateRuleInput `json:"rules"`
}

type updateBillingRateRuleInput struct {
	Meter          string `json:"meter"`
	CreditsPerUnit string `json:"creditsPerUnit"`
	Enabled        bool   `json:"enabled"`
}

type billingWalletTransactionInput struct {
	ProjectID     string `json:"projectId"`
	AmountCredits string `json:"amountCredits"`
	Type          string `json:"type"`
	Description   string `json:"description"`
}

type externalBillingTransactionInput struct {
	ProjectID      string `json:"projectId"`
	AmountCredits  string `json:"amountCredits"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotencyKey"`
}
