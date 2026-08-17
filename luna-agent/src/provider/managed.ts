import type { ProviderConfigClient, RemoteAIModel, RemoteProviderConfig } from "./config-client.js"
import { OpenAICompatibleProvider } from "./openai-compatible.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

type ProviderFactory = (config: RemoteProviderConfig, modelName?: string) => ModelProvider
type ResolvedProvider = { expiresAt: number, version: string, modelKey: string, provider: ModelProvider }

export class ManagedProvider implements ModelProvider {
  #cached: ResolvedProvider | undefined
  #config: { expiresAt: number, value: RemoteProviderConfig } | undefined
  #loading: { version: string, modelKey: string, promise: Promise<ResolvedProvider> } | undefined
  #configLoading: Promise<RemoteProviderConfig> | undefined

  constructor(
    private readonly resolver: Pick<ProviderConfigClient, "get">,
    private readonly ttlMs: number,
    private readonly factory: ProviderFactory = defaultFactory,
  ) {}

  capabilities(): ModelCapabilities {
    return this.#cached?.provider.capabilities() ?? { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    return (await this.resolve(request)).provider.complete(request)
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    yield* (await this.resolve(request)).provider.stream(request)
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    return (await this.resolve({ messages: [], maxOutputTokens: 1 })).provider.health()
  }

  invalidate(): void {
    this.#cached = undefined
    this.#config = undefined
  }

  currentVersion(): string | undefined {
    return this.#cached?.version ?? this.#config?.value.version
  }

  private async resolve(request: ModelRequest): Promise<ResolvedProvider> {
    const config = await this.getConfig(request.signal)
    const selected = resolveModel(config, request)
    if (!config.provider.configured || !config.provider.apiKey || !config.provider.baseUrl || !selected.name) {
      throw new Error("ai.not_configured")
    }
    const modelKey = selected.id || selected.name
    const now = Date.now()
    if (this.#cached && this.#cached.expiresAt > now && this.#cached.version === config.version && this.#cached.modelKey === modelKey)
      return this.#cached
    if (this.#loading?.version === config.version && this.#loading.modelKey === modelKey)
      return this.#loading.promise
    const promise: Promise<ResolvedProvider> = Promise.resolve().then(() => {
      const selectedConfig: RemoteProviderConfig = { ...config, provider: { ...config.provider, model: selected.name } }
      const resolved: ResolvedProvider = {
        version: config.version,
        modelKey,
        provider: this.factory(selectedConfig, selected.name),
        expiresAt: Date.now() + this.ttlMs,
      }
      this.#cached = resolved
      return resolved
    }).finally(() => {
      if (this.#loading?.promise === promise)
        this.#loading = undefined
    })
    this.#loading = { version: config.version, modelKey, promise }
    return promise
  }

  private async getConfig(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    if (this.#config && this.#config.expiresAt > Date.now()) return this.#config.value
    if (this.#configLoading) return this.#configLoading
    this.#configLoading = this.resolver.get(signal).then(value => {
      this.#config = { value, expiresAt: Date.now() + this.ttlMs }
      return value
    }).finally(() => { this.#configLoading = undefined })
    return this.#configLoading
  }
}

function resolveModel(config: RemoteProviderConfig, request: ModelRequest): RemoteAIModel {
  const models = config.provider.models ?? []
  if (request.modelId) {
    const selected = models.find(model => model.id === request.modelId)
    if (selected) return selected
    // Existing Runs retain their immutable snapshot even after the model is disabled.
    if (request.modelName && request.modelPricing) return request.modelPricing
    throw new Error("ai.model_not_available")
  }
  if (request.modelName) {
    return models.find(model => model.name === request.modelName) ?? request.modelPricing ?? snapshotModel(request.modelName)
  }
  return models[0] ?? (config.provider.model ? snapshotModel(config.provider.model) : snapshotModel(""))
}

function snapshotModel(name: string): RemoteAIModel {
  return { id: "legacy", name, maxContextTokens: 524_288, maxOutputTokens: 65_536, inputCreditsPerMillion: "0", outputCreditsPerMillion: "0", cachedInputCreditsPerMillion: "0", cachedOutputCreditsPerMillion: "0" }
}

function defaultFactory(config: RemoteProviderConfig): ModelProvider {
  return new OpenAICompatibleProvider({
    baseUrl: config.provider.baseUrl,
    apiKey: config.provider.apiKey,
    model: config.provider.model,
    timeoutMs: config.runtime.providerTimeoutMs,
    maxRetries: config.runtime.maxRequestRetries,
  })
}
