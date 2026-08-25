import type { ComponentProps } from 'react'
import type { AIAssistantComposer } from './composer'
import type { AIConversationList } from './conversation-list'
import type { AISuggestionSet } from './suggestions'
import type { AIAssistantTimeline } from './timeline'
import type { AICapabilities, AIUIAction } from '@/api'
import { createContext, use } from 'react'

export interface AIAssistantWorkspaceLocation {
  hash: string
  pathname: string
  search: string
}

export type AIAssistantConversationListProps = Omit<ComponentProps<typeof AIConversationList>, 'onBack' | 'variant'> & {
  error: Error | null
  refetch: () => Promise<void>
}

export type AIAssistantTimelineProps = Omit<ComponentProps<typeof AIAssistantTimeline>, 'bottomInset'>
export type AIAssistantComposerProps = ComponentProps<typeof AIAssistantComposer>

export interface AIAssistantRuntimeValue {
  capabilities?: AICapabilities
  canCreateConversation: boolean
  canDebugInternalTools: boolean
  closeAssistant: () => void
  composerProps: AIAssistantComposerProps
  conversationListProps: AIAssistantConversationListProps
  conversationSearch: string
  conversationTitle?: string
  createConversation: () => void
  creatingConversation: boolean
  enabled: boolean
  executeAction: (action: AIUIAction) => Promise<boolean>
  open: boolean
  openAssistant: () => void
  rememberWorkspaceLocation: (location: AIAssistantWorkspaceLocation) => void
  selectedConversationId?: string
  selectConversation: (conversationId: string) => void
  setConversationSearch: (search: string) => void
  startNewConversation: () => void
  surfaceVisible: boolean
  suggestions: AISuggestionSet | null
  timelineProps: AIAssistantTimelineProps
  transitionAssistantToPage: () => void
  toolDebugMode: {
    enabled: boolean
    toggle: () => void
  }
  workspaceLocation: AIAssistantWorkspaceLocation
}

export const AIAssistantRuntimeContext = createContext<AIAssistantRuntimeValue | null>(null)

export function useAIAssistantRuntime() {
  const value = use(AIAssistantRuntimeContext)
  if (!value)
    throw new Error('useAIAssistantRuntime must be used inside AIAssistantRuntimeProvider')
  return value
}
