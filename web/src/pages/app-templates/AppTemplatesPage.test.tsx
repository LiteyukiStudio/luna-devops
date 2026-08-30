import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { AppTemplatesPage } from './AppTemplatesPage'

const mocks = vi.hoisted(() => ({
  createProjectVolume: vi.fn(),
  getAppTemplate: vi.fn(),
  getProject: vi.fn(),
  getProjectVolume: vi.fn(),
  installAppTemplate: vi.fn(),
  listAppTemplates: vi.fn(),
  listProjectVolumes: vi.fn(),
  listProjectVolumeStorageClasses: vi.fn(),
  listProjects: vi.fn(),
  listRuntimeClusters: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
  session: {
    user: {
      id: 'usr_1',
      role: 'user',
    },
  },
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => mocks.session,
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      createProjectVolume: mocks.createProjectVolume,
      getAppTemplate: mocks.getAppTemplate,
      getProject: mocks.getProject,
      getProjectVolume: mocks.getProjectVolume,
      installAppTemplate: mocks.installAppTemplate,
      listAppTemplates: mocks.listAppTemplates,
      listProjectVolumes: mocks.listProjectVolumes,
      listProjectVolumeStorageClasses: mocks.listProjectVolumeStorageClasses,
      listProjects: mocks.listProjects,
      listRuntimeClusters: mocks.listRuntimeClusters,
    },
  }
})

vi.mock('sonner', () => ({
  toast: {
    error: mocks.toastError,
    success: mocks.toastSuccess,
  },
}))

const template = {
  id: 'redis',
  slug: 'redis',
  name: 'Redis',
  description: 'Redis cache',
  category: 'middleware',
  icon: '',
  officialWebsite: 'https://redis.io',
  officialRepository: 'https://github.com/redis/redis',
  popularityWeight: 100,
  image: 'redis:7-alpine',
  version: '7',
  servicePort: 6379,
  defaultReplicas: 1,
  defaultCPU: '500m',
  defaultMemory: '512Mi',
  dataVolumes: [{ logicalName: 'data', sourceType: 'projectVolume' as const, mountPath: '/data' }],
  values: [],
}

const createdVolume = {
  id: 'pvol_new',
  projectId: 'prj_1',
  displayName: 'Redis data',
  clusterId: 'cluster_1',
  namespace: 'project-one',
  claimName: 'pvc-pvol-new',
  ownershipMode: 'managed',
  sourceKind: 'blank',
  lifecycleState: 'provisioning',
  pendingOperation: 'provision',
  availability: 'unavailable',
  capacity: '10Gi',
  capacityBytes: 10 * 1024 * 1024 * 1024,
  storageClassName: 'fast',
  accessMode: 'ReadWriteOnce',
  volumeMode: 'Filesystem',
  bindingSummary: { active: 0, reserved: 0 },
  revision: 1,
  observation: {
    status: 'unavailable',
    exists: false,
    phase: '',
    capacity: '',
    storageClassName: '',
    accessModes: [],
    volumeMode: 'Filesystem',
    boundVolumeName: '',
    observedAt: '',
    observationCode: 'project_volume.not_observed',
  },
  createdAt: '2026-08-29T00:00:00Z',
  updatedAt: '2026-08-29T00:00:00Z',
}

