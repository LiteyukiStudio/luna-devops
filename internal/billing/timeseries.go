package billing

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

const (
	GranularityHour  = "hour"
	GranularityDay   = "day"
	GranularityMonth = "month"

	// MaxSpendTimeseriesBuckets caps continuous time buckets returned in one
	// chart query (covers full 31-day months at hourly resolution and >2 years at
	// daily resolution while preventing unbounded multi-year hourly scans).
	MaxSpendTimeseriesBuckets = 800
)

var (
	ErrInvalidGranularity          = errors.New("invalid billing timeseries granularity")
	ErrPeriodTooWideForGranularity = errors.New("billing period exceeds maximum buckets for granularity")
)

type SpendTimeseriesInput struct {
	UserIDs     []string
	ProjectIDs  []string
	Now         time.Time
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	Granularity string
}

type SpendTimeseries struct {
	Granularity  string                  `json:"granularity"`
	TimeZone     string                  `json:"timeZone"`
	PeriodStart  time.Time               `json:"periodStart"`
	PeriodEnd    time.Time               `json:"periodEnd"`
	TotalCredits decimal.Decimal         `json:"totalCredits"`
	Categories   []BillingSpendCategory  `json:"categories"`
	Buckets      []SpendTimeseriesBucket `json:"buckets"`
}

type SpendTimeseriesBucket struct {
	BucketStart  time.Time              `json:"bucketStart"`
	PeriodStart  time.Time              `json:"periodStart"`
	PeriodEnd    time.Time              `json:"periodEnd"`
	TotalCredits decimal.Decimal        `json:"totalCredits"`
	Categories   []BillingSpendCategory `json:"categories"`
}

func ValidGranularity(granularity string) bool {
	switch granularity {
	case GranularityHour, GranularityDay, GranularityMonth:
		return true
	default:
		return false
	}
}

func (s Service) SpendTimeseries(input SpendTimeseriesInput) (SpendTimeseries, error) {
	if !ValidGranularity(input.Granularity) {
		return SpendTimeseries{}, ErrInvalidGranularity
	}
	loc := input.Now.Location()
	if loc == nil {
		loc = time.UTC
	}
	tzName := loc.String()

	var periodStart, periodEnd time.Time
	if input.PeriodStart != nil && input.PeriodEnd != nil {
		periodStart = input.PeriodStart.In(loc)
		periodEnd = input.PeriodEnd.In(loc)
	} else {
		_, monthStart := DayAndMonthStart(input.Now.In(loc))
		periodStart = monthStart
		periodEnd = localDayStart(monthStart.Year(), int(monthStart.Month())+1, 1, loc)
	}

	buckets, err := buildTimeseriesBuckets(periodStart, periodEnd, input.Granularity, loc)
	if err != nil {
		return SpendTimeseries{}, err
	}

	result := SpendTimeseries{
		Granularity:  input.Granularity,
		TimeZone:     tzName,
		PeriodStart:  periodStart,
		PeriodEnd:    periodEnd,
		TotalCredits: decimal.Zero,
		Categories:   []BillingSpendCategory{},
		Buckets:      buckets,
	}
	if len(input.UserIDs) == 0 || len(buckets) == 0 {
		return result, nil
	}

	bucketIndexByEpoch := make(map[int64]int, len(buckets))
	bucketCategoryTotals := make([]map[string]decimal.Decimal, len(buckets))
	for idx, bucket := range buckets {
		bucketIndexByEpoch[bucket.BucketStart.UTC().Unix()] = idx
	}

	var rows []struct {
		BucketStart   time.Time
		Reason        string
		Meter         string
		AmountCredits decimal.Decimal
	}
	truncExpr := "date_trunc(?, " + OccurredAtSQL + ", ?)"
	period := spendPeriod{start: &periodStart, end: &periodEnd}
	if err := s.debitLedger(input.UserIDs, input.ProjectIDs, period).
		Select(truncExpr+" AS bucket_start, ledger.reason, ledger.meter, COALESCE(SUM(ABS(ledger.amount_credits)), 0) AS amount_credits", input.Granularity, tzName).
		Group("1, ledger.reason, ledger.meter").
		Scan(&rows).Error; err != nil {
		return SpendTimeseries{}, err
	}

	overallCategories := map[string]decimal.Decimal{}
	for _, row := range rows {
		if row.AmountCredits.IsZero() {
			continue
		}
		idx, ok := bucketIndexByEpoch[row.BucketStart.UTC().Unix()]
		if !ok {
			idx = findBucketIndexByInstant(buckets, row.BucketStart)
		}
		if idx < 0 {
			continue
		}
		category := billingCategory(row.Reason, row.Meter)
		overallCategories[category] = overallCategories[category].Add(row.AmountCredits)
		result.TotalCredits = result.TotalCredits.Add(row.AmountCredits)

		if bucketCategoryTotals[idx] == nil {
			bucketCategoryTotals[idx] = map[string]decimal.Decimal{}
		}
		bucketCategoryTotals[idx][category] = bucketCategoryTotals[idx][category].Add(row.AmountCredits)
		result.Buckets[idx].TotalCredits = result.Buckets[idx].TotalCredits.Add(row.AmountCredits)
	}

	result.Categories = orderedCategories(overallCategories)
	for idx := range result.Buckets {
		if bucketCategoryTotals[idx] != nil {
			result.Buckets[idx].Categories = orderedCategories(bucketCategoryTotals[idx])
		}
	}
	return result, nil
}

