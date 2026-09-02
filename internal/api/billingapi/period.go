package billingapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
