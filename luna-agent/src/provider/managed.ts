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

/**
 * Provider 请求方言。它是唯一决定“是否需要回传 reasoning_content、max_tokens 字段、
 * 渠道亲和与 prompt 缓存键”的事实源。
 * 显式配置优先；自动模式先看官方端点，再看模型名（网关隐藏上游地址时唯一的可用信号）。
 */
export type ProviderDialect = "openai" | "deepseek"

export function providerDialect(config: RemoteProviderConfig, modelName?: string): ProviderDialect {
  const configured = config.provider.providerCompatibility
  if (configured === "deepseek") return "deepseek"
  if (configured === "openai") return "openai"
  if (isDeepSeekEndpoint(config.provider.baseUrl)) return "deepseek"
  return modelName && isDeepSeekModelName(modelName) ? "deepseek" : "openai"
}

export function createConfiguredProvider(config: RemoteProviderConfig, modelName: string): ModelProvider {
  const dialect = providerDialect(config, modelName)
  const Provider = dialect === "deepseek"
    ? DeepSeekChatCompletionsProvider
    : OpenAIChatCompletionsProvider
  return new Provider({
    baseUrl: config.provider.baseUrl,
    apiKey: config.provider.apiKey,
    channelAffinityEnabled: config.provider.channelAffinityEnabled,
    promptCacheKeyEnabled: dialect === "openai" && promptCacheKeyEnabled(config),
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

/**
 * 网关隐藏上游地址时的模型名嗅探：`deepseek-flash`、`deepseek/deepseek-chat`、
 * `deepseek-ai/DeepSeek-V3.2` 等命名都指向 DeepSeek 方言。只用于自动模式，
 * 显式选择 OpenAI 兼容仍可关掉回传。
 */
function isDeepSeekModelName(modelName: string): boolean {
  return /(?:^|[^a-z0-9])deepseek(?:$|[^a-z0-9])/i.test(modelName.trim())
}

function isOfficialOpenAIEndpoint(baseUrl: string): boolean {
  try { return new URL(baseUrl).hostname.toLowerCase() === "api.openai.com" }
  catch { return false }
}
