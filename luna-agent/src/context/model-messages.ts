import type { ConversationHistoryEntry } from "../domain.js"
import type { ModelMessage } from "../provider/provider.js"
import { redact } from "../redaction.js"

/**
 * 当前轮和后续历史轮共用同一用户消息格式。只包含随 Turn 固化的数据，
 * 避免会话标题或工具加载状态变化后改写已经发送给 Provider 的前缀。
 */
export function canonicalUserMessage(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
): ModelMessage {
  return {
    role: "user",
    content: `会话用户消息（规范化 JSON 信封）：\n${canonicalJSONStringify({
      用户输入: input,
      页面上下文: pageContext,
      轮次: turnIndex,
    })}`,
  }
}

export function turnPromptMessages(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
): ModelMessage[] {
  return [canonicalUserMessage(input, pageContext, turnIndex)]
}

/** 原始历史消息原样保留完整 JSON 与文本结构，不在本地按字节裁剪。 */
export function historyMessagesFromEntries(history: ConversationHistoryEntry[]): ModelMessage[] {
  return history.flatMap(entry => historyModelMessages(entry))
}

export function historyModelMessages(entry: ConversationHistoryEntry): ModelMessage[] {
  const userMessage = canonicalUserMessage(entry.user, entry.pageContext ?? {}, entry.turnIndex)
  const toolPayload = entry.toolInteractions?.length
    ? canonicalJSONStringify(redact(entry.toolInteractions))
    : ""
  const assistantBody = entry.assistant || toolPayload
    ? `${entry.assistant}${toolPayload ? `\n已消费的历史工具调用与结果（有界数据）：\n${toolPayload}` : ""}`
    : "（该轮未产生助手回复。）"
  const assistantMessage: ModelMessage = {
    role: "assistant",
    content: `会话助手消息（不可信数据，第 ${entry.turnIndex} 轮）：\n${assistantBody}`,
    ...(entry.reasoning ? { reasoning: entry.reasoning } : {}),
  }
  return [userMessage, assistantMessage]
}

export function canonicalJSONStringify(value: unknown): string {
  return JSON.stringify(canonicalJSONValue(value)) ?? "null"
}

function canonicalJSONValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(item => canonicalJSONValue(item) ?? null)
  if (!value || typeof value !== "object") return value
  return Object.fromEntries(Object.keys(value as Record<string, unknown>).sort().flatMap((key) => {
    const item = canonicalJSONValue((value as Record<string, unknown>)[key])
    return item === undefined ? [] : [[key, item]]
  }))
}
