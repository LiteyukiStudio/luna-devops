import { z } from "zod"
import { trace } from "@opentelemetry/api"
import type { RemoteRuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, clientSpanOptions, errorDiagnostic, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"
import { agentRuntimeInternals, defaultRuntimeSettings } from "../runtime-settings.js"
import type { AIModelSnapshot } from "../domain.js"
import { ToolCatalog } from "../tools/catalog.js"

export type RemoteAIModel = AIModelSnapshot

export type ProviderCompatibility = "auto" | "openai" | "deepseek"
export type PromptCacheKeyMode = "auto" | "enabled" | "disabled"

export type RemoteProviderConfig = {
  version: string
  provider: {
    baseUrl: string
    apiKey: string
    providerCompatibility: ProviderCompatibility
    promptCacheKeyMode: PromptCacheKeyMode
    channelAffinityEnabled: boolean
    configured: boolean
    models: RemoteAIModel[]
  }
  runtime: RemoteRuntimeSettings
  toolCatalog: unknown[]
}

type PreparedRemoteConfig = Readonly<{
  config: RemoteProviderConfig
  catalog: ToolCatalog
}>

export type RemoteConfigListener = (config: RemoteProviderConfig, catalog: ToolCatalog) => void

export interface RemoteConfigSource {
  current(): RemoteProviderConfig | undefined
  currentCatalog(): ToolCatalog | undefined
  subscribe(listener: RemoteConfigListener): () => void
}

const runtimeSettingsSchema = z.object({
  providerTimeoutMs: z.number().int().min(1_000).max(900_000),
  maxRequestRetries: z.number().int().min(0).max(10),
  runTimeoutMs: z.number().int().min(30_000).max(7_200_000),
  agentConcurrentRuns: z.number().int().min(1).max(100),
  userConcurrentRuns: z.number().int().min(1).max(100),
}).strict()

const remoteProviderConfigSchema = z.object({
  version: z.string().min(1),
  provider: z.object({
    baseUrl: z.string(),
    apiKey: z.string(),
    providerCompatibility: z.enum(["auto", "openai", "deepseek"]),
    promptCacheKeyMode: z.enum(["auto", "enabled", "disabled"]),
    channelAffinityEnabled: z.boolean(),
    configured: z.boolean(),
    models: z.array(z.object({
      id: z.string().min(1), name: z.string().min(1),
      maxContextTokens: z.number().int().min(4096).max(2097152),
      maxOutputTokens: z.number().int().min(256).max(262144),
      inputCreditsPerMillion: z.string(), outputCreditsPerMillion: z.string(),
      cachedInputCreditsPerMillion: z.string(),
    })),
  }),
  runtime: runtimeSettingsSchema,
  toolCatalog: z.array(z.record(z.string(), z.unknown())),
})

export class RemoteConfigSnapshot implements RemoteConfigSource {
  private currentSnapshot?: PreparedRemoteConfig
  private readonly listeners = new Set<RemoteConfigListener>()
  private timer: ReturnType<typeof setTimeout> | undefined
  private refreshInFlight: Promise<RemoteProviderConfig> | undefined
  private polling = false

  constructor(
    private readonly baseUrl: string,
    private readonly serviceToken: string,
    private readonly refreshMs: number = agentRuntimeInternals.configRefreshMs,
  ) {}

  current(): RemoteProviderConfig | undefined {
    return this.currentSnapshot?.config
  }

  currentCatalog(): ToolCatalog | undefined {
    return this.currentSnapshot?.catalog
  }

  async initialize(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    return this.refresh(signal)
  }

  refresh(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    if (!this.refreshInFlight) {
      const inFlight = this.refreshOnce().finally(() => {
        if (this.refreshInFlight === inFlight) this.refreshInFlight = undefined
      })
      this.refreshInFlight = inFlight
    }
    return waitForRefresh(this.refreshInFlight, signal)
  }

  private async refreshOnce(): Promise<RemoteProviderConfig> {
    const candidate = await this.fetchCandidate()
    this.currentSnapshot = candidate
    for (const listener of this.listeners) listener(candidate.config, candidate.catalog)
    return candidate.config
  }

  subscribe(listener: RemoteConfigListener): () => void {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  start(): void {
    if (this.polling) return
    this.polling = true
    this.scheduleRefresh()
  }

  private scheduleRefresh(): void {
    if (!this.polling) return
    this.timer = setTimeout(() => {
      this.timer = undefined
      void this.refresh().catch(() => undefined).finally(() => this.scheduleRefresh())
    }, this.refreshMs)
    this.timer.unref()
  }

  stop(): void {
    this.polling = false
    if (this.timer) clearTimeout(this.timer)
    this.timer = undefined
  }

  private async fetchCandidate(): Promise<PreparedRemoteConfig> {
    return withSpan("luna_api.provider_config.get", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
    }), async span => {
    let response: Response
    try {
      response = await this.fetchConfig()
    }
    catch (error) {
	  telemetryLog("agent.provider_config.failed", "warn", {
		"operation": "agent.provider_config.get",
		"outcome": "failed",
		...errorDiagnostic(error, "ai.provider_config_unavailable"),
	  })
      throw new Error("ai.provider_config_unavailable", { cause: error })
    }
    span.setAttribute("http.response.status_code", response.status)
    agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: response.ok ? "success" : String(response.status) })
    if (!response.ok) {
	  const error = new Error(`Luna API provider configuration returned HTTP ${response.status}`)
	  telemetryLog("agent.provider_config.failed", "warn", {
		"operation": "agent.provider_config.get",
		"outcome": "failed",
		"http.response.status_code": response.status,
		...errorDiagnostic(error, "ai.provider_config_unavailable"),
	  })
      throw new Error("ai.provider_config_unavailable", { cause: error })
    }
    let payload: unknown
    try {
      payload = await response.json()
    }
    catch {
      logInvalidProviderConfig(["$"], ["invalid_json"])
      throw new Error("ai.provider_config_invalid")
    }
    const parsed = remoteProviderConfigSchema.safeParse(payload)
    if (!parsed.success) {
      logInvalidProviderConfig(
        parsed.error.issues.map(issue => stableConfigIssuePath(issue.path)),
        parsed.error.issues.map(issue => issue.code),
      )
      throw new Error("ai.provider_config_invalid")
    }
    const config = immutableRemoteProviderConfig(parsed.data)
    let catalog: ToolCatalog
    try {
      catalog = ToolCatalog.load(config.toolCatalog)
    }
    catch (error) {
      const issues = error instanceof z.ZodError ? error.issues : []
      logInvalidProviderConfig(
        issues.length ? issues.map(issue => stableConfigIssuePath(["toolCatalog", ...issue.path])) : ["toolCatalog"],
        issues.length ? issues.map(issue => issue.code) : ["invalid_catalog"],
      )
      throw new Error("ai.provider_config_invalid", { cause: error })
    }
    span.setAttribute("luna.provider.config_version", config.version)
    return Object.freeze({ config, catalog })
    })
  }

  private async fetchConfig(): Promise<Response> {
    const maxRetries = this.currentSnapshot?.config.runtime.maxRequestRetries ?? defaultRuntimeSettings.maxRequestRetries
    const request = configFetchSignal()
    try {
      for (let retry = 0; ; retry += 1) {
        try {
          const response = await fetch(new URL("/internal/v1/ai/provider-config", this.baseUrl), {
            headers: { authorization: `Bearer ${this.serviceToken}`, accept: "application/json" },
            signal: request.signal,
          })
          if (!isRetryableHTTPStatus(response.status) || retry >= maxRetries)
            return response
          agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: String(response.status) })
          await this.scheduleRetry(retry + 1, maxRetries, request.signal, parseRetryAfter(response.headers), String(response.status))
        }
        catch (error) {
          if (request.signal.aborted || retry >= maxRetries) throw error
          agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: "network_error" })
          await this.scheduleRetry(retry + 1, maxRetries, request.signal, undefined, "network_error")
        }
      }
    }
    finally {
      request.dispose()
    }
  }

  private async scheduleRetry(attempt: number, maxRetries: number, signal: AbortSignal | undefined, retryAfterMs: number | undefined, reason: string): Promise<void> {
    trace.getActiveSpan()?.addEvent("luna_api.provider_config.retry_scheduled", {
      "retry.attempt": attempt,
      "retry.max_retries": maxRetries,
      "retry.reason": reason,
    })
    telemetryLog("agent.provider_config.retry_scheduled", "warn", {
      "retry.attempt": attempt,
      "retry.max_retries": maxRetries,
      "retry.reason": reason,
    })
    await waitForRetry(attempt, { maxRetries, ...(signal ? { signal } : {}), ...(retryAfterMs !== undefined ? { retryAfterMs } : {}) })
  }
}

