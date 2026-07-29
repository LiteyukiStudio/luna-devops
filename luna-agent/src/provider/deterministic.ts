import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

export class DeterministicProvider implements ModelProvider {
  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: false, structuredOutput: true }
  }

  async health(): Promise<{ ok: boolean, requestId: string }> {
    return { ok: true, requestId: "deterministic-local" }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    const input = request.messages.at(-1)?.content ?? ""
    const directive = input.split("\n").findLast(line => line.startsWith("tool:"))
    const match = /^tool:([A-Za-z][A-Za-z0-9._-]+)\s+(\{.*\})$/s.exec(directive ?? "")
    if (!input.includes("Tool results") && match?.[1] && match[2]) {
      return { text: "", toolCalls: [{ operationId: match[1], arguments: JSON.parse(match[2]) as Record<string, unknown> }], usage: { inputTokens: input.length, outputTokens: 0 } }
    }
    const text = `已收到你的问题：“${input}”。当前为确定性本地 Provider；服务已完成上下文校验和持久化，可安全替换为 OpenAI-compatible Provider。`
    return { text, reasoningSummary: "正在检查会话上下文与可用的只读能力。", usage: { inputTokens: input.length, outputTokens: text.length } }
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    const response = await this.complete(request)
    yield { type: "reasoning_summary_delta", delta: response.reasoningSummary ?? "" }
    for (const chunk of response.text.match(/.{1,24}/gu) ?? []) {
      if (request.signal?.aborted) throw request.signal.reason
      yield { type: "message_delta", delta: chunk }
    }
    yield { type: "completed", usage: response.usage, ...(response.toolCalls ? { toolCalls: response.toolCalls } : {}) }
  }
}
