package billingapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func (h *Handler) GetBillingSummary(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, ok := h.billingScopeForUser(ctx, user)
	if !ok {
		return
	}
	platformTotal := user.Role == authz.PlatformRoleAdmin && scope.SelectedUserID == ""
	if strings.TrimSpace(ctx.Query("accountScope")) == "current" {
		platformTotal = false
		accountUserID := user.ID
		if user.Role == authz.PlatformRoleAdmin && scope.SelectedUserID != "" {
			accountUserID = scope.SelectedUserID
		}
		scope.UserIDs = []string{accountUserID}
		scope.ProjectIDs = nil
		scope.FilterProjectIDs = false
	}
	lowBalanceLimit := decimal.RequireFromString("100")
	if configuredLimit, err := decimal.NewFromString(strings.TrimSpace(h.configValues([]string{"billing.lowBalanceThresholdCredits"})["billing.lowBalanceThresholdCredits"])); err == nil && !configuredLimit.IsNegative() {
		lowBalanceLimit = configuredLimit
	}
	period, ok := billingPeriodFromQuery(ctx)
	if !ok {
		return
	}
	location, ok := billingTimeZoneFromQuery(ctx)
	if !ok {
		return
	}
	summary, err := (billing.Service{DB: h.dbFor(ctx)}).Summary(billing.SummaryInput{
		UserIDs: scope.UserIDs, ProjectIDs: scope.ProjectIDs, Now: time.Now().In(location),
		LowBalanceLimit: lowBalanceLimit, PeriodStart: period.Start, PeriodEnd: period.End, PlatformTotal: platformTotal,
	})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, summary)
}
