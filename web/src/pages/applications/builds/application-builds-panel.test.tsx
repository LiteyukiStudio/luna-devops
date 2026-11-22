import type { BuildsPanelHandle } from './application-builds-panel'
import type { BuildJob, BuildRun, DeploymentTarget } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { Link, MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationBuildsPanel } from './application-builds-panel'

class FakeBuildLogEventSource {
  static instances: FakeBuildLogEventSource[] = []
  listeners: Record<string, Array<(event: Event) => void>> = {}
  onerror: (() => void) | null = null
  closed = false

  constructor() {
    FakeBuildLogEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: Event) => void) {
    this.listeners[type] ??= []
    this.listeners[type].push(listener)
  }

  removeEventListener(type: string, listener: (event: Event) => void) {
    this.listeners[type] = (this.listeners[type] ?? []).filter(item => item !== listener)
  }

  emitChunk(content: string) {
    const event = { data: JSON.stringify({ content }) } as unknown as Event
    for (const listener of this.listeners.chunk ?? [])
      listener(event)
  }

  fail() {
    this.onerror?.()
  }

  close() {
    this.closed = true
  }
}

const mocks = vi.hoisted(() => ({
  listBuildRunsPage: vi.fn(),
}))

vi.mock('@/lib/telemetry', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/telemetry')>()
  return {
    ...actual,
    createTracedEventSource: vi.fn(() => new FakeBuildLogEventSource() as unknown as EventSource),
  }
})

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listBuildRunsPage: mocks.listBuildRunsPage,
    },
  }
})

vi.mock('@/lib/billing-display', () => ({
  useBillingDisplay: () => ({
    buildMinuteCost: () => 0,
    formatAmountWithUnit: () => '0 Credits',
  }),
}))

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

const buildRun: BuildRun = {
  id: 'bldr_history1',
  projectId: 'prj_1',
  applicationId: 'app_1',
  deploymentTargetId: 'dpt_deleted',
  buildVariableSetIds: [],
  status: 'succeeded',
  failureCode: '',
  triggerType: 'manual',
  sourceBranch: 'main',
  sourceTag: '',
  sourceCommit: '1234567890abcdef',
  buildDefinitionMode: 'repository_dockerfile',
  buildTemplateId: '',
  buildTemplateVersion: '',
  buildTemplateValues: '{}',
  buildTemplateChecksum: '',
  dockerfilePath: 'Dockerfile',
  buildContext: '.',
  buildDirectory: '',
  buildArgs: '',
  buildEnvironmentId: '',
  buildCpuRequest: '2',
  buildMemoryRequest: '4Gi',
  buildTimeoutSeconds: 1800,
  targetRegistryId: 'reg_1',
  targetRepository: 'registry.example.test/example',
  targetTag: 'latest',
  imageRef: 'registry.example.test/example:latest',
  imageDigest: 'sha256:1234',
  startedAt: '2026-10-06T00:00:00Z',
  finishedAt: '2026-10-06T00:01:00Z',
  createdBy: 'usr_1',
  triggeredByName: 'Tester',
  triggeredByEmail: 'tester@example.test',
  sourceAuthorName: 'Developer',
  sourceAuthorEmail: 'developer@example.test',
  sourceCommitMessage: 'feat: add login',
  createdAt: '2026-10-06T00:00:00Z',
}

