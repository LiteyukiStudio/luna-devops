import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { PlatformRole } from '@/lib/roles'
import { OAuthDevicePage } from './OAuthDevicePage'

const mocks = vi.hoisted(() => ({
  decideOAuthDeviceVerification: vi.fn(),
  getOAuthDeviceVerification: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      decideOAuthDeviceVerification: mocks.decideOAuthDeviceVerification,
      getOAuthDeviceVerification: mocks.getOAuthDeviceVerification,
    },
  }
})

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    initialized: true,
    user: {
      id: 'usr_test',
      email: 'owner@example.test',
      name: 'Owner',
      role: PlatformRole.User,
    },
  }),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
  },
}))

function renderPage(initialEntry: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <TooltipProvider>
          <OAuthDevicePage />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function verification(userCode: string) {
  return {
    application: {
      id: 'oapp_luna_cli',
      ownerUserId: '',
      name: 'Luna CLI',
      description: 'Luna DevOps command-line client',
      homepageUrl: '',
      logoUrl: '',
      clientId: 'luna-cli',
      redirectUris: [],
      allowedScopes: 'project:read build:write',
      accessTokenLifetimeDays: 1,
      createdAt: '2026-07-27T00:00:00Z',
      updatedAt: '2026-07-27T00:00:00Z',
    },
    scope: 'project:read build:write',
    userCode,
    expiresAt: '2026-07-27T12:30:00Z',
  }
}

describe('oauth device page', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
    mocks.decideOAuthDeviceVerification.mockResolvedValue({ status: 'approved' })
  })

  it('loads a query code and approves the displayed device', async () => {
    const user = userEvent.setup()
    mocks.getOAuthDeviceVerification.mockResolvedValue(verification('ABCD-EFGH'))

    renderPage('/oauth/device?user_code=abcd-efgh')

    await waitFor(() => expect(mocks.getOAuthDeviceVerification).toHaveBeenCalledWith('ABCD-EFGH'))
    expect(await screen.findByText('Luna CLI')).toBeInTheDocument()
    expect(screen.getByText('ABCD-EFGH')).toBeInTheDocument()
    expect(screen.getByText(i18next.t('accessTokens.scopeLabels.project.read'))).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('oauthApps.device.approve') }))

    await waitFor(() => expect(mocks.decideOAuthDeviceVerification).toHaveBeenCalledWith({
      approved: true,
      userCode: 'ABCD-EFGH',
    }))
    expect(await screen.findByText(i18next.t('oauthApps.device.approvedTitle'))).toBeInTheDocument()
  })

  it('accepts a manually entered code and denies the request', async () => {
    const user = userEvent.setup()
    mocks.getOAuthDeviceVerification.mockResolvedValue(verification('WXYZ-1234'))

    renderPage('/oauth/device')

    const codeInput = screen.getByLabelText(i18next.t('oauthApps.device.codeLabel'))
    expect(codeInput).toHaveAttribute('autocomplete', 'off')
    await user.type(codeInput, 'wxyz-1234')
    await user.click(screen.getByRole('button', { name: i18next.t('oauthApps.device.continue') }))

    await waitFor(() => expect(mocks.getOAuthDeviceVerification).toHaveBeenCalledWith('WXYZ-1234'))
    await user.click(await screen.findByRole('button', { name: i18next.t('oauthApps.device.deny') }))

    await waitFor(() => expect(mocks.decideOAuthDeviceVerification).toHaveBeenCalledWith({
      approved: false,
      userCode: 'WXYZ-1234',
    }))
    expect(await screen.findByText(i18next.t('oauthApps.device.deniedTitle'))).toBeInTheDocument()
  })
})
