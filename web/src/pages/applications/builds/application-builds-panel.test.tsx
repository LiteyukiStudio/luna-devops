import type { BuildRun, DeploymentTarget } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Link, MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationBuildsPanel } from './application-builds-panel'

const mocks = vi.hoisted(() => ({
  listBuildRunsPage: vi.fn(),
}))

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
