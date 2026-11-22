import type { ArtifactRegistry, RegistryCredential } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { RegistriesPage } from './RegistriesPage'

const mocks = vi.hoisted(() => ({
  listContainerImages: vi.fn(),
  listProjects: vi.fn(),
  listRegistries: vi.fn(),
  listRegistriesPage: vi.fn(),
  listAllRegistryCredentialsPage: vi.fn(),
  listRegistryCredentialsPage: vi.fn(),
  session: { user: { role: 'user' } },
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
      listContainerImages: mocks.listContainerImages,
      listProjects: mocks.listProjects,
      listRegistries: mocks.listRegistries,
      listRegistriesPage: mocks.listRegistriesPage,
      listAllRegistryCredentialsPage: mocks.listAllRegistryCredentialsPage,
      listRegistryCredentialsPage: mocks.listRegistryCredentialsPage,
    },
  }
})

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const registries: ArtifactRegistry[] = [
  registry('registry-one', 'Registry One'),
  registry('registry-two', 'Registry Two'),
]
const credentials: RegistryCredential[] = [
  credential('credential-one', 'registry-one', 'Credential One'),
  credential('credential-two', 'registry-two', 'Credential Two'),
]

describe('registry credentials', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.history.replaceState(null, '', '/registries#tab=credentials')
    await i18next.changeLanguage('en-US')
    mocks.listProjects.mockResolvedValue([])
    mocks.listRegistries.mockResolvedValue(registries)
    mocks.listRegistriesPage.mockResolvedValue(page(registries))
    mocks.listContainerImages.mockResolvedValue(page([]))
    mocks.listAllRegistryCredentialsPage.mockResolvedValue(page(credentials))
    mocks.listRegistryCredentialsPage.mockImplementation(async (registryId: string) =>
      page(credentials.filter(item => item.registryId === registryId)))
  })

  it('keeps the unfiltered list visible while editing a credential', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('Credential One')).toBeInTheDocument()
    expect(screen.getByText('Credential Two')).toBeInTheDocument()

    await user.click(screen.getAllByRole('button', { name: i18next.t('common.actions') })[0])
    await user.click(await screen.findByRole('menuitem', { name: i18next.t('edit') }))
    await screen.findByRole('heading', { name: i18next.t('registriesPage.editCredentialTitle') })

    const configuredSecret = screen.getByPlaceholderText(i18next.t('common.secretSetPlaceholder'))
    expect(configuredSecret).toHaveValue('')
    expect(screen.getByText('Credential One')).toBeInTheDocument()
    expect(screen.getByText('Credential Two')).toBeInTheDocument()
    expect(mocks.listRegistryCredentialsPage).not.toHaveBeenCalled()
  })

  it('renders skeletons for registry and project scope names while lookup queries are pending', async () => {
    mocks.listRegistries.mockReturnValue(new Promise(() => undefined))
    mocks.listRegistriesPage.mockReturnValue(new Promise(() => undefined))
    mocks.listProjects.mockReturnValue(new Promise(() => undefined))
    mocks.listAllRegistryCredentialsPage.mockResolvedValue(page([
      { ...credentials[0], scope: 'project', projectIds: ['prj_pending'] },
    ]))

    const { container } = renderPage()
    expect(await screen.findByText('Credential One')).toBeInTheDocument()
    expect(screen.queryByText('registry-one')).not.toBeInTheDocument()
    expect(screen.queryByText('prj_pending')).not.toBeInTheDocument()
    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThanOrEqual(2)
  })

  it('shows unavailable with retry instead of noOptions in CredentialDialog when registryOptions fails', async () => {
    const user = userEvent.setup()
    mocks.listRegistries
      .mockRejectedValueOnce(new Error('registries failed'))
      .mockResolvedValueOnce(registries)
    renderPage()

    await screen.findByText('Credential One')
    await user.click(screen.getByRole('button', { name: i18next.t('registriesPage.createCredentialTitle') }))
    const dialog = await screen.findByRole('dialog', { name: i18next.t('registriesPage.createCredentialTitle') })

    const registrySelect = within(dialog).getByRole('combobox', { name: new RegExp(i18next.t('registries')) })
    expect(registrySelect).toBeDisabled()
    expect(registrySelect).toHaveTextContent(i18next.t('common.unavailable'))
    expect(within(dialog).queryByText(i18next.t('common.noOptions'))).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: i18next.t('common.retry') }))
    expect(mocks.listRegistries).toHaveBeenCalledTimes(2)
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/registries#tab=credentials']}>
        <TooltipProvider>
          <RegistriesPage />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function registry(id: string, name: string): ArtifactRegistry {
  return {
    capabilities: ['push', 'pull'],
    createdAt: '2026-07-15T00:00:00Z',
    createdBy: 'usr_test',
    credentialSet: true,
    defaultProjectIds: [],
    endpoint: `https://${id}.example.test`,
    id,
    isDefault: false,
    name,
    namespace: 'devops',
    ownerRef: '',
    projectIds: [],
    provider: 'generic-oci',
    scope: 'global',
  }
}

function credential(id: string, registryId: string, name: string): RegistryCredential {
  return {
    createdAt: '2026-07-15T00:00:00Z',
    id,
    name,
    passwordSet: true,
    registryId,
    repositoryTemplate: '{registryNamespace}/{projectIdentifier}-{appIdentifier}',
    usage: 'push-pull',
    scope: 'user',
    ownerRef: 'user_test',
    projectIds: [],
    tagTemplate: 'latest',
    tokenSet: false,
    username: 'test-user',
  }
}

function page<T>(items: T[]) {
  return {
    items,
    page: 1,
    pageSize: 10,
    sortBy: 'createdAt',
    sortOrder: 'desc' as const,
    total: items.length,
    totalPages: items.length > 0 ? 1 : 0,
  }
}
