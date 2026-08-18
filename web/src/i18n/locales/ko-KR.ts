import accessTokens from './ko-KR/accessTokens'
import accountPage from './ko-KR/accountPage'
import aiAssistant from './ko-KR/aiAssistant'
import apps from './ko-KR/apps'
import appTemplatesPage from './ko-KR/appTemplatesPage'
import auth from './ko-KR/auth'
import authProvidersPage from './ko-KR/authProvidersPage'
import billingPage from './ko-KR/billingPage'
import bootstrap from './ko-KR/bootstrap'
import buildsPage from './ko-KR/buildsPage'
import buildTemplates from './ko-KR/buildTemplates'
import clustersPage from './ko-KR/clustersPage'
import codeRepositoriesPage from './ko-KR/codeRepositoriesPage'
import codeRepositoriesView from './ko-KR/codeRepositoriesView'
import common from './ko-KR/common'
import dashboardPage from './ko-KR/dashboardPage'
import debugPanel from './ko-KR/debugPanel'
import deploymentsPage from './ko-KR/deploymentsPage'
import errors from './ko-KR/errors'
import eventsPage from './ko-KR/eventsPage'
import gatewayRoutesPage from './ko-KR/gatewayRoutesPage'
import inbox from './ko-KR/inbox'
import languages from './ko-KR/languages'
import loginPage from './ko-KR/loginPage'
import nav from './ko-KR/nav'
import notificationsPage from './ko-KR/notificationsPage'
import oauthApps from './ko-KR/oauthApps'
import operationsDashboardPage from './ko-KR/operationsDashboardPage'
import pagination from './ko-KR/pagination'
import projectHooks from './ko-KR/projectHooks'
import projectMembers from './ko-KR/projectMembers'
import projectSpaces from './ko-KR/projectSpaces'
import projectTopology from './ko-KR/projectTopology'
import projectVolumes from './ko-KR/projectVolumes'
import registriesPage from './ko-KR/registriesPage'
import repositories from './ko-KR/repositories'
import root from './ko-KR/root'
import runtimeConfigFilesEditor from './ko-KR/runtimeConfigFilesEditor'
import runtimeConfigSets from './ko-KR/runtimeConfigSets'
import settings from './ko-KR/settings'
import theme from './ko-KR/theme'
import time from './ko-KR/time'
import usersPage from './ko-KR/usersPage'

const koKR = {
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

export default koKR
