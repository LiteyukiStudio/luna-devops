import type { ComponentType } from 'react'
import { lazy } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { LazyLoadBoundary } from './components/common/lazy-load-boundary'
import { TooltipProvider } from './components/ui/tooltip'
import { loadTranslationBundles } from './i18n'

const AppLayout = lazyTranslated(() => import('./layouts/AppLayout'), 'AppLayout', ['nav', 'inbox', 'accountPage', 'aiAssistant', 'projectSpaces', 'apps'])
const AccountPage = lazyTranslated(() => import('./pages/settings/account/AccountPage'), 'AccountPage', ['accountPage', 'accessTokens', 'settings', 'usersPage'])
const AppTemplatesPage = lazyTranslated(() => import('./pages/app-templates/AppTemplatesPage'), 'AppTemplatesPage', ['appTemplatesPage', 'deploymentsPage', 'projectSpaces', 'projectVolumes'])
const ApplicationConfigPage = lazyTranslated(() => import('./pages/applications/ApplicationConfigPage'), 'ApplicationConfigPage', ['apps', 'buildTemplates', 'buildsPage', 'deploymentsPage', 'gatewayRoutesPage', 'repositories', 'runtimeConfigSets', 'runtimeConfigFilesEditor', 'clustersPage', 'billingPage', 'projectHooks', 'codeRepositoriesView', 'projectVolumes'])
const AuthProvidersPage = lazyTranslated(() => import('./pages/settings/identity/AuthProvidersPage'), 'AuthProvidersPage', ['authProvidersPage', 'settings', 'usersPage'])
const BillingPage = lazyTranslated(() => import('./pages/billing/BillingPage'), 'BillingPage', ['billingPage', 'projectSpaces', 'settings'])
const ClustersPage = lazyTranslated(() => import('./pages/clusters/ClustersPage'), 'ClustersPage', ['clustersPage', 'codeRepositoriesView', 'deploymentsPage', 'gatewayRoutesPage', 'projectSpaces'])
const CodeRepositoriesPage = lazyTranslated(() => import('./pages/code-repositories/CodeRepositoriesPage'), 'CodeRepositoriesPage', ['codeRepositoriesPage', 'codeRepositoriesView', 'repositories', 'projectSpaces'])
const DashboardPage = lazyTranslated(() => import('./pages/dashboard/DashboardPage'), 'DashboardPage', ['dashboardPage', 'registriesPage', 'clustersPage', 'eventsPage', 'projectSpaces'])
const EventsPage = lazyTranslated(() => import('./pages/events/EventsPage'), 'EventsPage', ['eventsPage'])
const LoginPage = lazyNamed(() => import('./pages/login/LoginPage'), 'LoginPage')
const InboxPage = lazyTranslated(() => import('./pages/inbox/InboxPage'), 'InboxPage', ['inbox', 'eventsPage'])
const RegisterPage = lazyTranslated(() => import('./pages/login/RegisterPage'), 'RegisterPage', ['usersPage'])
const NotificationsPage = lazyTranslated(() => import('./pages/settings/notifications/NotificationsPage'), 'NotificationsPage', ['notificationsPage', 'settings'])
const OAuthAuthorizePage = lazyNamed(() => import('./pages/oauth/OAuthAuthorizePage'), 'OAuthAuthorizePage')
const OAuthDevicePage = lazyNamed(() => import('./pages/oauth/OAuthDevicePage'), 'OAuthDevicePage')
const OperationsDashboardPage = lazyTranslated(() => import('./pages/settings/operations/OperationsDashboardPage'), 'OperationsDashboardPage', ['operationsDashboardPage', 'settings'])
const ProjectsPage = lazyTranslated(() => import('./pages/projects/overview/ProjectsPage'), 'ProjectsPage', ['projectSpaces'])
const ProjectWorkspacePage = lazyTranslated(() => import('./pages/projects/overview/ProjectWorkspacePage'), 'ProjectWorkspacePage', ['projectSpaces', 'apps', 'buildsPage', 'runtimeConfigSets', 'projectHooks', 'projectMembers', 'projectVolumes', 'projectTopology', 'deploymentsPage', 'gatewayRoutesPage', 'billingPage', 'eventsPage', 'runtimeConfigFilesEditor'])
const RegistriesPage = lazyTranslated(() => import('./pages/registries/RegistriesPage'), 'RegistriesPage', ['registriesPage', 'projectSpaces'])
const SiteSettingsPage = lazyTranslated(() => import('./pages/settings/site/SiteSettingsPage'), 'SiteSettingsPage', ['settings', 'buildsPage'])
const UsersPage = lazyTranslated(() => import('./pages/settings/identity/UsersPage'), 'UsersPage', ['usersPage', 'settings'])
const AIInteractionCardGallery = import.meta.env.DEV
  ? lazyNamed(() => import('./dev/AIInteractionCardGallery'), 'AIInteractionCardGallery')
  : null

export default function App() {
  return (
    <TooltipProvider>
      <LazyLoadBoundary fallback={<RouteFallback />}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/oauth/authorize" element={<OAuthAuthorizePage />} />
          <Route path="/oauth/device" element={<OAuthDevicePage />} />
          {AIInteractionCardGallery && <Route path="/__dev/ai-interaction-cards" element={<AIInteractionCardGallery />} />}
          <Route element={<AppLayout />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="/ai-assistant" element={<></>} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="/events" element={<EventsPage />} />
            <Route path="/inbox" element={<InboxPage />} />
            <Route path="/projects/:projectId" element={<ProjectWorkspacePage />} />
            <Route path="/projects/:projectId/apps/:applicationId" element={<ApplicationConfigPage />} />
            <Route path="/app-templates" element={<AppTemplatesPage />} />
            <Route path="/code-repositories" element={<CodeRepositoriesPage />} />
            <Route path="/registries" element={<RegistriesPage />} />
            <Route path="/clusters" element={<ClustersPage />} />
            <Route path="/billing" element={<BillingPage />} />
            <Route path="/settings/account" element={<AccountPage />} />
            <Route path="/settings/auth-providers" element={<AuthProvidersPage />} />
            <Route path="/settings/notifications" element={<NotificationsPage />} />
            <Route path="/settings/operations" element={<OperationsDashboardPage />} />
            <Route path="/settings/site" element={<SiteSettingsPage />} />
            <Route path="/settings/users" element={<UsersPage />} />
          </Route>
        </Routes>
      </LazyLoadBoundary>
    </TooltipProvider>
  )
}

function lazyNamed<T extends Record<string, ComponentType<object>>, K extends keyof T>(
  loader: () => Promise<T>,
  exportName: K,
) {
  return lazy(async () => ({ default: (await loader())[exportName] }))
}

function lazyTranslated<T extends Record<string, ComponentType<object>>, K extends keyof T>(
  loader: () => Promise<T>,
  exportName: K,
  bundleNames: readonly string[],
) {
  return lazy(async () => {
    await loadTranslationBundles(bundleNames)
    return { default: (await loader())[exportName] }
  })
}

function RouteFallback() {
  return <div className="min-h-screen bg-primary-subtle" />
}
