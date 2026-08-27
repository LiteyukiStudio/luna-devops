import type { AICapabilities, CurrentUser } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { SessionContext } from '@/app/session-context'
import { useAIAssistantRuntime } from './runtime-context'
import { AIAssistantRuntimeProvider } from './runtime-provider'

const runStreamManagerMock = vi.hoisted(() => ({
  connect: vi.fn(),
  reconnect: vi.fn(),
  syncConversation: vi.fn(),
}))

vi.mock('./run-stream-manager', () => ({
  useAIRunStreamManager: () => ({
    connect: runStreamManagerMock.connect,
    reconnect: runStreamManagerMock.reconnect,
    streamStates: [],
    subscriptions: [],
    syncConversation: runStreamManagerMock.syncConversation,
  }),
}))

const enabledCapabilities: AICapabilities = { enabled: true, maxInputBytes: 8_192 }
const disabledCapabilities: AICapabilities = { enabled: false, maxInputBytes: 0 }
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
  initialized: true,
  isLoading: false,
  isLoggingIn: false,
  isLoggingOut: false,
  recentLoginUsers: [],
  user: currentUser,
  initializeAdmin: vi.fn(async () => currentUser),
  login: vi.fn(async () => currentUser),
  logout: vi.fn(async () => {}),
  refreshUser: vi.fn(async () => {}),
  resumeLogin: vi.fn(async () => currentUser),
  updateProfile: vi.fn(async () => currentUser),
  updateLanguage: vi.fn(async () => currentUser),
}

beforeEach(() => {
  runStreamManagerMock.connect.mockReset()
  runStreamManagerMock.reconnect.mockReset()
  runStreamManagerMock.syncConversation.mockReset()
  vi.spyOn(api, 'listAIModels').mockResolvedValue([])
  vi.spyOn(api, 'listAIConversations').mockResolvedValue({
    items: [],
    page: 1,
    pageSize: 50,
    sortBy: 'updatedAt',
    sortOrder: 'desc',
    total: 0,
    totalPages: 0,
  })
  vi.spyOn(api, 'getAIConversationTimeline').mockResolvedValue({
    conversation: {
      id: 'conversation-1',
      title: 'Conversation',
      titleSource: 'user',
      status: 'active',
      modelId: 'model-1',
    },
    turns: [],
    eventCursors: [],
    pageInfo: { hasOlder: false },
  })
})

afterEach(() => vi.restoreAllMocks())

describe('ai assistant runtime provider', () => {
  it('does not restore an open surface or conversation session after capabilities restart', async () => {
    const user = userEvent.setup()
    renderRuntimeHarness()

    expect(screen.getByTestId('runtime-open')).toHaveTextContent('true')
    await user.click(screen.getByRole('button', { name: 'select-conversation' }))
    expect(screen.getByTestId('runtime-conversation')).toHaveTextContent('conversation-1')

    await user.click(screen.getByRole('button', { name: 'disable-capability' }))
    await waitFor(() => {
      expect(screen.getByTestId('runtime-enabled')).toHaveTextContent('false')
      expect(screen.getByTestId('runtime-open')).toHaveTextContent('false')
      expect(screen.getByTestId('runtime-conversation')).toBeEmptyDOMElement()
    })

    await user.click(screen.getByRole('button', { name: 'open-assistant' }))
    await user.click(screen.getByRole('button', { name: 'enable-capability' }))
    await waitFor(() => {
      expect(screen.getByTestId('runtime-enabled')).toHaveTextContent('true')
      expect(screen.getByTestId('runtime-open')).toHaveTextContent('false')
      expect(screen.getByTestId('runtime-conversation')).toBeEmptyDOMElement()
    })
  })

  it('reconciles a successful tool action after the surface closes', async () => {
    const user = userEvent.setup()
    let resolveToolAction!: (value: Awaited<ReturnType<typeof api.executeAIToolAction>>) => void
    vi.spyOn(api, 'executeAIToolAction').mockImplementationOnce(() => new Promise((resolve) => {
      resolveToolAction = resolve
    }))
    renderRuntimeHarness()

    await user.click(screen.getByRole('button', { name: 'select-conversation' }))
    await user.click(screen.getByRole('button', { name: 'request-tool' }))
    await waitFor(() => expect(api.executeAIToolAction).toHaveBeenCalledWith(
      'conversation-1',
      {
        operationId: 'retryBuildRun',
        arguments: { runId: 'run-1' },
        message: 'retry',
      },
      expect.any(String),
    ))
    await user.click(screen.getByRole('button', { name: 'close-assistant' }))

    resolveToolAction({
      turnId: 'turn-1',
      turnIndex: 1,
      runId: 'run-1',
      state: 'queued',
      eventsUrl: '/api/v1/ai/runs/run-1/events',
    })

    await waitFor(() => expect(runStreamManagerMock.connect).toHaveBeenCalledWith({
      after: 0,
      conversationId: 'conversation-1',
      eventsUrl: '/api/v1/ai/runs/run-1/events',
      runId: 'run-1',
    }))
    expect(screen.getByTestId('runtime-open')).toHaveTextContent('false')
  })
})

function renderRuntimeHarness() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SessionContext value={sessionValue}>
        <MemoryRouter initialEntries={['/dashboard']}>
          <CapabilityHarness />
        </MemoryRouter>
      </SessionContext>
    </QueryClientProvider>,
  )
}

function CapabilityHarness() {
  const [capabilities, setCapabilities] = useState(enabledCapabilities)
  return (
    <AIAssistantRuntimeProvider capabilities={capabilities} initiallyOpen>
      <RuntimeProbe
        onDisable={() => setCapabilities(disabledCapabilities)}
        onEnable={() => setCapabilities(enabledCapabilities)}
      />
    </AIAssistantRuntimeProvider>
  )
}

function RuntimeProbe({ onDisable, onEnable }: { onDisable: () => void, onEnable: () => void }) {
  const runtime = useAIAssistantRuntime()
  const location = useLocation()
  const navigate = useNavigate()
  return (
    <div>
      <span data-testid="runtime-enabled">{String(runtime.enabled)}</span>
      <span data-testid="runtime-open">{String(runtime.open)}</span>
      <span data-testid="runtime-conversation">{runtime.selectedConversationId}</span>
      <span data-testid="runtime-pathname">{location.pathname}</span>
      <span data-testid="runtime-surface-visible">{String(runtime.surfaceVisible)}</span>
      <button aria-label="select-conversation" type="button" onClick={() => runtime.selectConversation('conversation-1')} />
      <button aria-label="open-assistant" type="button" onClick={runtime.openAssistant} />
      <button
        aria-label="request-tool"
        type="button"
        onClick={() => void runtime.executeAction({
          version: 1,
          type: 'request_tool',
          payload: { operationId: 'retryBuildRun', arguments: { runId: 'run-1' }, message: 'retry' },
        })}
      />
      <button aria-label="close-assistant" type="button" onClick={runtime.closeAssistant} />
      <button
        aria-label="transition-to-page"
        type="button"
        onClick={runtime.transitionAssistantToPage}
      />
      <button aria-label="navigate-to-page" type="button" onClick={() => navigate('/ai-assistant')} />
      <button aria-label="disable-capability" type="button" onClick={onDisable} />
      <button aria-label="enable-capability" type="button" onClick={onEnable} />
    </div>
  )
}
