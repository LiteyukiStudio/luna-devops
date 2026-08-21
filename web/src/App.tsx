import type { ComponentType } from 'react'
import { lazy } from 'react'
import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { LazyLoadBoundary } from './components/common/lazy-load-boundary'
import { TooltipProvider } from './components/ui/tooltip'
import { loadTranslationBundles } from './i18n'

const AppLayout = lazyTranslated(() => import('./layouts/AppLayout'), 'AppLayout', ['nav', 'inbox', 'accountPage', 'aiAssistant', 'debugPanel'])
const AccountPage = lazyTranslated(() => import('./pages/settings/AccountPage'), 'AccountPage', ['accountPage', 'accessTokens', 'settings'])
const AppTemplatesPage = lazyTranslated(() => import('./pages/app-templates/AppTemplatesPage'), 'AppTemplatesPage', ['appTemplatesPage'])
const ApplicationConfigPage = lazyTranslated(() => import('./pages/applications/ApplicationConfigPage'), 'ApplicationConfigPage', ['apps', 'buildsPage', 'deploymentsPage', 'gatewayRoutesPage', 'repositories', 'runtimeConfigSets', 'runtimeConfigFilesEditor', 'clustersPage', 'billingPage', 'projectHooks'])
const AuthProvidersPage = lazyTranslated(() => import('./pages/settings/AuthProvidersPage'), 'AuthProvidersPage', ['authProvidersPage', 'settings'])
const BillingPage = lazyTranslated(() => import('./pages/billing/BillingPage'), 'BillingPage', ['billingPage'])
const BootstrapPage = lazyNamed(() => import('./pages/bootstrap/BootstrapPage'), 'BootstrapPage')
const ClustersPage = lazyTranslated(() => import('./pages/clusters/ClustersPage'), 'ClustersPage', ['clustersPage'])
const CodeRepositoriesPage = lazyTranslated(() => import('./pages/code-repositories/CodeRepositoriesPage'), 'CodeRepositoriesPage', ['codeRepositoriesPage', 'codeRepositoriesView', 'repositories'])
const DashboardPage = lazyTranslated(() => import('./pages/dashboard/DashboardPage'), 'DashboardPage', ['dashboardPage', 'registriesPage', 'clustersPage'])
const EventsPage = lazyTranslated(() => import('./pages/events/EventsPage'), 'EventsPage', ['eventsPage'])
const LoginPage = lazyNamed(() => import('./pages/login/LoginPage'), 'LoginPage')
const InboxPage = lazyTranslated(() => import('./pages/inbox/InboxPage'), 'InboxPage', ['inbox'])
const RegisterPage = lazyNamed(() => import('./pages/login/RegisterPage'), 'RegisterPage')
const NotificationsPage = lazyTranslated(() => import('./pages/settings/NotificationsPage'), 'NotificationsPage', ['notificationsPage', 'settings'])
const OAuthAuthorizePage = lazyNamed(() => import('./pages/oauth/OAuthAuthorizePage'), 'OAuthAuthorizePage')
const OAuthDevicePage = lazyNamed(() => import('./pages/oauth/OAuthDevicePage'), 'OAuthDevicePage')
const OperationsDashboardPage = lazyTranslated(() => import('./pages/settings/OperationsDashboardPage'), 'OperationsDashboardPage', ['operationsDashboardPage', 'settings'])
const ProjectsPage = lazyTranslated(() => import('./pages/projects/ProjectsPage'), 'ProjectsPage', ['projectSpaces'])
const ProjectWorkspacePage = lazyTranslated(() => import('./pages/projects/ProjectWorkspacePage'), 'ProjectWorkspacePage', ['projectSpaces', 'apps', 'buildsPage', 'runtimeConfigSets', 'projectHooks', 'projectMembers', 'projectVolumes', 'projectTopology', 'deploymentsPage', 'gatewayRoutesPage', 'billingPage'])
const RegistriesPage = lazyTranslated(() => import('./pages/registries/RegistriesPage'), 'RegistriesPage', ['registriesPage'])
const SiteSettingsPage = lazyTranslated(() => import('./pages/settings/SiteSettingsPage'), 'SiteSettingsPage', ['settings'])
const UsersPage = lazyTranslated(() => import('./pages/settings/UsersPage'), 'UsersPage', ['usersPage', 'settings'])
const AIInteractionCardGallery = import.meta.env.DEV
  ? lazyNamed(() => import('./dev/AIInteractionCardGallery'), 'AIInteractionCardGallery')
  : null

export default function App() {
  return (
    <TooltipProvider>
      <LazyLoadBoundary fallback={<RouteFallback />}>
        <Routes>
          <Route path="/bootstrap" element={<BootstrapPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/oauth/authorize" element={<OAuthAuthorizePage />} />
          <Route path="/oauth/device" element={<OAuthDevicePage />} />
          {AIInteractionCardGallery && <Route path="/__dev/ai-interaction-cards" element={<AIInteractionCardGallery />} />}
          <Route element={<AppLayout />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="/events" element={<EventsPage />} />
            <Route path="/inbox" element={<InboxPage />} />
            <Route path="/projects/:projectId" element={<ProjectWorkspacePage />} />
            <Route path="/projects/:projectId/members" element={<ProjectRootRedirect />} />
            <Route path="/projects/:projectId/apps" element={<ProjectRootRedirect />} />
            <Route path="/projects/:projectId/repositories" element={<ProjectAppsRedirect />} />
            <Route path="/projects/:projectId/apps/:applicationId" element={<ApplicationConfigPage />} />
            <Route path="/app-templates" element={<AppTemplatesPage />} />
            <Route path="/code-repositories" element={<CodeRepositoriesPage />} />
            <Route path="/registries" element={<RegistriesPage />} />
            <Route path="/clusters" element={<ClustersPage />} />
            <Route path="/billing" element={<BillingPage />} />
            <Route path="/access-tokens" element={<Navigate to="/settings/account" replace />} />
            <Route path="/settings/security" element={<Navigate to="/settings/account" replace />} />
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

function ProjectRootRedirect() {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}`} replace />
}

function ProjectAppsRedirect() {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}#tab=apps`} replace />
}
