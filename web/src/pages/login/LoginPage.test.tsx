import type { SessionContextValue } from '@/app/session-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionContext } from '@/app/session-context'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { LoginPage } from './LoginPage'

const mocks = vi.hoisted(() => ({
  getAuthRegistrationStatus: vi.fn(),
  listAuthProviders: vi.fn(),
  login: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getAuthRegistrationStatus: mocks.getAuthRegistrationStatus,
      listAuthProviders: mocks.listAuthProviders,
    },
  }
})

vi.mock('@/app/public-config-context', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/app/public-config-context')>()
  return {
    ...actual,
    usePublicConfig: () => ({}),
  }
})

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}))

function renderLogin(overrides: Partial<SessionContextValue> = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const session: SessionContextValue = {
    initialized: true,
    isLoading: false,
    isLoggingIn: false,
    isLoggingOut: false,
    login: mocks.login,
    logout: vi.fn(async () => {}),
    recentLoginUsers: [],
    refreshUser: vi.fn(async () => {}),
    updateLanguage: vi.fn(),
    updateProfile: vi.fn(),
    user: undefined,
    ...overrides,
  }
  return render(
    <QueryClientProvider client={queryClient}>
      <SessionContext value={session}>
        <TooltipProvider>
          <MemoryRouter initialEntries={['/login']}>
            <LoginPage />
          </MemoryRouter>
        </TooltipProvider>
      </SessionContext>
    </QueryClientProvider>,
  )
}

describe('login page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.listAuthProviders.mockResolvedValue([{ id: 'oidc_1', name: 'Liteyuki SSO' }])
    mocks.getAuthRegistrationStatus.mockResolvedValue({ emailRegistrationEnabled: true })
    mocks.login.mockResolvedValue(undefined)
  })

  it('keeps the submit button enabled for browser autofill and validates on submit', async () => {
    const user = userEvent.setup()
    renderLogin()

    const submit = screen.getByRole('button', { name: i18next.t('login') })
    expect(submit).toBeEnabled()

    await user.click(submit)
    expect(await screen.findByText(i18next.t('common.validEmailRequired'))).toBeInTheDocument()
    expect(screen.getByText(i18next.t('loginPage.passwordRequired'))).toBeInTheDocument()

    const registerLink = await screen.findByRole('link', { name: i18next.t('loginPage.registration.createAccount') })
    expect(registerLink).not.toHaveAttribute('type')
    expect(await screen.findByRole('button', { name: i18next.t('loginPage.useProviderLogin', { provider: 'Liteyuki SSO' }) })).toBeInTheDocument()
  })

  it('disables submit while logging in', () => {
    renderLogin({ isLoggingIn: true })
    expect(screen.getByRole('button', { name: i18next.t('login') })).toBeDisabled()
  })
})
