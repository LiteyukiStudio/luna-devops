import type { BuildRun, DeploymentTarget } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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
    expect(screen.queryByText('Bind a code repository to this application first')).not.toBeInTheDocument()
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
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDisabled()
  })
})

function renderPanel(options: {
  binding?: { defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  deploymentTargets?: DeploymentTarget[]
} = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ApplicationBuildsPanel
        applicationId="app_1"
        applicationIdentifier="example"
        binding={options.binding}
        buildJobs={[]}
        buildRuns={[buildRun]}
        deploymentTargets={options.deploymentTargets ?? []}
        projectId="prj_1"
        projectIdentifier="project"
        registries={[]}
        repositoryBindings={[]}
      />
    </QueryClientProvider>,
  )
}
