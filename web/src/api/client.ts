import { aiApi } from './domains/ai'
import { applicationsApi } from './domains/applications'
import { authApi } from './domains/auth'
import { buildsApi } from './domains/builds'
import { dashboardApi } from './domains/dashboard'
import { eventsApi } from './domains/events'
import { gatewayApi } from './domains/gateway'
import { gitApi } from './domains/git'
import { inboxApi } from './domains/inbox'
import { kubectlApi } from './domains/kubectl'
import { metaApi } from './domains/meta'
import { notificationsApi } from './domains/notifications'
import { oauthApi } from './domains/oauth'
import { projectsApi } from './domains/projects'
import { registriesApi } from './domains/registries'
import { runtimeApi } from './domains/runtime'
import { topologyApi } from './domains/topology'
import { volumesApi } from './domains/volumes'

type ApiClient = typeof aiApi
  & typeof applicationsApi
  & typeof authApi
  & typeof buildsApi
  & typeof dashboardApi
  & typeof eventsApi
  & typeof gatewayApi
  & typeof gitApi
  & typeof inboxApi
  & typeof kubectlApi
  & typeof metaApi
  & typeof notificationsApi
  & typeof oauthApi
  & typeof projectsApi
  & typeof registriesApi
  & typeof runtimeApi
  & typeof topologyApi
  & typeof volumesApi

export const api = {
  ...aiApi,
  ...applicationsApi,
  ...authApi,
  ...buildsApi,
  ...dashboardApi,
  ...eventsApi,
  ...gatewayApi,
  ...gitApi,
  ...inboxApi,
  ...kubectlApi,
  ...metaApi,
  ...notificationsApi,
  ...oauthApi,
  ...projectsApi,
  ...registriesApi,
  ...runtimeApi,
  ...topologyApi,
  ...volumesApi,
} satisfies ApiClient
