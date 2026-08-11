import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { PlatformRole } from '@/lib/roles'
import { AccountMFAPanel } from './account-mfa-panel'
import { AccountPasswordPanel } from './account-password-panel'
import { UsersPage } from './UsersPage'

const mocks = vi.hoisted(() => ({
  enrollMFA: vi.fn(),
  getAuthRegistrationStatus: vi.fn(),
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
      getAuthRegistrationStatus: mocks.getAuthRegistrationStatus,
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
      role: PlatformRole.Admin,
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
    mocks.getAuthRegistrationStatus.mockResolvedValue({ externalIdentityPasswordEnabled: true })
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
        role: PlatformRole.User,
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
    expect(password).toHaveAttribute('name', 'password')
    expect(password?.closest('form')?.querySelector('input[autocomplete="username"]')).toHaveValue('admin@example.test')
    await user.type(password as HTMLInputElement, 'current-password')
    await user.click(screen.getByRole('button', { name: i18next.t('accountPage.mfa.continueEnrollment') }))

    await waitFor(() => expect(mocks.enrollMFA.mock.calls[0]?.[0]).toEqual({ currentPassword: 'current-password' }))
    const oneTimeCode = await screen.findByRole('textbox', { name: i18next.t('accountPage.mfa.otpPlaceholder') })
    expect(oneTimeCode).toHaveAttribute('autocomplete', 'one-time-code')
    expect(oneTimeCode.closest('form')?.querySelector('input[autocomplete="username"]')).toHaveValue('admin@example.test')
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

  it('associates a password change with the signed-in credential', () => {
    renderPage(<AccountPasswordPanel />)

    const currentPassword = document.querySelector('input[autocomplete="current-password"]')
    expect(currentPassword).toBeInstanceOf(HTMLInputElement)
    const form = (currentPassword as HTMLInputElement).closest('form')
    expect(currentPassword).toHaveAttribute('autocomplete', 'current-password')
    expect(form?.querySelector('input[autocomplete="username"]')).toHaveValue('admin@example.test')
    expect(form?.querySelectorAll('input[autocomplete="new-password"]')).toHaveLength(2)
  })

  it('associates an administrator password reset with the target user credential', async () => {
    const user = userEvent.setup()
    renderPage(<UsersPage />)

    await user.click(await screen.findByRole('button', { name: i18next.t('edit') }))
    await screen.findByRole('heading', { name: i18next.t('usersPage.editTitle') })
    const newPassword = document.querySelector('input[autocomplete="new-password"]')
    expect(newPassword).toBeInstanceOf(HTMLInputElement)
    const form = (newPassword as HTMLInputElement).closest('form')
    expect(newPassword).toHaveAttribute('autocomplete', 'new-password')
    expect(form?.querySelector('input[autocomplete="username"]')).toHaveValue('target@example.test')
  })
})
