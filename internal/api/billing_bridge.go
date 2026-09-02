package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/api/billingapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type billingHost struct {
	handlers *Handlers
}

func (host billingHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.handlers.dbFor(ctx)
}

func (host billingHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host billingHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}

func (host billingHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host billingHost) ConfigValues(keys []string) map[string]string {
	return host.handlers.configs.get(keys)
}

func (host billingHost) DefaultRuntimeClusterID(ctx context.Context) string {
	return host.handlers.defaultRuntimeClusterID(ctx)
}

func (host billingHost) ObserveSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation) {
	host.handlers.observeSystemComponentInstallations(ctx, items)
}

func (host billingHost) SystemComponentForBearerToken(token, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool) {
	return host.handlers.systemComponentForBearerToken(token, componentID, ctx)
}

func (h *Handlers) billingAPI() *billingapi.Handler {
	if h.billingHandlers != nil {
		return h.billingHandlers
	}
	return billingapi.New(billingHost{handlers: h})
}

func (h *Handlers) GetBillingSummary(ctx *gin.Context) { h.billingAPI().GetBillingSummary(ctx) }
func (h *Handlers) ListBillingLedgerEntries(ctx *gin.Context) {
	h.billingAPI().ListBillingLedgerEntries(ctx)
}
func (h *Handlers) ListBillingUsageRecords(ctx *gin.Context) {
	h.billingAPI().ListBillingUsageRecords(ctx)
}
func (h *Handlers) ListBillingDeploymentSpend(ctx *gin.Context) {
	h.billingAPI().ListBillingDeploymentSpend(ctx)
}
func (h *Handlers) ListBillingRateRules(ctx *gin.Context) {
	h.billingAPI().ListBillingRateRules(ctx)
}
func (h *Handlers) UpdateBillingRateRules(ctx *gin.Context) {
	h.billingAPI().UpdateBillingRateRules(ctx)
}
func (h *Handlers) CreateBillingWalletTransaction(ctx *gin.Context) {
	h.billingAPI().CreateBillingWalletTransaction(ctx)
}
func (h *Handlers) CreateExternalBillingTransaction(ctx *gin.Context) {
	h.billingAPI().CreateExternalBillingTransaction(ctx)
}
func (h *Handlers) GetGatewayTrafficStatus(ctx *gin.Context) {
	h.billingAPI().GetGatewayTrafficStatus(ctx)
}
func (h *Handlers) CreateGatewayTrafficUsage(ctx *gin.Context) {
	h.billingAPI().CreateGatewayTrafficUsage(ctx)
}
func (h *Handlers) CreateGatewayTrafficProbeHello(ctx *gin.Context) {
	h.billingAPI().CreateGatewayTrafficProbeHello(ctx)
}
func (h *Handlers) ensureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	return h.billingAPI().EnsureBillingAllowsNewBuild(ctx, projectID)
}
func (h *Handlers) ensureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return h.billingAPI().EnsureBillingAllowsDeployChange(ctx, projectID)
}
func (h *Handlers) ensureBillingAllowsManagedVolumeChange(ctx *gin.Context, projectID string) bool {
	return h.billingAPI().EnsureBillingAllowsManagedVolumeChange(ctx, projectID)
}
