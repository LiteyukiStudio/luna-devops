package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/billing"
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
	wallet, err := (billing.Service{DB: h.db}).EnsureWallet(projectID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if !wallet.BalanceCredits.IsPositive() {
		writeErrorCode(ctx, http.StatusPaymentRequired, "billing.insufficient_balance", "project balance is insufficient")
		return false
	}
	return true
}

func (h *Handlers) configBool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(h.configs.get([]string{key})[key]), "true")
}
