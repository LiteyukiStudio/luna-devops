import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { AccountMFAPanel } from './account-mfa-panel'
import { UsersPage } from './UsersPage'

const mocks = vi.hoisted(() => ({
  enrollMFA: vi.fn(),
  getMFAStatus: vi.fn(),
  listUsers: vi.fn(),
  resetUserMFA: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      enrollMFA: mocks.enrollMFA,
      getMFAStatus: mocks.getMFAStatus,
      listUsers: mocks.listUsers,
      resetUserMFA: mocks.resetUserMFA,
    },
  }
})

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    user: {
      passwordSet: true,
      avatarUrl: '',
      email: 'admin@example.test',
      id: 'usr_admin',
      language: 'en-US',
      name: 'Admin',
      permissions: ['user.manage'],
      role: 'platform_admin',
    },
  }),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

function renderPage(page: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>{page}</TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('mfa settings flows', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getMFAStatus.mockResolvedValue({
      confirmedAt: null,
      enabled: false,
      enrollmentReauthMode: 'password',
      pending: false,
      policyEnabled: false,
      recoveryCodesRemaining: 0,
    })
    mocks.enrollMFA.mockResolvedValue({
      otpauthUrl: 'otpauth://totp/Luna%20DevOps:test',
      secret: 'TESTSECRET',
    })
    mocks.resetUserMFA.mockResolvedValue(undefined)
    mocks.listUsers.mockResolvedValue({
      items: [{
        passwordSet: true,
        avatarUrl: '',
        balanceCredits: '0',
        createdAt: '2026-07-12T00:00:00Z',
        disabled: false,
        email: 'target@example.test',
        id: 'usr_target',
        language: 'en-US',
        mfaEnabled: true,
        name: 'Target User',
        role: 'user',
      }],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
  })

  it('reauthenticates local enrollment with the current password', async () => {
    const user = userEvent.setup()
    renderPage(<AccountMFAPanel />)

    await user.click(await screen.findByRole('button', { name: i18next.t('accountPage.mfa.enable') }))
    await screen.findByRole('heading', { name: i18next.t('accountPage.mfa.reauthTitle') })
    const password = document.querySelector('input[autocomplete="current-password"]')
    expect(password).toBeInstanceOf(HTMLInputElement)
    await user.type(password as HTMLInputElement, 'current-password')
    await user.click(screen.getByRole('button', { name: i18next.t('accountPage.mfa.continueEnrollment') }))

    await waitFor(() => expect(mocks.enrollMFA.mock.calls[0]?.[0]).toEqual({ currentPassword: 'current-password' }))
  })

  it('confirms an administrator reset for another user', async () => {
    const user = userEvent.setup()
    renderPage(<UsersPage />)

    const resetTrigger = await screen.findByRole('button', { name: i18next.t('usersPage.resetMFA') })
    await user.click(resetTrigger)
    const resetButtons = await screen.findAllByRole('button', { name: i18next.t('usersPage.resetMFA') })
    await user.click(resetButtons.at(-1)!)

    await waitFor(() => expect(mocks.resetUserMFA.mock.calls[0]?.[0]).toBe('usr_target'))
  })
})
