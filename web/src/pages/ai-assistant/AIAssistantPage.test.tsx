import type { ComponentProps } from 'react'
import type { AICapabilities, AIConversation, CurrentUser } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { aiApi } from '@/api/domains/ai'
import { SessionContext } from '@/app/session-context'
import { createAIAssistantRouteState } from '@/components/common/ai-assistant/route-state'
import { useAIAssistantRuntime } from '@/components/common/ai-assistant/runtime-context'
import { AIAssistantRuntimeProvider } from '@/components/common/ai-assistant/runtime-provider'
import i18next from '@/i18n'
import { AIAssistantPage } from './AIAssistantPage'

const capabilities: AICapabilities = { enabled: true, maxInputBytes: 8_192 }
const conversation: AIConversation = {
  id: 'conversation-1',
  title: '构建失败诊断',
  titleSource: 'assistant',
  status: 'active',
  modelId: 'model-1',
  createdAt: '2026-08-25T08:00:00.000Z',
  updatedAt: '2026-08-25T08:30:00.000Z',
}
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
  vi.spyOn(aiApi, 'listAIModels').mockResolvedValue([{
    id: 'model-1',
    name: 'Luna Test Model',
    maxContextTokens: 128_000,
    maxOutputTokens: 8_192,
  }])
  vi.spyOn(aiApi, 'listPendingAIUIActions').mockResolvedValue({ items: [], agentAvailable: true })
  vi.spyOn(aiApi, 'listAIConversations').mockResolvedValue({
    items: [conversation],
    page: 1,
    pageSize: 50,
    sortBy: 'updatedAt',
    sortOrder: 'desc',
    total: 1,
    totalPages: 1,
  })
  vi.spyOn(aiApi, 'getAIConversationTimeline').mockResolvedValue({
    conversation: {
      id: conversation.id,
      title: conversation.title,
      titleSource: conversation.titleSource,
      status: conversation.status,
      modelId: conversation.modelId,
    },
    turns: [],
    eventCursors: [],
    pageInfo: { hasOlder: false },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  document.querySelector('[data-focus-sentinel]')?.remove()
})

