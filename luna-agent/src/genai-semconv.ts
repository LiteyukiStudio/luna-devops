import type { Attributes } from "@opentelemetry/api"
import type { ModelMessage, ModelToolCall, ModelToolDefinition } from "./provider/provider.js"
import { redact, redactSensitivePaths } from "./redaction.js"

export const genAISchemaURL = "https://opentelemetry.io/schemas/gen-ai-dev/1.42.0-dev"
export const genAIAgentName = "Luna Agent"
export const genAIAgentDescription = "Operates and diagnoses Luna DevOps resources."

let genAIAgentVersion: string | undefined
const registeredCatalogSensitivePaths = new Map<string, readonly string[]>()

export function configureGenAIAgentVersion(value?: string): void {
  genAIAgentVersion = value?.trim() || undefined
}

/**
 * 将平台工具目录中的全量 sensitivePaths 注入全局映射，确保历史轮次中调用过、
 * 当前模型步已被 LRU 淘汰未挂载的工具参数在 OTel 内容采集中仍按路径脱敏。
 */
export function registerToolCatalogSensitivePaths(entries: Iterable<readonly [string, readonly string[]]>): void {
  for (const [operationId, paths] of entries) {
    if (!paths.length) continue
    const existing = registeredCatalogSensitivePaths.get(operationId)
    if (!existing) {
      registeredCatalogSensitivePaths.set(operationId, [...paths])
      continue
    }
    const merged = new Set([...existing, ...paths])
    if (merged.size > existing.length) {
      registeredCatalogSensitivePaths.set(operationId, [...merged])
    }
  }
}

export function sensitivePathsForOperation(operationId: string): readonly string[] {
  return registeredCatalogSensitivePaths.get(operationId) ?? []
}

export function redactToolArgumentsForOperation<T>(operationId: string, args: T, explicitPaths?: readonly string[]): T {
  const paths = explicitPaths?.length ? explicitPaths : sensitivePathsForOperation(operationId)
  return redact(redactSensitivePaths(args, paths))
}

type GenAITextPart = { type: "text", content: string }
type GenAIReasoningPart = { type: "reasoning", content: string }
type GenAIToolCallPart = { type: "tool_call", name: string, id?: string, arguments?: Record<string, unknown> }
type GenAIToolCallResponsePart = { type: "tool_call_response", response: unknown, id?: string }
type GenAIMessagePart = GenAITextPart | GenAIReasoningPart | GenAIToolCallPart | GenAIToolCallResponsePart

export type GenAISystemInstruction = {
  type: "text"
  content: string
}

export type GenAIInputMessage = {
  role: ModelMessage["role"]
  parts: GenAIMessagePart[]
}

export type GenAIOutputMessage = {
  role: "assistant"
  parts: GenAIMessagePart[]
  finish_reason: string
}

export type GenAIToolDefinition = {
  type: "function"
  name: string
  description?: string
  parameters?: Record<string, unknown>
}

export function genAIAgentSpanAttributes(conversationId: string, model?: string, maxTokens?: number): Attributes {
  return {
    "gen_ai.operation.name": "invoke_agent",
    "gen_ai.agent.name": genAIAgentName,
    "gen_ai.agent.description": genAIAgentDescription,
    "gen_ai.conversation.id": conversationId,
    "gen_ai.output.type": "text",
    ...(model ? { "gen_ai.request.model": model } : {}),
    ...(maxTokens !== undefined ? { "gen_ai.request.max_tokens": maxTokens } : {}),
    ...(genAIAgentVersion ? { "gen_ai.agent.version": genAIAgentVersion } : {}),
  }
}

export function genAIModelSpan(baseUrl: string, providerName: string, model: string, maxTokens: number, streaming: boolean) {
  const endpoint = new URL(baseUrl)
  const port = endpoint.port ? Number(endpoint.port) : endpoint.protocol === "https:" ? 443 : 80
  return {
    name: `chat ${model}`,
    attributes: {
      "gen_ai.operation.name": "chat",
      "gen_ai.provider.name": providerName,
      "gen_ai.request.model": model,
      "gen_ai.request.max_tokens": maxTokens,
      "gen_ai.output.type": "text",
      "server.address": endpoint.hostname,
      "server.port": port,
      ...(streaming ? { "gen_ai.request.stream": true } : {}),
    } satisfies Attributes,
  }
}

/**
 * 低基数 Token 用量指标标签：仅保留平台受控的 provider / request.model / operation / token.type，
 * 上游自由文本 gen_ai.response.model 只写入 Span Attribute，严禁进入 Metric Label。
 */
