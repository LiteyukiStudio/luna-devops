import type { AIBlock } from './state'

export type MessageBlock = Extract<AIBlock, { type: 'message' }>

export interface AIAssistantTurn {
  id: string
  index: number
  userMessage?: MessageBlock
  responseBlocks: AIBlock[]
}

function isTurnEndInteractionCard(block: AIBlock): boolean {
  if (block.type !== 'tool_call')
    return false
  const isPreparing = block.operationId === 'prepare_interaction_cards' && block.status === 'running'
  const isCreated = block.operationId === 'create_interaction_cards' && block.status === 'succeeded'
  return (isPreparing || isCreated) && block.arguments.placement === 'turn_end'
}

/**
 * 只投影展示顺序，不修改权威 timelineIndex。一个回合出现多个末尾卡片时，
 * 说明模型没有形成唯一等待点，保持真实事件顺序比猜测卡片优先级更安全。
 */
export function projectAIAssistantResponseBlocks(blocks: AIBlock[]): AIBlock[] {
  const deferred = blocks.filter(isTurnEndInteractionCard)
  const deferredBlock = deferred[0]
  if (deferred.length !== 1 || !deferredBlock)
    return blocks
  return [...blocks.filter(block => block.id !== deferredBlock.id), deferredBlock]
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
  return [...turns.values()]
    .map(turn => ({ ...turn, responseBlocks: projectAIAssistantResponseBlocks(turn.responseBlocks) }))
    .sort((a, b) => a.index - b.index)
}
