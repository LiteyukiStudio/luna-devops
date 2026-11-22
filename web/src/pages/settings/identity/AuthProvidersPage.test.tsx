import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { AuthProvidersPage } from './AuthProvidersPage'

const mocks = vi.hoisted(() => ({
  getAuthAdmissionPolicy: vi.fn(),
  getOIDCCallbackConfig: vi.fn(),
  listAuthProviders: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getAuthAdmissionPolicy: mocks.getAuthAdmissionPolicy,
      getOIDCCallbackConfig: mocks.getOIDCCallbackConfig,
      listAuthProviders: mocks.listAuthProviders,
    },
  }
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <AuthProvidersPage />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe('auth providers page loading and error states', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.history.replaceState(null, '', '/')
    await i18next.changeLanguage('en-US')
    mocks.listAuthProviders.mockResolvedValue([])
    mocks.getAuthAdmissionPolicy.mockResolvedValue({
      allowLocalLogin: true,
      allowOidcLogin: true,
      allowedEmailDomains: [],
      allowedOidcGroups: [],
      defaultRole: 'user',
      invitedEmails: [],
      requireVerifiedOidcEmail: true,
    })
    mocks.getOIDCCallbackConfig.mockResolvedValue({
      callbackUrl: 'https://devops.example.com/api/v1/auth/oidc/callback',
      configured: true,
    })
  })

  it('renders a skeleton instead of callbackUrlMissing while oidcCallbackConfig is pending', async () => {
    mocks.getOIDCCallbackConfig.mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('button', { name: i18next.t('authProvidersPage.createTitle') }))

    const dialog = await screen.findByRole('dialog')
    expect(dialog.querySelector('[data-slot="skeleton"]')).not.toBeNull()
    expect(screen.queryByText(i18next.t('authProvidersPage.callbackUrlMissing'))).not.toBeInTheDocument()
  })

  it('renders SettingsSkeleton on policy tab while policy query is pending', async () => {
    mocks.getAuthAdmissionPolicy.mockReturnValue(new Promise(() => {}))
    const user = userEvent.setup()
    const { container } = renderPage()

    await user.click(screen.getByRole('tab', { name: i18next.t('authProvidersPage.policyTab') }))

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.queryByText(i18next.t('authProvidersPage.allowLocalLogin'))).not.toBeInTheDocument()
  })

  it('shows actual error message on non-403 failures and platformAdminPermissionRequired on 403', async () => {
    mocks.listAuthProviders.mockRejectedValueOnce(new Error('upstream gateway timeout'))
    const { unmount } = renderPage()

    expect(await screen.findByText('upstream gateway timeout')).toBeInTheDocument()
    expect(screen.queryByText(i18next.t('common.platformAdminPermissionRequired'))).not.toBeInTheDocument()

    unmount()
    mocks.listAuthProviders.mockRejectedValueOnce(new ApiError('forbidden', { code: 'FORBIDDEN', path: '/auth/providers', status: 403 }))
    renderPage()

    expect(await screen.findByText(i18next.t('common.platformAdminPermissionRequired'))).toBeInTheDocument()
  })
})
