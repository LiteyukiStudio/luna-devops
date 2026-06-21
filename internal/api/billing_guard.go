package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ensureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	if !h.configBool("billing.blockNewBuildsWhenInsufficient") {
		return true
	}
	return h.ensureProjectBalanceNonNegative(ctx, projectID)
}

func (h *Handlers) ensureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	if !h.configBool("billing.blockDeployChangesWhenInsufficient") {
		return true
	}
	return h.ensureProjectBalanceNonNegative(ctx, projectID)
}

func (h *Handlers) ensureProjectBalanceNonNegative(ctx *gin.Context, projectID string) bool {
	var project model.Project
	if err := h.db.Select("billing_owner_user_id").First(&project, "id = ?", projectID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	ownerID := strings.TrimSpace(project.BillingOwnerUserID)
	if ownerID == "" {
		writeErrorCode(ctx, http.StatusPaymentRequired, "billing.owner_required", "project billing owner is required")
		return false
	}
	wallet, err := (billing.Service{DB: h.db}).EnsureWallet(ownerID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if !wallet.BalanceCredits.IsPositive() {
		writeErrorCode(ctx, http.StatusPaymentRequired, "billing.insufficient_balance", "billing owner balance is insufficient")
		return false
	}
	return true
}

func (h *Handlers) configBool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(h.configs.get([]string{key})[key]), "true")
}
