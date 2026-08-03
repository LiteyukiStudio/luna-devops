import type {
  InteractionCard,
  InteractionCardAction,
  InteractionCardGroup,
  InteractionContentBlock,
  InteractionFormField,
} from '@luna-devops/ai-interaction-card-contract'

export type {
  InteractionCard,
  InteractionCardAction,
  InteractionCardGroup,
  InteractionContentBlock,
  InteractionFormField,
}

/**
 * 卡片参数只从 Agent 已完成权威 Schema 校验并持久化的成功工具项进入渲染层。
 * Web 不再维护第二套业务 Schema；这里只拒绝版本不匹配或明显损坏的事件包络。
 */
export function readValidatedInteractionCardGroup(value: unknown): InteractionCardGroup | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    return undefined
  const candidate = value as Record<string, unknown>
  if (candidate.schemaVersion !== 1 || typeof candidate.generationId !== 'string' || !Array.isArray(candidate.cards))
    return undefined
  return candidate as unknown as InteractionCardGroup
}
