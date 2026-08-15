import accessTokens from './zh-CN/accessTokens'
import accountPage from './zh-CN/accountPage'
import aiAssistant from './zh-CN/aiAssistant'
import apps from './zh-CN/apps'
import appTemplatesPage from './zh-CN/appTemplatesPage'
import auth from './zh-CN/auth'
import authProvidersPage from './zh-CN/authProvidersPage'
import billingPage from './zh-CN/billingPage'
import bootstrap from './zh-CN/bootstrap'
import buildsPage from './zh-CN/buildsPage'
import buildTemplates from './zh-CN/buildTemplates'
import clustersPage from './zh-CN/clustersPage'
import codeRepositoriesPage from './zh-CN/codeRepositoriesPage'
import codeRepositoriesView from './zh-CN/codeRepositoriesView'
import common from './zh-CN/common'
import dashboardPage from './zh-CN/dashboardPage'
import debugPanel from './zh-CN/debugPanel'
import deploymentsPage from './zh-CN/deploymentsPage'
import errors from './zh-CN/errors'
import eventsPage from './zh-CN/eventsPage'
import gatewayRoutesPage from './zh-CN/gatewayRoutesPage'
import inbox from './zh-CN/inbox'
import languages from './zh-CN/languages'
import loginPage from './zh-CN/loginPage'
import nav from './zh-CN/nav'
import notificationsPage from './zh-CN/notificationsPage'
import oauthApps from './zh-CN/oauthApps'
import operationsDashboardPage from './zh-CN/operationsDashboardPage'
import pagination from './zh-CN/pagination'
import projectHooks from './zh-CN/projectHooks'
import projectMembers from './zh-CN/projectMembers'
import projectSpaces from './zh-CN/projectSpaces'
import projectTopology from './zh-CN/projectTopology'
import projectVolumes from './zh-CN/projectVolumes'
import registriesPage from './zh-CN/registriesPage'
import repositories from './zh-CN/repositories'
import root from './zh-CN/root'
import runtimeConfigFilesEditor from './zh-CN/runtimeConfigFilesEditor'
import runtimeConfigSets from './zh-CN/runtimeConfigSets'
import settings from './zh-CN/settings'
import theme from './zh-CN/theme'
import time from './zh-CN/time'
import usersPage from './zh-CN/usersPage'

const zhCN = {
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

export default zhCN
