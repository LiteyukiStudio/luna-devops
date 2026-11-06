package billingapi

import (
	"net/http"
	"strings"
	"time"
	// Embed the IANA database: runtime images do not ship tzdata, and billing
	// day/month boundaries depend on the caller's time zone.
	_ "time/tzdata"

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

// billingTimeZoneFromQuery reads the IANA time zone that defines "today" and
// "this month" for the caller. Requests without timeZone use UTC.
func billingTimeZoneFromQuery(ctx *gin.Context) (*time.Location, bool) {
	name := strings.TrimSpace(ctx.Query("timeZone"))
	if name == "" {
		return time.UTC, true
	}
	location, err := time.LoadLocation(name)
	if err != nil || name == "Local" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.time_zone_invalid", "timeZone must be an IANA time zone name")
		return nil, false
	}
	return location, true
}

// applyBillingOccurredPeriod keeps rows whose billing occurrence time falls in
// [periodStart, periodEnd). Usage occurs at its period_start; it is never
// split or counted by period overlap.
func applyBillingOccurredPeriod(query *gorm.DB, column string, period billingPeriodQuery) *gorm.DB {
	if period.Start != nil {
		query = query.Where(column+" >= ?", *period.Start)
	}
	if period.End != nil {
		query = query.Where(column+" < ?", *period.End)
	}
	return query
}

// billingDeploymentTargetIDSQL attributes usage to a deployment target. Project
// volume storage and volume transfers belong to the project, never to a target.
func billingDeploymentTargetIDSQL() string {
	return `CASE
		WHEN usage.resource_type = 'build_run' THEN COALESCE(build_runs.deployment_target_id, '')
		WHEN usage.resource_type = 'runtime_target' THEN split_part(usage.resource_id, ':', 1)
		WHEN usage.resource_type = 'gateway_route' THEN COALESCE(gateway_routes.deployment_target_id, '')
		ELSE ''
	END`
}

// billingSpendGroupSQL classifies a deployment-spend row: a deployment target,
// the project-level storage group, or usage without a target.
func billingSpendGroupSQL() string {
	return `CASE
		WHEN usage.resource_type IN ('storage_volume', 'volume_transfer') THEN 'project_storage'
		WHEN (` + billingDeploymentTargetIDSQL() + `) <> '' THEN 'deployment_target'
		ELSE 'unassigned'
	END`
}
