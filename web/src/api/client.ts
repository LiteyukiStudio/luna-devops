import { applicationsApi } from './domains/applications'
import { authApi } from './domains/auth'
import { buildsApi } from './domains/builds'
import { gatewayApi } from './domains/gateway'
import { gitApi } from './domains/git'
import { notificationsApi } from './domains/notifications'
import { projectsApi } from './domains/projects'
import { registriesApi } from './domains/registries'
import { runtimeApi } from './domains/runtime'

export { ApiError } from './core'
export type * from './types'
export {
  apiBaseOrigin,
  buildJobLogsStreamUrl,
  deploymentTargetDataExportUrl,
  deploymentTargetMetricsStreamUrl,
  gitOAuthStartUrl,
  oidcStartUrl,
  releaseRuntimeTerminalUrl,
  runtimeClusterPodTerminalUrl,
} from './urls'

export const api = {
  ...authApi,
  ...gitApi,
  ...projectsApi,
  ...applicationsApi,
  ...registriesApi,
  ...buildsApi,
  ...runtimeApi,
  ...gatewayApi,
  ...notificationsApi,
}
