import accessTokens from './ja-JP/accessTokens'
import accountPage from './ja-JP/accountPage'
import aiAssistant from './ja-JP/aiAssistant'
import apps from './ja-JP/apps'
import appTemplatesPage from './ja-JP/appTemplatesPage'
import auth from './ja-JP/auth'
import authProvidersPage from './ja-JP/authProvidersPage'
import billingPage from './ja-JP/billingPage'
import bootstrap from './ja-JP/bootstrap'
import buildsPage from './ja-JP/buildsPage'
import buildTemplates from './ja-JP/buildTemplates'
import clustersPage from './ja-JP/clustersPage'
import codeRepositoriesPage from './ja-JP/codeRepositoriesPage'
import codeRepositoriesView from './ja-JP/codeRepositoriesView'
import common from './ja-JP/common'
import dashboardPage from './ja-JP/dashboardPage'
import debugPanel from './ja-JP/debugPanel'
import deploymentsPage from './ja-JP/deploymentsPage'
import errors from './ja-JP/errors'
import eventsPage from './ja-JP/eventsPage'
import gatewayRoutesPage from './ja-JP/gatewayRoutesPage'
import inbox from './ja-JP/inbox'
import languages from './ja-JP/languages'
import loginPage from './ja-JP/loginPage'
import nav from './ja-JP/nav'
import notificationsPage from './ja-JP/notificationsPage'
import oauthApps from './ja-JP/oauthApps'
import operationsDashboardPage from './ja-JP/operationsDashboardPage'
import pagination from './ja-JP/pagination'
import projectHooks from './ja-JP/projectHooks'
import projectMembers from './ja-JP/projectMembers'
import projectSpaces from './ja-JP/projectSpaces'
import projectTopology from './ja-JP/projectTopology'
import projectVolumes from './ja-JP/projectVolumes'
import registriesPage from './ja-JP/registriesPage'
import repositories from './ja-JP/repositories'
import root from './ja-JP/root'
import runtimeConfigFilesEditor from './ja-JP/runtimeConfigFilesEditor'
import runtimeConfigSets from './ja-JP/runtimeConfigSets'
import settings from './ja-JP/settings'
import theme from './ja-JP/theme'
import time from './ja-JP/time'
import usersPage from './ja-JP/usersPage'

const jaJP = {
  ...root,
  ...aiAssistant,
  languages,
  common,
  time,
  errors,
  ...inbox,
  eventsPage,
  auth,
  pagination,
  operationsDashboardPage,
  oauthApps,
  theme,
  nav,
  notificationsPage,
  debugPanel,
  loginPage,
  projectSpaces,
  projectTopology,
  projectVolumes,
  dashboardPage,
  buildsPage,
  buildTemplates,
  runtimeConfigSets,
  runtimeConfigFilesEditor,
  deploymentsPage,
  clustersPage,
  gatewayRoutesPage,
  projectMembers,
  apps,
  accessTokens,
  bootstrap,
  repositories,
  codeRepositoriesPage,
  codeRepositoriesView,
  registriesPage,
  settings,
  accountPage,
  appTemplatesPage,
  usersPage,
  projectHooks,
  authProvidersPage,
  billingPage,
}

export default jaJP
