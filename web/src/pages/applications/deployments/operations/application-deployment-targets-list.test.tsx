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
  id: 'target_1',
  imageRef: 'registry.example/app:v1',
  kubernetesName: 'app-dev',
  memoryRequest: '1Gi',
  name: 'app-dev',
  projectId: 'prj_1',
  readyReplicas: 1,
  replicas: 1,
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
  forceImagePull: false,
  id: 'release_1',
  imageRef: 'registry.example/app:v1',
  message: 'Deployment has minimum availability.',
  projectId: 'prj_1',
  revision: 3,
  rollbackFromId: '',
  status: 'succeeded',
  failureCode: '',
  type: 'deploy',
} satisfies Release

function renderList(items: DeploymentTargetRow[] = [], handlers: { onCancelRelease?: (release: Release) => void, onViewLogs?: (release: Release) => void } = {}) {
  return render(
    <TooltipProvider>
      <ApplicationDeploymentTargetsList
        applicationId="app_1"
        cancelReleasePending={false}
        createReleasePending={false}
        deletePending={false}
        deployableBuildRuns={[]}
        items={items}
        listState={{ loading: false }}
        operationPending={{}}
        projectId="prj_1"
        onCreateTarget={vi.fn()}
        onCancelRelease={handlers.onCancelRelease ?? vi.fn()}
        onDeleteTarget={vi.fn()}
        onOpenConsole={vi.fn()}
        onOpenReleaseDialog={vi.fn()}
        onEditTarget={vi.fn()}
        onPullLatestImageDeploy={vi.fn()}
        onRestart={vi.fn()}
        onRollback={vi.fn()}
        onViewLogs={handlers.onViewLogs ?? vi.fn()}
      />
    </TooltipProvider>,
  )
}

describe('application deployment targets behavior', () => {
  it('opens operational details from the actions menu', async () => {
    const user = userEvent.setup()
    renderList([{
      deployed: true,
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
    const card = screen.getAllByText('app-dev', { selector: 'button' })
      .map(element => element.closest<HTMLElement>('[data-slot="deployment-target-card"]'))
      .find(element => element !== null) as HTMLElement
    expect(within(card).getByRole('link', { name: /outline\.example\.com/ })).toHaveAttribute('rel', 'noopener noreferrer')
    expect(within(card).getByText('Ready').closest('[data-slot="status-indicator"]')).toHaveAttribute('data-tone', 'success')
    expect(within(card).getByRole('button', { name: 'Create release' })).toBeDisabled()
    expect(screen.queryByText('Deployment has minimum availability.')).not.toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: 'More actions for deploy config app-dev' })[0])
    await user.click(await screen.findByRole('menuitem', { name: 'View details' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Access addresses')).toBeInTheDocument()
    expect(within(dialog).getByText('app-dev.luna-dev.svc.cluster.local')).toBeInTheDocument()
    expect(within(dialog).getByRole('link', { name: /outline\.example\.com/ })).toHaveAttribute('href', 'https://outline.example.com')
    expect(within(dialog).getByText('0.5 vCPU · 1G')).toBeInTheDocument()
    expect(within(dialog).getByText('#3')).toBeInTheDocument()
  })

  it('keeps actionable rollout diagnostics in deployment details', async () => {
    const user = userEvent.setup()
    renderList([{
      deployed: true,
      release: { ...release, message: 'ImagePullBackOff', status: 'failed' },
      routes: [],
      runtimeStatus: { podCount: 1, summary: 'ImagePullBackOff', value: 'image-pull-back-off' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    await user.click(screen.getAllByRole('button', { name: 'More actions for deploy config app-dev' })[0])
    await user.click(await screen.findByRole('menuitem', { name: 'View details' }))
    expect(within(await screen.findByRole('dialog')).getAllByText('ImagePullBackOff').length).toBeGreaterThan(0)
  })

  it('shows not deployed when the backend reports no succeeded release even if a failed release exists', () => {
    renderList([{
      deployed: false,
      release: { ...release, status: 'failed' },
      routes: [],
      runtimeStatus: { podCount: 0, summary: '', value: 'not-found' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    const card = screen.getAllByText('app-dev', { selector: 'button' })
      .map(element => element.closest<HTMLElement>('[data-slot="deployment-target-card"]'))
      .find(element => element !== null) as HTMLElement
    expect(within(card).getAllByText('Not deployed').length).toBeGreaterThan(0)
    expect(within(card).queryByText('No Pod')).not.toBeInTheDocument()
  })

  it('shows the localized failure reason next to a failed release with a log entry', async () => {
    const user = userEvent.setup()
    const onViewLogs = vi.fn()
    const failedRelease: Release = { ...release, failureCode: 'rollout_failed', message: 'progress deadline exceeded', status: 'failed' }
    renderList([{
      deployed: true,
      release: failedRelease,
      routes: [],
      runtimeStatus: { podCount: 1, summary: '', value: 'ready' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }], { onViewLogs })

    const card = screen.getAllByText('app-dev', { selector: 'button' })
      .map(element => element.closest<HTMLElement>('[data-slot="deployment-target-card"]'))
      .find(element => element !== null) as HTMLElement
    expect(within(card).getByText('Workload rollout did not complete')).toBeInTheDocument()
    expect(within(card).queryByText('progress deadline exceeded')).not.toBeInTheDocument()
    await user.click(within(card).getByRole('button', { name: 'View logs' }))
    expect(onViewLogs).toHaveBeenCalledWith(failedRelease)
  })

  it('offers cancel only while the release is active', async () => {
    const user = userEvent.setup()
    const onCancelRelease = vi.fn()
    const runningRelease: Release = { ...release, status: 'running' }
    renderList([{
      deployed: true,
      release: runningRelease,
      routes: [],
      runtimeStatus: { podCount: 1, summary: '', value: 'progressing' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }], { onCancelRelease })

    await user.click(screen.getAllByLabelText('More actions for deploy config app-dev')[0])
    await user.click(await screen.findByRole('menuitem', { name: 'Cancel release' }))
    expect(onCancelRelease).toHaveBeenCalledWith(runningRelease)
  })

  it('hides cancel for a finished release', async () => {
    const user = userEvent.setup()
    renderList([{
      deployed: true,
      release,
      routes: [],
      runtimeStatus: { podCount: 1, summary: '', value: 'ready' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }])

    await user.click(screen.getAllByLabelText('More actions for deploy config app-dev')[0])
    await screen.findByRole('menuitem', { name: 'View details' })
    expect(screen.queryByRole('menuitem', { name: 'Cancel release' })).not.toBeInTheDocument()
  })
})
