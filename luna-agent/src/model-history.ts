import type { ConversationHistoryEntry } from "./domain.js"

/**
 * Timeline 为兼容 Web 仍把窄卡片工具投影成 create_interaction_cards；
 * 模型历史必须恢复真实 operationId，避免模型误判自己使用了已下线工具。
 */
export function modelVisibleHistory(history: ConversationHistoryEntry[]): ConversationHistoryEntry[] {
  return history.map(entry => entry.toolInteractions?.length
    ? { ...entry, toolInteractions: entry.toolInteractions.map(modelVisibleToolInteraction) }
    : entry)
}

function modelVisibleToolInteraction(interaction: Record<string, unknown>): Record<string, unknown> {
  const content = interaction.content
  if (!content || typeof content !== "object" || Array.isArray(content)) return interaction
  const contentRecord = content as Record<string, unknown>
  const modelOperationId = contentRecord.modelOperationId
  if (contentRecord.operationId !== "create_interaction_cards" || typeof modelOperationId !== "string") return interaction
  return {
    ...interaction,
    content: {
      ...contentRecord,
      operationId: modelOperationId,
      timelineOperationId: "create_interaction_cards",
    },
  }
}
