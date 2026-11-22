import type { RepositoryBinding } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { RepositoryBindingsPage } from './RepositoryBindingsPage'

const mocks = vi.hoisted(() => ({
  listApplications: vi.fn(),
  listGitAccounts: vi.fn(),
  listGitBranches: vi.fn(),
  listGitProviders: vi.fn(),
  listGitRepositories: vi.fn(),
  listRepositoryBindings: vi.fn(),
  listRepositoryBindingsPage: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listApplications: mocks.listApplications,
      listGitAccounts: mocks.listGitAccounts,
      listGitBranches: mocks.listGitBranches,
      listGitProviders: mocks.listGitProviders,
      listGitRepositories: mocks.listGitRepositories,
      listRepositoryBindings: mocks.listRepositoryBindings,
      listRepositoryBindingsPage: mocks.listRepositoryBindingsPage,
    },
  }
})

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

const bindingFixture: RepositoryBinding = {
  id: 'rb_1',
  projectId: 'prj_1',
  applicationId: 'app_1',
  applicationName: 'Web App',
  gitProviderId: 'gp_1',
  gitAccountId: 'ga_1',
  owner: 'liteyuki',
  repo: 'devops',
  cloneUrl: 'https://github.com/liteyuki/devops.git',
  defaultBranch: 'main',
  webhookEnabled: true,
  webhookStatus: 'ready',
  webhookId: 'wh_1',
  webhookCallbackUrl: '',
  lastEvent: '',
  lastCommitSha: '',
  createdAt: '2026-11-01T00:00:00Z',
}

function renderPage(props?: { applicationId?: string, projectId?: string }) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <MemoryRouter initialEntries={['/projects/prj_1/repositories']}>
          <Routes>
            <Route
              element={<RepositoryBindingsPage {...props} />}
              path="/projects/:projectId/repositories"
            />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('repository bindings page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listGitProviders.mockResolvedValue([{ id: 'gp_1', name: 'GitHub', type: 'github', enabled: true }])
    mocks.listGitAccounts.mockResolvedValue([{ id: 'ga_1', providerId: 'gp_1', username: 'octo', scope: 'user' }])
    mocks.listApplications.mockResolvedValue([{ id: 'app_1', name: 'Web App', identifier: 'web-app' }])
    mocks.listGitRepositories.mockResolvedValue({ items: [], page: 1, pageSize: 50, total: 0, totalPages: 0 })
    mocks.listGitBranches.mockResolvedValue({ items: [{ name: 'main' }], limited: false })
    mocks.listRepositoryBindings.mockResolvedValue([bindingFixture])
    mocks.listRepositoryBindingsPage.mockResolvedValue({
      items: [bindingFixture],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
  })

  it('hides the empty state while bindings or git metadata queries are still pending', () => {
    mocks.listRepositoryBindingsPage.mockReturnValue(new Promise(() => undefined))
    renderPage()

    expect(screen.queryByText(i18next.t('repositories.emptyTitle'))).not.toBeInTheDocument()
    expect(screen.queryByText(i18next.t('repositories.loadFailedTitle'))).not.toBeInTheDocument()
  })

  it('renders a mutually exclusive DataList error state without the empty state and retries failed queries', async () => {
    const user = userEvent.setup()
    mocks.listGitAccounts.mockRejectedValueOnce(new Error('git accounts failed'))
    mocks.listRepositoryBindingsPage.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    })
    renderPage()

    expect(await screen.findByText(i18next.t('repositories.loadFailedTitle'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('repositories.emptyTitle'))).not.toBeInTheDocument()

    mocks.listGitAccounts.mockResolvedValueOnce([{ id: 'ga_1', providerId: 'gp_1', username: 'octo', scope: 'user' }])
    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))

    expect(await screen.findByText(i18next.t('repositories.emptyTitle'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('repositories.loadFailedTitle'))).not.toBeInTheDocument()
  })

  it('surfaces branch loading failures with a retry action inside the edit dialog', async () => {
    const user = userEvent.setup()
    mocks.listGitBranches.mockRejectedValueOnce(new Error('rate limited'))
    renderPage()

    expect(await screen.findByText('liteyuki/devops')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: i18next.t('common.actions') }))
    await user.click(await screen.findByRole('menuitem', { name: i18next.t('repositories.editAria') }))

    const dialog = await screen.findByRole('dialog', { name: i18next.t('repositories.editBinding') })
    const alert = await within(dialog).findByRole('alert')
    expect(within(alert).getByText(i18next.t('common.unavailable'))).toBeInTheDocument()

    mocks.listGitBranches.mockResolvedValueOnce({ items: [{ name: 'main' }, { name: 'release' }], limited: false })
    await user.click(within(alert).getByRole('button', { name: i18next.t('common.retry') }))
    expect(mocks.listGitBranches).toHaveBeenCalledTimes(2)
  })
})
