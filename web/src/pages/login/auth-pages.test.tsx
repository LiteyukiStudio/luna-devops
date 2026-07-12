import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { BootstrapPage } from '@/pages/bootstrap/BootstrapPage'
import { LoginPage } from './LoginPage'

const mocks = vi.hoisted(() => ({
  getBootstrapStatus: vi.fn(),
  initializeAdmin: vi.fn(),
  listAuthProviders: vi.fn(),
  login: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getBootstrapStatus: mocks.getBootstrapStatus,
      initializeAdmin: mocks.initializeAdmin,
      listAuthProviders: mocks.listAuthProviders,
      login: mocks.login,
    },
  }
})

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    initialized: true,
    initializeAdmin: mocks.initializeAdmin,
    isLoading: false,
    isLoggingIn: false,
    isLoggingOut: false,
    login: mocks.login,
    recentLoginUsers: [],
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
    mocks.listAuthProviders.mockResolvedValue([])
    mocks.login.mockResolvedValue({})
    mocks.initializeAdmin.mockResolvedValue({})
  })

  it('submits login with rememberMe disabled by default', async () => {
    mocks.getBootstrapStatus.mockResolvedValue({ initialized: true, mode: 'development' })
    const user = userEvent.setup()
    const { container } = renderPage(<LoginPage />)
    const rememberMe = screen.getByRole('checkbox')

    expect(rememberMe).not.toBeChecked()
    await user.type(inputWithAutocomplete(container, 'email'), 'login@example.test')
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

  it('submits bootstrap with rememberMe disabled by default', async () => {
    mocks.getBootstrapStatus.mockResolvedValue({
      bootstrapTokenRequired: false,
      initialized: false,
      mode: 'development',
    })
    const user = userEvent.setup()
    const { container } = renderPage(<BootstrapPage />)
    const rememberMe = screen.getByRole('checkbox')

    expect(rememberMe).not.toBeChecked()
    await user.type(inputWithAutocomplete(container, 'email'), 'bootstrap@example.test')
    await user.type(inputWithAutocomplete(container, 'new-password'), 'password')
    const submit = screen.getByRole('button', { name: i18next.t('bootstrap.create') })
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)

    await waitFor(() => expect(mocks.initializeAdmin).toHaveBeenCalledWith({
      bootstrapToken: '',
      email: 'bootstrap@example.test',
      language: 'en-US',
      name: 'Platform Admin',
      password: 'password',
      rememberMe: false,
    }))
  })
})
