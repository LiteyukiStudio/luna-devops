package api

import (
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
	return &domainHandlers{
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
}
