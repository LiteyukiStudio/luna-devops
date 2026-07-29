import type { ProviderConfigClient, RemoteProviderConfig } from "./config-client.js"
import { OpenAICompatibleProvider } from "./openai-compatible.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

type ProviderFactory = (config: RemoteProviderConfig) => ModelProvider

export class ManagedProvider implements ModelProvider {
  private cached: { expiresAt: number, version: string, provider: ModelProvider } | undefined
  private loading: Promise<{ expiresAt: number, version: string, provider: ModelProvider }> | undefined

  constructor(
    private readonly resolver: Pick<ProviderConfigClient, "get">,
    private readonly ttlMs: number,
    private readonly factory: ProviderFactory = defaultFactory,
  ) {}

  capabilities(): ModelCapabilities {
    return this.cached?.provider.capabilities() ?? { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    return (await this.resolve(request.signal)).provider.complete(request)
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    yield* (await this.resolve(request.signal)).provider.stream(request)
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    return (await this.resolve()).provider.health()
  }

  invalidate(): void {
    this.cached = undefined
  }

  currentVersion(): string | undefined {
    return this.cached?.version
  }

  private async resolve(signal?: AbortSignal) {
    const now = Date.now()
    if (this.cached && this.cached.expiresAt > now) return this.cached
    if (this.loading) return this.loading
    this.loading = this.resolver.get(signal).then(config => {
      if (!config.provider.configured || !config.provider.apiKey || !config.provider.baseUrl || !config.provider.defaultModel) {
        throw new Error("ai.not_configured")
      }
      const resolved = { version: config.version, provider: this.factory(config), expiresAt: Date.now() + this.ttlMs }
      this.cached = resolved
      return resolved
    }).finally(() => { this.loading = undefined })
    return this.loading
  }
}

function defaultFactory(config: RemoteProviderConfig): ModelProvider {
  if (config.provider.type !== "openai-compatible") throw new Error("ai.provider_unsupported")
  return new OpenAICompatibleProvider({
    baseUrl: config.provider.baseUrl,
    apiKey: config.provider.apiKey,
    model: config.provider.defaultModel,
    timeoutMs: 30000,
  })
}
