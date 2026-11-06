package billing

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	// BalanceScopeAccount means balanceCredits is one wallet and balanceStatus applies to it.
	BalanceScopeAccount = "account"
	// BalanceScopePlatformTotal means balanceCredits sums every wallet in the
	// platform; no single account balance status exists for that total.
	BalanceScopePlatformTotal = "platform_total"
)

// OccurredAtSQL is the billing occurrence time of a ledger entry joined as
// `ledger` with its usage record joined as `usage`: usage debits occur at the
// start of the metered usage period, while entries without a usage record
// (manual or external adjustments) occur when they are posted.
const OccurredAtSQL = "COALESCE(usage.period_start, ledger.created_at)"

type ProjectBillingSummary struct {
	BalanceScope      string                 `json:"balanceScope"`
	BalanceCredits    decimal.Decimal        `json:"balanceCredits"`
	TodaySpend        decimal.Decimal        `json:"todaySpend"`
	MonthSpend        decimal.Decimal        `json:"monthSpend"`
	PeriodSpend       decimal.Decimal        `json:"periodSpend"`
	PendingSpend      decimal.Decimal        `json:"pendingSpend"`
	AvailableCredits  decimal.Decimal        `json:"availableCredits"`
	LowBalanceLimit   decimal.Decimal        `json:"lowBalanceLimit"`
	BalanceStatus     string                 `json:"balanceStatus,omitempty"`
	MonthlyCategories []BillingSpendCategory `json:"monthlyCategories"`
	PeriodCategories  []BillingSpendCategory `json:"periodCategories"`
}

type BillingSpendCategory struct {
	Category      string          `json:"category"`
	AmountCredits decimal.Decimal `json:"amountCredits"`
}

type SummaryInput struct {
	UserIDs    []string
	ProjectIDs []string
	// Now carries the caller's time zone; today and this month start at local
	// midnight of that zone.
	Now             time.Time
	LowBalanceLimit decimal.Decimal
	PeriodStart     *time.Time
	PeriodEnd       *time.Time
	// PlatformTotal sums every wallet in UserIDs without an account status.
	PlatformTotal bool
}

type spendPeriod struct {
	start *time.Time
	end   *time.Time
}

// DayAndMonthStart returns local midnight of today and of the first day of the
// current month in now's location.
func DayAndMonthStart(now time.Time) (time.Time, time.Time) {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return dayStart, monthStart
}

func (s Service) Summary(input SummaryInput) (ProjectBillingSummary, error) {
	summary := ProjectBillingSummary{BalanceScope: BalanceScopeAccount, LowBalanceLimit: input.LowBalanceLimit}
	if input.PlatformTotal {
		summary.BalanceScope = BalanceScopePlatformTotal
	}
	if len(input.UserIDs) == 0 {
		if !input.PlatformTotal {
			summary.BalanceStatus = "ok"
		}
		return summary, nil
	}
	balance, err := s.balance(input.UserIDs, input.PlatformTotal)
	if err != nil {
		return summary, err
	}
	summary.BalanceCredits = balance
	dayStart, monthStart := DayAndMonthStart(input.Now)
	if summary.TodaySpend, err = s.spend(input.UserIDs, input.ProjectIDs, spendPeriod{start: &dayStart}); err != nil {
		return summary, err
	}
	if summary.MonthSpend, err = s.spend(input.UserIDs, input.ProjectIDs, spendPeriod{start: &monthStart}); err != nil {
		return summary, err
	}
	if summary.PendingSpend, err = s.pendingSpend(input.UserIDs, input.ProjectIDs); err != nil {
		return summary, err
	}
	summary.AvailableCredits = summary.BalanceCredits.Sub(summary.PendingSpend)
	if !input.PlatformTotal {
		summary.BalanceStatus = balanceStatus(summary.AvailableCredits, input.LowBalanceLimit)
	}
	if summary.MonthlyCategories, err = s.spendCategories(input.UserIDs, input.ProjectIDs, spendPeriod{start: &monthStart}); err != nil {
		return summary, err
	}
	if input.PeriodStart != nil && input.PeriodEnd != nil {
		period := spendPeriod{start: input.PeriodStart, end: input.PeriodEnd}
		if summary.PeriodSpend, err = s.spend(input.UserIDs, input.ProjectIDs, period); err != nil {
			return summary, err
		}
		if summary.PeriodCategories, err = s.spendCategories(input.UserIDs, input.ProjectIDs, period); err != nil {
			return summary, err
		}
	} else {
		summary.PeriodSpend = summary.MonthSpend
		summary.PeriodCategories = summary.MonthlyCategories
	}
	return summary, nil
}

