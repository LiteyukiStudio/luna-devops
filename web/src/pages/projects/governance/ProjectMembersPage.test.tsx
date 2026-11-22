import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { BillingOwnerTransferDialog } from './billing-owner-transfer-dialog'
import { ProjectMembersPage } from './ProjectMembersPage'

const mocks = vi.hoisted(() => ({
  getProjectNetworkPolicy: vi.fn(),
  listProjectMembersPage: vi.fn(),
  searchProjectMemberCandidates: vi.fn(),
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
  return { items: [], page: 1, pageSize: 10, sortBy: 'createdAt', sortOrder: 'asc' as const, total: 0, totalPages: 0 }
}

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

describe('project members and billing owner transfer loading/error states', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getProjectNetworkPolicy.mockResolvedValue({
      ingressMode: 'isolated',
      egressMode: 'internet',
      egressAllowlist: '',
      egressExceptions: [],
    })
  })

  it('shows skeletons while members are pending and renders mutually exclusive ErrorState on failure', async () => {
    mocks.listProjectMembersPage.mockReturnValueOnce(new Promise(() => undefined))
    const { container, unmount } = renderWithProviders(<ProjectMembersPage projectId="prj_1" />)

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.queryByText(i18next.t('projectMembers.emptyTitle'))).not.toBeInTheDocument()
    unmount()

    const user = userEvent.setup()
    mocks.listProjectMembersPage
      .mockRejectedValueOnce(new Error('members failed'))
      .mockResolvedValueOnce(emptyPage())
    renderWithProviders(<ProjectMembersPage projectId="prj_1" />)

    expect(await screen.findByText(i18next.t('projectMembers.loadFailedTitle'))).toBeInTheDocument()
    expect(screen.getAllByText(i18next.t('projectMembers.loadFailedTitle'))).toHaveLength(1)
    expect(screen.queryByText(i18next.t('projectMembers.emptyTitle'))).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    expect(await screen.findByText(i18next.t('projectMembers.emptyTitle'))).toBeInTheDocument()
  })

  it('shows search hint before typing, loading state while searching, and ErrorState with retry on candidate search failure', async () => {
    const user = userEvent.setup()
    mocks.listProjectMembersPage.mockResolvedValue(emptyPage())
    mocks.searchProjectMemberCandidates
      .mockRejectedValueOnce(new Error('candidate search failed'))
      .mockResolvedValueOnce([{ id: 'usr_2', name: 'Bob', email: 'bob@example.com' }])

    renderWithProviders(<ProjectMembersPage projectId="prj_1" />)

    await user.click(await screen.findByRole('button', { name: i18next.t('projectMembers.addTitle') }))
    const dialog = await screen.findByRole('dialog', { name: i18next.t('projectMembers.addTitle') })

    await user.click(within(dialog).getByRole('button', { name: i18next.t('projectMembers.userSearch') }))
    expect((await screen.findAllByText(i18next.t('projectMembers.userSearchHint'))).length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText(i18next.t('projectMembers.noUserResults'))).not.toBeInTheDocument()

    const searchInput = screen.getByPlaceholderText(i18next.t('projectMembers.userSearchPlaceholder'))
    await user.click(searchInput)
    await user.paste('bob')

    expect(await within(dialog).findByText(i18next.t('errors.request.failed'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('projectMembers.noUserResults'))).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: i18next.t('common.retry') }))
    await user.click(within(dialog).getByRole('button', { name: i18next.t('projectMembers.userSearch') }))
    expect(await screen.findByRole('button', { name: /Bob/ })).toBeInTheDocument()
  })

  it('renders skeletons while members load and ErrorState on failure in BillingOwnerTransferDialog instead of noCandidatesTitle', async () => {
    const user = userEvent.setup()
    const onRetryMembers = vi.fn()
    const project = {
      id: 'prj_1',
      identifier: 'demo',
      kubernetesNamespace: 'prj-demo',
      name: 'Demo',
      description: '',
      maxConcurrentBuilds: 2,
      webConsoleEnabled: true,
      billingOwnerUserId: 'usr_owner',
      systemKey: '',
      deleteStatus: 'active',
      deleteMessage: '',
      dashboardOrder: 0,
      useCount: 0,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    }

    const { rerender } = renderWithProviders(
      <BillingOwnerTransferDialog members={[]} membersLoading project={project} />,
    )

    await user.click(screen.getByRole('button', { name: i18next.t('projectSpaces.billingOwnerTransfer.trigger') }))
    const dialog = await screen.findByRole('dialog', { name: i18next.t('projectSpaces.billingOwnerTransfer.title') })
    expect(dialog.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(within(dialog).queryByText(i18next.t('projectSpaces.billingOwnerTransfer.noCandidatesTitle'))).not.toBeInTheDocument()

    rerender(
      <MemoryRouter>
        <QueryClientProvider client={new QueryClient()}>
          <TooltipProvider>
            <BillingOwnerTransferDialog
              members={[]}
              membersError
              project={project}
              onRetryMembers={onRetryMembers}
            />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    )

    expect(within(dialog).getByText(i18next.t('projectMembers.loadFailedTitle'))).toBeInTheDocument()
    expect(within(dialog).queryByText(i18next.t('projectSpaces.billingOwnerTransfer.noCandidatesTitle'))).not.toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: i18next.t('common.retry') }))
    expect(onRetryMembers).toHaveBeenCalledTimes(1)

    rerender(
      <MemoryRouter>
        <QueryClientProvider client={new QueryClient()}>
          <TooltipProvider>
            <BillingOwnerTransferDialog
              members={[
                { id: 'm_owner', projectId: 'prj_1', userId: 'usr_owner', role: 'owner', name: 'Owner', email: 'owner@example.com' },
                { id: 'm_admin', projectId: 'prj_1', userId: 'usr_admin', role: 'admin', name: 'Admin', email: 'admin@example.com' },
              ]}
              project={project}
            />
          </TooltipProvider>
        </QueryClientProvider>
      </MemoryRouter>,
    )

    const recipientTrigger = within(dialog).getByRole('combobox', { name: new RegExp(i18next.t('projectSpaces.billingOwnerTransfer.recipient')) })
    expect(recipientTrigger).toHaveAttribute('aria-describedby')
    expect(recipientTrigger).toHaveTextContent(i18next.t('projectSpaces.billingOwnerTransfer.recipientPlaceholder'))
  })
})
