import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

type Options = { baseUrl: string, apiKey: string, model: string, timeoutMs: number }

export class OpenAICompatibleProvider implements ModelProvider {
  constructor(private readonly options: Options) {}

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    const response = await this.request([{ role: "user", content: "Reply with OK." }], 4)
    return { ok: response.text.length > 0, ...(response.requestId ? { requestId: response.requestId } : {}) }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    const response = await this.request(request.messages, request.maxOutputTokens, request.signal)
    return { text: response.text, usage: response.usage }
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    // The adapter normalizes provider output. P0 uses a complete request so no
    // provider-specific partial JSON can escape into Luna's event stream.
    const result = await this.complete(request)
    yield { type: "message_delta", delta: result.text }
    yield { type: "completed", usage: result.usage }
  }

  private async request(messages: ModelRequest["messages"], maxTokens: number, signal?: AbortSignal) {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), this.options.timeoutMs)
    const abort = () => controller.abort(signal?.reason)
    signal?.addEventListener("abort", abort, { once: true })
    try {
      const response = await fetch(new URL("chat/completions", ensureTrailingSlash(this.options.baseUrl)), {
        method: "POST",
        headers: { authorization: `Bearer ${this.options.apiKey}`, "content-type": "application/json" },
        body: JSON.stringify({ model: this.options.model, messages, max_tokens: maxTokens, stream: false }),
        signal: controller.signal,
      })
      if (!response.ok) throw new Error(`Provider request failed with status ${response.status}`)
      const body = await response.json() as {
        choices?: Array<{ message?: { content?: string } }>
        usage?: { prompt_tokens?: number, completion_tokens?: number }
      }
      return {
        text: body.choices?.[0]?.message?.content ?? "",
        usage: { inputTokens: body.usage?.prompt_tokens ?? 0, outputTokens: body.usage?.completion_tokens ?? 0 },
        requestId: response.headers.get("x-request-id") ?? undefined,
      }
    } finally {
      clearTimeout(timer)
      signal?.removeEventListener("abort", abort)
    }
  }
}

function ensureTrailingSlash(value: string): string {
  return value.endsWith("/") ? value : `${value}/`
}
