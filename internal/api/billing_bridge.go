package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type billingHost struct {
	domainHost
}

func (host billingHost) ConfigValues(keys []string) map[string]string {
	return host.handlers.configs.get(keys)
}

func (host billingHost) DefaultRuntimeClusterID(ctx context.Context) string {
	return host.handlers.domains.runtime.DefaultRuntimeClusterID(ctx)
}

func (host billingHost) ObserveSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation) {
	host.handlers.domains.runtime.ObserveSystemComponentInstallations(ctx, items)
}

func (host billingHost) SystemComponentForBearerToken(token, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool) {
	return host.handlers.domains.runtime.SystemComponentForBearerToken(token, componentID, ctx)
}

func (h *Handlers) ensureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	return h.domains.billing.EnsureBillingAllowsNewBuild(ctx, projectID)
}
func (h *Handlers) ensureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return h.domains.billing.EnsureBillingAllowsDeployChange(ctx, projectID)
}
func (h *Handlers) ensureBillingAllowsManagedVolumeChange(ctx *gin.Context, projectID string) bool {
	return h.domains.billing.EnsureBillingAllowsManagedVolumeChange(ctx, projectID)
}
