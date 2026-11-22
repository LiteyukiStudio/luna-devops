import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { mcpDocumentationUrl } from '@/lib/documentation-url'
import { AccessTokensPanel } from './access-tokens-panel'
import { OAuthApplicationsPanel, OAuthGrantsPanel } from './account-oauth-panels'

const apiMocks = vi.hoisted(() => ({
  createAccessToken: vi.fn(),
  listAccessTokenScopes: vi.fn(),
  listAccessTokens: vi.fn(),
  listMyOAuthGrants: vi.fn(),
  listOAuthApplications: vi.fn(),
}))

vi.mock('@/api', async () => {
  const actual = await vi.importActual<typeof import('@/api')>('@/api')
  return {
    ...actual,
    api: {
      ...actual.api,
      createAccessToken: apiMocks.createAccessToken,
      listAccessTokenScopes: apiMocks.listAccessTokenScopes,
      listAccessTokens: apiMocks.listAccessTokens,
      listMyOAuthGrants: apiMocks.listMyOAuthGrants,
      listOAuthApplications: apiMocks.listOAuthApplications,
    },
  }
})

describe('account PAT and OAuth in-place MCP linkage', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    window.history.replaceState(null, '', '/settings/account')
    localStorage.clear()
    await i18next.changeLanguage('zh-CN')
    apiMocks.listAccessTokens.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    })
    apiMocks.listAccessTokenScopes.mockResolvedValue({
      items: [
        {
          group: 'build',
          recommended: true,
          requiresAdminRole: false,
          value: 'build:trigger',
        },
      ],
    })
    apiMocks.listOAuthApplications.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    })
  })

  it('offers one-click MCP prompt copy with a newly issued PAT without persisting the token to URL or storage', async () => {
    const user = userEvent.setup()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })

    const issuedToken = 'lyd_test_one_time_mcp_secret_99'
    apiMocks.createAccessToken.mockResolvedValue({
      accessToken: issuedToken,
      token: {
        createdAt: '2026-11-22T00:00:00Z',
        id: 'tok_1',
        name: 'MCP Agent',
        scope: 'build:trigger',
      },
    })

    const onOpenMcpGuide = vi.fn()
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })

    function StatefulTokenPanelHarness() {
      const [dialogOpen, setDialogOpen] = useState(true)
      return (
        <AccessTokensPanel
          createDialogOpen={dialogOpen}
          onCreateDialogOpenChange={setDialogOpen}
          onOpenMcpGuide={onOpenMcpGuide}
        />
      )
    }

    render(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <StatefulTokenPanelHarness />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    const nameInput = await screen.findByPlaceholderText(i18next.t('accessTokens.namePlaceholder'))
    await user.type(nameInput, 'MCP Agent')
    await user.click(screen.getByRole('button', { name: i18next.t('accessTokens.create') }))

    await waitFor(() => expect(screen.getByText(issuedToken)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: i18next.t('accessTokens.copyMcpPromptWithToken') }))
    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1))
    const copiedPrompt = String(writeText.mock.calls[0]?.[0] ?? '')
    expect(copiedPrompt).toContain(`${window.location.origin}/api/v1/mcp`)
    expect(copiedPrompt).toContain(`Authorization: Bearer ${issuedToken}`)
    expect(window.location.href).not.toContain(issuedToken)
    expect(JSON.stringify(localStorage)).not.toContain(issuedToken)

    await user.click(screen.getByRole('button', { name: i18next.t('accessTokens.openMcpGuide') }))
    expect(onOpenMcpGuide).toHaveBeenCalledTimes(1)
  })

  it('renders MCP OAuth 2.0 auto-discovery guidance and localized documentation link in OAuthApplicationsPanel', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <OAuthApplicationsPanel
            createDialogOpen={false}
            onCreateDialogOpenChange={vi.fn()}
          />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    expect(screen.getByText(i18next.t('oauthApps.mcpDiscoveryTitle'))).toBeInTheDocument()
    expect(screen.getByText(/\/\.well-known\/oauth-protected-resource\/api\/v1\/mcp/)).toBeInTheDocument()

    const docsLink = screen.getByRole('link', { name: i18next.t('oauthApps.mcpDocsLink') })
    expect(docsLink).toHaveAttribute('href', mcpDocumentationUrl('zh-CN'))
    expect(docsLink).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('renders skeletons instead of empty copy while access tokens and OAuth lists are pending', () => {
    apiMocks.listAccessTokens.mockReturnValue(new Promise(() => {}))
    apiMocks.listOAuthApplications.mockReturnValue(new Promise(() => {}))
    apiMocks.listMyOAuthGrants.mockReturnValue(new Promise(() => {}))
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    })

    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <AccessTokensPanel createDialogOpen={false} onCreateDialogOpenChange={vi.fn()} />
          <OAuthApplicationsPanel createDialogOpen={false} onCreateDialogOpenChange={vi.fn()} />
          <OAuthGrantsPanel />
        </TooltipProvider>
      </QueryClientProvider>,
    )

    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.queryByText(i18next.t('accessTokens.empty'))).not.toBeInTheDocument()
    expect(screen.queryByText(i18next.t('oauthApps.emptyApplications'))).not.toBeInTheDocument()
    expect(screen.queryByText(i18next.t('oauthApps.emptyGrants'))).not.toBeInTheDocument()
  })
})
