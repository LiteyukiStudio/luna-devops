import type { Attributes } from "@opentelemetry/api"
import type { ModelMessage, ModelToolCall, ModelToolDefinition } from "./provider/provider.js"
import { redactSensitivePaths } from "./redaction.js"

export const genAISchemaURL = "https://opentelemetry.io/schemas/gen-ai-dev/1.42.0-dev"
export const genAIAgentName = "Luna Agent"
export const genAIAgentDescription = "Operates and diagnoses Luna DevOps resources."

type GenAITextPart = { type: "text", content: string }
type GenAIReasoningPart = { type: "reasoning", content: string }
type GenAIToolCallPart = { type: "tool_call", name: string, id?: string, arguments?: Record<string, unknown> }
type GenAIToolCallResponsePart = { type: "tool_call_response", response: unknown, id?: string }
type GenAIMessagePart = GenAITextPart | GenAIReasoningPart | GenAIToolCallPart | GenAIToolCallResponsePart

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
    ...(process.env.OTEL_SERVICE_VERSION?.trim()
      ? { "gen_ai.agent.version": process.env.OTEL_SERVICE_VERSION.trim() }
      : {}),
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

export function genAIClientTokenUsageAttributes(
  providerName: string,
  requestModel: string,
  tokenType: "input" | "output",
  responseModel?: string,
): Attributes {
  return {
    "gen_ai.operation.name": "chat",
    "gen_ai.provider.name": providerName,
    "gen_ai.request.model": requestModel,
    "gen_ai.token.type": tokenType,
    ...(responseModel ? { "gen_ai.response.model": responseModel } : {}),
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

export function genAIInputMessages(messages: ModelMessage[], tools?: ModelToolDefinition[]): GenAIInputMessage[] {
  const sensitivePaths = toolSensitivePaths(tools)
  return messages.map((message) => {
    if (message.role === "tool") {
      return {
        role: "tool",
        parts: [{
          type: "tool_call_response",
          id: message.toolCallId,
          response: parseJSONValue(message.content),
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

export function genAIOutputMessages(input: {
  text: string
  reasoningSummary?: string
  toolCalls?: ModelToolCall[]
  finishReason?: string
}, tools?: ModelToolDefinition[]): GenAIOutputMessage[] {
  const sensitivePaths = toolSensitivePaths(tools)
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
  return {
    type: "tool_call",
    name: call.operationId,
    ...(call.id ? { id: call.id } : {}),
    arguments: paths.length ? redactSensitivePaths(call.arguments, paths) : call.arguments,
  }
}

function toolSensitivePaths(tools: ModelToolDefinition[] | undefined): ReadonlyMap<string, readonly string[]> {
  return new Map((tools ?? [])
    .filter(tool => tool.sensitivePaths?.length)
    .map(tool => [tool.operationId, tool.sensitivePaths ?? []]))
}

function parseJSONValue(value: string): unknown {
  try {
    return JSON.parse(value)
  }
  catch {
    return value
  }
}
