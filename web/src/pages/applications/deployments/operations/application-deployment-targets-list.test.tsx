import type { DeploymentTargetRow } from './application-deployment-targets-list'
import type { DeploymentTarget, DeploymentTargetMetrics, GatewayRoute, Release } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import '@/i18n'

type Listener = (event: MessageEvent) => void

class FakeEventSource {
  static instances: FakeEventSource[] = []
  listeners: Record<string, Listener[]> = {}
  closed = false
  url: string

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: Listener) {
    this.listeners[type] ??= []
    this.listeners[type].push(listener)
  }

  removeEventListener(type: string, listener: Listener) {
    this.listeners[type] = (this.listeners[type] ?? []).filter(item => item !== listener)
  }

  emit(type: string, data?: unknown) {
    const event = { data: data === undefined ? '' : JSON.stringify(data) } as MessageEvent
    for (const listener of this.listeners[type] ?? [])
      listener(event)
  }

  close() {
    this.closed = true
  }
}

vi.mock('@/lib/telemetry', () => ({
  createTracedEventSource: vi.fn((url: string) => new FakeEventSource(url) as unknown as EventSource),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listDeploymentTargetEvents: vi.fn(),
      listDeploymentTargetRuntimePods: vi.fn(),
    },
  }
})

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

function renderList(items: DeploymentTargetRow[] = [], handlers: {
  onCancelRelease?: (release: Release) => void
  onOpenConsole?: (release: Release, pod?: string) => void
  onViewLogs?: (release: Release, options?: { view?: 'deployment' | 'runtime', pod?: string, previous?: boolean }) => void
} = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
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
          onOpenConsole={handlers.onOpenConsole ?? vi.fn()}
          onOpenReleaseDialog={vi.fn()}
          onEditTarget={vi.fn()}
          onPullLatestImageDeploy={vi.fn()}
          onRestart={vi.fn()}
          onRollback={vi.fn()}
          onViewLogs={handlers.onViewLogs ?? vi.fn()}
        />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('application deployment targets behavior', () => {
  beforeEach(() => {
    FakeEventSource.instances = []
  })
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

  it('shows inline AI diagnosis status and opens release logs when clicking View AI Diagnosis', async () => {
    const user = userEvent.setup()
    const onViewLogs = vi.fn()
    const diagnosedRelease: Release = {
      ...release,
      failureCode: 'rollout_failed',
      status: 'failed',
      aiDiagnosis: {
        id: 'aidg_rel_1',
        subjectType: 'release',
        subjectId: release.id,
        status: 'succeeded',
        failureCode: 'rollout_failed',
        rootCauseSummary: 'Readiness probe failed on port 8080',
        confidence: 'high',
        suggestions: ['Update readiness probe port to 3000'],
        createdAt: '2026-11-22T06:53:45Z',
      },
    }
    renderList([{
      deployed: true,
      release: diagnosedRelease,
      routes: [],
      runtimeStatus: { podCount: 1, summary: '', value: 'ready' },
      target: deploymentTarget,
      webConsoleEnabled: false,
    }], { onViewLogs })

    const card = screen.getAllByText('app-dev', { selector: 'button' })
      .map(element => element.closest<HTMLElement>('[data-slot="deployment-target-card"]'))
      .find(element => element !== null) as HTMLElement
    await user.click(within(card).getByRole('button', { name: 'View AI Diagnosis' }))
    expect(onViewLogs).toHaveBeenCalledWith(diagnosedRelease)
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

  it('displays live OOMKilled pod diagnostics, cluster warning events, and jumps to previous crash logs', async () => {
    const user = userEvent.setup()
    const onViewLogs = vi.fn()
    vi.mocked(api.listDeploymentTargetRuntimePods).mockResolvedValue({
      items: [
        {
          name: 'app-dev-7f9c8-oom',
          phase: 'Running',
          status: 'oom-killed',
          ready: false,
          readyContainers: 0,
          totalContainers: 1,
          terminating: false,
          restartCount: 5,
          releaseId: 'release_1',
          nodeName: 'node-a',
          containers: ['app'],
          containerStatuses: [
            {
              name: 'app',
              image: 'registry.example/app:v1',
              init: false,
              ready: false,
              restartCount: 5,
              currentState: { state: 'waiting', reason: 'CrashLoopBackOff', message: 'back-off 2m40s restarting failed container' },
              lastTermination: { reason: 'OOMKilled', exitCode: 137, finishedAt: '2026-09-06T01:02:00Z' },
            },
          ],
          createdAt: '2026-09-06T00:00:00Z',
        },
      ],
      page: 1,
      pageSize: 100,
      sortBy: 'restartCount',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
      status: 'ready',
      observationCode: 'deployment_target.runtime_pods_ready',
      observedAt: '2026-09-06T01:03:00Z',
    })
    vi.mocked(api.listDeploymentTargetEvents).mockResolvedValue({
      items: [
        {
          id: 'evt_pod_1',
          involvedKind: 'Pod',
          involvedName: 'app-dev-7f9c8-oom',
          type: 'Warning',
          reason: 'BackOff',
          source: 'kubelet',
          message: 'Back-off restarting failed container app in pod app-dev-7f9c8-oom',
          count: 12,
          firstSeen: '2026-09-06T00:50:00Z',
          lastSeen: '2026-09-06T01:02:30Z',
        },
        {
          id: 'evt_pod_2',
          involvedKind: 'Pod',
          involvedName: 'app-dev-7f9c8-oom',
          type: 'Normal',
          reason: 'Pulled',
          source: 'kubelet',
          message: 'Container image pulled successfully',
          count: 1,
          firstSeen: '2026-09-06T00:49:00Z',
          lastSeen: '2026-09-06T00:49:00Z',
        },
      ],
      page: 1,
      pageSize: 20,
      sortBy: 'lastSeen',
      sortOrder: 'desc',
      total: 2,
      totalPages: 1,
      status: 'ready',
      observationCode: 'deployment_target.events_ready',
      observedAt: '2026-09-06T01:03:00Z',
    })

    renderList([{
      deployed: true,
      release,
      routes: [],
      runtimeStatus: { podCount: 1, summary: 'OOMKilled (Exit 137)', value: 'oom-killed' },
      target: { ...deploymentTarget, enabled: true },
      webConsoleEnabled: false,
    }], { onViewLogs })

    await user.click(screen.getAllByRole('button', { name: 'More actions for deploy config app-dev' })[0])
    await user.click(await screen.findByRole('menuitem', { name: 'View details' }))

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('app-dev-7f9c8-oom')).toBeInTheDocument()
    expect(within(dialog).getByText(/Exit 137/)).toBeInTheDocument()
    expect(await within(dialog).findByText('1 recent warning event(s) detected')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'View events' }))
    expect(within(dialog).getByRole('tab', { name: /Cluster events/ })).toHaveAttribute('aria-selected', 'true')
    expect(within(dialog).getByRole('button', { name: 'Warnings only' })).toHaveAttribute('aria-pressed', 'true')
    expect(await within(dialog).findByText(/Back-off restarting failed container/)).toBeInTheDocument()
    expect(within(dialog).queryByText('Container image pulled successfully')).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Warnings only' }))
    expect(await within(dialog).findByText('Container image pulled successfully')).toBeInTheDocument()
    expect(api.listDeploymentTargetEvents).toHaveBeenCalledTimes(1)

    await user.click(within(dialog).getByRole('tab', { name: 'Overview' }))
    await user.click(within(dialog).getByRole('button', { name: 'Previous crash logs' }))
    expect(onViewLogs).toHaveBeenCalledWith(release, { pod: 'app-dev-7f9c8-oom', previous: true, view: 'runtime' })
    // Detail drawer remains open when opening secondary log drawer.
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('merges runtime resources and instances with per-pod mini rings, unified outline buttons, and persistent primary drawer', async () => {
    const user = userEvent.setup()
    const onViewLogs = vi.fn()
    const onOpenConsole = vi.fn()
    vi.mocked(api.listDeploymentTargetRuntimePods).mockResolvedValue({
      items: [
        {
          name: 'app-dev-pod-1',
          phase: 'Running',
          status: 'ready',
          ready: true,
          readyContainers: 1,
          totalContainers: 1,
          terminating: false,
          restartCount: 1,
          releaseId: 'release_1',
          nodeName: 'node-1',
          containers: ['app'],
          containerStatuses: [],
          createdAt: '2026-09-06T00:00:00Z',
        },
        {
          name: 'app-dev-pod-pending',
          phase: 'Pending',
          status: 'pending',
          ready: false,
          readyContainers: 0,
          totalContainers: 1,
          terminating: false,
          restartCount: 0,
          releaseId: 'release_1',
          containers: ['app'],
          containerStatuses: [],
          createdAt: '2026-09-06T01:00:00Z',
        },
      ],
      page: 1,
      pageSize: 100,
      sortBy: 'restartCount',
      sortOrder: 'desc',
      total: 2,
      totalPages: 1,
      status: 'ready',
      observationCode: 'deployment_target.runtime_pods_ready',
      observedAt: '2026-09-06T01:03:00Z',
    })
    vi.mocked(api.listDeploymentTargetEvents).mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 20,
      sortBy: 'lastSeen',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
      status: 'ready',
      observationCode: 'deployment_target.events_ready',
      observedAt: '2026-09-06T01:03:00Z',
    })

    renderList([{
      deployed: true,
      release,
      routes: [],
      runtimeStatus: { podCount: 2, summary: '', value: 'ready' },
      target: { ...deploymentTarget, desiredReplicas: 2, enabled: true, readyReplicas: 1 },
      webConsoleEnabled: true,
    }], { onOpenConsole, onViewLogs })

    await user.click(screen.getAllByRole('button', { name: 'More actions for deploy config app-dev' })[0])
    await user.click(await screen.findByRole('menuitem', { name: 'View details' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Runtime resources & instances')).toBeInTheDocument()
    expect(await within(dialog).findByText('Instances (2)')).toBeInTheDocument()

    const liveMetrics: DeploymentTargetMetrics = {
      available: true,
      availableReplicas: 1,
      configuredReplicas: 2,
      containerCount: 1,
      cpuLimitMilli: 2000,
      cpuRequestMilli: 500,
      cpuUsageMilli: 940,
      cpuUsagePercent: 47,
      desiredReplicas: 2,
      memoryLimitBytes: 4 * 1024 ** 3,
      memoryRequestBytes: 1024 ** 3,
      memoryUsageBytes: Math.round(1.62 * 1024 ** 3),
      memoryUsagePercent: 40.5,
      podCount: 1,
      pods: [
        {
          podName: 'app-dev-pod-1',
          cpuUsageMilli: 940,
          cpuUsagePercent: 94,
          memoryUsageBytes: Math.round(1.62 * 1024 ** 3),
          memoryUsagePercent: 81,
        },
      ],
      readyReplicas: 1,
      replicaCpuLimitMilli: 1000,
      replicaCpuRequestMilli: 250,
      replicaMemoryLimitBytes: 2 * 1024 ** 3,
      replicaMemoryRequestBytes: 512 * 1024 ** 2,
      status: 'ready',
      updatedAt: '2026-09-06T01:03:00Z',
    }

    act(() => {
      FakeEventSource.instances[0].emit('metrics', liveMetrics)
    })

    expect(within(dialog).getByText('940m / 1')).toBeInTheDocument()
    expect(within(dialog).getByText('(94%)')).toBeInTheDocument()
    expect(within(dialog).getByText('1.62 / 2Gi')).toBeInTheDocument()
    expect(within(dialog).getByText('(81%)')).toBeInTheDocument()

    const logsButtons = within(dialog).getAllByRole('button', { name: 'Logs' })
    const crashButton = within(dialog).getByRole('button', { name: 'Previous crash logs' })
    const consoleButtons = within(dialog).getAllByRole('button', { name: 'Web Console' })
    expect(logsButtons[0].className).toBe(consoleButtons[0].className)
    expect(crashButton.className).toBe(consoleButtons[0].className)

    await user.click(logsButtons[0])
    expect(onViewLogs).toHaveBeenCalledWith(release, { pod: 'app-dev-pod-1', previous: false, view: 'runtime' })
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await user.click(consoleButtons[0])
    expect(onOpenConsole).toHaveBeenCalledWith(release, 'app-dev-pod-1')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