describe('app template installation', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listAppTemplates.mockResolvedValue([{ ...template, valueCount: 0, requiredValueCount: 0 }])
    mocks.getAppTemplate.mockResolvedValue(template)
    mocks.listProjects.mockResolvedValue([{
      id: 'prj_1',
      name: 'Project One',
      identifier: 'project-one',
      description: '',
    }])
    mocks.getProject.mockResolvedValue({
      id: 'prj_1',
      name: 'Project One',
      identifier: 'project-one',
      description: '',
      currentUserRole: 'developer',
    })
    mocks.listRuntimeClusters.mockResolvedValue([{ id: 'cluster_1', name: 'Primary cluster', isDefault: true }])
    mocks.listProjectVolumes.mockResolvedValue(page([]))
    mocks.listProjectVolumeStorageClasses.mockResolvedValue(page([{ name: 'fast', isDefault: true }], 'name'))
    mocks.createProjectVolume.mockResolvedValue(createdVolume)
    mocks.getProjectVolume.mockResolvedValue({
      ...createdVolume,
      availability: 'available',
      bindings: [],
      bindingPage: 1,
      bindingPageSize: 20,
      bindingTotal: 0,
      bindingTotalPages: 0,
      recentTransfers: [],
      transferPage: 1,
      transferPageSize: 20,
      transferTotal: 0,
      transferTotalPages: 0,
    })
    mocks.installAppTemplate.mockResolvedValue({
      application: { id: 'app_1', projectId: 'prj_1' },
      deploymentTarget: { id: 'dplt_1' },
      installation: { id: 'atpl_1' },
    })
  })

  it('creates a compatible volume in place, selects it, and installs with its ID', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('appTemplatesPage.install') }))
    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    expect(await within(installDialog).findByText(i18next.t('projectVolumes.deploymentSelectorEmpty'))).toBeInTheDocument()
    const initialIdentifier = (within(installDialog).getByDisplayValue(/^redis-/) as HTMLInputElement).value

    await user.click(within(installDialog).getByRole('button', { name: i18next.t('projectVolumes.create') }))
    const createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    expect(within(createDialog).getByDisplayValue('Primary cluster')).toBeDisabled()
    const blankSource = within(createDialog).getByRole('option', { name: i18next.t('projectVolumes.sourceBlank') }) as HTMLOptionElement
    const filesystemMode = within(createDialog).getByRole('option', { name: i18next.t('deploymentsPage.kubernetesValues.Filesystem') }) as HTMLOptionElement
    expect(blankSource.selected).toBe(true)
    expect(blankSource.parentElement).toBeDisabled()
    expect(filesystemMode.selected).toBe(true)
    expect(filesystemMode.parentElement).toBeDisabled()

    await user.type(within(createDialog).getByPlaceholderText(i18next.t('projectVolumes.displayNamePlaceholder')), 'Redis data')
    const storageClass = await within(createDialog).findByRole('option', { name: i18next.t('projectVolumes.storageClassDefault', { name: 'fast' }) })
    await user.selectOptions(storageClass.parentElement as HTMLSelectElement, 'fast')
    await user.click(within(createDialog).getByRole('button', { name: i18next.t('projectVolumes.create') }))

    await waitFor(() => expect(mocks.createProjectVolume).toHaveBeenCalledWith('prj_1', {
      accessMode: 'ReadWriteOnce',
      capacity: '10Gi',
      clusterId: 'cluster_1',
      displayName: 'Redis data',
      source: { type: 'blank' },
      storageClassName: 'fast',
      volumeMode: 'Filesystem',
    }, expect.any(String)))

    await within(installDialog).findByRole('option', { name: 'Redis data · 10Gi' })
    expect(within(installDialog).getAllByRole('combobox').some(element => (element as HTMLSelectElement).value === 'pvol_new')).toBe(true)
    expect(within(installDialog).getByDisplayValue(initialIdentifier)).toBeInTheDocument()
    const installButton = within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') })
    expect(installButton).toBeEnabled()
    await user.click(installButton)

    await waitFor(() => expect(mocks.installAppTemplate).toHaveBeenCalledWith('prj_1', 'redis', expect.objectContaining({
      clusterId: 'cluster_1',
      projectVolumeId: 'pvol_new',
    })))
  })

  it('uses the project detail role and keeps viewers from creating or installing', async () => {
    mocks.getProject.mockResolvedValue({
      id: 'prj_1',
      name: 'Project One',
      identifier: 'project-one',
      description: '',
      currentUserRole: 'viewer',
    })
    const availableVolume = {
      ...createdVolume,
      id: 'pvol_existing',
      displayName: 'Existing Redis data',
      lifecycleState: 'ready',
      pendingOperation: '',
      availability: 'available',
    }
    mocks.listProjectVolumes.mockResolvedValue(page([availableVolume]))
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('appTemplatesPage.install') }))
    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    const volumeOption = await within(installDialog).findByRole('option', { name: 'Existing Redis data · 10Gi' })
    await user.selectOptions(volumeOption.parentElement as HTMLSelectElement, 'pvol_existing')
    await waitFor(() => expect(mocks.getProject).toHaveBeenCalledWith('prj_1'))

    expect(within(installDialog).queryByRole('button', { name: i18next.t('projectVolumes.create') })).not.toBeInTheDocument()
    expect(within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') })).toBeDisabled()
  })

  it('prevents dismissing an in-flight volume creation', async () => {
    let resolveCreate!: (volume: typeof createdVolume) => void
    mocks.createProjectVolume.mockImplementation(() => new Promise((resolve) => {
      resolveCreate = resolve
    }))
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('appTemplatesPage.install') }))
    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    await user.click(await within(installDialog).findByRole('button', { name: i18next.t('projectVolumes.create') }))
    const createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    await user.type(within(createDialog).getByPlaceholderText(i18next.t('projectVolumes.displayNamePlaceholder')), 'Redis data')
    const storageClass = await within(createDialog).findByRole('option', { name: i18next.t('projectVolumes.storageClassDefault', { name: 'fast' }) })
    await user.selectOptions(storageClass.parentElement as HTMLSelectElement, 'fast')
    await user.click(within(createDialog).getByRole('button', { name: i18next.t('projectVolumes.create') }))
    await waitFor(() => expect(mocks.createProjectVolume).toHaveBeenCalledOnce())

    expect(within(createDialog).getByRole('button', { name: i18next.t('common.cancel') })).toBeDisabled()
    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog', { name: i18next.t('projectVolumes.create') })).toBeInTheDocument()

    resolveCreate(createdVolume)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: i18next.t('projectVolumes.create') })).not.toBeInTheDocument())
    expect(within(installDialog).getAllByRole('combobox').some(element => (element as HTMLSelectElement).value === 'pvol_new')).toBe(true)
  })

  it('keeps the volume form and idempotency key available for retry after an ambiguous failure', async () => {
    mocks.createProjectVolume.mockRejectedValueOnce(new Error('network response lost')).mockResolvedValueOnce(createdVolume)
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('appTemplatesPage.install') }))
    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    await user.click(await within(installDialog).findByRole('button', { name: i18next.t('projectVolumes.create') }))
    const createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    const nameInput = within(createDialog).getByPlaceholderText(i18next.t('projectVolumes.displayNamePlaceholder'))
    await user.type(nameInput, 'Redis data')
    const storageClass = await within(createDialog).findByRole('option', { name: i18next.t('projectVolumes.storageClassDefault', { name: 'fast' }) })
    await user.selectOptions(storageClass.parentElement as HTMLSelectElement, 'fast')
    const createButton = within(createDialog).getByRole('button', { name: i18next.t('projectVolumes.create') })

    await user.click(createButton)
    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith('network response lost'))
    expect(screen.getByRole('dialog', { name: i18next.t('projectVolumes.create') })).toBeInTheDocument()
    expect(nameInput).toHaveValue('Redis data')
    const firstIdempotencyKey = mocks.createProjectVolume.mock.calls[0][2]
    expect(firstIdempotencyKey).toEqual(expect.any(String))

    await waitFor(() => expect(createButton).toBeEnabled())
    await user.click(createButton)
    await waitFor(() => expect(mocks.createProjectVolume).toHaveBeenCalledTimes(2))
    expect(mocks.createProjectVolume.mock.calls[1][2]).toBe(firstIdempotencyKey)
    await waitFor(() => expect(screen.queryByRole('dialog', { name: i18next.t('projectVolumes.create') })).not.toBeInTheDocument())
    expect(within(installDialog).getAllByRole('combobox').some(element => (element as HTMLSelectElement).value === 'pvol_new')).toBe(true)
  })

  it('resets quick-create values after cancellation', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('appTemplatesPage.install') }))
    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    const openCreateDialog = () => within(installDialog).getByRole('button', { name: i18next.t('projectVolumes.create') })
    await user.click(openCreateDialog())
    let createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    await user.type(within(createDialog).getByPlaceholderText(i18next.t('projectVolumes.displayNamePlaceholder')), 'Discard me')
    await user.click(within(createDialog).getByRole('button', { name: i18next.t('common.cancel') }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: i18next.t('projectVolumes.create') })).not.toBeInTheDocument())

    await user.click(openCreateDialog())
    createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    expect(within(createDialog).getByPlaceholderText(i18next.t('projectVolumes.displayNamePlaceholder'))).toHaveValue('')
  })

  it('locks quick creation to a block volume when the template declares a device path', async () => {
    mocks.getAppTemplate.mockResolvedValue({
      ...template,
      dataVolumes: [{ logicalName: 'data', sourceType: 'projectVolume', devicePath: '/dev/redis-data' }],
    })
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('appTemplatesPage.install') }))
    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    await user.click(await within(installDialog).findByRole('button', { name: i18next.t('projectVolumes.create') }))
    const createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    const blockMode = within(createDialog).getByRole('option', { name: i18next.t('deploymentsPage.kubernetesValues.Block') }) as HTMLOptionElement

    expect(blockMode.selected).toBe(true)
    expect(blockMode.parentElement).toBeDisabled()
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/app-templates']}>
        <TooltipProvider>
          <AppTemplatesPage />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function page<T>(items: T[], sortBy = 'displayName') {
  return {
    items,
    page: 1,
    pageSize: 20,
    sortBy,
    sortOrder: 'asc',
    total: items.length,
    totalPages: items.length > 0 ? 1 : 0,
  }
}
