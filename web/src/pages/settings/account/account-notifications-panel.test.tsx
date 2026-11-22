import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { AccountNotificationsPanel } from './account-notifications-panel'

const mocks = vi.hoisted(() => ({
  getMyNotificationPreferences: vi.fn(),
  listMyNotificationChannels: vi.fn(),
  listMyNotificationDeliveries: vi.fn(),
  listMyNotificationPresets: vi.fn(),
  session: {
    user: {
      email: 'owner@example.com',
      id: 'usr_owner',
      name: 'Owner',
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
      getMyNotificationPreferences: mocks.getMyNotificationPreferences,
      listMyNotificationChannels: mocks.listMyNotificationChannels,
      listMyNotificationDeliveries: mocks.listMyNotificationDeliveries,
      listMyNotificationPresets: mocks.listMyNotificationPresets,
    },
  }
})

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

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <AccountNotificationsPanel />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('account notifications panel error mutual exclusivity', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getMyNotificationPreferences.mockResolvedValue({ emailEnabled: true, eventTypes: ['build.failed'] })
    mocks.listMyNotificationDeliveries.mockResolvedValue(page([]))
  })

  it('replaces emptyChannels with retryable ErrorState when channels are empty and presets fail', async () => {
    mocks.listMyNotificationChannels.mockResolvedValue(page([]))
    mocks.listMyNotificationPresets
      .mockRejectedValueOnce(new Error('presets unavailable'))
      .mockResolvedValueOnce([])
    const user = userEvent.setup()

    renderPanel()

    expect(await screen.findByText(i18next.t('errors.request.failed'))).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('accountPage.notifications.emptyChannels'))).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.retry') }))
    await waitFor(() => expect(mocks.listMyNotificationPresets).toHaveBeenCalledTimes(2))
  })

  it('keeps existing channels visible with a single retryable ErrorState banner when presets fail', async () => {
    mocks.listMyNotificationChannels.mockResolvedValue(page([{
      adapterKind: 'webhook',
      createdAt: '2026-11-01T00:00:00Z',
      enabled: true,
      id: 'nch_1',
      name: 'Personal Feishu',
      presetId: 'feishu_bot',
      secretState: {},
      updatedAt: '2026-11-01T00:00:00Z',
    }]))
    mocks.listMyNotificationPresets.mockRejectedValue(new Error('presets unavailable'))

    renderPanel()

    expect(await screen.findByText('Personal Feishu')).toBeInTheDocument()
    expect(screen.getAllByText(i18next.t('errors.request.failed'))).toHaveLength(1)
  })
})