func orderedCategories(amounts map[string]decimal.Decimal) []BillingSpendCategory {
	order := []string{"build", "runtime", "storage", "gateway", "ai", "adjustment", "other"}
	categories := make([]BillingSpendCategory, 0, len(order))
	for _, category := range order {
		amount := amounts[category]
		if amount.IsZero() {
			continue
		}
		categories = append(categories, BillingSpendCategory{
			Category:      category,
			AmountCredits: amount,
		})
	}
	return categories
}

func buildTimeseriesBuckets(periodStart, periodEnd time.Time, granularity string, loc *time.Location) ([]SpendTimeseriesBucket, error) {
	if !periodEnd.After(periodStart) {
		return []SpendTimeseriesBucket{}, nil
	}
	cursor := truncateBucketStart(periodStart, granularity, loc)
	buckets := make([]SpendTimeseriesBucket, 0, 32)
	for cursor.Before(periodEnd) {
		if len(buckets) >= MaxSpendTimeseriesBuckets {
			return nil, ErrPeriodTooWideForGranularity
		}
		next := nextBucketStart(cursor, granularity, loc)
		if !next.After(cursor) {
			next = cursor.Add(time.Hour).In(loc)
		}
		effectiveStart := cursor
		if effectiveStart.Before(periodStart) {
			effectiveStart = periodStart
		}
		effectiveEnd := next
		if effectiveEnd.After(periodEnd) {
			effectiveEnd = periodEnd
		}
		buckets = append(buckets, SpendTimeseriesBucket{
			BucketStart:  cursor,
			PeriodStart:  effectiveStart,
			PeriodEnd:    effectiveEnd,
			TotalCredits: decimal.Zero,
			Categories:   []BillingSpendCategory{},
		})
		cursor = next
	}
	return buckets, nil
}

// localDayStart returns the earliest valid local instant of (year, month, day)
// in loc. In zones that spring forward at 00:00 (e.g. America/Santiago), Go's
// time.Date(y, m, d, 0, 0, 0, 0, loc) resolves to 23:00 of the previous day;
// stepping forward across the DST gap yields 01:00 on the target date, matching
// PostgreSQL date_trunc('day', ..., tz).
func localDayStart(year, month, day int, loc *time.Location) time.Time {
	targetUTC := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	ty, tm, td := targetUTC.Date()
	candidate := time.Date(ty, tm, td, 0, 0, 0, 0, loc)
	if cy, cm, cd := candidate.In(loc).Date(); cy == ty && cm == tm && cd == td {
		return candidate
	}
	for step := 1; step <= 12; step++ {
		shifted := candidate.Add(time.Duration(step) * 15 * time.Minute).In(loc)
		if sy, sm, sd := shifted.Date(); sy == ty && sm == tm && sd == td {
			return shifted
		}
	}
	return candidate.Add(2 * time.Hour).In(loc)
}

func truncateBucketStart(instant time.Time, granularity string, loc *time.Location) time.Time {
	local := instant.In(loc)
	switch granularity {
	case GranularityHour:
		elapsed := time.Duration(local.Minute())*time.Minute +
			time.Duration(local.Second())*time.Second +
			time.Duration(local.Nanosecond())
		return instant.Add(-elapsed).In(loc)
	case GranularityMonth:
		return localDayStart(local.Year(), int(local.Month()), 1, loc)
	case GranularityDay:
		fallthrough
	default:
		return localDayStart(local.Year(), int(local.Month()), local.Day(), loc)
	}
}

func nextBucketStart(bucketStart time.Time, granularity string, loc *time.Location) time.Time {
	local := bucketStart.In(loc)
	switch granularity {
	case GranularityHour:
		candidate := bucketStart.Add(time.Hour).In(loc)
		if candidate.Minute() != 0 || candidate.Second() != 0 || candidate.Nanosecond() != 0 {
			truncated := truncateBucketStart(candidate, GranularityHour, loc)
			if truncated.After(bucketStart) {
				return truncated
			}
			return truncateBucketStart(bucketStart.Add(90*time.Minute), GranularityHour, loc)
		}
		return candidate
	case GranularityMonth:
		return localDayStart(local.Year(), int(local.Month())+1, 1, loc)
	case GranularityDay:
		fallthrough
	default:
		return localDayStart(local.Year(), int(local.Month()), local.Day()+1, loc)
	}
}

func findBucketIndexByInstant(buckets []SpendTimeseriesBucket, instant time.Time) int {
	if len(buckets) == 0 {
		return -1
	}
	for idx, bucket := range buckets {
		if !instant.Before(bucket.BucketStart) && instant.Before(bucket.PeriodEnd) {
			return idx
		}
	}
	if instant.Before(buckets[0].BucketStart) {
		return 0
	}
	return -1
}