describe('application builds panel', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    FakeBuildLogEventSource.instances = []
    await i18next.changeLanguage('en-US')
    mocks.listBuildRunsPage.mockResolvedValue({
      items: [buildRun],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
  })

  it('shows returned build history when the current repository binding is unavailable', async () => {
    renderPanel()

    expect(await screen.findByText(/1 build run/)).toBeVisible()
    expect(screen.getByText('registry.example.test/example:latest')).toBeVisible()
    expect(screen.getByText('Succeeded', { selector: '[data-slot="status-indicator"] span' }).closest('[data-slot="status-indicator"]')).toHaveAttribute('data-tone', 'success')
    expect(screen.getByRole('button', { name: 'Logs' })).toBeDisabled()
    expect(screen.queryByText('Bind a code repository to this application first')).not.toBeInTheDocument()
  })

  it('shows the localized failure reason on a timed-out build row', async () => {
    mocks.listBuildRunsPage.mockResolvedValue({
      items: [{ ...buildRun, status: 'timeout', failureCode: 'queue_timeout' } satisfies BuildRun],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
    renderPanel()

    expect(await screen.findByText('Waited in the queue too long to start')).toBeVisible()
    expect(screen.getByText('Timed out', { selector: '[data-slot="status-indicator"] span' }).closest('[data-slot="status-indicator"]')).toHaveAttribute('data-tone', 'danger')
  })

  it('does not attribute an old build to a replacement repository binding', async () => {
    const user = userEvent.setup()
    renderPanel({
      binding: { defaultBranch: 'main', gitAccountId: 'git_1', owner: 'new-owner', repo: 'new-repo' },
      deploymentTargets: [{
        id: buildRun.deploymentTargetId,
        name: 'Production',
        enabled: true,
        repositoryBindingId: 'rpb_deleted',
        sourceType: 'repository',
      } as DeploymentTarget],
    })

    expect(await screen.findByText(/1 build run/)).toBeVisible()
    expect(screen.queryByTitle('new-owner/new-repo')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Build run actions' }))
    expect(await screen.findByRole('menuitem', { name: 'Retry' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('shows inline View AI Diagnosis button and opens diagnosis drawer on click', async () => {
    const user = userEvent.setup()
    await i18next.changeLanguage('en-US')
    const failedRun: BuildRun = {
      ...buildRun,
      id: 'bldr_failed_ai',
      status: 'failed',
      failureCode: 'job_failed',
      aiDiagnosis: {
        id: 'aidg_1',
        subjectType: 'build_run',
        subjectId: 'bldr_failed_ai',
        status: 'succeeded',
        failureCode: 'job_failed',
        rootCauseSummary: 'FROM nginxx:1.27-alpine does not exist on Docker Hub',
        confidence: 'high',
        suggestions: ['Fix FROM nginx:1.27-alpine'],
        createdAt: '2026-11-22T06:53:45Z',
      },
    }
    mocks.listBuildRunsPage.mockResolvedValue({
      items: [failedRun],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })

    renderPanel({
      buildRuns: [failedRun],
      initialEntry: '/projects/prj_1/apps/app_1?tab=builds',
    })

    const viewBtn = await screen.findByRole('button', { name: 'View AI Diagnosis' })
    await user.click(viewBtn)
    expect(await screen.findByText('FROM nginxx:1.27-alpine does not exist on Docker Hub')).toBeInTheDocument()
  })

  it('auto-opens the AI diagnosis drawer when navigated with #buildRunId hash', async () => {
    await i18next.changeLanguage('en-US')
    const failedRun: BuildRun = {
      ...buildRun,
      id: 'bldr_failed_ai',
      status: 'failed',
      failureCode: 'job_failed',
      aiDiagnosis: {
        id: 'aidg_1',
        subjectType: 'build_run',
        subjectId: 'bldr_failed_ai',
        status: 'succeeded',
        failureCode: 'job_failed',
        rootCauseSummary: 'FROM nginxx:1.27-alpine does not exist on Docker Hub',
        confidence: 'high',
        suggestions: ['Fix FROM nginx:1.27-alpine'],
        createdAt: '2026-11-22T06:53:45Z',
      },
    }
    mocks.listBuildRunsPage.mockResolvedValue({
      items: [failedRun],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })

    renderPanel({
      buildRuns: [failedRun],
      initialEntry: '/projects/prj_1/apps/app_1?tab=builds#buildRunId=bldr_failed_ai',
    })

    expect(await screen.findByText('FROM nginxx:1.27-alpine does not exist on Docker Hub')).toBeInTheDocument()
  })

  it('switches to a second #buildRunId and re-opens the same #buildRunId on repeated SPA navigation', async () => {
    const user = userEvent.setup()
    await i18next.changeLanguage('en-US')
    const run1: BuildRun = {
      ...buildRun,
      id: 'bldr_1',
      status: 'failed',
      failureCode: 'job_failed',
      aiDiagnosis: {
        id: 'aidg_1',
        subjectType: 'build_run',
        subjectId: 'bldr_1',
        status: 'succeeded',
        failureCode: 'job_failed',
        rootCauseSummary: 'Diagnosis summary one',
        confidence: 'high',
        suggestions: [],
        createdAt: '2026-11-22T06:50:00Z',
      },
    }
    const run2: BuildRun = {
      ...buildRun,
      id: 'bldr_2',
      status: 'failed',
      failureCode: 'job_failed',
      aiDiagnosis: {
        id: 'aidg_2',
        subjectType: 'build_run',
        subjectId: 'bldr_2',
        status: 'succeeded',
        failureCode: 'job_failed',
        rootCauseSummary: 'Diagnosis summary two',
        confidence: 'high',
        suggestions: [],
        createdAt: '2026-11-22T06:55:00Z',
      },
    }
    mocks.listBuildRunsPage.mockImplementation(async (_projectId, params) => {
      const matched = [run1, run2].filter(r => !params.search || r.id === params.search)
      return {
        items: matched,
        page: 1,
        pageSize: 10,
        sortBy: 'createdAt',
        sortOrder: 'desc',
        total: matched.length,
        totalPages: 1,
      }
    })

    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })
    render(
      <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1?tab=builds#buildRunId=bldr_1']}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <Link to="/projects/prj_1/apps/app_1?tab=builds#buildRunId=bldr_2">Jump Run 2</Link>
            <ApplicationBuildsPanel
              applicationId="app_1"
              applicationIdentifier="example"
              buildJobs={[]}
              buildRuns={[run1, run2]}
              deploymentTargets={[]}
              projectId="prj_1"
              projectIdentifier="project"
              registries={[]}
              repositoryBindings={[]}
            />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    )

    expect(await screen.findByText('Diagnosis summary one')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Close' }))

    // Navigate to #buildRunId=bldr_2 while already mounted -> must query bldr_2, NOT stale bldr_1!
    await user.click(screen.getByRole('link', { name: 'Jump Run 2' }))
    expect(await screen.findByText('Diagnosis summary two')).toBeInTheDocument()

    // Close and click the SAME notification link again -> new location.key must re-open the drawer!
    await user.click(screen.getByRole('button', { name: 'Close' }))
    await user.click(screen.getByRole('link', { name: 'Jump Run 2' }))
    expect(await screen.findByText('Diagnosis summary two')).toBeInTheDocument()
  })

  it('does not flash repositoryBindingRequired while repositoryBindings are still loading', async () => {
    mocks.listBuildRunsPage.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    })

    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })
    render(
      <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1?tab=builds']}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <ApplicationBuildsPanel
              applicationId="app_1"
              applicationIdentifier="example"
              buildJobs={[]}
              buildRuns={[]}
              deploymentTargets={[]}
              projectId="prj_1"
              projectIdentifier="project"
              registries={[]}
              repositoryBindings={[]}
              repositoryBindingsLoading
            />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    )

    await screen.findByRole('button', { name: i18next.t('buildsPage.applicationBuildEnvironment') })
    expect(screen.queryByText(i18next.t('buildsPage.repositoryBindingRequired'))).not.toBeInTheDocument()
  })

  it('keeps the log viewer in loading state while a queued build run awaits job creation', async () => {
    const queuedRun: BuildRun = {
      ...buildRun,
      id: 'bldr_queued',
      status: 'queued',
    }
    mocks.listBuildRunsPage.mockResolvedValue({
      items: [queuedRun],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })

    renderPanel({
      buildRuns: [queuedRun],
      initialEntry: '/projects/prj_1/apps/app_1?tab=builds#buildRunId=bldr_queued',
    })

    expect(await screen.findByText(i18next.t('common.logViewer.waiting'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('buildsPage.noLogs'))).not.toBeInTheDocument()
  })

  it('shows unavailable status and toolbar retry instead of Logs updated when build log SSE disconnects mid-stream', async () => {
    const user = userEvent.setup()
    const runningJob: BuildJob = {
      id: 'bldj_stream_1',
      buildRunId: buildRun.id,
      projectId: 'prj_1',
      status: 'running',
      failureCode: '',
      message: 'run_command',
      currentStep: '[1/3] FROM node:22-alpine',
      stepsCompleted: 1,
      stepsTotal: 3,
      logRef: '',
      attempts: 1,
      startedAt: '2026-10-06T00:00:00Z',
      finishedAt: '',
      createdAt: '2026-10-06T00:00:00Z',
    }
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })
    render(
      <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1?tab=builds']}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <ApplicationBuildsPanel
              applicationId="app_1"
              applicationIdentifier="example"
              buildJobs={[runningJob]}
              buildRuns={[buildRun]}
              deploymentTargets={[]}
              projectId="prj_1"
              projectIdentifier="project"
              registries={[]}
              repositoryBindings={[]}
            />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    )

    await user.click(await screen.findByRole('button', { name: 'Logs' }))
    expect(FakeBuildLogEventSource.instances).toHaveLength(1)

    act(() => {
      FakeBuildLogEventSource.instances[0].emitChunk('Step 1/3: FROM node:22-alpine\n')
      FakeBuildLogEventSource.instances[0].fail()
    })

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Step 1\/3: FROM node:22-alpine/)).toBeInTheDocument()
    expect(within(dialog).getByText(i18next.t('common.unavailable'))).toBeInTheDocument()
    expect(within(dialog).queryByText(i18next.t('buildsPage.logsUpdated'))).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: i18next.t('common.retry') }))
    expect(FakeBuildLogEventSource.instances).toHaveLength(2)
  })

  it('surfaces repositoryBindingsError with retry and disables retry buttons while retrying in Trigger Build dialog', async () => {
    const user = userEvent.setup()
    const onRetryRepositoryBindings = vi.fn()
    const onRetryRegistries = vi.fn()
    const panelRef = createRef<BuildsPanelHandle>()
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })
    render(
      <MemoryRouter initialEntries={['/projects/prj_1/apps/app_1?tab=builds']}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <ApplicationBuildsPanel
              ref={panelRef}
              applicationId="app_1"
              applicationIdentifier="example"
              buildJobs={[]}
              buildRuns={[]}
              deploymentTargets={[{
                id: 'dpt_1',
                name: 'Production',
                enabled: true,
                repositoryBindingId: 'rpb_1',
                sourceType: 'repository',
              } as DeploymentTarget]}
              projectId="prj_1"
              projectIdentifier="project"
              registries={[]}
              registriesError
              registriesRetrying
              repositoryBindings={[]}
              repositoryBindingsError
              onRetryRegistries={onRetryRegistries}
              onRetryRepositoryBindings={onRetryRepositoryBindings}
            />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    )

    await screen.findByRole('button', { name: i18next.t('buildsPage.applicationBuildEnvironment') })
    act(() => {
      panelRef.current?.openTriggerDrawer()
    })

    const dialog = await screen.findByRole('dialog', { name: i18next.t('buildsPage.triggerBuild') })
    const alerts = within(dialog).getAllByRole('alert')
    expect(alerts).toHaveLength(2)

    const retryButtons = within(dialog).getAllByRole('button', { name: i18next.t('common.retry') })
    expect(retryButtons[0]).toBeEnabled()
    expect(retryButtons[1]).toBeDisabled()

    await user.click(retryButtons[0])
    expect(onRetryRepositoryBindings).toHaveBeenCalledOnce()
  })
})

function renderPanel(options: {
  binding?: { defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  buildRuns?: BuildRun[]
  deploymentTargets?: DeploymentTarget[]
  initialEntry?: string
} = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <MemoryRouter initialEntries={[options.initialEntry ?? '/projects/prj_1/apps/app_1?tab=builds']}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <ApplicationBuildsPanel
            applicationId="app_1"
            applicationIdentifier="example"
            binding={options.binding}
            buildJobs={[]}
            buildRuns={options.buildRuns ?? [buildRun]}
            deploymentTargets={options.deploymentTargets ?? []}
            projectId="prj_1"
            projectIdentifier="project"
            registries={[]}
            repositoryBindings={[]}
          />
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}
