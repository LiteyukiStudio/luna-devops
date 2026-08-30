import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { NotificationsPage } from './NotificationsPage'

const mocks = vi.hoisted(() => ({
  createNotificationRule: vi.fn(),
  listNotificationChannels: vi.fn(),
  listNotificationDeliveries: vi.fn(),
  listNotificationPresets: vi.fn(),
  listNotificationRules: vi.fn(),
  listNotificationTemplates: vi.fn(),
  listProjects: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      createNotificationRule: mocks.createNotificationRule,
      listNotificationChannels: mocks.listNotificationChannels,
      listNotificationDeliveries: mocks.listNotificationDeliveries,
      listNotificationPresets: mocks.listNotificationPresets,
      listNotificationRules: mocks.listNotificationRules,
      listNotificationTemplates: mocks.listNotificationTemplates,
      listProjects: mocks.listProjects,
    },
  }
})

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

describe('notification rule scope', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listNotificationPresets.mockResolvedValue([])
    mocks.listNotificationChannels.mockResolvedValue(page([{ id: 'nch_team', name: 'Team channel', adapterKind: 'webhook', enabled: true }]))
    mocks.listNotificationTemplates.mockResolvedValue(page([]))
    mocks.listNotificationRules.mockResolvedValue(page([]))
    mocks.listNotificationDeliveries.mockResolvedValue(page([]))
    mocks.listProjects.mockResolvedValue([{ id: 'prj_alpha', name: 'Alpha', identifier: 'alpha', description: '' }])
    mocks.createNotificationRule.mockResolvedValue({})
  })

  it('defaults to project scope and requires an explicit all selection', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('tab', { name: 'Rules' }))
    await user.click(screen.getByRole('button', { name: 'Create rule' }))

    expect(screen.getByRole('dialog', { name: 'Notification rule' })).toBeInTheDocument()
    const name = screen.getByText('Name', { selector: '[data-slot="field-label"] span' }).closest('[data-slot="field"]')?.querySelector('input')
    const scope = screen.getByText('Notification scope', { selector: '[data-slot="field-label"] span' }).closest('[data-slot="field"]')?.querySelector('select')
    expect(name).not.toBeNull()
    expect(scope).not.toBeNull()
    expect(scope).toHaveValue('projects')
    expect(screen.getByText('Select at least one project space before saving the rule.')).toBeInTheDocument()
    expect(mocks.listProjects).toHaveBeenCalledWith('all')

    await user.type(name!, 'Build failures')
    await user.click(screen.getByRole('checkbox', { name: 'Team channel' }))
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()

    await user.selectOptions(scope!, 'all')
    expect(screen.getByText(/matches events platform-wide/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()

    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.createNotificationRule).toHaveBeenCalledWith(expect.objectContaining({
      filter: { scope: 'all' },
    })))
  })

  it('blocks malformed advanced filter JSON instead of falling back to an empty filter', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('tab', { name: 'Rules' }))
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    const name = screen.getByText('Name', { selector: '[data-slot="field-label"] span' }).closest('[data-slot="field"]')?.querySelector('input')
    const scope = screen.getByText('Notification scope', { selector: '[data-slot="field-label"] span' }).closest('[data-slot="field"]')?.querySelector('select')
    await user.type(name!, 'Build failures')
    await user.click(screen.getByRole('checkbox', { name: 'Team channel' }))
    await user.selectOptions(scope!, 'all')

    const filterField = screen.getByText('Advanced filter JSON', { selector: '[data-slot="field-label"] span' }).closest('[data-slot="field"]')
    const filter = filterField?.querySelector('textarea')
    expect(filter).not.toBeNull()
    expect(filterField).toHaveClass('md:col-span-2')
    fireEvent.change(filter!, { target: { value: '{' } })

    expect(screen.getByText('Enter a valid JSON object and remove unknown fields or invalid arrays.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(mocks.createNotificationRule).not.toHaveBeenCalled()
  })
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false }, queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <NotificationsPage />
    </QueryClientProvider>,
  )
}

function page<T>(items: T[]) {
  return { items, page: 1, pageSize: 100, sortBy: 'createdAt', sortOrder: 'desc' as const, total: items.length, totalPages: items.length > 0 ? 1 : 0 }
}
