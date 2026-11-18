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
   * 按 DeepSeek 官方多轮工具调用规范：
   * - 历史轮次（messageIndex < lastUserIndex）的 assistant 消息直接省略 reasoning_content，不注入伪造占位文本；
   * - 当前轮次工具调用链（messageIndex > lastUserIndex）的 assistant 步必须携带非空字符串字段（无思考文本时回退单空格 `" "`，防止中间网关 `omitempty` 剥离空字符串引发 DeepSeek 400）。
   */
  protected override providerMessages(messages: ModelRequest["messages"]): unknown[] {
    let lastUserIndex = -1
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      if (messages[index]?.role === "user") {
        lastUserIndex = index
        break
      }
    }
    return messages.map((message, messageIndex) => {
      if (message.role === "tool") {
        return { role: "tool", tool_call_id: message.toolCallId, content: message.content }
      }
      if (message.role === "assistant") {
        const base = super.assistantPayload(message, messageIndex)
        if (messageIndex > lastUserIndex) {
          return { ...base, reasoning_content: message.reasoning || " " }
        }
        return base
      }
      return message
    })
  }

  protected override reasoningDelta(delta: Record<string, unknown> | undefined): string {
    return typeof delta?.reasoning_content === "string" ? delta.reasoning_content : ""
  }

  protected override adaptUsagePayload(raw: unknown): unknown {
    if (!isRecord(raw) || raw.prompt_cache_hit_tokens === undefined || raw.prompt_cache_hit_tokens === null) return raw
    if (raw.prompt_tokens_details !== undefined && raw.prompt_tokens_details !== null && !isRecord(raw.prompt_tokens_details)) return raw
    const details = isRecord(raw.prompt_tokens_details) ? raw.prompt_tokens_details : {}
    if (details.cached_tokens !== undefined && details.cached_tokens !== null) return raw
    return {
      ...raw,
      prompt_tokens_details: { ...details, cached_tokens: raw.prompt_cache_hit_tokens },
    }
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value)
}
