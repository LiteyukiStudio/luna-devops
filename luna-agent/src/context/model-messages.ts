import type { ConversationHistoryEntry } from "../domain.js"
import { dynamicSkillGuidanceFor } from "../prompt/system.js"
import type { ModelMessage } from "../provider/provider.js"
import { redact } from "../redaction.js"

// 平台单轮输入最多 8 MiB；该固定协议上限不随热配置、重启或副本变化。
export const fixedTurnPromptPayloadBytes = 8 * 1024 * 1024 + 64 * 1024
const fixedAuxiliaryFieldPayloadBytes = 32 * 1024
const fixedHistoryAssistantPayloadBytes = 64 * 1024
const fixedHistoryTurnPayloadBytes = fixedTurnPromptPayloadBytes + fixedHistoryAssistantPayloadBytes
const fixedContinuationMessagePayloadBytes = 64 * 1024
const truncationMarker = "\n[内容已按字节上限截断]"

/**
 * 当前轮和后续历史轮共用同一用户消息格式。只包含随 Turn 固化的数据，
 * 避免会话标题或工具加载状态变化后改写已经发送给 Provider 的前缀。
 */
export function canonicalUserMessage(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
): ModelMessage {
  const workflowReference = dynamicSkillGuidanceFor({ userInput: input, pageContext })
  return canonicalTurnEnvelopeMessage(input, pageContext, workflowReference, turnIndex)
}

function boundedCanonicalUserMessage(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
  maxPayloadBytes: number,
): ModelMessage | undefined {
  if (maxPayloadBytes <= 0) return undefined
  const workflowReference = dynamicSkillGuidanceFor({ userInput: input, pageContext })
  const complete = canonicalTurnEnvelopeMessage(input, pageContext, workflowReference, turnIndex)
  if (messagePayloadBytes(complete) <= maxPayloadBytes) return complete

  const pageContextText = canonicalJSONStringify(pageContext)
  const boundedPageContext = Buffer.byteLength(pageContextText, "utf8") <= fixedAuxiliaryFieldPayloadBytes
    ? pageContext
    : {
        内容摘录: truncateUTF8(pageContextText, fixedAuxiliaryFieldPayloadBytes),
        已按字节上限截断: true,
      }
  const boundedWorkflowReference = workflowReference
    ? truncateUTF8(workflowReference, fixedAuxiliaryFieldPayloadBytes)
    : undefined
  const empty = canonicalTurnEnvelopeMessage("", boundedPageContext, boundedWorkflowReference, turnIndex, true)
  const emptyPayloadBytes = messagePayloadBytes(empty)
  if (emptyPayloadBytes > maxPayloadBytes) return undefined

  // 空字符串在 JSON 中占两个引号字节；把整条信封剩余的实际编码预算全部交给用户输入。
  const jsonStringBudget = maxPayloadBytes - emptyPayloadBytes + 2
  const boundedInput = truncateJSONStringToEncodedBytes(input, jsonStringBudget)
  const bounded = canonicalTurnEnvelopeMessage(
    boundedInput,
    boundedPageContext,
    boundedWorkflowReference,
    turnIndex,
    true,
  )
  return messagePayloadBytes(bounded) <= maxPayloadBytes ? bounded : undefined
}

function canonicalTurnEnvelopeMessage(
  input: string,
  pageContext: unknown,
  workflowReference: string | undefined,
  turnIndex: number,
  truncated = false,
): ModelMessage {
  return {
    role: "user",
    content: `会话用户消息（规范化 JSON 信封）：\n${canonicalJSONStringify({
      用户输入: input,
      页面上下文: pageContext,
      平台工作流参考: workflowReference ?? null,
      轮次: turnIndex,
      ...(truncated ? { 上下文信封已按字节上限截断: true } : {}),
    })}`,
  }
}

/**
 * 每个 Turn 的可信工作流 reference 与用户消息组成不可变消息组。
 * 下一轮从持久化历史重建同一组，再在末尾追加新 Turn，保持 Provider 前缀稳定。
 */
export function turnPromptMessages(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
): ModelMessage[] {
  const message = boundedCanonicalUserMessage(input, pageContext, turnIndex, fixedTurnPromptPayloadBytes)
  return message ? [message] : []
}

