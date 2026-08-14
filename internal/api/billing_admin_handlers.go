package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func (h *Handlers) ListBillingRateRules(ctx *gin.Context) {
	_, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	rules, err := (billing.Service{DB: h.dbFor(ctx)}).ListRateRules()
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
	if user.Role != authz.PlatformRoleAdmin {
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
	rules, err := (billing.Service{DB: h.dbFor(ctx)}).UpdateRateRules(updates)
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
	if user.Role != authz.PlatformRoleAdmin {
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
	if err := h.dbFor(ctx).First(&targetUser, "id = ?", userID).Error; err != nil {
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
	entry, err := (billing.Service{DB: h.dbFor(ctx)}).ApplyWalletTransaction(billing.WalletTransactionInput{
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
	h.auditWithContext(user.ID, "billing.wallet_transaction", entry.ID, true, "", ctx.Request.Context())
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
	if user.Role != authz.PlatformRoleAdmin {
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
	if err := h.dbFor(ctx).First(&targetUser, "id = ?", userID).Error; err != nil {
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
	entry, err := (billing.Service{DB: h.dbFor(ctx)}).ApplyWalletTransaction(billing.WalletTransactionInput{
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
	h.auditWithContext(user.ID, "billing.external_transaction", entry.ID, true, idempotencyKey, ctx.Request.Context())
	ctx.JSON(http.StatusOK, entry)
}
