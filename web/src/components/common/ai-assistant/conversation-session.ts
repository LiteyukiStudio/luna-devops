export const REFRESH_CONVERSATION_RETURN_DURATION_MS = 15_000

export interface AIConversationSessionState {
  activeConversationId?: string
  hasOpened: boolean
  refreshReturnExpiresAt?: number
}

export type AIConversationSessionAction
  = | { type: 'open', now: number }
    | { type: 'select', conversationId: string }
    | { type: 'start_new' }
    | { type: 'clear_deleted', conversationIds: string[] }
    | { type: 'dismiss_refresh_return' }

export const initialAIConversationSessionState: AIConversationSessionState = {
  hasOpened: false,
}

export function aiConversationSessionReducer(
  state: AIConversationSessionState,
  action: AIConversationSessionAction,
): AIConversationSessionState {
  switch (action.type) {
    case 'open':
      if (state.hasOpened)
        return state
      return {
        ...state,
        hasOpened: true,
        refreshReturnExpiresAt: action.now + REFRESH_CONVERSATION_RETURN_DURATION_MS,
      }
    case 'select':
      return {
        ...state,
        activeConversationId: action.conversationId,
        refreshReturnExpiresAt: undefined,
      }
    case 'start_new':
      return {
        ...state,
        activeConversationId: undefined,
        refreshReturnExpiresAt: undefined,
      }
    case 'clear_deleted':
      if (!state.activeConversationId || !action.conversationIds.includes(state.activeConversationId))
        return state
      return {
        ...state,
        activeConversationId: undefined,
      }
    case 'dismiss_refresh_return':
      if (!state.refreshReturnExpiresAt)
        return state
      return {
        ...state,
        refreshReturnExpiresAt: undefined,
      }
  }
}
