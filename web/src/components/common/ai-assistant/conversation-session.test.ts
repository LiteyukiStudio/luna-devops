import { describe, expect, it } from 'vitest'
import {
  aiConversationSessionReducer,
  initialAIConversationSessionState,
  REFRESH_CONVERSATION_RETURN_DURATION_MS,
} from './conversation-session'

describe('ai conversation session lifecycle', () => {
  it('starts with an unsaved new conversation and offers a bounded return after the first open', () => {
    const state = aiConversationSessionReducer(initialAIConversationSessionState, { type: 'open', now: 1_000 })

    expect(state.activeConversationId).toBeUndefined()
    expect(state.refreshReturnExpiresAt).toBe(1_000 + REFRESH_CONVERSATION_RETURN_DURATION_MS)
  })

  it('keeps the active conversation when the assistant is closed and reopened without a page refresh', () => {
    const opened = aiConversationSessionReducer(initialAIConversationSessionState, { type: 'open', now: 1_000 })
    const selected = aiConversationSessionReducer(opened, { type: 'select', conversationId: 'conversation-1' })
    const reopened = aiConversationSessionReducer(selected, { type: 'open', now: 5_000 })

    expect(reopened).toEqual(selected)
  })

  it('does not show the refresh-return notice for an intentional new conversation', () => {
    const opened = aiConversationSessionReducer(initialAIConversationSessionState, { type: 'open', now: 1_000 })
    const state = aiConversationSessionReducer(opened, { type: 'start_new' })

    expect(state.activeConversationId).toBeUndefined()
    expect(state.refreshReturnExpiresAt).toBeUndefined()
  })
})
