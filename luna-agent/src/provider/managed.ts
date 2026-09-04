import type { RemoteAIModel, RemoteConfigSnapshot, RemoteProviderConfig } from "./config-client.js"
import { DeepSeekChatCompletionsProvider } from "./deepseek-chat-completions.js"
import { OpenAIChatCompletionsProvider } from "./openai-chat-completions.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

type ProviderFactory = (config: RemoteProviderConfig, modelName: string) => ModelProvider
type ResolvedProvider = { version: string, modelKey: string, provider: ModelProvider }

export class ManagedProvider implements ModelProvider {
  #cacheVersion: string | undefined
  #cached = new Map<string, ResolvedProvider>()

  constructor(
    private readonly snapshot: Pick<RemoteConfigSnapshot, "current">,
    private readonly factory: ProviderFactory = createConfiguredProvider,
  ) {}

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    return this.resolve(request).provider.complete(request)
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    yield* this.resolve(request).provider.stream(request)
  }

  async health(): Promise<{ ok: boolean, requestId?: string }> {
    return this.resolve({ messages: [], maxOutputTokens: 1 }).provider.health()
  }

  private resolve(request: ModelRequest): ResolvedProvider {
    const config = request.providerConfig ?? this.snapshot.current()
    if (!config) throw new Error("ai.provider_config_unavailable")
    const selected = resolveModel(config, request)
    if (!config.provider.configured || !config.provider.apiKey || !config.provider.baseUrl) {
      throw new Error("ai.not_configured")
    }
    const modelKey = selected.id
    if (this.#cacheVersion !== config.version) {
      this.#cached.clear()
      this.#cacheVersion = config.version
    }
    const cached = this.#cached.get(modelKey)
    if (cached) return cached
    const resolved: ResolvedProvider = {
      version: config.version,
      modelKey,
      provider: this.factory(config, selected.name),
    }
    this.#cached.set(modelKey, resolved)
    return resolved
  }
}

function resolveModel(config: RemoteProviderConfig, request: ModelRequest): RemoteAIModel {
  const models = config.provider.models
  if (request.modelId) {
    const selected = models.find(model => model.id === request.modelId)
    if (selected) return selected
    // Existing Runs retain their immutable snapshot even after the model is disabled.
    if (request.modelPricing?.id === request.modelId && request.modelName === request.modelPricing.name) return request.modelPricing
    throw new Error("ai.model_not_available")
  }
  if (request.modelName) {
    const selected = models.find(model => model.name === request.modelName)
    if (selected) return selected
    if (request.modelPricing?.name === request.modelName) return request.modelPricing
    throw new Error("ai.model_not_available")
  }
  const selected = models[0]
  if (!selected) throw new Error("ai.model_not_available")
  return selected
}

export function createConfiguredProvider(config: RemoteProviderConfig, modelName: string): ModelProvider {
  const deepSeekCompatible = config.provider.providerCompatibility === "deepseek"
    || config.provider.providerCompatibility === "auto" && isDeepSeekEndpoint(config.provider.baseUrl)
  const Provider = deepSeekCompatible
    ? DeepSeekChatCompletionsProvider
    : OpenAIChatCompletionsProvider
  return new Provider({
    baseUrl: config.provider.baseUrl,
    apiKey: config.provider.apiKey,
    channelAffinityEnabled: config.provider.channelAffinityEnabled,
    promptCacheKeyEnabled: !deepSeekCompatible && promptCacheKeyEnabled(config),
    model: modelName,
    timeoutMs: config.runtime.providerTimeoutMs,
  })
}

function promptCacheKeyEnabled(config: RemoteProviderConfig): boolean {
  switch (config.provider.promptCacheKeyMode) {
    case "enabled": return true
    case "disabled": return false
    case "auto": return isOfficialOpenAIEndpoint(config.provider.baseUrl)
  }
}

function isDeepSeekEndpoint(baseUrl: string): boolean {
  try {
    const hostname = new URL(baseUrl).hostname.toLowerCase()
    return hostname === "deepseek.com" || hostname.endsWith(".deepseek.com")
  }
  catch { return false }
}

function isOfficialOpenAIEndpoint(baseUrl: string): boolean {
  try { return new URL(baseUrl).hostname.toLowerCase() === "api.openai.com" }
  catch { return false }
}
