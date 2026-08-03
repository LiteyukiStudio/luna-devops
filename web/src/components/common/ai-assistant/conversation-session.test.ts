import { describe, expect, it } from 'vitest'
import {
  aiConversationSessionReducer,
  initialAIConversationSessionState,
  isRecentConversationInteraction,
  REFRESH_CONVERSATION_RECENCY_MS,
  REFRESH_CONVERSATION_RETURN_DURATION_MS,
} from './conversation-session'

describe('ai conversation session lifecycle', () => {
  it('keeps the refresh return window at eight seconds', () => {
    expect(REFRESH_CONVERSATION_RETURN_DURATION_MS).toBe(8_000)
  })

  it('offers the return only when the previous interaction is within ten minutes', () => {
    const now = Date.parse('2026-08-03T10:10:00.000Z')

    expect(isRecentConversationInteraction('2026-08-03T10:00:00.000Z', now)).toBe(true)
    expect(isRecentConversationInteraction('2026-08-03T09:59:59.999Z', now)).toBe(false)
    expect(REFRESH_CONVERSATION_RECENCY_MS).toBe(10 * 60_000)
  })

  it('does not treat invalid or future timestamps as recent interactions', () => {
    const now = Date.parse('2026-08-03T10:10:00.000Z')

    expect(isRecentConversationInteraction('invalid', now)).toBe(false)
    expect(isRecentConversationInteraction('2026-08-03T10:10:00.001Z', now)).toBe(false)
  })

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
