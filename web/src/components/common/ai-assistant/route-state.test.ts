import { describe, expect, it } from 'vitest'
import {
  AI_ASSISTANT_DIRECT_ACCESS_FALLBACK_PATH,
  createAIAssistantRouteState,
  isAIAssistantRoutePath,
  readAIAssistantRouteState,
  resolveAIAssistantReturnPath,
  withAIAssistantRouteView,
} from './route-state'

describe('ai assistant route state', () => {
  it('stores only the internal pathname, search, and hash', () => {
    const state = createAIAssistantRouteState({
      pathname: '/projects/prj-1/applications/app-1',
      search: '?tab=deployments',
      hash: '#latest',
    })

    expect(state).toEqual({
      returnTo: {
        pathname: '/projects/prj-1/applications/app-1',
        search: '?tab=deployments',
        hash: '#latest',
      },
      aiView: 'chat',
      hasChatHistoryEntry: false,
    })
    expect(resolveAIAssistantReturnPath(state)).toBe('/projects/prj-1/applications/app-1?tab=deployments#latest')
  })

  it.each([
    'https://attacker.example/phishing',
    'javascript:alert(1)',
    '//attacker.example/phishing',
    '/ai-assistant',
    '/ai-assistant/',
    '/AI-ASSISTANT/conversations',
  ])('rejects unsafe or self-referencing return pathname %s', (pathname) => {
    const state = createAIAssistantRouteState({ pathname })
    expect(resolveAIAssistantReturnPath(state)).toBe(AI_ASSISTANT_DIRECT_ACCESS_FALLBACK_PATH)
  })

  it('rejects malformed structured location components', () => {
    expect(resolveAIAssistantReturnPath({
      returnTo: { pathname: '/projects', search: '//attacker.example', hash: '' },
      aiView: 'chat',
    })).toBe(AI_ASSISTANT_DIRECT_ACCESS_FALLBACK_PATH)
    expect(resolveAIAssistantReturnPath({
      returnTo: { pathname: '/projects', search: '?tab=all', hash: 'section' },
      aiView: 'chat',
    })).toBe(AI_ASSISTANT_DIRECT_ACCESS_FALLBACK_PATH)
  })

  it('uses the dashboard and chat view for direct access or untrusted history state', () => {
    expect(readAIAssistantRouteState(undefined)).toEqual({
      returnTo: { pathname: '/dashboard', search: '', hash: '' },
      aiView: 'chat',
      hasChatHistoryEntry: false,
    })
    expect(readAIAssistantRouteState({ returnTo: 'https://attacker.example', aiView: 'admin', hasChatHistoryEntry: true })).toEqual({
      returnTo: { pathname: '/dashboard', search: '', hash: '' },
      aiView: 'chat',
      hasChatHistoryEntry: false,
    })
  })

  it('preserves the safe return location while changing chat history views', () => {
    const conversations = createAIAssistantRouteState({ pathname: '/projects', search: '?page=2' }, 'conversations')
    expect(readAIAssistantRouteState(conversations).aiView).toBe('conversations')
    expect(conversations.hasChatHistoryEntry).toBe(false)

    const chat = withAIAssistantRouteView(conversations, 'chat')
    expect(chat).toEqual({
      returnTo: { pathname: '/projects', search: '?page=2', hash: '' },
      aiView: 'chat',
      hasChatHistoryEntry: false,
    })
    expect(conversations.aiView).toBe('conversations')

    const pushedConversations = withAIAssistantRouteView(chat, 'conversations')
    expect(pushedConversations.hasChatHistoryEntry).toBe(true)
    expect(readAIAssistantRouteState(pushedConversations)).toEqual(pushedConversations)
  })

  it('recognizes only the assistant route and its descendants', () => {
    expect(isAIAssistantRoutePath('/ai-assistant?view=chat')).toBe(true)
    expect(isAIAssistantRoutePath('/ai-assistant/conversations')).toBe(true)
    expect(isAIAssistantRoutePath('/ai-assistant-settings')).toBe(false)
    expect(isAIAssistantRoutePath('/dashboard')).toBe(false)
  })
})
