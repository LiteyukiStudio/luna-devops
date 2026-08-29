import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { LoginPage } from './LoginPage'
import { RegisterPage } from './RegisterPage'

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
      login: mocks.login,
    },
  }
})

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    initialized: true,
    isLoading: false,
    isLoggingIn: false,
    isLoggingOut: false,
    login: mocks.login,
    recentLoginUsers: [],
    refreshUser: vi.fn(),
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
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <TooltipProvider>{page}</TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function inputWithAutocomplete(container: HTMLElement, autocomplete: string) {
  const input = container.querySelector(`input[autocomplete="${autocomplete}"]`)
  expect(input).toBeInstanceOf(HTMLInputElement)
  return input as HTMLInputElement
}

describe('authentication form payloads', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.getAuthRegistrationStatus.mockResolvedValue({ emailRegistrationEnabled: false })
    mocks.listAuthProviders.mockResolvedValue([])
    mocks.login.mockResolvedValue({})
  })

  it('submits login with rememberMe disabled by default', async () => {
    const user = userEvent.setup()
    const { container } = renderPage(<LoginPage />)
    const rememberMe = screen.getByRole('checkbox')

    expect(rememberMe).not.toBeChecked()
    const username = inputWithAutocomplete(container, 'username')
    expect(username).toHaveAttribute('type', 'email')
    await user.type(username, 'login@example.test')
    await user.type(inputWithAutocomplete(container, 'current-password'), 'password')
    const submit = screen.getByRole('button', { name: i18next.t('login') })
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)

    await waitFor(() => expect(mocks.login).toHaveBeenCalledWith({
      email: 'login@example.test',
      password: 'password',
      rememberMe: false,
    }))
  })

  it('exposes registration as a new password credential form', async () => {
    mocks.getAuthRegistrationStatus.mockResolvedValue({ emailRegistrationEnabled: true })
    const { container } = renderPage(<RegisterPage />)

    await waitFor(() => expect(container.querySelector('input[autocomplete="username"]')).toBeInstanceOf(HTMLInputElement))
    const username = inputWithAutocomplete(container, 'username')
    const form = username.closest('form')
    expect(username).toHaveAttribute('type', 'email')
    expect(form?.querySelectorAll('input[autocomplete="new-password"]')).toHaveLength(2)
    expect(form?.querySelector('input[autocomplete="one-time-code"]')).toHaveAttribute('name', 'verification-code')
  })
})
