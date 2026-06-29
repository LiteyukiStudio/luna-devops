import type { ComponentType } from 'react'
import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { TooltipProvider } from './components/ui/tooltip'
import { AppLayout } from './layouts/AppLayout'

const AccountPage = lazyNamed(() => import('./pages/settings/AccountPage'), 'AccountPage')
const AppTemplatesPage = lazyNamed(() => import('./pages/app-templates/AppTemplatesPage'), 'AppTemplatesPage')
const ApplicationConfigPage = lazyNamed(() => import('./pages/applications/ApplicationConfigPage'), 'ApplicationConfigPage')
const AuthProvidersPage = lazyNamed(() => import('./pages/settings/AuthProvidersPage'), 'AuthProvidersPage')
const BillingPage = lazyNamed(() => import('./pages/billing/BillingPage'), 'BillingPage')
const BootstrapPage = lazyNamed(() => import('./pages/bootstrap/BootstrapPage'), 'BootstrapPage')
const ClustersPage = lazyNamed(() => import('./pages/clusters/ClustersPage'), 'ClustersPage')
const CodeRepositoriesPage = lazyNamed(() => import('./pages/code-repositories/CodeRepositoriesPage'), 'CodeRepositoriesPage')
const DashboardPage = lazyNamed(() => import('./pages/dashboard/DashboardPage'), 'DashboardPage')
const LoginPage = lazyNamed(() => import('./pages/login/LoginPage'), 'LoginPage')
const ProjectsPage = lazyNamed(() => import('./pages/projects/ProjectsPage'), 'ProjectsPage')
const ProjectWorkspacePage = lazyNamed(() => import('./pages/projects/ProjectWorkspacePage'), 'ProjectWorkspacePage')
const RegistriesPage = lazyNamed(() => import('./pages/registries/RegistriesPage'), 'RegistriesPage')
const SiteSettingsPage = lazyNamed(() => import('./pages/settings/SiteSettingsPage'), 'SiteSettingsPage')
const UsersPage = lazyNamed(() => import('./pages/settings/UsersPage'), 'UsersPage')

export default function App() {
  return (
    <TooltipProvider>
      <Suspense fallback={<RouteFallback />}>
        <Routes>
          <Route path="/bootstrap" element={<BootstrapPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route element={<AppLayout />}>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<DashboardPage />} />
            <Route path="/projects" element={<ProjectsPage />} />
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
            <Route path="/settings/site" element={<SiteSettingsPage />} />
            <Route path="/settings/users" element={<UsersPage />} />
          </Route>
        </Routes>
      </Suspense>
    </TooltipProvider>
  )
}

function lazyNamed<T extends Record<string, ComponentType<object>>, K extends keyof T>(
  loader: () => Promise<T>,
  exportName: K,
) {
  return lazy(async () => ({ default: (await loader())[exportName] }))
}

function RouteFallback() {
  return <div className="min-h-screen bg-background" />
}

function ProjectRootRedirect() {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}`} replace />
}

function ProjectAppsRedirect() {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}#tab=apps`} replace />
}
