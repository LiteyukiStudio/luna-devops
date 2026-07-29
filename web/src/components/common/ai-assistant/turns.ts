import type { AIBlock } from './state'

export type MessageBlock = Extract<AIBlock, { type: 'message' }>

export interface AIAssistantTurn {
  id: string
  index: number
  userMessage?: MessageBlock
  responseBlocks: AIBlock[]
}

export function groupAIAssistantBlocksByTurn(blocks: AIBlock[]): AIAssistantTurn[] {
  const turns = new Map<string, AIAssistantTurn>()
  for (const block of [...blocks].sort((a, b) => a.index - b.index)) {
    const turn = turns.get(block.turnId) ?? {
      id: block.turnId,
      index: block.index,
      responseBlocks: [],
    }
    turn.index = Math.min(turn.index, block.index)
    if (block.type === 'message' && block.role === 'user') {
      turn.userMessage ??= block
    }
    else {
      turn.responseBlocks.push(block)
    }
    turns.set(block.turnId, turn)
  }
  return [...turns.values()].sort((a, b) => a.index - b.index)
}
