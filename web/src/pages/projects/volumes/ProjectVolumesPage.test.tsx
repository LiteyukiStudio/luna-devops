import type { ProjectVolume } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ProjectVolumeDetailSheet } from './project-volume-detail-sheet'
import { ProjectVolumesPage } from './ProjectVolumesPage'

const mocks = vi.hoisted(() => ({
  getProjectVolume: vi.fn(),
  listProjectVolumes: vi.fn(),
  listRuntimeClustersPage: vi.fn(),
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

const capabilities = {
  canDelete: false,
  canExport: false,
  canImport: false,
  canManageOtherTransfers: false,
  canWrite: false,
  transferEnabled: false,
  userId: 'usr_1',
}

const volumeFixture: ProjectVolume = {
  id: 'vol_1',
  projectId: 'prj_1',
  clusterId: 'clu_raw_id',
  namespace: 'prj-demo',
  claimName: 'pvc-data',
  displayName: 'Postgres Data',
  capacity: '10Gi',
  capacityBytes: 10 * 1024 ** 3,
  storageClassName: 'local-path',
  accessMode: 'ReadWriteOnce',
  volumeMode: 'Filesystem',
  sourceKind: 'blank',
  ownershipMode: 'managed',
  availability: 'available',
  lifecycleState: 'ready',
  bindingSummary: { active: 0, reserved: 0 },
  revision: 1,
  lastErrorCode: '',
  observation: {
    status: 'available',
    exists: true,
    phase: 'Bound',
    capacity: '10Gi',
    storageClassName: 'local-path',
    accessModes: ['ReadWriteOnce'],
    volumeMode: 'Filesystem',
    boundVolumeName: 'pv-1',
    observationCode: '',
    observedAt: '2026-11-10T00:00:00Z',
  },
  createdAt: '2026-11-10T00:00:00Z',
  updatedAt: '2026-11-10T00:00:00Z',
}

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>{ui}</TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('project volumes loading and error states', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('shows a skeleton in the cluster column while runtime clusters are pending instead of raw clusterId, and wires retry on ErrorState', async () => {
    mocks.listRuntimeClustersPage.mockReturnValueOnce(new Promise(() => undefined))
    mocks.listProjectVolumes.mockResolvedValueOnce({
      items: [volumeFixture],
      page: 1,
      pageSize: 20,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })

    const { container, unmount } = renderWithProviders(
      <ProjectVolumesPage capabilities={capabilities} projectId="prj_1" />,
    )

    expect(await screen.findByText('Postgres Data')).toBeInTheDocument()
    expect(screen.queryByText('clu_raw_id')).not.toBeInTheDocument()
    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    unmount()

    const user = userEvent.setup()
    mocks.listRuntimeClustersPage.mockResolvedValue({ items: [{ id: 'clu_raw_id', name: 'Tokyo Prod' }], page: 1, pageSize: 50, sortBy: 'name', sortOrder: 'asc', total: 1, totalPages: 1 })
    mocks.listProjectVolumes
      .mockRejectedValueOnce(new Error('volumes failed'))
      .mockResolvedValueOnce({ items: [volumeFixture], page: 1, pageSize: 20, sortBy: 'createdAt', sortOrder: 'desc', total: 1, totalPages: 1 })

    renderWithProviders(<ProjectVolumesPage capabilities={capabilities} projectId="prj_1" />)

    expect(await screen.findByText(i18next.t('projectVolumes.loadFailedTitle'))).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(await screen.findByText('Postgres Data')).toBeInTheDocument()
    expect(screen.getByText('Tokyo Prod')).toBeInTheDocument()
  })

  it('renders a skeleton for cluster in ProjectVolumeDetailSheet while clusters are pending and wires retry on detail ErrorState', async () => {
    mocks.listRuntimeClustersPage.mockReturnValueOnce(new Promise(() => undefined))
    mocks.getProjectVolume.mockResolvedValueOnce({
      ...volumeFixture,
      bindings: [],
      recentTransfers: [],
    })

    const { unmount } = renderWithProviders(
      <ProjectVolumeDetailSheet
        capabilities={capabilities}
        clusterId="clu_raw_id"
        projectId="prj_1"
        volumeId="vol_1"
        onOpenChange={vi.fn()}
      />,
    )

    const sheet = await screen.findByRole('dialog', { name: 'Postgres Data' })
    expect(within(sheet).queryByText('clu_raw_id')).not.toBeInTheDocument()
    expect(sheet.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    unmount()

    const user = userEvent.setup()
    mocks.listRuntimeClustersPage.mockResolvedValue({ items: [{ id: 'clu_raw_id', name: 'Tokyo Prod' }], page: 1, pageSize: 50, sortBy: 'name', sortOrder: 'asc', total: 1, totalPages: 1 })
    mocks.getProjectVolume
      .mockRejectedValueOnce(new Error('detail failed'))
      .mockResolvedValueOnce({ ...volumeFixture, bindings: [], recentTransfers: [] })

    renderWithProviders(
      <ProjectVolumeDetailSheet
        capabilities={capabilities}
        clusterId="clu_raw_id"
        projectId="prj_1"
        volumeId="vol_1"
        onOpenChange={vi.fn()}
      />,
    )

    const errorSheet = await screen.findByRole('dialog')
    expect(await within(errorSheet).findByText(i18next.t('projectVolumes.loadFailedTitle'))).toBeInTheDocument()
    await user.click(within(errorSheet).getByRole('button', { name: i18next.t('common.retry') }))
    expect(await within(errorSheet).findByText('Tokyo Prod')).toBeInTheDocument()
  })
})
