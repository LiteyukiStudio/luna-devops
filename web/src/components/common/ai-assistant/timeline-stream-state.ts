import type { AIBlock } from './state'

export function shouldShowTypingIndicator({ activeTurnId, blocks, generating, outputStreaming = false }: {
  activeTurnId?: string
  blocks: AIBlock[]
  generating: boolean
  outputStreaming?: boolean
}) {
  if (!generating)
    return false
  const currentTurnHasStreamingBlock = Boolean(activeTurnId)
    && blocks.some(block => block.turnId === activeTurnId && block.status === 'streaming')
  return !currentTurnHasStreamingBlock || !outputStreaming
}
