import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { AccountPage } from './AccountPage'

const mocks = vi.hoisted(() => {
  const updateProfile = vi.fn()
  return {
    session: {
      updateProfile,
      user: {
        avatarUrl: '',
        brandColorPreset: '',
        email: 'owner@example.com',
        id: 'usr_owner',
        language: 'en-US',
        name: 'Owner',
        passwordSet: true,
        role: 'user',
      },
    },
  }
})

vi.mock('motion/react', () => ({
  motion: {
    div: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  },
}))

vi.mock('@/components/common/content-tabs', () => ({
  ContentTabs: ({ children, tools, onValueChange }: {
    children: ReactNode
    tools?: ReactNode
    onValueChange: (value: string) => void
  }) => (
    <div>
      <button type="button" onClick={() => onValueChange('tokens')}>tokens-tab</button>
      <button type="button" onClick={() => onValueChange('oauth-applications')}>oauth-applications-tab</button>
      <div data-testid="content-tabs-tools">{tools}</div>
      <div data-testid="account-tab-content">{children}</div>
    </div>
  ),
}))

vi.mock('@/components/ui/tabs', () => ({
  TabsContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}))

vi.mock('./access-tokens-panel', () => ({
  AccessTokensPanel: ({ createDialogOpen }: { createDialogOpen: boolean }) => (
    <div data-create-dialog-open={createDialogOpen} data-testid="access-tokens-panel" />
  ),
}))

vi.mock('./account-oauth-panels', () => ({
  OAuthApplicationsPanel: ({ createDialogOpen }: { createDialogOpen: boolean }) => (
    <div data-create-dialog-open={createDialogOpen} data-testid="oauth-applications-panel" />
  ),
  OAuthGrantsPanel: () => <div />,
}))

vi.mock('@/app/public-config-context', () => ({
  usePublicConfig: () => ({}),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => mocks.session,
}))

vi.mock('@/app/theme-context', () => ({
  useTheme: () => ({ mode: 'system', setMode: vi.fn() }),
}))

describe('account page tab tools', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('en-US')
  })

  it('places the access token create action in ContentTabs tools', () => {
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: 'tokens-tab' }))

    const label = i18next.t('accessTokens.createTitle')
    const toolButton = within(screen.getByTestId('content-tabs-tools')).getByRole('button', { name: label })
    expect(within(screen.getByTestId('account-tab-content')).queryByRole('button', { name: label })).not.toBeInTheDocument()

    fireEvent.click(toolButton)
    expect(screen.getByTestId('access-tokens-panel')).toHaveAttribute('data-create-dialog-open', 'true')
  })

  it('places the OAuth application create action in ContentTabs tools', () => {
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: 'oauth-applications-tab' }))

    const label = i18next.t('oauthApps.createApplication')
    const toolButton = within(screen.getByTestId('content-tabs-tools')).getByRole('button', { name: label })
    expect(within(screen.getByTestId('account-tab-content')).queryByRole('button', { name: label })).not.toBeInTheDocument()

    fireEvent.click(toolButton)
    expect(screen.getByTestId('oauth-applications-panel')).toHaveAttribute('data-create-dialog-open', 'true')
  })

  it('shows the floating save bar on profile edits, supports discarding changes, and clears after saving', async () => {
    const user = userEvent.setup()
    mocks.session.updateProfile.mockResolvedValue({})
    renderPage()

    expect(document.querySelector('[data-slot="floating-form-save-bar"]')).toBeNull()

    const nameInput = screen.getByRole('textbox', { name: new RegExp(i18next.t('accountPage.profileName')) })
    await user.clear(nameInput)
    await user.type(nameInput, 'Owner Updated')

    const saveBar = document.querySelector('[data-slot="floating-form-save-bar"]') as HTMLElement
    expect(saveBar).not.toBeNull()
    expect(within(saveBar).getByText(i18next.t('common.unsavedChanges'))).toBeInTheDocument()

    await user.click(within(saveBar).getByRole('button', { name: i18next.t('common.discardChanges') }))
    expect(nameInput).toHaveValue('Owner')
    expect(document.querySelector('[data-slot="floating-form-save-bar"]')).toBeNull()

    await user.clear(nameInput)
    await user.type(nameInput, 'Owner Saved')
    await user.click(screen.getByRole('button', { name: i18next.t('accountPage.saveProfile') }))

    await waitFor(() => expect(mocks.session.updateProfile).toHaveBeenCalledWith(expect.objectContaining({ name: 'Owner Saved' })))
    await waitFor(() => expect(document.querySelector('[data-slot="floating-form-save-bar"]')).toBeNull())
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <AccountPage />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}
