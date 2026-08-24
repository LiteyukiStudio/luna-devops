import type { ModelRequest } from "./provider.js"
import { OpenAIChatCompletionsProvider } from "./openai-chat-completions.js"

/** DeepSeek 的 thinking、reasoning_content 与 max_tokens 兼容只存在于此适配器。 */
export class DeepSeekChatCompletionsProvider extends OpenAIChatCompletionsProvider {
  protected override providerName(): string {
    return "deepseek"
  }

  protected override buildRequestBody(request: ModelRequest, stream: boolean): Record<string, unknown> {
    const official = super.buildRequestBody(request, stream)
    const { max_completion_tokens: maxCompletionTokens, messages, ...rest } = official
    return {
      ...rest,
      messages: deepSeekMessages(messages),
      max_tokens: maxCompletionTokens,
      ...(request.thinking ? { thinking: request.thinking } : {}),
    }
  }

  protected override reasoningDelta(delta: Record<string, unknown> | undefined): string {
    return typeof delta?.reasoning_content === "string" ? delta.reasoning_content : ""
  }

  protected override adaptUsagePayload(raw: unknown): unknown {
    if (!isRecord(raw) || raw.prompt_cache_hit_tokens === undefined) return raw
    if (raw.prompt_tokens_details !== undefined && !isRecord(raw.prompt_tokens_details)) return raw
    const details = raw.prompt_tokens_details ?? {}
    if (details.cached_tokens !== undefined) return raw
    return {
      ...raw,
      prompt_tokens_details: { ...details, cached_tokens: raw.prompt_cache_hit_tokens },
    }
  }
}

function deepSeekMessages(value: unknown): unknown {
  if (!Array.isArray(value)) return value
  return value.map((message: unknown) => {
    if (!message || typeof message !== "object" || Array.isArray(message)) return message
    const item = message as Record<string, unknown>
    if (item.role !== "assistant" || !Array.isArray(item.tool_calls)) return item
    return { ...item, reasoning_content: typeof item.content === "string" && item.content ? item.content : "继续处理此前已完成的工具调用。" }
  })
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}
