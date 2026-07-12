package billing

import (
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
)

type ProjectBillingSummary struct {
	BalanceCredits    decimal.Decimal        `json:"balanceCredits"`
	TodaySpend        decimal.Decimal        `json:"todaySpend"`
	MonthSpend        decimal.Decimal        `json:"monthSpend"`
	PeriodSpend       decimal.Decimal        `json:"periodSpend"`
	PendingSpend      decimal.Decimal        `json:"pendingSpend"`
	AvailableCredits  decimal.Decimal        `json:"availableCredits"`
	LowBalanceLimit   decimal.Decimal        `json:"lowBalanceLimit"`
	BalanceStatus     string                 `json:"balanceStatus"`
	MonthlyCategories []BillingSpendCategory `json:"monthlyCategories"`
	PeriodCategories  []BillingSpendCategory `json:"periodCategories"`
}

type BillingSpendCategory struct {
	Category      string          `json:"category"`
	AmountCredits decimal.Decimal `json:"amountCredits"`
}

func (s Service) Summary(userIDs []string, projectIDs []string, now time.Time, lowBalanceLimit decimal.Decimal, periodStart *time.Time, periodEnd *time.Time) (ProjectBillingSummary, error) {
	summary := ProjectBillingSummary{LowBalanceLimit: lowBalanceLimit, BalanceStatus: "ok"}
	if len(userIDs) == 0 {
		return summary, nil
	}
	for _, userID := range userIDs {
		wallet, err := s.EnsureWallet(userID)
		if err != nil {
			return summary, err
		}
		summary.BalanceCredits = summary.BalanceCredits.Add(wallet.BalanceCredits)
	}
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	summary.TodaySpend = s.spendSince(userIDs, projectIDs, dayStart)
	summary.MonthSpend = s.spendSince(userIDs, projectIDs, monthStart)
	summary.PendingSpend = s.pendingSpend(userIDs, projectIDs)
	summary.AvailableCredits = summary.BalanceCredits.Sub(summary.PendingSpend)
	summary.BalanceStatus = balanceStatus(summary.AvailableCredits, lowBalanceLimit)
	summary.MonthlyCategories = s.spendCategories(userIDs, projectIDs, &monthStart, nil)
	if periodStart != nil && periodEnd != nil {
		summary.PeriodSpend = s.spendBetween(userIDs, projectIDs, *periodStart, *periodEnd)
		summary.PeriodCategories = s.spendCategories(userIDs, projectIDs, periodStart, periodEnd)
	} else {
		summary.PeriodSpend = summary.MonthSpend
		summary.PeriodCategories = summary.MonthlyCategories
	}
	return summary, nil
}

func (s Service) spendSince(userIDs []string, projectIDs []string, since time.Time) decimal.Decimal {
	return s.spendWithPeriod(userIDs, projectIDs, &since, nil)
}

func (s Service) spendBetween(userIDs []string, projectIDs []string, start time.Time, end time.Time) decimal.Decimal {
	return s.spendWithPeriod(userIDs, projectIDs, &start, &end)
}

func (s Service) spendWithPeriod(userIDs []string, projectIDs []string, start *time.Time, end *time.Time) decimal.Decimal {
	var entries []model.BillingLedgerEntry
	query := s.DB.Where("user_id in ? and type = ?", userIDs, "debit")
	if len(projectIDs) > 0 {
		query = query.Where("project_id in ?", projectIDs)
	}
	if start != nil {
		query = query.Where("created_at >= ?", *start)
	}
	if end != nil {
		query = query.Where("created_at < ?", *end)
	}
	if err := query.Find(&entries).Error; err != nil {
		return decimal.Zero
	}
	total := decimal.Zero
	for _, entry := range entries {
		total = total.Add(entry.AmountCredits.Abs())
	}
	return total
}

func (s Service) pendingSpend(userIDs []string, projectIDs []string) decimal.Decimal {
	var records []model.BillingUsageRecord
	query := s.DB.Where("billed_user_id in ? and status = ?", userIDs, "pending")
	if len(projectIDs) > 0 {
		query = query.Where("project_id in ?", projectIDs)
	}
	if err := query.Find(&records).Error; err != nil {
		return decimal.Zero
	}
	total := decimal.Zero
	for _, record := range records {
		total = total.Add(record.AmountCredits)
	}
	return total
}

func (s Service) spendCategories(userIDs []string, projectIDs []string, start *time.Time, end *time.Time) []BillingSpendCategory {
	var entries []model.BillingLedgerEntry
	query := s.DB.Where("user_id in ? and type = ?", userIDs, "debit")
	if len(projectIDs) > 0 {
		query = query.Where("project_id in ?", projectIDs)
	}
	if start != nil {
		query = query.Where("created_at >= ?", *start)
	}
	if end != nil {
		query = query.Where("created_at < ?", *end)
	}
	if err := query.Find(&entries).Error; err != nil {
		return nil
	}
	amounts := map[string]decimal.Decimal{}
	for _, entry := range entries {
		category := billingCategory(entry.Reason, entry.Meter)
		amounts[category] = amounts[category].Add(entry.AmountCredits.Abs())
	}
	order := []string{"build", "runtime", "storage", "gateway", "adjustment", "other"}
	categories := make([]BillingSpendCategory, 0, len(order))
	for _, category := range order {
		amount := amounts[category]
		if amount.IsZero() {
			continue
		}
		categories = append(categories, BillingSpendCategory{Category: category, AmountCredits: amount})
	}
	return categories
}

func billingCategory(reason string, meter string) string {
	value := strings.TrimSpace(reason)
	if value == "" {
		value = strings.TrimSpace(meter)
	}
	switch {
	case strings.HasPrefix(value, "build."):
		return "build"
	case strings.HasPrefix(value, "runtime."):
		return "runtime"
	case strings.HasPrefix(value, "storage."):
		return "storage"
	case strings.HasPrefix(value, "gateway."):
		return "gateway"
	case strings.HasPrefix(value, "billing."):
		return "adjustment"
	default:
		return "other"
	}
}

func balanceStatus(available decimal.Decimal, lowBalanceLimit decimal.Decimal) string {
	if available.IsNegative() || available.IsZero() {
		return "insufficient"
	}
	if lowBalanceLimit.IsPositive() && available.LessThanOrEqual(lowBalanceLimit) {
		return "low"
	}
	return "ok"
}
