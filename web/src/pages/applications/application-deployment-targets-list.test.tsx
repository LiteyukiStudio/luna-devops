import type { DeploymentTargetRow } from './application-deployment-targets-list'
import type { DeploymentTarget, GatewayRoute, Release } from '@/api'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import '@/i18n'

const deploymentTarget = {
  applicationId: 'app_1',
  autoDeploy: false,
  availableReplicas: 1,
  clusterId: 'cluster_1',
  cpuRequest: '500m',
  dataVolumes: [],
  deleteStatus: 'active',
  desiredReplicas: 1,
  enabled: false,
  environmentId: 'env_1',
  id: 'target_1',
  imageRef: 'registry.example/app:v1',
  kubernetesName: 'app-dev',
  memoryRequest: '1Gi',
  name: 'app-dev',
  projectId: 'prj_1',
  readyReplicas: 1,
  replicas: 1,
  servicePort: 3000,
  servicePorts: [{ name: 'http', port: 3000 }],
  sourceType: 'image',
  stage: 'dev',
} as unknown as DeploymentTarget

const release = {
  applicationId: 'app_1',
  buildRunId: '',
  createdAt: '2026-08-17T00:00:00Z',
  createdBy: 'user_1',
  deploymentTargetId: 'target_1',
  environmentId: 'env_1',
  forceImagePull: false,
  id: 'release_1',
  imageRef: 'registry.example/app:v1',
  message: 'Deployment has minimum availability.',
  projectId: 'prj_1',
  revision: 3,
  rollbackFromId: '',
  status: 'succeeded',
  type: 'deploy',
} satisfies Release

function renderList(items: DeploymentTargetRow[] = []) {
  return render(
    <TooltipProvider>
      <ApplicationDeploymentTargetsList
        applicationId="app_1"
        createReleasePending={false}
        deletePending={false}
        deployableBuildRuns={[]}
        items={items}
        projectId="prj_1"
        pullLatestPending={false}
        restartPending={false}
        rollbackPending={false}
        onDeleteTarget={vi.fn()}
        onOpenConsole={vi.fn()}
        onOpenReleaseDialog={vi.fn()}
        onOpenTargetDialog={vi.fn()}
        onPullLatestImageDeploy={vi.fn()}
        onRestart={vi.fn()}
        onRollback={vi.fn()}
        onViewLogs={vi.fn()}
      />
    </TooltipProvider>,
  )
}

describe('application deployment targets behavior', () => {
  it('opens operational details from the actions menu', async () => {
    const user = userEvent.setup()
    renderList([{
      internalEndpoint: {
        fqdn: 'app-dev.luna-dev.svc.cluster.local',
        namespace: 'luna-dev',
        serviceName: 'app-dev',
      },
      release,
      routes: [{
        accessUrl: 'https://outline.example.com',
        deploymentTargetId: 'target_1',
        enabled: true,
        id: 'route_1',
        status: 'ready',
      } as unknown as GatewayRoute],
      runtimeStatus: { podCount: 1, summary: '', value: 'ready' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    expect(screen.queryByText(/自动部署|Auto deploy/i)).not.toBeInTheDocument()
    expect(screen.queryByText('Deployment has minimum availability.')).not.toBeInTheDocument()
    await user.click(screen.getAllByLabelText('Actions')[0])
    await user.click(await screen.findByRole('menuitem', { name: 'View details' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Access addresses')).toBeInTheDocument()
    expect(within(dialog).getByText('app-dev.luna-dev.svc.cluster.local')).toBeInTheDocument()
    expect(within(dialog).getByText('https://outline.example.com')).toBeInTheDocument()
    expect(within(dialog).getByText('0.5 vCPU · 1G')).toBeInTheDocument()
    expect(within(dialog).getByText('#3')).toBeInTheDocument()
  })

  it('keeps actionable rollout diagnostics in deployment details', async () => {
    const user = userEvent.setup()
    renderList([{
      release: { ...release, message: 'ImagePullBackOff', status: 'failed' },
      routes: [],
      runtimeStatus: { podCount: 1, summary: 'ImagePullBackOff', value: 'image-pull-back-off' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    await user.click(screen.getAllByLabelText('Actions')[0])
    await user.click(await screen.findByRole('menuitem', { name: 'View details' }))
    expect(within(await screen.findByRole('dialog')).getAllByText('ImagePullBackOff').length).toBeGreaterThan(0)
  })
})
