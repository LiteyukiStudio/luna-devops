import type { DataExportAuthorization } from '@/api'
import { api, deploymentTargetDataExportUrl } from '@/api'

interface DataExportDependencies {
  authorize?: (projectId: string, applicationId: string, targetId: string) => Promise<DataExportAuthorization>
  startDownload?: (url: string) => void
}

export async function openDeploymentTargetDataExport(
  projectId: string,
  applicationId: string,
  targetId: string,
  dependencies: DataExportDependencies = {},
) {
  const authorization = await (dependencies.authorize ?? api.authorizeDeploymentTargetDataExport)(projectId, applicationId, targetId)
  const baseExportUrl = deploymentTargetDataExportUrl(projectId, applicationId, targetId)
  const exportUrl = new URL(baseExportUrl, window.location.origin)
  exportUrl.searchParams.set('ticket', authorization.ticket)
  const href = baseExportUrl.startsWith('http://') || baseExportUrl.startsWith('https://')
    ? exportUrl.toString()
    : `${exportUrl.pathname}${exportUrl.search}`
  ;(dependencies.startDownload ?? startBrowserDownload)(href)
}

function startBrowserDownload(url: string) {
  const link = document.createElement('a')
  link.href = url
  link.download = ''
  link.hidden = true
  document.body.append(link)
  link.click()
  link.remove()
}
