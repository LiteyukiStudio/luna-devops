import { aiApi } from './domains/ai'
import { applicationsApi } from './domains/applications'
import { authApi } from './domains/auth'
import { buildsApi } from './domains/builds'
import { dashboardApi } from './domains/dashboard'
import { eventsApi } from './domains/events'
import { gatewayApi } from './domains/gateway'
import { gitApi } from './domains/git'
import { inboxApi } from './domains/inbox'
import { notificationsApi } from './domains/notifications'
import { oauthApi } from './domains/oauth'
import { projectsApi } from './domains/projects'
import { registriesApi } from './domains/registries'
import { runtimeApi } from './domains/runtime'
import { topologyApi } from './domains/topology'

export { ApiError } from './core'
export type * from './topology-types'
export type * from './types'
export {
  apiBaseOrigin,
  buildJobLogsStreamUrl,
  deploymentTargetDataExportUrl,
  deploymentTargetMetricsStreamUrl,
  gitOAuthStartUrl,
  inboxStreamUrl,
  oidcStartUrl,
  releaseRuntimeTerminalUrl,
  runtimeClusterPodTerminalUrl,
} from './urls'

export const api = {
  ...aiApi,
  ...authApi,
  ...gitApi,
  ...inboxApi,
  ...projectsApi,
  ...applicationsApi,
  ...registriesApi,
  ...buildsApi,
  ...dashboardApi,
  ...eventsApi,
  ...runtimeApi,
  ...gatewayApi,
  ...notificationsApi,
  ...oauthApi,
  ...topologyApi,
}
