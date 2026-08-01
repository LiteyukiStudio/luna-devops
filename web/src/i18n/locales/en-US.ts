import accessTokens from './en-US/accessTokens'
import accountPage from './en-US/accountPage'
import aiAssistant from './en-US/aiAssistant'
import apps from './en-US/apps'
import appTemplatesPage from './en-US/appTemplatesPage'
import auth from './en-US/auth'
import authProvidersPage from './en-US/authProvidersPage'
import billingPage from './en-US/billingPage'
import bootstrap from './en-US/bootstrap'
import buildsPage from './en-US/buildsPage'
import buildTemplates from './en-US/buildTemplates'
import clustersPage from './en-US/clustersPage'
import codeRepositoriesPage from './en-US/codeRepositoriesPage'
import codeRepositoriesView from './en-US/codeRepositoriesView'
import common from './en-US/common'
import dashboardPage from './en-US/dashboardPage'
import debugPanel from './en-US/debugPanel'
import deploymentsPage from './en-US/deploymentsPage'
import errors from './en-US/errors'
import eventsPage from './en-US/eventsPage'
import gatewayRoutesPage from './en-US/gatewayRoutesPage'
import inbox from './en-US/inbox'
import languages from './en-US/languages'
import loginPage from './en-US/loginPage'
import nav from './en-US/nav'
import notificationsPage from './en-US/notificationsPage'
import oauthApps from './en-US/oauthApps'
import operationsDashboardPage from './en-US/operationsDashboardPage'
import pagination from './en-US/pagination'
import projectHooks from './en-US/projectHooks'
import projectMembers from './en-US/projectMembers'
import projectSpaces from './en-US/projectSpaces'
import projectTopology from './en-US/projectTopology'
import registriesPage from './en-US/registriesPage'
import repositories from './en-US/repositories'
import root from './en-US/root'
import runtimeConfigFilesEditor from './en-US/runtimeConfigFilesEditor'
import runtimeConfigSets from './en-US/runtimeConfigSets'
import settings from './en-US/settings'
import theme from './en-US/theme'
import time from './en-US/time'
import usersPage from './en-US/usersPage'

const enUS = {
  ...root,
  ...aiAssistant,
  languages,
  theme,
  common,
  time,
  errors,
  ...inbox,
  eventsPage,
  pagination,
  operationsDashboardPage,
  oauthApps,
  debugPanel,
  apps,
  repositories,
  codeRepositoriesPage,
  codeRepositoriesView,
  accessTokens,
  settings,
  accountPage,
  appTemplatesPage,
  buildsPage,
  buildTemplates,
  runtimeConfigSets,
  runtimeConfigFilesEditor,
  deploymentsPage,
  clustersPage,
  gatewayRoutesPage,
  projectMembers,
  projectSpaces,
  projectTopology,
  dashboardPage,
  auth,
  nav,
  notificationsPage,
  loginPage,
  bootstrap,
  usersPage,
  registriesPage,
  projectHooks,
  authProvidersPage,
  billingPage,
}

export default enUS
