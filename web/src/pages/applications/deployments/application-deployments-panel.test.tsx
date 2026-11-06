import type { DeploymentListState } from './operations/application-deployment-targets-list'
import type { DeploymentTarget, Release } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationDeploymentsPanel } from './application-deployments-panel'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listRuntimeClusterResources: vi.fn(),
      listRuntimeClusters: vi.fn(),
      restartDeploymentTarget: vi.fn(),
      rollbackRelease: vi.fn(),
    },
  }
})

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

function target(id: string, name: string): DeploymentTarget {
  return {
    applicationId: 'app_1',
    availableReplicas: 1,
    clusterId: '',
    cpuRequest: '500m',
    dataVolumes: [],
    deleteStatus: 'active',
    desiredReplicas: 1,
    enabled: true,
    id,
    imageRef: `registry.example/${id}:v1`,
    kubernetesName: id,
    memoryRequest: '1Gi',
    name,
    projectId: 'prj_1',
    readyReplicas: 1,
    replicas: 1,
    servicePorts: [{ name: 'http', port: 3000 }],
    sourceType: 'image',
    status: 'ready',
    stage: 'dev',
  } as unknown as DeploymentTarget
}

function release(id: string, deploymentTargetId: string, revision: number, imageRef: string, status: Release['status'] = 'succeeded'): Release {
  return {
    applicationId: 'app_1',
    buildRunId: '',
    createdAt: `2026-08-${10 + revision}T00:00:00Z`,
    createdBy: 'user_1',
    deploymentTargetId,
    failureCode: '',
    forceImagePull: false,
    id,
    imageRef,
    message: '',
    projectId: 'prj_1',
    revision,
    rollbackFromId: '',
    status,
    type: 'deploy',
  }
}

const targets = [target('dpt_web', 'Web'), target('dpt_api', 'Api')]
const releases = [
  release('rel_web_1', 'dpt_web', 1, 'registry.example/team/web:v1'),
  release('rel_web_2', 'dpt_web', 2, 'registry.example/team/web:v2', 'failed'),
  release('rel_web_3', 'dpt_web', 3, 'registry.example/team/web:v3'),
  release('rel_api_1', 'dpt_api', 1, 'registry.example/team/api:v1'),
]

function renderPanel({ deploymentTargets = targets, listState = { loading: false } as DeploymentListState } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter>
          <ApplicationDeploymentsPanel
            applicationId="app_1"
            buildRuns={[]}
            deploymentTargets={deploymentTargets}
            listState={listState}
            projectId="prj_1"
            projectWebConsoleEnabled
            releases={releases}
            routes={[]}
          />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

async function openRowMenu(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getAllByRole('button', { name: `More actions for deploy config ${name}` })[0])
}

describe('application deployments panel', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    vi.mocked(api.listRuntimeClusters).mockResolvedValue([])
    vi.mocked(api.listRuntimeClusterResources).mockResolvedValue([])
  })

  it('asks for confirmation before restarting and only disables the restarted row while pending', async () => {
    const user = userEvent.setup()
    vi.mocked(api.restartDeploymentTarget).mockReturnValue(new Promise(() => undefined))
    renderPanel()

    await openRowMenu(user, 'Web')
    await user.click(await screen.findByRole('menuitem', { name: 'Restart' }))
    expect(api.restartDeploymentTarget).not.toHaveBeenCalled()

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/All instances of Web will be restarted/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Restart' }))
    expect(api.restartDeploymentTarget).toHaveBeenCalledWith('prj_1', 'app_1', 'dpt_web')
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())

    await openRowMenu(user, 'Web')
    expect(await screen.findByRole('menuitem', { name: 'Restart' })).toHaveAttribute('data-disabled')
    await user.keyboard('{Escape}')
    await openRowMenu(user, 'Api')
    expect(await screen.findByRole('menuitem', { name: 'Restart' })).not.toHaveAttribute('data-disabled')
  })

  it('shows the previous successful release before rolling back', async () => {
    const user = userEvent.setup()
    vi.mocked(api.rollbackRelease).mockResolvedValue(release('rel_web_4', 'dpt_web', 4, 'registry.example/team/web:v1', 'pending'))
    renderPanel()

    await openRowMenu(user, 'Web')
    await user.click(await screen.findByRole('menuitem', { name: 'Rollback' }))

    const dialog = await screen.findByRole('alertdialog')
    const summary = within(dialog).getByText('Image').closest('[data-slot="rollback-target"]') as HTMLElement
    // 失败的 #2 不是回滚目标，回到上一次成功的 #1。
    expect(within(summary).getByText('team/web:v1')).toBeInTheDocument()
    expect(within(summary).getByText(/#1$/)).toBeInTheDocument()
    expect(within(summary).getByText('Web')).toBeInTheDocument()
    expect(api.rollbackRelease).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: 'Roll back' }))
    expect(api.rollbackRelease).toHaveBeenCalledWith('prj_1', 'rel_web_3')
  })

  it('disables rollback when there is no earlier successful release', async () => {
    const user = userEvent.setup()
    renderPanel()

    await openRowMenu(user, 'Api')
    expect(await screen.findByRole('menuitem', { name: 'Rollback' })).toHaveAttribute('data-disabled')
  })

  it('renders loading, error and empty states exclusively', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    const { unmount } = renderPanel({ deploymentTargets: [], listState: { error: { onRetry, retrying: false }, loading: false } })

    expect(screen.getAllByText('Failed to load deploy configs').length).toBeGreaterThan(0)
    expect(screen.queryByText('No deploy configs')).not.toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: 'Retry' })[0])
    expect(onRetry).toHaveBeenCalled()
    unmount()

    const loading = renderPanel({ deploymentTargets: [], listState: { loading: true } })
    expect(screen.queryByText('Failed to load deploy configs')).not.toBeInTheDocument()
    expect(screen.queryByText('No deploy configs')).not.toBeInTheDocument()
    loading.unmount()

    renderPanel({ deploymentTargets: [] })
    expect(screen.getAllByText('No deploy configs').length).toBeGreaterThan(0)
    expect(screen.getAllByRole('button', { name: 'New deploy config' }).length).toBeGreaterThan(0)
  })

  it('requires typing the deploy config name before deleting', async () => {
    const user = userEvent.setup()
    renderPanel()

    await openRowMenu(user, 'Web')
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/Deploy config Web will be deleted/)).toBeInTheDocument()
    const confirm = within(dialog).getByRole('button', { name: 'Delete deploy config' })
    expect(confirm).toBeDisabled()
    await user.type(within(dialog).getByLabelText('Type Web to confirm deletion'), 'Web')
    expect(confirm).toBeEnabled()
  })
})