describe('ai assistant route page', () => {
  it('pushes the conversation view and returns to chat through browser history', async () => {
    const user = userEvent.setup()
    const { container } = renderAssistantPage()

    await waitFor(() => {
      expect(aiApi.listAIModels).toHaveBeenCalledOnce()
      expect(aiApi.listAIConversations).toHaveBeenCalledOnce()
    })
    const pageViewport = container.querySelector('[data-ai-page-viewport]')
    expect(pageViewport).toContainElement(container.querySelector('[data-ai-assistant-page]'))
    expect(container.querySelector('[data-ai-assistant-surface="page"]')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument()
    expect(screen.getByText(i18next.t('aiAssistant.context', { path: '/projects/project-1/apps/app-1' }))).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('aiAssistant.conversations.title') }))
    expect(container.querySelector('[data-ai-conversation-surface="page"]')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1, name: i18next.t('aiAssistant.conversations.title') })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.back') }))
    await waitFor(() => expect(container.querySelector('[data-ai-assistant-surface="page"]')).toBeInTheDocument())
    expect(container.querySelector('[data-ai-conversation-surface="page"]')).not.toBeInTheDocument()
  })

  it('selects a conversation and returns to the chat history entry', async () => {
    const user = userEvent.setup()
    const { container } = renderAssistantPage()

    await user.click(screen.getByRole('button', { name: i18next.t('aiAssistant.conversations.title') }))
    await user.click(await screen.findByRole('button', { name: /构建失败诊断/ }))

    await waitFor(() => {
      expect(container.querySelector('[data-ai-assistant-surface="page"]')).toBeInTheDocument()
      expect(screen.getByRole('heading', { name: conversation.title })).toBeInTheDocument()
      expect(aiApi.getAIConversationTimeline).toHaveBeenCalledWith(conversation.id, expect.any(Object))
    })
  })

  it('replaces a responsive direct conversation entry with chat before returning to the workspace', async () => {
    const user = userEvent.setup()
    const routeState = createAIAssistantRouteState({ pathname: '/dashboard' }, 'conversations')
    const { container } = renderAssistantPage({
      initialEntries: [
        '/dashboard',
        { pathname: '/ai-assistant', state: routeState },
      ],
      initialIndex: 1,
    })

    expect(await screen.findByRole('button', { name: i18next.t('common.back') })).toBeInTheDocument()
    expect(container.querySelector('[data-ai-conversation-surface="page"]')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: i18next.t('common.back') }))
    await waitFor(() => expect(container.querySelector('[data-ai-assistant-surface="page"]')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: i18next.t('aiAssistant.page.backToWorkspace') }))
    expect(await screen.findByTestId('dashboard-route')).toBeInTheDocument()
  })

  it('shows a friendly disabled state and returns direct access to the dashboard', async () => {
    const user = userEvent.setup()
    renderAssistantPage({ capabilities: { enabled: false, maxInputBytes: 0 }, initialEntries: ['/ai-assistant'] })

    expect(screen.getByRole('heading', { name: i18next.t('aiAssistant.page.unavailableTitle') })).toBeInTheDocument()
    expect(screen.getByText(i18next.t('aiAssistant.page.unavailableDescription'))).toBeInTheDocument()
    expect(aiApi.listAIModels).not.toHaveBeenCalled()
    expect(aiApi.listAIConversations).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: i18next.t('aiAssistant.page.backToWorkspace') }))
    expect(await screen.findByTestId('dashboard-route')).toBeInTheDocument()
  })

  it('does not move focus into the composer when the route page loads', async () => {
    const focusSentinel = document.createElement('button')
    focusSentinel.dataset.focusSentinel = 'true'
    document.body.append(focusSentinel)
    focusSentinel.focus()

    renderAssistantPage()
    const composer = await screen.findByRole('textbox', { name: i18next.t('aiAssistant.inputLabel') })
    await waitFor(() => expect(aiApi.listAIModels).toHaveBeenCalledOnce())
    await act(async () => {
      await new Promise(resolve => window.setTimeout(resolve, 10))
    })

    expect(focusSentinel).toHaveFocus()
    expect(composer).not.toHaveFocus()
  })

  it('does not apply pending automatic navigation after leaving the assistant page', async () => {
    const user = userEvent.setup()
    let resolvePendingActions!: (value: Awaited<ReturnType<typeof aiApi.listPendingAIUIActions>>) => void
    vi.mocked(aiApi.listPendingAIUIActions).mockImplementationOnce(() => new Promise((resolve) => {
      resolvePendingActions = resolve
    }))
    const acknowledge = vi.spyOn(aiApi, 'acknowledgeAIUIAction').mockResolvedValue()
    renderAssistantPage({
      initialEntries: [
        '/dashboard',
        {
          pathname: '/ai-assistant',
          state: createAIAssistantRouteState({ pathname: '/dashboard' }),
        },
      ],
      initialIndex: 1,
    })

    await waitFor(() => expect(aiApi.listPendingAIUIActions).toHaveBeenCalledOnce())
    await user.click(screen.getByRole('button', { name: i18next.t('aiAssistant.page.backToWorkspace') }))
    expect(await screen.findByTestId('dashboard-route')).toBeInTheDocument()

    await act(async () => resolvePendingActions({
      agentAvailable: true,
      items: [{
        actionId: 'aiuia_stale_navigation',
        action: {
          version: 1,
          type: 'navigate',
          activation: 'automatic',
          payload: { routeName: 'settings.notifications', params: {}, query: {} },
        },
        attempts: 0,
        expiresAt: '2099-01-01T00:00:00.000Z',
        runId: 'run-stale',
        toolCallId: 'tool-stale',
      }],
    }))

    await waitFor(() => expect(acknowledge).toHaveBeenCalledWith('aiuia_stale_navigation', expect.objectContaining({
      errorCode: 'ai.ui_action_rejected',
      status: 'failed',
    })))
    expect(screen.getByTestId('dashboard-route')).toBeInTheDocument()
    expect(screen.queryByTestId('notifications-route')).not.toBeInTheDocument()
  })

  it('uses a restored history entry return location on its first render', async () => {
    const user = userEvent.setup()
    const observedWorkspacePaths: string[] = []
    renderAssistantPage({
      initialEntries: [
        {
          pathname: '/ai-assistant',
          state: createAIAssistantRouteState({ pathname: '/projects/history-project', search: '?tab=runtime' }),
        },
        '/dashboard',
      ],
      initialIndex: 1,
      onPageRender: pathname => observedWorkspacePaths.push(pathname),
    })

    await user.click(screen.getByRole('button', { name: 'history-back' }))
    await screen.findByRole('textbox', { name: i18next.t('aiAssistant.inputLabel') })

    expect(observedWorkspacePaths[0]).toBe('/projects/history-project')
    expect(screen.getByText(i18next.t('aiAssistant.context', { path: '/projects/history-project' }))).toBeInTheDocument()
  })
})

interface RenderAssistantPageOptions {
  capabilities?: AICapabilities
  initialEntries?: ComponentProps<typeof MemoryRouter>['initialEntries']
  initialIndex?: number
  onPageRender?: (pathname: string) => void
}

function renderAssistantPage({
  capabilities: enabledCapabilities = capabilities,
  initialEntries = [{
    pathname: '/ai-assistant',
    state: createAIAssistantRouteState({ pathname: '/projects/project-1/apps/app-1', search: '?tab=runtime', hash: '#recent' }),
  }],
  initialIndex,
  onPageRender,
}: RenderAssistantPageOptions = {}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <SessionContext value={sessionValue}>
        <MemoryRouter initialEntries={initialEntries} initialIndex={initialIndex}>
          <AIAssistantRuntimeProvider capabilities={enabledCapabilities}>
            <Routes>
              <Route path="/ai-assistant" element={<ObservedAIAssistantPage onRender={onPageRender} />} />
              <Route path="/dashboard" element={<DashboardTestRoute />} />
              <Route path="/settings/notifications" element={<div data-testid="notifications-route">Notifications</div>} />
            </Routes>
          </AIAssistantRuntimeProvider>
        </MemoryRouter>
      </SessionContext>
    </QueryClientProvider>,
  )
}

function ObservedAIAssistantPage({ onRender }: { onRender?: (pathname: string) => void }) {
  const runtime = useAIAssistantRuntime()
  onRender?.(runtime.workspaceLocation.pathname)
  return <AIAssistantPage />
}

function DashboardTestRoute() {
  const navigate = useNavigate()
  return (
    <div data-testid="dashboard-route">
      Dashboard
      <button type="button" onClick={() => navigate(-1)}>history-back</button>
    </div>
  )
}
