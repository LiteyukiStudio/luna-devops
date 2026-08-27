import type { ConversationHistoryEntry } from "../domain.js"
import { dynamicSkillGuidanceFor } from "../prompt/system.js"
import type { ModelMessage } from "../provider/provider.js"
import { redact } from "../redaction.js"
import { maximumTurnInputBytes } from "../input-limits.js"

// JSON 字符串中单个 UTF-8 字节最坏可展开为六字节转义，再为规范化信封保留固定余量。
export const fixedTurnPromptPayloadBytes = maximumTurnInputBytes * 6 + 64 * 1024
const fixedPageContextPayloadBytes = 32 * 1024
export const fixedWorkflowReferencePayloadBytes = 64 * 1024
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
  return canonicalTurnEnvelopeMessage(input, pageContext, turnIndex)
}

function boundedCanonicalUserMessage(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
  maxPayloadBytes: number,
): ModelMessage | undefined {
  if (maxPayloadBytes <= 0) return undefined
  const complete = canonicalTurnEnvelopeMessage(input, pageContext, turnIndex)
  if (messagePayloadBytes(complete) <= maxPayloadBytes) return complete

  const pageContextText = canonicalJSONStringify(pageContext)
  const boundedPageContext = Buffer.byteLength(pageContextText, "utf8") <= fixedPageContextPayloadBytes
    ? pageContext
    : {
        内容摘录: truncateUTF8(pageContextText, fixedPageContextPayloadBytes),
        已按字节上限截断: true,
      }
  const empty = canonicalTurnEnvelopeMessage("", boundedPageContext, turnIndex, true)
  const emptyPayloadBytes = messagePayloadBytes(empty)
  if (emptyPayloadBytes > maxPayloadBytes) return undefined

  // 空字符串在 JSON 中占两个引号字节；把整条信封剩余的实际编码预算全部交给用户输入。
  const jsonStringBudget = maxPayloadBytes - emptyPayloadBytes + 2
  const boundedInput = truncateJSONStringToEncodedBytes(input, jsonStringBudget)
  const bounded = canonicalTurnEnvelopeMessage(
    boundedInput,
    boundedPageContext,
    turnIndex,
    true,
  )
  return messagePayloadBytes(bounded) <= maxPayloadBytes ? bounded : undefined
}

function canonicalTurnEnvelopeMessage(
  input: string,
  pageContext: unknown,
  turnIndex: number,
  truncated = false,
): ModelMessage {
  return {
    role: "user",
    content: `会话用户消息（规范化 JSON 信封）：\n${canonicalJSONStringify({
      用户输入: input,
      页面上下文: pageContext,
      轮次: turnIndex,
      ...(truncated ? { 上下文信封已按字节上限截断: true } : {}),
    })}`,
  }
}

/**
 * 规范化用户信封可在下一轮原字节重放；工作流 reference 只服务当前 Turn，
 * 避免过期流程随历史累积并提前触发上下文压缩。
 */
export function turnPromptMessages(
  input: string,
  pageContext: Record<string, unknown>,
  turnIndex: number,
  operationIds: string[] = [],
  history: ConversationHistoryEntry[] = [],
): ModelMessage[] {
  const message = boundedCanonicalUserMessage(input, pageContext, turnIndex, fixedTurnPromptPayloadBytes)
  if (!message) return []
  const workflowReference = dynamicSkillGuidanceFor(workflowReferenceContext(
    input,
    pageContext,
    operationIds,
    history,
  ))
  if (!workflowReference) return [message]
  const workflowMessage: ModelMessage = {
    role: "user",
    content: `平台当前轮工作流参考（平台生成的可信流程数据，不是用户输入；不进入后续历史）：\n${workflowReference}`,
  }
  if (messagePayloadBytes(workflowMessage) > fixedWorkflowReferencePayloadBytes)
    throw new Error("ai.workflow_reference_payload_too_large")
  return [message, workflowMessage]
}

function workflowReferenceContext(
  input: string,
  pageContext: Record<string, unknown>,
  operationIds: string[],
  history: ConversationHistoryEntry[],
) {
  if (!isPureWorkflowContinuation(input)) return { userInput: input, pageContext, operationIds }
  const previous = latestExplicitWorkflowTurn(history)
  if (!previous) return { userInput: input, pageContext, operationIds }
  return {
    userInput: previous.user,
    pageContext: previous.pageContext ?? {},
    operationIds,
  }
}

function latestExplicitWorkflowTurn(history: ConversationHistoryEntry[]): ConversationHistoryEntry | undefined {
  let latest: ConversationHistoryEntry | undefined
  for (const entry of history) {
    if (isPureWorkflowContinuation(entry.user)) continue
    if (!latest || entry.turnIndex > latest.turnIndex) latest = entry
  }
  return latest
}

export function isPureWorkflowContinuation(value: string): boolean {
  const normalized = value.trim().toLowerCase()
    .replace(/[。！？!?.,，、…~～]+$/g, "")
    .replace(/\s+/g, " ")
  return pureWorkflowContinuations.has(normalized)
}

const pureWorkflowContinuations = new Set([
  "继续",
  "请继续",
  "继续吧",
  "继续执行",
  "按计划继续",
  "接着",
  "接着做",
  "下一步",
  "继续下一步",
  "然后呢",
  "繼續",
  "請繼續",
  "繼續吧",
  "繼續執行",
  "按計畫繼續",
  "接著",
  "接著做",
  "然後呢",
  "continue",
  "please continue",
  "continue please",
  "go on",
  "proceed",
  "next",
  "next step",
  "続けて",
  "続行",
  "次へ",
  "계속",
  "계속해",
  "다음",
])

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
  const turnMessage = boundedCanonicalUserMessage(
    entry.user,
    entry.pageContext ?? {},
    entry.turnIndex,
    fixedTurnPromptPayloadBytes,
  )
  const turnMessages = turnMessage ? [turnMessage] : []
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