/** 历史消息按固定单项上限裁剪；新增轮次不会重新分配旧轮次的字节预算。 */
export function boundHistoryMessages(history: ConversationHistoryEntry[], maxPayloadBytes: number): ModelMessage[] {
  if (!history.length || maxPayloadBytes <= 0) return []
  const groups = history.map(entry => historyModelMessages(entry))
  return latestCompleteGroups(groups, maxPayloadBytes, true)
}

export function historyModelMessages(entry: ConversationHistoryEntry): ModelMessage[] {
  const toolPayload = entry.toolInteractions?.length
    ? canonicalJSONStringify(redact(entry.toolInteractions))
    : ""
  const turnMessages = turnPromptMessages(entry.user, entry.pageContext ?? {}, entry.turnIndex)
  const remainingBytes = fixedHistoryTurnPayloadBytes - messageGroupPayloadBytes(turnMessages)
  if ((!entry.assistant && !toolPayload) || remainingBytes <= 0) return turnMessages
  const assistantPayloadBytes = Math.min(fixedHistoryAssistantPayloadBytes, remainingBytes)
  const assistantTextBytes = toolPayload ? Math.floor(assistantPayloadBytes * 0.7) : assistantPayloadBytes
  const toolPayloadBytes = toolPayload ? assistantPayloadBytes - assistantTextBytes : 0
  const assistant = boundModelMessage({
    role: "assistant",
    content: `会话助手消息（不可信数据，第 ${entry.turnIndex} 轮）：\n${truncateUTF8(entry.assistant, assistantTextBytes)}${toolPayload ? `\n已消费的历史工具调用与结果（有界数据）：\n${truncateUTF8(toolPayload, toolPayloadBytes)}` : ""}`,
  }, assistantPayloadBytes)
  return assistant.content ? [...turnMessages, assistant] : turnMessages
}

