import { Navigate, Route, Routes } from 'react-router-dom'
import { AppLayout } from './layouts/AppLayout'
import { ApplicationConfigPage } from './pages/applications/ApplicationConfigPage'
import { ApplicationsPage } from './pages/applications/ApplicationsPage'
import { BootstrapPage } from './pages/bootstrap/BootstrapPage'
import { LoginPage } from './pages/login/LoginPage'
import { ProjectMembersPage } from './pages/projects/ProjectMembersPage'
import { ProjectsPage } from './pages/projects/ProjectsPage'
import { AuthProvidersPage } from './pages/settings/AuthProvidersPage'
import { SecurityPage } from './pages/settings/SecurityPage'
import { SiteSettingsPage } from './pages/settings/SiteSettingsPage'
import { UsersPage } from './pages/settings/UsersPage'

export default function App() {
  return (
    <Routes>
      <Route path="/bootstrap" element={<BootstrapPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route element={<AppLayout />}>
        <Route index element={<Navigate to="/projects" replace />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/projects/:projectId/members" element={<ProjectMembersPage />} />
        <Route path="/projects/:projectId/apps" element={<ApplicationsPage />} />
        <Route path="/projects/:projectId/apps/:applicationId" element={<ApplicationConfigPage />} />
        <Route path="/access-tokens" element={<Navigate to="/settings/security" replace />} />
        <Route path="/settings/security" element={<SecurityPage />} />
        <Route path="/settings/auth-providers" element={<AuthProvidersPage />} />
        <Route path="/settings/site" element={<SiteSettingsPage />} />
        <Route path="/settings/users" element={<UsersPage />} />
      </Route>
    </Routes>
  )
}
