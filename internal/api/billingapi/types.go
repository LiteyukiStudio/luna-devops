package billingapi

import (
	"time"

	"github.com/shopspring/decimal"
)

type updateBillingRateRulesInput struct {
	Rules []updateBillingRateRuleInput `json:"rules"`
}

type updateBillingRateRuleInput struct {
	Meter          string `json:"meter"`
	CreditsPerUnit string `json:"creditsPerUnit"`
	Enabled        bool   `json:"enabled"`
}

type billingLedgerEntryItem struct {
	ID                    string          `json:"id"`
	UserID                string          `json:"userId"`
	ProjectID             string          `json:"projectId"`
	ApplicationID         string          `json:"applicationId"`
	ApplicationName       string          `json:"applicationName"`
	ApplicationIdentifier string          `json:"applicationIdentifier"`
	Type                  string          `json:"type"`
	AmountCredits         decimal.Decimal `json:"amountCredits"`
	BalanceAfterCredits   decimal.Decimal `json:"balanceAfterCredits"`
	Reason                string          `json:"reason"`
	Meter                 string          `json:"meter"`
	UsageRecordID         string          `json:"usageRecordId"`
	ResourceType          string          `json:"resourceType"`
	ResourceID            string          `json:"resourceId"`
	Description           string          `json:"description"`
	CreatedBy             string          `json:"createdBy"`
	CreatedAt             time.Time       `json:"createdAt"`
}

type billingUsageRecordItem struct {
	ID                    string          `json:"id"`
	ProjectID             string          `json:"projectId"`
	BilledUserID          string          `json:"billedUserId"`
	ApplicationID         string          `json:"applicationId"`
	ApplicationName       string          `json:"applicationName"`
	ApplicationIdentifier string          `json:"applicationIdentifier"`
	Meter                 string          `json:"meter"`
	Quantity              decimal.Decimal `json:"quantity"`
	Unit                  string          `json:"unit"`
	AmountCredits         decimal.Decimal `json:"amountCredits"`
	ResourceType          string          `json:"resourceType"`
	ResourceID            string          `json:"resourceId"`
	PeriodStart           time.Time       `json:"periodStart"`
	PeriodEnd             time.Time       `json:"periodEnd"`
	Status                string          `json:"status"`
	Metadata              string          `json:"metadata"`
	SettledAt             *time.Time      `json:"settledAt"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type billingDeploymentSpendItem struct {
	ProjectID             string          `json:"projectId"`
	ProjectName           string          `json:"projectName"`
	ProjectIdentifier     string          `json:"projectIdentifier"`
	ApplicationID         string          `json:"applicationId"`
	ApplicationName       string          `json:"applicationName"`
	ApplicationIdentifier string          `json:"applicationIdentifier"`
	DeploymentTargetID    string          `json:"deploymentTargetId"`
	DeploymentTargetName  string          `json:"deploymentTargetName"`
	DeploymentTargetStage string          `json:"deploymentTargetStage"`
	AmountCredits         decimal.Decimal `json:"amountCredits"`
	BuildCredits          decimal.Decimal `json:"buildCredits"`
	RuntimeCredits        decimal.Decimal `json:"runtimeCredits"`
	StorageCredits        decimal.Decimal `json:"storageCredits"`
	GatewayCredits        decimal.Decimal `json:"gatewayCredits"`
	OtherCredits          decimal.Decimal `json:"otherCredits"`
}

type billingWalletTransactionInput struct {
	AmountCredits string `json:"amountCredits"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	UserID        string `json:"userId"`
}

type externalBillingTransactionInput struct {
	AmountCredits  string `json:"amountCredits"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	IdempotencyKey string `json:"idempotencyKey"`
	UserID         string `json:"userId"`
}

type gatewayTrafficUsageInput struct {
	RouteID       string `json:"routeId"`
	ResponseBytes int64  `json:"responseBytes"`
	RequestCount  int64  `json:"requestCount"`
	PeriodStart   string `json:"periodStart"`
	PeriodEnd     string `json:"periodEnd"`
}
