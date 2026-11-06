import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'
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
  featured: false,
  image: 'redis:7-alpine',
  version: '7',
  servicePort: 6379,
  defaultReplicas: 1,
  defaultCPU: '500m',
  defaultMemory: '512Mi',
  dataVolumes: [{ logicalName: 'data', sourceType: 'projectVolume' as const, mountPath: '/data' }],
  components: [],
  values: [{
    key: 'password',
    label: 'Password',
    description: 'Optional Redis password',
    default: '',
    required: false,
    secret: true,
    autoGenerate: false,
  }],
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
    mocks.listAppTemplates.mockResolvedValue([{ ...template, valueCount: 1, requiredValueCount: 0 }])
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
      currentUserActions: ['project:read', 'project:write', 'volume:read', 'volume:write'],
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

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
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

  it('offers the optional recommended Redis password and submits it as a template value', async () => {
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

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
    const passwordLabel = i18next.t('appTemplatesPage.valueLabels.password')
    const passwordInput = within(installDialog).getByPlaceholderText(i18next.t('appTemplatesPage.valuePlaceholders.redisPassword'))
    expect(passwordInput).toHaveAccessibleName(passwordLabel)
    expect(passwordInput).toHaveAttribute('type', 'password')
    expect(passwordInput).toHaveAttribute('autocomplete', 'new-password')
    expect(passwordInput).not.toBeRequired()
    expect(passwordInput).toHaveAttribute('placeholder', i18next.t('appTemplatesPage.valuePlaceholders.redisPassword'))

    await user.hover(within(installDialog).getByRole('button', { name: `${passwordLabel}${i18next.t('common.helpSuffix')}` }))
    expect(await screen.findByRole('tooltip')).toHaveTextContent(i18next.t('appTemplatesPage.valueHints.redisPassword'))

    const volumeOption = await within(installDialog).findByRole('option', { name: 'Existing Redis data · 10Gi' })
    await user.selectOptions(volumeOption.parentElement as HTMLSelectElement, 'pvol_existing')
    await user.type(passwordInput, 'redis-test-password')
    await user.click(within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') }))

    await waitFor(() => expect(mocks.installAppTemplate).toHaveBeenCalledWith('prj_1', 'redis', expect.objectContaining({
      values: { password: 'redis-test-password' },
    })))
    expect(await screen.findByTestId('location')).toHaveTextContent('/projects/prj_1/apps/app_1?tab=deployments')
  })

  it('uses the project detail actions and keeps viewers from creating or installing', async () => {
    mocks.getProject.mockResolvedValue({
      id: 'prj_1',
      name: 'Project One',
      identifier: 'project-one',
      description: '',
      currentUserRole: 'viewer',
      currentUserActions: ['project:read', 'volume:read'],
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

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
    const volumeOption = await within(installDialog).findByRole('option', { name: 'Existing Redis data · 10Gi' })
    await user.selectOptions(volumeOption.parentElement as HTMLSelectElement, 'pvol_existing')
    await waitFor(() => expect(mocks.getProject).toHaveBeenCalledWith('prj_1'))

    expect(within(installDialog).queryByRole('button', { name: i18next.t('projectVolumes.create') })).not.toBeInTheDocument()
    expect(within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') })).toBeDisabled()
    expect(within(installDialog).getByText(i18next.t('appTemplatesPage.projectWriteRequired'))).toBeInTheDocument()
  })

  it('authorizes template installation separately from inline volume creation', async () => {
    mocks.getProject.mockResolvedValue({
      id: 'prj_1',
      name: 'Project One',
      identifier: 'project-one',
      description: '',
      currentUserActions: ['project:read', 'project:write', 'volume:read'],
    })
    mocks.listProjectVolumes.mockResolvedValue(page([{
      ...createdVolume,
      id: 'pvol_existing',
      displayName: 'Existing Redis data',
      lifecycleState: 'ready',
      pendingOperation: '',
      availability: 'available',
    }]))
    const user = userEvent.setup()
    renderPage()

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
    const volumeOption = await within(installDialog).findByRole('option', { name: 'Existing Redis data · 10Gi' })
    await user.selectOptions(volumeOption.parentElement as HTMLSelectElement, 'pvol_existing')
    await waitFor(() => expect(mocks.getProject).toHaveBeenCalledWith('prj_1'))

    expect(within(installDialog).queryByRole('button', { name: i18next.t('projectVolumes.create') })).not.toBeInTheDocument()
    expect(within(installDialog).queryByText(i18next.t('appTemplatesPage.projectWriteRequired'))).not.toBeInTheDocument()
    expect(within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') })).toBeEnabled()
  })

  it('prevents dismissing an in-flight volume creation', async () => {
    let resolveCreate!: (volume: typeof createdVolume) => void
    mocks.createProjectVolume.mockImplementation(() => new Promise((resolve) => {
      resolveCreate = resolve
    }))
    const user = userEvent.setup()
    renderPage()

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
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

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
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

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
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

  it('installs a multi-component template with one volume per persistent component', async () => {
    const component = (key: string, name: string, persistent: boolean) => ({
      key,
      name,
      image: `example/${key}:latest`,
      servicePort: 8080,
      defaultReplicas: 1,
      defaultCPU: '500m',
      defaultMemory: '512Mi',
      dataVolumes: persistent ? [{ logicalName: 'data', sourceType: 'projectVolume' as const, mountPath: '/data' }] : [],
    })
    const blogTemplate = {
      ...template,
      id: 'neo-blog',
      slug: 'neo-blog',
      name: 'Neo Blog',
      image: '',
      servicePort: 0,
      dataVolumes: [],
      components: [component('web', 'Frontend', false), component('api', 'Backend', true), component('postgres', 'PostgreSQL', true)],
      values: [{ key: 'databasePassword', label: 'Database password', description: '', default: '', required: true, secret: true, autoGenerate: true }],
    }
    mocks.listAppTemplates.mockResolvedValue([{ ...blogTemplate, valueCount: 1, requiredValueCount: 1 }])
    mocks.getAppTemplate.mockResolvedValue(blogTemplate)
    mocks.listProjectVolumes.mockResolvedValue(page([
      { ...createdVolume, id: 'pvol_api', displayName: 'Blog uploads', lifecycleState: 'ready', pendingOperation: '', availability: 'available' },
      { ...createdVolume, id: 'pvol_db', displayName: 'Blog database', lifecycleState: 'ready', pendingOperation: '', availability: 'available' },
    ]))
    const user = userEvent.setup()
    renderPage()

    const installDialog = await openInstallSheet(user, 'Neo Blog')
    await expandCustomParameters(user, installDialog)
    expect(within(installDialog).queryByText(i18next.t('appTemplatesPage.imageRef'))).not.toBeInTheDocument()
    expect(within(installDialog).getByText('example/web:latest')).toBeInTheDocument()
    const identifier = within(installDialog).getByDisplayValue(/^neo-blog/) as HTMLInputElement
    expect(identifier.value.length + '-postgres'.length).toBeLessThanOrEqual(22)
    const installButton = within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') })

    const apiVolume = within(installDialog).getByText(i18next.t('appTemplatesPage.componentProjectVolume', { name: 'Backend' })).closest('div')!.parentElement!
    await user.selectOptions(within(apiVolume).getAllByRole('combobox')[0], await within(apiVolume).findByRole('option', { name: 'Blog uploads · 10Gi' }))
    expect(installButton).toBeEnabled()
    await user.click(installButton)
    expect(await within(installDialog).findByText(i18next.t('appTemplatesPage.validation.projectVolumeRequired'))).toBeInTheDocument()
    expect(mocks.installAppTemplate).not.toHaveBeenCalled()
    const databaseVolume = within(installDialog).getByText(i18next.t('appTemplatesPage.componentProjectVolume', { name: 'PostgreSQL' })).closest('div')!.parentElement!
    await user.selectOptions(within(databaseVolume).getAllByRole('combobox')[0], await within(databaseVolume).findByRole('option', { name: 'Blog database · 10Gi' }))
    expect(installButton).toBeEnabled()
    await user.click(installButton)

    await waitFor(() => expect(mocks.installAppTemplate).toHaveBeenCalledWith('prj_1', 'neo-blog', expect.objectContaining({
      componentProjectVolumeIds: { api: 'pvol_api', postgres: 'pvol_db' },
      cpuRequest: '',
      imageRef: '',
      memoryRequest: '',
      projectVolumeId: '',
      replicas: 0,
    })))
  })

  it('locks quick creation to a block volume when the template declares a device path', async () => {
    mocks.getAppTemplate.mockResolvedValue({
      ...template,
      dataVolumes: [{ logicalName: 'data', sourceType: 'projectVolume', devicePath: '/dev/redis-data' }],
    })
    const user = userEvent.setup()
    renderPage()

    const installDialog = await openInstallSheet(user, 'Redis')
    await expandCustomParameters(user, installDialog)
    await user.click(await within(installDialog).findByRole('button', { name: i18next.t('projectVolumes.create') }))
    const createDialog = await screen.findByRole('dialog', { name: i18next.t('projectVolumes.create') })
    const blockMode = within(createDialog).getByRole('option', { name: i18next.t('deploymentsPage.kubernetesValues.Block') }) as HTMLOptionElement

    expect(blockMode.selected).toBe(true)
    expect(blockMode.parentElement).toBeDisabled()
  })

  it('filters templates by category chips with counts and pressed state', async () => {
    mocks.listAppTemplates.mockResolvedValue([
      { ...template, valueCount: 1, requiredValueCount: 0 },
      { ...template, id: 'postgresql', slug: 'postgresql', name: 'PostgreSQL', category: 'database', popularityWeight: 90, valueCount: 0, requiredValueCount: 0 },
      { ...template, id: 'mysql', slug: 'mysql', name: 'MySQL', category: 'database', popularityWeight: 80, valueCount: 0, requiredValueCount: 0 },
    ])
    const user = userEvent.setup()
    renderPage()

    const filters = await screen.findByRole('group', { name: i18next.t('appTemplatesPage.categoryFilter') })
    const all = await within(filters).findByRole('button', { name: `${i18next.t('appTemplatesPage.allCategories')} 3` })
    const database = within(filters).getByRole('button', { name: `${i18next.t('appTemplatesPage.categories.database')} 2` })
    expect(all).toHaveAttribute('aria-pressed', 'true')
    expect(database).toHaveAttribute('aria-pressed', 'false')

    await user.click(database)
    expect(database).toHaveAttribute('aria-pressed', 'true')
    expect(all).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: /^PostgreSQL/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^MySQL/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Redis/ })).not.toBeInTheDocument()

    await user.type(screen.getByRole('searchbox', { name: i18next.t('appTemplatesPage.searchPlaceholder') }), 'nothing-matches')
    expect(screen.getByText(i18next.t('appTemplatesPage.noResultsTitle'))).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: i18next.t('appTemplatesPage.clearFilters') }))
    expect(screen.getByRole('button', { name: /^Redis/ })).toBeInTheDocument()
    expect(within(filters).getByRole('button', { name: `${i18next.t('appTemplatesPage.allCategories')} 3` })).toHaveAttribute('aria-pressed', 'true')
  })

  it('shows featured templates in their own section only while browsing all templates', async () => {
    mocks.listAppTemplates.mockResolvedValue([
      { ...template, valueCount: 1, requiredValueCount: 0 },
      { ...template, id: 'postgresql', slug: 'postgresql', name: 'PostgreSQL', category: 'database', featured: true, popularityWeight: 90, valueCount: 0, requiredValueCount: 0 },
      { ...template, id: 'mysql', slug: 'mysql', name: 'MySQL', category: 'database', popularityWeight: 80, valueCount: 0, requiredValueCount: 0 },
    ])
    const user = userEvent.setup()
    renderPage()

    const featured = await screen.findByRole('region', { name: i18next.t('appTemplatesPage.featuredSection') })
    const all = screen.getByRole('region', { name: i18next.t('appTemplatesPage.allTemplatesSection') })
    expect(within(featured).getByRole('button', { name: /^PostgreSQL/ })).toBeInTheDocument()
    expect(within(featured).queryByRole('button', { name: /^Redis/ })).not.toBeInTheDocument()
    expect(within(all).getByRole('button', { name: /^Redis/ })).toBeInTheDocument()
    expect(within(all).queryByRole('button', { name: /^PostgreSQL/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: `${i18next.t('appTemplatesPage.categories.database')} 2` }))
    expect(screen.queryByRole('region', { name: i18next.t('appTemplatesPage.featuredSection') })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /^PostgreSQL/ })).toHaveLength(1)

    await user.click(screen.getByRole('button', { name: `${i18next.t('appTemplatesPage.allCategories')} 3` }))
    await user.type(screen.getByRole('searchbox', { name: i18next.t('appTemplatesPage.searchPlaceholder') }), 'sql')
    expect(screen.queryByRole('region', { name: i18next.t('appTemplatesPage.featuredSection') })).not.toBeInTheDocument()
  })

  it('opens the install sheet from the whole card with the keyboard', async () => {
    const user = userEvent.setup()
    renderPage()

    const card = await screen.findByRole('button', { name: /^Redis/ })
    card.focus()
    await user.keyboard('{Enter}')

    const installDialog = await screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name: 'Redis' }) })
    expect(within(installDialog).getByText(i18next.t('appTemplatesPage.templates.redis.description'))).toBeInTheDocument()
    expect(within(installDialog).getByRole('link', { name: i18next.t('appTemplatesPage.officialWebsite') })).toHaveAttribute('href', 'https://redis.io')
  })

  it('keeps custom parameters collapsed until a validation error needs them', async () => {
    const user = userEvent.setup()
    renderPage()

    const installDialog = await openInstallSheet(user, 'Redis')
    const customParameters = await within(installDialog).findByRole('button', { name: i18next.t('appTemplatesPage.customParameters') })
    expect(customParameters).toHaveAttribute('aria-expanded', 'false')
    expect(within(installDialog).getByLabelText(new RegExp(`^${i18next.t('appTemplatesPage.applicationName')}`))).toHaveValue('Redis')
    expect(within(installDialog).queryByDisplayValue('redis:7-alpine')).not.toBeInTheDocument()
    expect(customParameters).toHaveTextContent('redis:7-alpine')

    const installButton = within(installDialog).getByRole('button', { name: i18next.t('appTemplatesPage.install') })
    await waitFor(() => expect(installButton).toBeEnabled())
    await user.click(installButton)

    await waitFor(() => expect(customParameters).toHaveAttribute('aria-expanded', 'true'))
    expect(within(installDialog).getByDisplayValue('redis:7-alpine')).toBeInTheDocument()
    expect(within(installDialog).getByText(i18next.t('appTemplatesPage.validation.projectVolumeRequired'))).toBeInTheDocument()
    expect(mocks.installAppTemplate).not.toHaveBeenCalled()
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
          <LocationProbe />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function LocationProbe() {
  const location = useLocation()
  return <output data-testid="location">{`${location.pathname}${location.search}`}</output>
}

async function openInstallSheet(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(await screen.findByRole('button', { name: new RegExp(`^${name}`) }))
  return screen.findByRole('dialog', { name: i18next.t('appTemplatesPage.installDialogTitle', { name }) })
}

async function expandCustomParameters(user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement) {
  await user.click(await within(dialog).findByRole('button', { name: i18next.t('appTemplatesPage.customParameters') }))
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
