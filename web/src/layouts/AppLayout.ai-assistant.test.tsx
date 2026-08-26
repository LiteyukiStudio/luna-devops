import type { ReactNode } from 'react'
import type { AICapabilities, CurrentUser } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionContext } from '@/app/session-context'
import { readAIAssistantRouteState } from '@/components/common/ai-assistant/route-state'
import i18next from '@/i18n'
import { AppLayout } from './AppLayout'

const assistantMocks = vi.hoisted(() => ({
  presentationMode: 'window' as 'page' | 'window',
  runtime: {
    closeAssistant: vi.fn(),
    enabled: true,
    open: false,
    openAssistant: vi.fn(),
    rememberWorkspaceLocation: vi.fn(),
    transitionAssistantToPage: vi.fn(),
  },
}))

vi.mock('@/components/common/ai-assistant/runtime-provider', () => ({
  AIAssistantRuntimeProvider: ({ children }: { children: ReactNode }) => children,
}))

vi.mock('@/components/common/ai-assistant/runtime-context', () => ({
  useAIAssistantRuntime: () => assistantMocks.runtime,
}))

vi.mock('@/components/common/ai-assistant/presentation-mode', () => ({
  useAIAssistantPresentationMode: () => assistantMocks.presentationMode,
}))

vi.mock('@/components/common/ai-assistant/desktop-host', () => ({
  AIAssistantDesktopHost: ({ onViewChange, view }: {
    view: 'chat' | 'conversations'
    onViewChange: (view: 'chat' | 'conversations') => void
  }) => (
    <button data-testid="desktop-assistant-host" data-view={view} type="button" onClick={() => onViewChange('conversations')}>
      Desktop assistant
    </button>
  ),
}))

vi.mock('@/pages/ai-assistant/AIAssistantPage', () => ({
  AIAssistantPage: () => <AssistantRouteProbe />,
}))

function AssistantRouteProbe() {
  const location = useLocation()
  const routeState = readAIAssistantRouteState(location.state)
  return <div data-ai-view={routeState.aiView} data-testid="assistant-route-outlet">Assistant page</div>
}

const capabilities: AICapabilities = { enabled: true, maxInputBytes: 8_192 }
const currentUser: CurrentUser = {
  id: 'usr_member',
  email: 'member@example.com',
  name: 'Member',
  avatarUrl: '',
  passwordSet: true,
  role: 'user',
  language: 'zh-CN',
  brandColorPreset: 'blue',
  interfaceStyle: 'themed',
  permissions: [],
}
const sessionValue: SessionContextValue = {
  actualUser: currentUser,
  initialized: true,
  isLoading: false,
  isLoggingIn: false,
  isLoggingOut: false,
  recentLoginUsers: [],
  user: currentUser,
  clearDebugOverride: vi.fn(),
  initializeAdmin: vi.fn(async () => currentUser),
  login: vi.fn(async () => currentUser),
  logout: vi.fn(async () => {}),
  refreshUser: vi.fn(async () => {}),
  resumeLogin: vi.fn(async () => currentUser),
  setDebugOverride: vi.fn(),
  updateProfile: vi.fn(async () => currentUser),
  updateLanguage: vi.fn(async () => currentUser),
}

beforeEach(async () => {
  await i18next.changeLanguage('zh-CN')
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn(() => ({
      addEventListener: vi.fn(),
      matches: false,
      removeEventListener: vi.fn(),
    })),
  })
  assistantMocks.presentationMode = 'window'
  assistantMocks.runtime.enabled = true
  assistantMocks.runtime.open = false
  assistantMocks.runtime.closeAssistant.mockClear()
  assistantMocks.runtime.openAssistant.mockClear()
  assistantMocks.runtime.rememberWorkspaceLocation.mockClear()
  assistantMocks.runtime.transitionAssistantToPage.mockClear()
})

describe('app layout ai assistant route', () => {
  it('opens the assistant window directly from the navigation entry', async () => {
    const user = userEvent.setup()
    renderAppLayout('/dashboard')

    await user.click(screen.getByRole('button', { name: i18next.t('aiAssistantNav') }))

    expect(assistantMocks.runtime.openAssistant).toHaveBeenCalledOnce()
  })

  it('renders the assistant outlet without the sidebar or workspace chrome', () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      },
    })
    queryClient.setQueryData(['projects'], [])
    queryClient.setQueryData(['ai', 'capabilities'], capabilities)

    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <SessionContext value={sessionValue}>
          <MemoryRouter initialEntries={['/ai-assistant']}>
            <Routes>
              <Route element={<AppLayout />}>
                <Route path="/ai-assistant" element={<div data-testid="assistant-route-outlet">Assistant page</div>} />
              </Route>
            </Routes>
          </MemoryRouter>
        </SessionContext>
      </QueryClientProvider>,
    )

    const outlet = screen.getByTestId('assistant-route-outlet')
    expect(outlet).toBeInTheDocument()
    expect(container.querySelector('main')).toContainElement(outlet)
    expect(container.querySelector('[data-slot="sidebar"]')).not.toBeInTheDocument()
    expect(container.querySelector('[data-slot="workspace-topbar-tabs"]')).not.toBeInTheDocument()
    expect(container.querySelector('.workspace-canvas')).not.toBeInTheDocument()
  })

  it.each([
    { expectedView: 'chat' as const, openConversations: false },
    { expectedView: 'conversations' as const, openConversations: true },
  ])('keeps the $expectedView view when an open desktop window changes to page mode', async ({ expectedView, openConversations }) => {
    const user = userEvent.setup()
    assistantMocks.runtime.open = true
    const { rerenderApp } = renderAppLayout('/dashboard')

    const desktopHost = screen.getByTestId('desktop-assistant-host')
    expect(desktopHost).toHaveAttribute('data-view', 'chat')
    if (openConversations) {
      await user.click(desktopHost)
      expect(desktopHost).toHaveAttribute('data-view', 'conversations')
    }

    assistantMocks.presentationMode = 'page'
    rerenderApp()

    expect(document.querySelector('[data-ai-assistant-transition], [data-testid="assistant-route-outlet"]')).toBeInTheDocument()
    const assistantPage = await screen.findByTestId('assistant-route-outlet')
    expect(assistantPage).toHaveAttribute('data-ai-view', expectedView)
    expect(assistantMocks.runtime.rememberWorkspaceLocation).toHaveBeenCalledWith(expect.objectContaining({ pathname: '/dashboard' }))
    expect(assistantMocks.runtime.transitionAssistantToPage).toHaveBeenCalledOnce()
    expect(assistantMocks.runtime.closeAssistant).not.toHaveBeenCalled()
  })
})

function renderAppLayout(initialEntry: string) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
    },
  })
  queryClient.setQueryData(['projects'], [])
  queryClient.setQueryData(['ai', 'capabilities'], capabilities)
  const createUI = () => (
    <QueryClientProvider client={queryClient}>
      <SessionContext value={sessionValue}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route element={<AppLayout />}>
              <Route path="/dashboard" element={<div>Dashboard</div>} />
              <Route path="/ai-assistant" element={<div>Assistant route</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </SessionContext>
    </QueryClientProvider>
  )
  const rendered = render(createUI())
  return {
    ...rendered,
    rerenderApp: () => rendered.rerender(createUI()),
  }
}