function configFetchSignal(): { signal: AbortSignal, dispose: () => void } {
  const controller = new AbortController()
  const timeout = setTimeout(
    () => controller.abort(new Error("ai.provider_config_timeout")),
    agentRuntimeInternals.configFetchTimeoutMs,
  )
  timeout.unref()
  return {
    signal: controller.signal,
    dispose: () => clearTimeout(timeout),
  }
}

function waitForRefresh(refresh: Promise<RemoteProviderConfig>, signal?: AbortSignal): Promise<RemoteProviderConfig> {
  if (!signal) return refresh
  if (signal.aborted) return Promise.reject(abortReason(signal))
  return new Promise((resolve, reject) => {
    const onAbort = () => reject(abortReason(signal))
    signal.addEventListener("abort", onAbort, { once: true })
    void refresh.then(resolve, reject).finally(() => signal.removeEventListener("abort", onAbort))
  })
}

function abortReason(signal: AbortSignal): Error {
  return signal.reason instanceof Error ? signal.reason : new Error("ai.provider_config_aborted")
}

export function immutableRemoteProviderConfig(config: RemoteProviderConfig): RemoteProviderConfig {
  return deepFreeze(config)
}

export function parseRemoteProviderConfig(input: unknown): RemoteProviderConfig {
  const parsed = remoteProviderConfigSchema.safeParse(input)
  if (!parsed.success) throw new Error("ai.provider_config_invalid")
  return immutableRemoteProviderConfig(parsed.data)
}

function stableConfigIssuePath(path: PropertyKey[]): string {
  if (path.length === 0) return "$"
  return path.map(segment => typeof segment === "number" ? "[]" : String(segment)).join(".")
}

function logInvalidProviderConfig(paths: string[], issueCodes: string[]): void {
  telemetryLog("agent.provider_config.failed", "warn", {
    "operation": "agent.provider_config.validate",
    "outcome": "rejected",
    "error.code": "ai.provider_config_invalid",
    "error.type": "AgentProviderConfigError",
    "error.message": "ai.provider_config_invalid",
    "luna.provider_config.invalid_fields": [...new Set(paths)].slice(0, 20),
    "luna.provider_config.issue_codes": [...new Set(issueCodes)].slice(0, 20),
  })
}

function deepFreeze<T>(value: T): T {
  if (!value || typeof value !== "object" || Object.isFrozen(value)) return value
  for (const child of Object.values(value)) deepFreeze(child)
  return Object.freeze(value)
}
