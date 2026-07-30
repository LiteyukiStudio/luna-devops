package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
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
		accountUserID := user.ID
		if user.Role == authz.PlatformRoleAdmin && scope.SelectedUserID != "" {
			accountUserID = scope.SelectedUserID
		}
		scope.UserIDs = []string{accountUserID}
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
