import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ProjectBuildVariableSetsPage } from './ProjectBuildVariableSetsPage'
import { ProjectHooksPage } from './ProjectHooksPage'
import { ProjectRuntimeConfigSetsPage } from './ProjectRuntimeConfigSetsPage'

const mocks = vi.hoisted(() => ({
  listBuildVariableSetsPage: vi.fn(),
  listProjectHooksPage: vi.fn(),
  listProjectRuntimeConfigSetsPage: vi.fn(),
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

function emptyPage() {
  return { items: [], page: 1, pageSize: 10, sortBy: 'createdAt', sortOrder: 'desc' as const, total: 0, totalPages: 0 }
}

function renderWithQueryClient(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('project shared delivery config pages loading and error states', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('shows loading skeletons instead of empty state on ProjectBuildVariableSetsPage and retries after error', async () => {
    mocks.listBuildVariableSetsPage.mockReturnValueOnce(new Promise(() => undefined))
    const { container, unmount } = renderWithQueryClient(<ProjectBuildVariableSetsPage projectId="prj_1" />)

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.queryByText(i18next.t('buildsPage.emptyVariableSets'))).not.toBeInTheDocument()
    unmount()

    const user = userEvent.setup()
    mocks.listBuildVariableSetsPage
      .mockRejectedValueOnce(new Error('network error'))
      .mockResolvedValueOnce(emptyPage())
    renderWithQueryClient(<ProjectBuildVariableSetsPage projectId="prj_1" />)

    expect(await screen.findByText(i18next.t('buildsPage.variableSetLoadFailedTitle'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('buildsPage.emptyVariableSets'))).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(await screen.findByText(i18next.t('buildsPage.emptyVariableSets'))).toBeInTheDocument()
  })

  it('shows loading skeletons instead of empty state on ProjectRuntimeConfigSetsPage and retries after error', async () => {
    mocks.listProjectRuntimeConfigSetsPage.mockReturnValueOnce(new Promise(() => undefined))
    const { container, unmount } = renderWithQueryClient(<ProjectRuntimeConfigSetsPage projectId="prj_1" />)

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.queryByText(i18next.t('runtimeConfigSets.emptyTitle'))).not.toBeInTheDocument()
    unmount()

    const user = userEvent.setup()
    mocks.listProjectRuntimeConfigSetsPage
      .mockRejectedValueOnce(new Error('network error'))
      .mockResolvedValueOnce(emptyPage())
    renderWithQueryClient(<ProjectRuntimeConfigSetsPage projectId="prj_1" />)

    expect(await screen.findByText(i18next.t('runtimeConfigSets.loadFailedTitle'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('runtimeConfigSets.emptyTitle'))).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(await screen.findByText(i18next.t('runtimeConfigSets.emptyTitle'))).toBeInTheDocument()
  })

  it('renders DataList skeletons while ProjectHooksPage is pending instead of empty state, and retries on error', async () => {
    mocks.listProjectHooksPage.mockReturnValueOnce(new Promise(() => undefined))
    const { container, unmount } = renderWithQueryClient(<ProjectHooksPage projectId="prj_1" />)

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.queryByText(i18next.t('projectHooks.emptyTitle'))).not.toBeInTheDocument()
    unmount()

    const user = userEvent.setup()
    mocks.listProjectHooksPage
      .mockRejectedValueOnce(new Error('network error'))
      .mockResolvedValueOnce(emptyPage())
    renderWithQueryClient(<ProjectHooksPage projectId="prj_1" />)

    expect(await screen.findByText(i18next.t('projectHooks.loadFailedTitle'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('projectHooks.emptyTitle'))).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    await waitFor(() => expect(screen.getByText(i18next.t('projectHooks.emptyTitle'))).toBeInTheDocument())
    expect(screen.getByText(i18next.t('projectHooks.emptyDescription'))).toBeInTheDocument()
  })
})
