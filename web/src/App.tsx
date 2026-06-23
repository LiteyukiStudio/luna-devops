import { Navigate, Route, Routes, useParams } from 'react-router-dom'
import { TooltipProvider } from './components/ui/tooltip'
import { AppLayout } from './layouts/AppLayout'
import { AppTemplatesPage } from './pages/app-templates/AppTemplatesPage'
import { ApplicationConfigPage } from './pages/applications/ApplicationConfigPage'
import { BillingPage } from './pages/billing/BillingPage'
import { BootstrapPage } from './pages/bootstrap/BootstrapPage'
import { ClustersPage } from './pages/clusters/ClustersPage'
import { CodeRepositoriesPage } from './pages/code-repositories/CodeRepositoriesPage'
import { DashboardPage } from './pages/dashboard/DashboardPage'
import { LoginPage } from './pages/login/LoginPage'
import { ProjectsPage } from './pages/projects/ProjectsPage'
import { ProjectWorkspacePage } from './pages/projects/ProjectWorkspacePage'
import { RegistriesPage } from './pages/registries/RegistriesPage'
import { AccountPage } from './pages/settings/AccountPage'
import { AuthProvidersPage } from './pages/settings/AuthProvidersPage'
import { SiteSettingsPage } from './pages/settings/SiteSettingsPage'
import { UsersPage } from './pages/settings/UsersPage'

export default function App() {
  return (
    <TooltipProvider>
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
    </TooltipProvider>
  )
}

function ProjectRootRedirect() {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}`} replace />
}

function ProjectAppsRedirect() {
  const { projectId = '' } = useParams()
  return <Navigate to={`/projects/${projectId}#tab=apps`} replace />
}
