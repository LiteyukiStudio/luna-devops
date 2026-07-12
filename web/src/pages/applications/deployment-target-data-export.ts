import { api, deploymentTargetDataExportUrl } from '@/api'

interface ExportWindow {
  close: () => void
  location: Pick<Location, 'replace'>
  opener: unknown
}

interface DataExportDependencies {
  authorize?: (projectId: string, applicationId: string, targetId: string) => Promise<void>
  openWindow?: () => ExportWindow | null
}

export async function openDeploymentTargetDataExport(
  projectId: string,
  applicationId: string,
  targetId: string,
  dependencies: DataExportDependencies = {},
) {
  const exportWindow = (dependencies.openWindow ?? (() => window.open('about:blank', '_blank')))()
  if (!exportWindow)
    throw new Error('data_export_window_blocked')

  exportWindow.opener = null
  try {
    await (dependencies.authorize ?? api.authorizeDeploymentTargetDataExport)(projectId, applicationId, targetId)
    exportWindow.location.replace(deploymentTargetDataExportUrl(projectId, applicationId, targetId))
  }
  catch (error) {
    exportWindow.close()
    throw error
  }
}
