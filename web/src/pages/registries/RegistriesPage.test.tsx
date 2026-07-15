import type { ArtifactRegistry, RegistryCredential } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { RegistriesPage } from './RegistriesPage'

const mocks = vi.hoisted(() => ({
  listContainerImages: vi.fn(),
  listProjects: vi.fn(),
  listRegistries: vi.fn(),
  listRegistriesPage: vi.fn(),
  listRegistryCredentials: vi.fn(),
  listRegistryCredentialsPage: vi.fn(),
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
      listRegistryCredentials: mocks.listRegistryCredentials,
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
    mocks.listRegistryCredentials.mockImplementation(async (registryId: string) =>
      credentials.filter(item => item.registryId === registryId))
    mocks.listRegistryCredentialsPage.mockImplementation(async (registryId: string) =>
      page(credentials.filter(item => item.registryId === registryId)))
  })

  it('keeps the unfiltered list visible while editing a credential', async () => {
    const user = userEvent.setup()
    renderPage()

    expect(await screen.findByText('Credential One')).toBeInTheDocument()
    expect(screen.getByText('Credential Two')).toBeInTheDocument()

    await user.click(screen.getAllByRole('button', { name: i18next.t('edit') })[0])
    await screen.findByRole('heading', { name: i18next.t('registriesPage.editCredentialTitle') })

    expect(screen.getByText('Credential One')).toBeInTheDocument()
    expect(screen.getByText('Credential Two')).toBeInTheDocument()
    expect(mocks.listRegistryCredentialsPage).not.toHaveBeenCalled()
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RegistriesPage />
      </TooltipProvider>
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
    accessScope: 'personal',
    createdAt: '2026-07-15T00:00:00Z',
    id,
    name,
    passwordSet: true,
    registryId,
    repositoryTemplate: '{registryNamespace}/{projectSlug}-{appSlug}',
    scope: 'push-pull',
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