export function genAIClientTokenUsageAttributes(
  providerName: string,
  requestModel: string,
  tokenType: "input" | "output",
): Attributes {
  return {
    "gen_ai.operation.name": "chat",
    "gen_ai.provider.name": providerName,
    "gen_ai.request.model": requestModel,
    "gen_ai.token.type": tokenType,
  }
}

export function genAIToolSpanAttributes(input: {
  name: string
  callId?: string
  description?: string
}): Attributes {
  return {
    "gen_ai.operation.name": "execute_tool",
    "gen_ai.agent.name": genAIAgentName,
    "gen_ai.tool.name": input.name,
    "gen_ai.tool.type": "extension",
    ...(input.callId ? { "gen_ai.tool.call.id": input.callId } : {}),
    ...(input.description ? { "gen_ai.tool.description": input.description } : {}),
  }
}

export function genAISystemInstructions(messages: ModelMessage[]): GenAISystemInstruction[] {
  return messages.flatMap((message) => {
    if (message.role !== "system" || !message.content) return []
    return [{ type: "text" as const, content: redact(message.content) }]
  })
}

export function genAIInputMessages(
  messages: ModelMessage[],
  tools?: ModelToolDefinition[],
  catalogSensitivePaths?: ReadonlyMap<string, readonly string[]>,
): GenAIInputMessage[] {
  const sensitivePaths = toolSensitivePaths(tools, catalogSensitivePaths)
  return messages.map((message) => {
    if (message.role === "tool") {
      return {
        role: "tool",
        parts: [{
          type: "tool_call_response",
          id: message.toolCallId,
          response: redact(parseJSONValue(message.content)),
        }],
      }
    }
    const parts: GenAIMessagePart[] = []
    if (message.content) parts.push({ type: "text", content: message.content })
    if (message.role === "assistant") {
      for (const call of message.toolCalls ?? []) parts.push(genAIToolCallPart(call, sensitivePaths))
    }
    return { role: message.role, parts }
  })
}

export function genAIOutputMessages(
  input: {
    text: string
    reasoningSummary?: string
    toolCalls?: ModelToolCall[]
    finishReason?: string
  },
  tools?: ModelToolDefinition[],
  catalogSensitivePaths?: ReadonlyMap<string, readonly string[]>,
): GenAIOutputMessage[] {
  const sensitivePaths = toolSensitivePaths(tools, catalogSensitivePaths)
  const parts: GenAIMessagePart[] = []
  if (input.reasoningSummary) parts.push({ type: "reasoning", content: input.reasoningSummary })
  if (input.text) parts.push({ type: "text", content: input.text })
  for (const call of input.toolCalls ?? []) parts.push(genAIToolCallPart(call, sensitivePaths))
  return [{
    role: "assistant",
    parts,
    finish_reason: input.finishReason || (input.toolCalls?.length ? "tool_call" : "stop"),
  }]
}

export function genAIToolDefinitions(tools: ModelToolDefinition[] | undefined): GenAIToolDefinition[] {
  return (tools ?? []).map(tool => ({
    type: "function",
    name: tool.operationId,
    ...(tool.description ? { description: tool.description } : {}),
    parameters: tool.inputSchema,
  }))
}

export function genAIToolCallObject(value: unknown): Record<string, unknown> {
  if (value && typeof value === "object" && !Array.isArray(value)) return value as Record<string, unknown>
  if (Array.isArray(value)) return { items: value }
  return value === undefined ? {} : { value }
}

function genAIToolCallPart(call: ModelToolCall, sensitivePaths: ReadonlyMap<string, readonly string[]>): GenAIToolCallPart {
  const paths = sensitivePaths.get(call.operationId) ?? []
  const pathRedacted = paths.length ? redactSensitivePaths(call.arguments, paths) : call.arguments
  return {
    type: "tool_call",
    name: call.operationId,
    ...(call.id ? { id: call.id } : {}),
    arguments: redact(pathRedacted),
  }
}

function toolSensitivePaths(
  tools: ModelToolDefinition[] | undefined,
  catalogSensitivePaths?: ReadonlyMap<string, readonly string[]>,
): ReadonlyMap<string, readonly string[]> {
  const merged = new Map<string, readonly string[]>(registeredCatalogSensitivePaths)
  if (catalogSensitivePaths) {
    for (const [operationId, paths] of catalogSensitivePaths) {
      if (paths.length) merged.set(operationId, paths)
    }
  }
  for (const tool of tools ?? []) {
    if (!tool.sensitivePaths?.length) continue
    const existing = merged.get(tool.operationId)
    merged.set(tool.operationId, existing ? [...new Set([...existing, ...tool.sensitivePaths])] : tool.sensitivePaths)
  }
  return merged
}

function parseJSONValue(value: string): unknown {
  try {
    return JSON.parse(value)
  }
  catch {
    return value
  }
}