/** continuation 以 assistant 开始的完整工具交换为组，固定裁剪单项并在总量溢出时丢弃最旧完整组。 */
export function boundContinuationMessages(messages: ModelMessage[], maxPayloadBytes: number): ModelMessage[] {
  if (!messages.length || maxPayloadBytes <= 0) return []
  const itemPayloadBytes = Math.min(fixedContinuationMessagePayloadBytes, maxPayloadBytes)
  const groups = continuationGroups(messages)
    .map(group => fitMessageGroup(group.map(message => boundModelMessage(message, itemPayloadBytes)), maxPayloadBytes, 512))
  return latestCompleteGroups(groups, maxPayloadBytes)
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

function continuationGroups(messages: ModelMessage[]): ModelMessage[][] {
  const groups: ModelMessage[][] = []
  for (const message of messages) {
    if (message.role === "assistant" || groups.length === 0) groups.push([message])
    else groups.at(-1)!.push(message)
  }
  return groups
}

function fitMessageGroup(messages: ModelMessage[], maxPayloadBytes: number, emergencyItemPayloadBytes: number): ModelMessage[] {
  if (messageGroupPayloadBytes(messages) <= maxPayloadBytes) return messages
  // 单个完整交换自身超过总预算时才触发兜底；预算只由该交换和固定总上限决定，
  // 后续追加新的交换不会再次改写这组消息。
  const itemPayloadBytes = Math.min(emergencyItemPayloadBytes, Math.floor(maxPayloadBytes / messages.length))
  const bounded = messages.map(message => boundModelMessage(message, itemPayloadBytes))
  return messageGroupPayloadBytes(bounded) <= maxPayloadBytes ? bounded : []
}

function latestCompleteGroups(
  groups: ModelMessage[][],
  maxPayloadBytes: number,
  retainOversizedLatest = false,
): ModelMessage[] {
  const kept: ModelMessage[][] = []
  // 历史的最新完整 Turn 必须在紧邻下一轮按原字节重放；continuation 仍严格服从硬总预算。
  const latestPayloadBytes = messageGroupPayloadBytes(groups.at(-1) ?? [])
  let remainingBytes = retainOversizedLatest ? Math.max(maxPayloadBytes, latestPayloadBytes) : maxPayloadBytes
  for (let index = groups.length - 1; index >= 0; index -= 1) {
    const group = groups[index]!
    if (!group.length) break
    const payloadBytes = messageGroupPayloadBytes(group)
    if (payloadBytes > remainingBytes) break
    kept.unshift(group)
    remainingBytes -= payloadBytes
  }
  return kept.flat()
}

function boundModelMessage(message: ModelMessage, maxPayloadBytes: number): ModelMessage {
  if (message.role !== "assistant" || !message.toolCalls?.length)
    return { ...message, content: truncateUTF8(message.content, maxPayloadBytes) }
  const argumentBytes = Math.floor(maxPayloadBytes / 2)
  return {
    ...message,
    content: truncateUTF8(message.content, Math.floor(maxPayloadBytes / 2)),
    toolCalls: message.toolCalls.map(call => ({
      ...call,
      arguments: boundArguments(call.arguments, Math.floor(argumentBytes / message.toolCalls!.length)),
    })),
  }
}

function boundArguments(value: Record<string, unknown>, maxBytes: number): Record<string, unknown> {
  const serialized = canonicalJSONStringify(value)
  return Buffer.byteLength(serialized, "utf8") <= maxBytes
    ? value
    : { _contextExcerpt: truncateUTF8(serialized, Math.max(0, maxBytes - 32)), _truncatedByBytes: true }
}

function messageGroupPayloadBytes(messages: ModelMessage[]): number {
  return messages.reduce((total, message) => total + messagePayloadBytes(message), 0)
}

function messagePayloadBytes(message: ModelMessage): number {
  return Buffer.byteLength(message.content, "utf8")
    + (message.role === "assistant" && message.toolCalls
      ? Buffer.byteLength(canonicalJSONStringify(message.toolCalls), "utf8")
      : 0)
}

export function splitUTF8(value: string, maxBytes: number): string[] {
  const chunks: string[] = []
  let current = ""
  let currentBytes = 0
  for (const character of value) {
    const characterBytes = Buffer.byteLength(character, "utf8")
    if (current && currentBytes + characterBytes > maxBytes) {
      chunks.push(current)
      current = ""
      currentBytes = 0
    }
    current += character
    currentBytes += characterBytes
  }
  if (current) chunks.push(current)
  return chunks
}

export function truncateUTF8(value: string, maxBytes: number): string {
  if (maxBytes <= 0) return ""
  if (Buffer.byteLength(value, "utf8") <= maxBytes) return value
  const available = maxBytes - Buffer.byteLength(truncationMarker, "utf8")
  if (available <= 0) return ""
  let usedBytes = 0
  let endIndex = 0
  for (const character of value) {
    const characterBytes = Buffer.byteLength(character, "utf8")
    if (usedBytes + characterBytes > available) break
    usedBytes += characterBytes
    endIndex += character.length
  }
  return `${value.slice(0, endIndex)}${truncationMarker}`
}

/** 在一次线性扫描中按 JSON 字符串的实际 UTF-8 编码长度截取，避免大输入反复序列化。 */
function truncateJSONStringToEncodedBytes(value: string, maxEncodedBytes: number): string {
  const fullEncodedBytes = Buffer.byteLength(JSON.stringify(value), "utf8")
  if (fullEncodedBytes <= maxEncodedBytes) return value
  const markerEncodedBytes = jsonStringContentByteLength(truncationMarker)
  const availableContentBytes = maxEncodedBytes - 2 - markerEncodedBytes
  if (availableContentBytes <= 0) return ""
  let usedBytes = 0
  let endIndex = 0
  for (const character of value) {
    const characterBytes = jsonStringContentByteLength(character)
    if (usedBytes + characterBytes > availableContentBytes) break
    usedBytes += characterBytes
    endIndex += character.length
  }
  return `${value.slice(0, endIndex)}${truncationMarker}`
}

function jsonStringContentByteLength(value: string): number {
  let bytes = 0
  for (const character of value) {
    const codePoint = character.codePointAt(0)!
    if (character === "\"" || character === "\\" || codePoint === 0x08 || codePoint === 0x09
      || codePoint === 0x0a || codePoint === 0x0c || codePoint === 0x0d) bytes += 2
    else if (codePoint <= 0x1f || (codePoint >= 0xd800 && codePoint <= 0xdfff)) bytes += 6
    else bytes += Buffer.byteLength(character, "utf8")
  }
  return bytes
}
