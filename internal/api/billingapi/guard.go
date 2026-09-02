package billingapi

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handler) EnsureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	if !h.configBool("billing.blockNewBuildsWhenInsufficient") {
		return true
	}
	return h.ensureProjectBalanceNonNegative(ctx, projectID)
}

func (h *Handler) EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	if !h.configBool("billing.blockDeployChangesWhenInsufficient") {
		return true
	}
	return h.ensureProjectBalanceNonNegative(ctx, projectID)
}

// Managed project volumes begin accruing storage usage as soon as capacity is
// reserved. Unlike the optional deploy-wide policy, their create/import/expand
// admission always requires a positive billing-owner balance.
func (h *Handler) EnsureBillingAllowsManagedVolumeChange(ctx *gin.Context, projectID string) bool {
	return h.ensureProjectBalanceNonNegative(ctx, projectID)
}

func (h *Handler) ensureProjectBalanceNonNegative(ctx *gin.Context, projectID string) bool {
	var project model.Project
	if err := h.dbFor(ctx).Select("billing_owner_user_id").First(&project, "id = ?", projectID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	ownerID := strings.TrimSpace(project.BillingOwnerUserID)
	if ownerID == "" {
		writeErrorCode(ctx, http.StatusPaymentRequired, "billing.owner_required", "project billing owner is required")
		return false
	}
	wallet, err := (billing.Service{DB: h.dbFor(ctx)}).EnsureWallet(ownerID)
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

func (h *Handler) configBool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(h.configValues([]string{key})[key]), "true")
}
