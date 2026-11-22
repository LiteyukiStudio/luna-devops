import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectTopologyDetailSheet } from './project-topology-detail-sheet'
import { ProjectTopologyRelationDialog } from './project-topology-relation-dialog'

const mocks = vi.hoisted(() => ({
  listDeploymentTargets: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      ...mocks,
    },
  }
})

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>{ui}</TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('project topology dialog and detail sheet loading/empty states', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('shows loading state on application selectors and loading/noOptions on targetPortName select', async () => {
    mocks.listDeploymentTargets.mockReturnValue(new Promise(() => undefined))

    const { unmount } = renderWithProviders(
      <ProjectTopologyRelationDialog
        applications={[]}
        applicationsLoading
        editingServiceBinding={{
          id: 'sb_1',
          projectId: 'prj_1',
          sourceApplicationId: 'app_src',
          sourceDeploymentTargetId: 'dplt_src',
          targetApplicationId: 'app_tgt',
          targetDeploymentTargetId: 'dplt_tgt',
          targetPortName: 'http',
          targetPort: 8080,
          protocol: 'http',
          path: '',
          injectionMode: 'url',
          urlEnvVar: 'API_URL',
          hostEnvVar: '',
          portEnvVar: '',
          enabled: true,
          createdBy: 'usr_1',
          createdAt: '2026-11-01T00:00:00Z',
          updatedAt: '2026-11-01T00:00:00Z',
        }}
        projectId="prj_1"
        onOpenChange={vi.fn()}
        onSaved={vi.fn()}
      />,
    )

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getAllByText(i18next.t('common.loading')).length).toBeGreaterThanOrEqual(3)
    unmount()

    const user = userEvent.setup()
    mocks.listDeploymentTargets.mockResolvedValue([
      {
        id: 'dplt_portless',
        applicationId: 'app_tgt',
        name: 'Portless target',
        projectId: 'prj_1',
        stage: 'prod',
        clusterId: 'clu_1',
        servicePorts: [],
      },
    ])

    renderWithProviders(
      <ProjectTopologyRelationDialog
        applications={[
          { id: 'app_tgt', identifier: 'api', name: 'API', projectId: 'prj_1', icon: 'box', deleteStatus: 'active', deleteCode: '', deleteMessage: '', createdAt: '', updatedAt: '' },
        ]}
        projectId="prj_1"
        onOpenChange={vi.fn()}
        onSaved={vi.fn()}
      />,
    )

    const readyDialog = await screen.findByRole('dialog')
    await user.click(within(readyDialog).getByRole('button', { name: new RegExp(i18next.t('projectTopology.form.targetApplication')) }))
    await user.click(await screen.findByRole('button', { name: /API/ }))

    await user.click(within(readyDialog).getByRole('button', { name: new RegExp(i18next.t('projectTopology.form.targetTarget')) }))
    await user.click(await screen.findByRole('button', { name: /Portless target/ }))

    const portTrigger = within(readyDialog).getByRole('combobox', { name: new RegExp(i18next.t('projectTopology.form.targetPort')) })
    expect(portTrigger).toBeDisabled()
    expect(portTrigger).toHaveTextContent(i18next.t('common.noOptions'))
  })

  it('renders skeletons in ProjectTopologyDetailSheet while relation details are loading', async () => {
    renderWithProviders(
      <ProjectTopologyDetailSheet
        canManage
        edge={{
          id: 'sb_1',
          origin: 'service_binding',
          relationType: 'calls',
          source: 'app_1',
          target: 'app_2',
          status: 'ready',
        }}
        nodes={[
          { id: 'app_1', kind: 'application', identifier: 'web', name: 'Web', status: 'ready', deploymentTargets: [] },
          { id: 'app_2', kind: 'application', identifier: 'api', name: 'API', status: 'ready', deploymentTargets: [] },
        ]}
        projectId="prj_1"
        relationLoading
        onEdit={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    )

    const sheet = await screen.findByRole('dialog', { name: i18next.t('projectTopology.details') })
    expect(sheet.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
  })

  it('surfaces applicationsError and targetTargets error with inline retry buttons instead of empty labels', async () => {
    const user = userEvent.setup()
    const onRetryApplications = vi.fn()

    const { unmount } = renderWithProviders(
      <ProjectTopologyRelationDialog
        applications={[]}
        applicationsError
        projectId="prj_1"
        onOpenChange={vi.fn()}
        onRetryApplications={onRetryApplications}
        onSaved={vi.fn()}
      />,
    )

    const appErrorDialog = await screen.findByRole('dialog')
    expect(within(appErrorDialog).getAllByText(i18next.t('common.unavailable')).length).toBeGreaterThanOrEqual(2)
    expect(within(appErrorDialog).queryByText(i18next.t('projectTopology.form.noApplications'))).not.toBeInTheDocument()

    const appRetryButtons = within(appErrorDialog).getAllByRole('button', { name: i18next.t('common.retry') })
    expect(appRetryButtons.length).toBeGreaterThanOrEqual(2)
    expect(appRetryButtons[0]).toHaveAttribute('type', 'button')
    await user.click(appRetryButtons[0]!)
    expect(onRetryApplications).toHaveBeenCalledTimes(1)
    unmount()

    mocks.listDeploymentTargets
      .mockRejectedValueOnce(new Error('targets unavailable'))
      .mockResolvedValueOnce([
        {
          id: 'dplt_1',
          applicationId: 'app_tgt',
          name: 'Prod target',
          projectId: 'prj_1',
          stage: 'prod',
          clusterId: 'clu_1',
          servicePorts: [{ name: 'http', port: 8080, protocol: 'TCP' }],
        },
      ])

    renderWithProviders(
      <ProjectTopologyRelationDialog
        applications={[
          { id: 'app_tgt', identifier: 'api', name: 'API', projectId: 'prj_1', icon: 'box', deleteStatus: 'active', deleteCode: '', deleteMessage: '', createdAt: '', updatedAt: '' },
        ]}
        projectId="prj_1"
        onOpenChange={vi.fn()}
        onSaved={vi.fn()}
      />,
    )

    const targetErrorDialog = await screen.findByRole('dialog')
    await user.click(within(targetErrorDialog).getByRole('button', { name: new RegExp(i18next.t('projectTopology.form.targetApplication')) }))
    await user.click(await screen.findByRole('button', { name: /API/ }))

    const retryTargetButton = await within(targetErrorDialog).findByRole('button', { name: i18next.t('common.retry') })
    expect(retryTargetButton).toHaveAttribute('type', 'button')
    expect(within(targetErrorDialog).getByText(i18next.t('common.unavailable'))).toBeInTheDocument()
    expect(within(targetErrorDialog).queryByText(i18next.t('projectTopology.form.noTargets'))).not.toBeInTheDocument()

    await user.click(retryTargetButton)
    await user.click(within(targetErrorDialog).getByRole('button', { name: new RegExp(i18next.t('projectTopology.form.targetTarget')) }))
    expect(await screen.findByRole('button', { name: /Prod target/ })).toBeInTheDocument()
  })
})
