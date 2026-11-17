import type { AICapabilities, CurrentUser } from '@/api'
import type { SessionContextValue } from '@/app/session-context'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/api'
import { SessionContext } from '@/app/session-context'
import { useAIAssistantRuntime } from './runtime-context'
import { AIAssistantRuntimeProvider } from './runtime-provider'

const runStreamManagerMock = vi.hoisted(() => ({
  connect: vi.fn(),
  reconnect: vi.fn(),
  syncConversation: vi.fn(),
  streamStates: [] as Array<{ runId: string, status: string }>,
  subscriptions: [] as Array<{ conversationId: string, eventsUrl: string, runId: string, after: number }>,
}))

vi.mock('./run-stream-manager', () => ({
  useAIRunStreamManager: () => ({
    connect: runStreamManagerMock.connect,
    reconnect: runStreamManagerMock.reconnect,
    streamStates: runStreamManagerMock.streamStates,
    subscriptions: runStreamManagerMock.subscriptions,
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
  permissions: [],
}
const sessionValue: SessionContextValue = {
  initialized: true,
  isLoading: false,
  isLoggingIn: false,
  isLoggingOut: false,
  recentLoginUsers: [],
  user: currentUser,
  login: vi.fn(async () => currentUser),
  logout: vi.fn(async () => {}),
  refreshUser: vi.fn(async () => {}),
  updateProfile: vi.fn(async () => currentUser),
  updateLanguage: vi.fn(async () => currentUser),
}

beforeEach(() => {
  runStreamManagerMock.connect.mockReset()
  runStreamManagerMock.reconnect.mockReset()
  runStreamManagerMock.syncConversation.mockReset()
  runStreamManagerMock.streamStates.length = 0
  runStreamManagerMock.subscriptions.length = 0
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

  it('keeps the latest completed run subscribed until its stream reaches terminal', async () => {
    vi.spyOn(api, 'getAIConversationTimeline').mockResolvedValue({
      conversation: {
        id: 'conversation-1',
        title: 'Conversation',
        titleSource: 'user',
        status: 'active',
        modelId: 'model-1',
      },
      turns: [{
        id: 'turn-1',
        turnIndex: 0,
        status: 'completed',
        input: {
          id: 'turn-1:input',
          type: 'user_message',
          createdAt: '2026-10-20T00:00:00Z',
          parts: [{ id: 'input:0', partIndex: 0, type: 'text', text: 'hello' }],
        },
        selectedRun: {
          id: 'run-1',
          runIndex: 0,
          status: 'completed',
          items: [],
        },
      }],
      eventCursors: [{ runId: 'run-1', after: 4 }],
      pageInfo: { hasOlder: false },
    })
    const user = userEvent.setup()
    renderRuntimeHarness()

    await user.click(screen.getByRole('button', { name: 'select-conversation' }))

    await waitFor(() => expect(runStreamManagerMock.syncConversation).toHaveBeenCalledWith(
      'conversation-1',
      [expect.objectContaining({
        conversationId: 'conversation-1',
        eventsUrl: '/api/v1/ai/runs/run-1/events',
        runId: 'run-1',
        after: 4,
      })],
    ))
  })

  it('does not recreate a completed run subscription after its stream is terminal', async () => {
    vi.spyOn(api, 'getAIConversationTimeline').mockResolvedValue({
      conversation: {
        id: 'conversation-1',
        title: 'Conversation',
        titleSource: 'user',
        status: 'active',
        modelId: 'model-1',
      },
      turns: [{
        id: 'turn-1',
        turnIndex: 0,
        status: 'completed',
        input: {
          id: 'turn-1:input',
          type: 'user_message',
          createdAt: '2026-10-20T00:00:00Z',
          parts: [{ id: 'input:0', partIndex: 0, type: 'text', text: 'hello' }],
        },
        selectedRun: {
          id: 'run-1',
          runIndex: 0,
          status: 'completed',
          items: [],
        },
      }],
      eventCursors: [{ runId: 'run-1', after: 4 }],
      pageInfo: { hasOlder: false },
    })
    runStreamManagerMock.streamStates.push({ runId: 'run-1', status: 'terminal' })
    const user = userEvent.setup()
    renderRuntimeHarness()

    await user.click(screen.getByRole('button', { name: 'select-conversation' }))
    await waitFor(() => expect(api.getAIConversationTimeline).toHaveBeenCalled())

    expect(runStreamManagerMock.syncConversation).not.toHaveBeenCalledWith(
      'conversation-1',
      [expect.objectContaining({ runId: 'run-1' })],
    )
  })

  it('submits { text, expectedVersion } on waiting_input and applies exponential backoff before stalled reconnect', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const submitSpy = vi.spyOn(api, 'submitAIRunInput').mockResolvedValue()
    vi.spyOn(api, 'listAIModels').mockResolvedValue([
      { id: 'model-1', name: 'Model 1', maxContextTokens: 128_000, maxOutputTokens: 16_000 },
    ])
    vi.spyOn(api, 'getAIConversationTimeline').mockResolvedValue({
      conversation: {
        id: 'conversation-1',
        title: 'Conversation',
        titleSource: 'user',
        status: 'active',
        modelId: 'model-1',
      },
      turns: [{
        id: 'turn-1',
        turnIndex: 0,
        status: 'waiting_input',
        input: { id: 'turn-1:input', type: 'user_message', createdAt: '2026-11-17T00:00:00Z', parts: [{ id: 'p0', partIndex: 0, type: 'text', text: 'initial' }] },
        selectedRun: {
          id: 'run-waiting',
          runIndex: 0,
          status: 'waiting_input',
          expectedVersion: 7,
          items: [],
        },
      }],
      eventCursors: [{ runId: 'run-waiting', after: 3 }],
      pageInfo: { hasOlder: false },
    })
    runStreamManagerMock.streamStates.push({ runId: 'run-waiting', status: 'stalled' })

    renderRuntimeHarness()
    await user.click(screen.getByRole('button', { name: 'select-conversation' }))
    await waitFor(() => expect(api.getAIConversationTimeline).toHaveBeenCalledTimes(1))

    // Stalled recovery should NOT fire synchronously at 0ms (prevents 503 tight reconnect loop)
    expect(runStreamManagerMock.reconnect).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1_600)
    await waitFor(() => expect(runStreamManagerMock.reconnect).toHaveBeenCalledWith('run-waiting'))

    // Submitting supplemental input uses { text, expectedVersion } matching backend contract
    await user.click(screen.getByRole('button', { name: 'submit-waiting-input' }))
    await user.click(await screen.findByRole('button', { name: 'confirm-waiting-input' }))
    await waitFor(() => expect(submitSpy).toHaveBeenCalledWith('run-waiting', {
      text: 'prj_demo',
      expectedVersion: 7,
    }))
    vi.useRealTimers()
  })

  it('[P1-5 & P1-2] handles 503 during stalled recovery without UnhandledPromiseRejection, skips reconnect on terminal snapshot, and refreshes timeline on 409 input conflict', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const unhandled = vi.fn()
    window.addEventListener('unhandledrejection', unhandled)
    try {
      let timelineCall = 0
      vi.spyOn(api, 'getAIConversationTimeline').mockImplementation(async () => {
        timelineCall += 1
        if (timelineCall === 2) {
          throw new ApiError('service unavailable', { status: 503, code: 'service.unavailable', path: '/api/v1/ai/conversations/conversation-1/timeline' })
        }
        if (timelineCall === 3) {
          return {
            conversation: { id: 'conversation-1', title: 'Stalled', titleSource: 'default', status: 'active', modelId: 'aimod_primary' },
            turns: [{
              id: 'turn-1',
              turnIndex: 0,
              status: 'waiting_input',
              input: { id: 'turn-1:input', type: 'user_message', createdAt: '2026-11-17T00:00:00Z', parts: [{ id: 'p0', partIndex: 0, type: 'text', text: 'initial' }] },
              selectedRun: {
                id: 'run-stalled',
                runIndex: 0,
                status: 'waiting_input',
                expectedVersion: 6,
                items: [],
              },
            }],
            eventCursors: [{ runId: 'run-stalled', after: 5 }],
            pageInfo: { hasOlder: false },
          }
        }
        if (timelineCall >= 4) {
          return {
            conversation: { id: 'conversation-1', title: 'Stalled', titleSource: 'default', status: 'active', modelId: 'aimod_primary' },
            turns: [{
              id: 'turn-1',
              turnIndex: 0,
              status: 'failed',
              input: { id: 'turn-1:input', type: 'user_message', createdAt: '2026-11-17T00:00:00Z', parts: [{ id: 'p0', partIndex: 0, type: 'text', text: 'initial' }] },
              selectedRun: {
                id: 'run-stalled',
                runIndex: 0,
                status: 'failed',
                expectedVersion: 9,
                items: [],
              },
            }],
            eventCursors: [{ runId: 'run-stalled', after: 6 }],
            pageInfo: { hasOlder: false },
          }
        }
        return {
          conversation: { id: 'conversation-1', title: 'Stalled', titleSource: 'default', status: 'active', modelId: 'aimod_primary' },
          turns: [{
            id: 'turn-1',
            turnIndex: 0,
            status: 'waiting_input',
            input: { id: 'turn-1:input', type: 'user_message', createdAt: '2026-11-17T00:00:00Z', parts: [{ id: 'p0', partIndex: 0, type: 'text', text: 'initial' }] },
            selectedRun: {
              id: 'run-stalled',
              runIndex: 0,
              status: 'waiting_input',
              expectedVersion: 4,
              items: [],
            },
          }],
          eventCursors: [{ runId: 'run-stalled', after: 2 }],
          pageInfo: { hasOlder: false },
        }
      })
      vi.spyOn(api, 'submitAIRunInput').mockRejectedValueOnce(
        new ApiError('conflict', { status: 409, code: 'ai.run_not_resumable', path: '/api/v1/ai/runs/run-stalled/input' }),
      )
      runStreamManagerMock.streamStates.push({ runId: 'run-stalled', status: 'stalled' })

      renderRuntimeHarness()
      await user.click(screen.getByRole('button', { name: 'select-conversation' }))
      await waitFor(() => expect(api.getAIConversationTimeline).toHaveBeenCalledTimes(1))

      // 1) stalled 第 1 次恢复遇到 503 时捕获异常（不抛 UnhandledPromiseRejection）并重连
      await vi.advanceTimersByTimeAsync(1_600)
      await waitFor(() => expect(runStreamManagerMock.reconnect).toHaveBeenCalledTimes(1))
      expect(unhandled).not.toHaveBeenCalled()

      // 2) submitRunInput 返回 409 时触发 timeline 刷新（第 3 次查询拉回 expectedVersion=6）
      await user.click(screen.getByRole('button', { name: 'submit-waiting-input' }))
      await user.click(await screen.findByRole('button', { name: 'confirm-waiting-input' }))
      await waitFor(() => expect(timelineCall).toBe(3))
      expect(unhandled).not.toHaveBeenCalled()
    }
    finally {
      window.removeEventListener('unhandledrejection', unhandled)
      vi.useRealTimers()
    }
  })

  it('[P1-5] skips reconnectRunStream when stalled recovery snapshot already reached a terminal status', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    try {
      let call = 0
      vi.spyOn(api, 'getAIConversationTimeline').mockImplementation(async () => {
        call += 1
        return {
          conversation: { id: 'conversation-1', title: 'Terminal', titleSource: 'default', status: 'active', modelId: 'aimod_primary' },
          turns: [{
            id: 'turn-1',
            turnIndex: 0,
            status: call === 1 ? 'running' : 'completed',
            input: { id: 'turn-1:input', type: 'user_message', createdAt: '2026-11-17T00:00:00Z', parts: [{ id: 'p0', partIndex: 0, type: 'text', text: 'initial' }] },
            selectedRun: {
              id: 'run-terminal',
              runIndex: 0,
              status: call === 1 ? 'running' : 'completed',
              expectedVersion: call === 1 ? 2 : 5,
              items: [],
            },
          }],
          eventCursors: [{ runId: 'run-terminal', after: call === 1 ? 1 : 8 }],
          pageInfo: { hasOlder: false },
        }
      })
      runStreamManagerMock.streamStates.push({ runId: 'run-terminal', status: 'stalled' })

      renderRuntimeHarness()
      await user.click(screen.getByRole('button', { name: 'select-conversation' }))
      await waitFor(() => expect(api.getAIConversationTimeline).toHaveBeenCalledTimes(1))

      await vi.advanceTimersByTimeAsync(1_600)
      await waitFor(() => expect(call).toBe(2))
      expect(runStreamManagerMock.reconnect).not.toHaveBeenCalled()
    }
    finally {
      vi.useRealTimers()
    }
  })

  it('reconnects/attaches the active Run SSE stream and invalidates timeline after approval decision', async () => {
    const user = userEvent.setup()
    const decideSpy = vi.spyOn(api, 'decideAIToolApproval').mockResolvedValue()
    vi.spyOn(api, 'getAIConversationTimeline').mockResolvedValue({
      conversation: {
        id: 'conversation-1',
        title: 'Approval Flow',
        titleSource: 'user',
        status: 'active',
        modelId: 'aimod_primary',
      },
      turns: [{
        id: 'turn-approval',
        turnIndex: 0,
        status: 'waiting_approval',
        input: { id: 'turn-approval:input', type: 'user_message', createdAt: '2026-11-17T00:00:00Z', parts: [{ id: 'p0', partIndex: 0, type: 'text', text: 'deploy' }] },
        selectedRun: {
          id: 'run-approval',
          runIndex: 0,
          status: 'waiting_approval',
          expectedVersion: 2,
          items: [],
        },
      }],
      eventCursors: [{ runId: 'run-approval', after: 4 }],
      pageInfo: { hasOlder: false },
    })

    renderRuntimeHarness()
    await user.click(screen.getByRole('button', { name: 'select-conversation' }))
    await waitFor(() => expect(api.getAIConversationTimeline).toHaveBeenCalledTimes(1))

    runStreamManagerMock.connect.mockClear()
    runStreamManagerMock.reconnect.mockClear()

    await user.click(screen.getByRole('button', { name: 'approve-pending-tool' }))

    await waitFor(() => expect(decideSpy).toHaveBeenCalledWith('run-approval', 'aitool_approval_1', { decision: 'approve' }))
    expect(runStreamManagerMock.reconnect).toHaveBeenCalledWith('run-approval')
    expect(runStreamManagerMock.connect).toHaveBeenCalledWith({
      conversationId: 'conversation-1',
      eventsUrl: '/api/v1/ai/runs/run-approval/events',
      runId: 'run-approval',
      after: 4,
    })
    await waitFor(() => expect(api.getAIConversationTimeline).toHaveBeenCalledTimes(2))
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
      <button
        aria-label="submit-waiting-input"
        type="button"
        onClick={() => {
          runtime.composerProps.onDraftChange('prj_demo')
        }}
      />
      {runtime.composerProps.draft === 'prj_demo' && (
        <button aria-label="confirm-waiting-input" type="button" onClick={runtime.composerProps.onSubmit} />
      )}
      <button
        aria-label="approve-pending-tool"
        type="button"
        onClick={() => void runtime.timelineProps.onApproval({
          id: 'item-approval',
          turnId: 'turn-approval',
          runId: 'run-approval',
          index: 1,
          type: 'tool_call',
          toolCallId: 'aitool_approval_1',
          operationId: 'createRelease',
          visibility: 'normal',
          status: 'awaiting_approval',
          arguments: {},
          uiActions: [],
        }, 'approve')}
      />
    </div>
  )
}
