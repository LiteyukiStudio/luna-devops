package api

import (
	"fmt"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/api/billingapi"
	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/deploymentapi"
	"github.com/LiteyukiStudio/devops/internal/api/gatewayapi"
	"github.com/LiteyukiStudio/devops/internal/api/gitapi"
	"github.com/LiteyukiStudio/devops/internal/api/identityapi"
	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	"github.com/LiteyukiStudio/devops/internal/api/platformapi"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/api/registryapi"
	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/api/volumeapi"
)

type domainHandlers struct {
	ai           *aiapi.Handler
	application  *applicationapi.Handler
	billing      *billingapi.Handler
	build        *buildapi.Handler
	deployment   *deploymentapi.Handler
	gateway      *gatewayapi.Handler
	git          *gitapi.Handler
	identity     *identityapi.Handler
	notification *notificationapi.Handler
	platform     *platformapi.Handler
	project      *projectapi.Handler
	registry     *registryapi.Handler
	runtime      *runtimeapi.Handler
	volume       *volumeapi.Handler
}

func newDomainHandlers(handlers *Handlers) *domainHandlers {
	host := domainHost{handlers: handlers}
	domains := &domainHandlers{
		ai:           aiapi.New(aiHost{domainHost: host}),
		application:  applicationapi.New(applicationHost{domainHost: host}),
		billing:      billingapi.New(billingHost{domainHost: host}),
		build:        buildapi.New(buildHost{domainHost: host}),
		deployment:   deploymentapi.New(deploymentHost{domainHost: host}),
		gateway:      gatewayapi.New(gatewayHost{domainHost: host}),
		git:          gitapi.New(gitHost{domainHost: host}),
		identity:     identityapi.New(identityHost{domainHost: host}),
		notification: notificationapi.New(notificationHost{domainHost: host}),
		platform:     platformapi.New(platformHost{domainHost: host}),
		project:      projectapi.New(projectHost{domainHost: host}),
		registry:     registryapi.New(registryHost{domainHost: host}),
		runtime:      runtimeapi.New(runtimeHost{domainHost: host}),
		volume: volumeapi.New(volumeHost{domainHost: host}, volumeapi.Dependencies{
			Volumes:          handlers.volumes,
			Clusters:         handlers.volumeClusters,
			Content:          handlers.volumeContent,
			TransferMaxBytes: handlers.volumeTransferMaxBytes,
			TransferEnabled:  handlers.volumeTransferEnabled,
		}),
	}
	if handlers != nil && handlers.aiToolRegistry != nil {
		registerAllDomainAITools(handlers, domains, handlers.aiToolRegistry)
	}
	return domains
}

func registerAllDomainAITools(handlers *Handlers, domains *domainHandlers, reg *aitool.Registry) {
	domains.ai.RegisterAITools(reg)
	domains.application.RegisterAITools(reg)
	domains.billing.RegisterAITools(reg)
	domains.build.RegisterAITools(reg)
	domains.deployment.RegisterAITools(reg)
	domains.gateway.RegisterAITools(reg)
	domains.git.RegisterAITools(reg)
	domains.identity.RegisterAITools(reg)
	domains.notification.RegisterAITools(reg)
	domains.platform.RegisterAITools(reg)
	domains.project.RegisterAITools(reg)
	domains.registry.RegisterAITools(reg)
	domains.runtime.RegisterAITools(reg)
	domains.volume.RegisterAITools(reg)
	handlers.registerConfigAITools(reg)
	if err := reg.ValidateCrossReferences(); err != nil {
		panic(fmt.Sprintf("invalid AI domain tool registry: %v", err))
	}
}

// DefaultAIToolCatalog returns the deterministic 166-tool AI Domain Facade Catalog
// without requiring live database or Redis connections.
func DefaultAIToolCatalog() []aitool.CatalogOperation {
	reg := aitool.NewRegistry()
	h := &Handlers{aiToolRegistry: reg}
	_ = newDomainHandlers(h)
	return reg.Catalog()
}
