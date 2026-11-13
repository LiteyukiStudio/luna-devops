package billingapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	code, err := projectBalanceDenialCode(h.dbFor(ctx), projectID)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	switch code {
	case "":
		return true
	case billingOwnerRequiredCode:
		writeErrorCode(ctx, http.StatusPaymentRequired, code, "project billing owner is required")
	default:
		writeErrorCode(ctx, http.StatusPaymentRequired, code, "billing owner balance is insufficient")
	}
	return false
}

// NewBuildDenialCode evaluates the same admission policy as
// EnsureBillingAllowsNewBuild for callers without an HTTP response to write,
// such as Git webhooks. It returns the stable billing code that blocks the
// build, or an empty string when the build is allowed.
func (h *Handler) NewBuildDenialCode(ctx context.Context, projectID string) (string, error) {
	if !h.configBool("billing.blockNewBuildsWhenInsufficient") {
		return "", nil
	}
	return projectBalanceDenialCode(h.dbWithContext(ctx), projectID)
}

const (
	billingOwnerRequiredCode       = "billing.owner_required"
	billingInsufficientBalanceCode = "billing.insufficient_balance"
)

func projectBalanceDenialCode(db *gorm.DB, projectID string) (string, error) {
	var project model.Project
	if err := db.Select("billing_owner_user_id", "system_key").First(&project, "id = ?", projectID).Error; err != nil {
		return "", err
	}
	if strings.TrimSpace(project.SystemKey) != "" {
		return "", nil
	}
	ownerID := strings.TrimSpace(project.BillingOwnerUserID)
	if ownerID == "" {
		return billingOwnerRequiredCode, nil
	}
	wallet, err := (billing.Service{DB: db}).EnsureWallet(ownerID)
	if err != nil {
		return "", err
	}
	if !wallet.BalanceCredits.IsPositive() {
		return billingInsufficientBalanceCode, nil
	}
	return "", nil
}

func (h *Handler) configBool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(h.configValues([]string{key})[key]), "true")
}