// balance reads one account wallet (creating it on first use) or sums the
// existing wallets of a platform-wide scope without creating new ones.
func (s Service) balance(userIDs []string, platformTotal bool) (decimal.Decimal, error) {
	if !platformTotal && len(userIDs) == 1 {
		wallet, err := s.EnsureWallet(userIDs[0])
		return wallet.BalanceCredits, err
	}
	var total decimal.Decimal
	err := s.DB.Table("user_wallets").Where("user_id IN ?", userIDs).
		Select("COALESCE(SUM(balance_credits), 0)").Scan(&total).Error
	return total, err
}

func (s Service) debitLedger(userIDs []string, projectIDs []string, period spendPeriod) *gorm.DB {
	query := s.DB.Table("billing_ledger_entries AS ledger").
		Joins("LEFT JOIN billing_usage_records AS usage ON usage.id = ledger.usage_record_id").
		Where("ledger.user_id IN ? AND ledger.type = ?", userIDs, "debit")
	if len(projectIDs) > 0 {
		query = query.Where("ledger.project_id IN ?", projectIDs)
	}
	if period.start != nil {
		query = query.Where(OccurredAtSQL+" >= ?", *period.start)
	}
	if period.end != nil {
		query = query.Where(OccurredAtSQL+" < ?", *period.end)
	}
	return query
}

func (s Service) spend(userIDs []string, projectIDs []string, period spendPeriod) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := s.debitLedger(userIDs, projectIDs, period).
		Select("COALESCE(SUM(ABS(ledger.amount_credits)), 0)").Scan(&total).Error
	return total, err
}

func (s Service) pendingSpend(userIDs []string, projectIDs []string) (decimal.Decimal, error) {
	query := s.DB.Table("billing_usage_records").Where("billed_user_id IN ? AND status = ?", userIDs, "pending")
	if len(projectIDs) > 0 {
		query = query.Where("project_id IN ?", projectIDs)
	}
	var total decimal.Decimal
	err := query.Select("COALESCE(SUM(amount_credits), 0)").Scan(&total).Error
	return total, err
}

func (s Service) spendCategories(userIDs []string, projectIDs []string, period spendPeriod) ([]BillingSpendCategory, error) {
	var rows []struct {
		Reason        string
		Meter         string
		AmountCredits decimal.Decimal
	}
	if err := s.debitLedger(userIDs, projectIDs, period).
		Select("ledger.reason, ledger.meter, COALESCE(SUM(ABS(ledger.amount_credits)), 0) AS amount_credits").
		Group("ledger.reason, ledger.meter").Scan(&rows).Error; err != nil {
		return nil, err
	}
	amounts := map[string]decimal.Decimal{}
	for _, row := range rows {
		category := billingCategory(row.Reason, row.Meter)
		amounts[category] = amounts[category].Add(row.AmountCredits)
	}
	order := []string{"build", "runtime", "storage", "gateway", "ai", "adjustment", "other"}
	categories := make([]BillingSpendCategory, 0, len(order))
	for _, category := range order {
		amount := amounts[category]
		if amount.IsZero() {
			continue
		}
		categories = append(categories, BillingSpendCategory{Category: category, AmountCredits: amount})
	}
	return categories, nil
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
	case strings.HasPrefix(value, "ai."):
		return "ai"
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
