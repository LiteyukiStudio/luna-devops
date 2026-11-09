import type { ModelRequest } from "./provider.js"
import { OpenAIChatCompletionsProvider } from "./openai-chat-completions.js"

/** DeepSeek 的 reasoning_content、max_tokens 与用量字段兼容只存在于此适配器。 */
export class DeepSeekChatCompletionsProvider extends OpenAIChatCompletionsProvider {
  protected override providerName(): string {
    return "deepseek"
  }

  protected override supportsPromptCacheKey(): boolean {
    return false
  }

  protected override buildRequestBody(request: ModelRequest, stream: boolean): Record<string, unknown> {
    const official = super.buildRequestBody(request, stream)
    const { max_completion_tokens: maxCompletionTokens, ...rest } = official
    return { ...rest, max_tokens: maxCompletionTokens }
  }

  /**
   * 思考模式要求回传每一轮 assistant 的完整 reasoning_content：
   * 带 tools 的请求连没有工具调用的 assistant 轮次也要回传，缺失即 400。
   * 无法从持久化事实恢复的轮次用空串占位，保证字段存在且不伪造思维链。
   */
  protected override assistantPayload(
    message: Extract<ModelRequest["messages"][number], { role: "assistant" }>,
    messageIndex: number,
  ): Record<string, unknown> {
    return { ...super.assistantPayload(message, messageIndex), reasoning_content: message.reasoning ?? "" }
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}
