import { z } from "zod"
import { trace } from "@opentelemetry/api"
import type { RuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, clientSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"
import { defaultRuntimeSettings } from "../runtime-settings.js"
import type { AIModelSnapshot } from "../domain.js"

export type RemoteAIModel = AIModelSnapshot

export type RemoteProviderConfig = {
  version: string
  provider: {
    baseUrl: string
    model: string
    apiKey: string
    configured: boolean
    models?: RemoteAIModel[]
  }
  runtime: RuntimeSettings
  toolCatalog?: unknown[]
}

function compatibleRuntimeNumber(schema: z.ZodNumber, fallback: number) {
  return schema.catch(fallback)
}

const runtimeSettingsSchema = z.object({
  providerTimeoutMs: compatibleRuntimeNumber(z.number().int().min(1_000).max(900_000), defaultRuntimeSettings.providerTimeoutMs),
  maxRequestRetries: compatibleRuntimeNumber(z.number().int().min(0).max(10), defaultRuntimeSettings.maxRequestRetries),
  runTimeoutMs: compatibleRuntimeNumber(z.number().int().min(30_000).max(7_200_000), defaultRuntimeSettings.runTimeoutMs),
  agentConcurrentRuns: compatibleRuntimeNumber(z.number().int().min(1).max(100), defaultRuntimeSettings.agentConcurrentRuns),
  userConcurrentRuns: compatibleRuntimeNumber(z.number().int().min(1).max(100), defaultRuntimeSettings.userConcurrentRuns),
  contextInputTokenBudget: compatibleRuntimeNumber(z.number().int().min(64 * 1024).max(2048 * 1024), defaultRuntimeSettings.contextInputTokenBudget),
  assistantMaxOutputTokens: compatibleRuntimeNumber(z.number().int().min(256).max(128 * 1024), defaultRuntimeSettings.assistantMaxOutputTokens),
  maxModelSteps: compatibleRuntimeNumber(z.number().int().min(1).max(1024), defaultRuntimeSettings.maxModelSteps),
  runMaxToolCalls: compatibleRuntimeNumber(z.number().int().min(32).max(2048), defaultRuntimeSettings.runMaxToolCalls),
  maxInputBytes: compatibleRuntimeNumber(z.number().int().min(8 * 1024).max(8 * 1024 * 1024), defaultRuntimeSettings.maxInputBytes),
  navigateActionTtlSeconds: compatibleRuntimeNumber(z.number().int().min(10).max(600), defaultRuntimeSettings.navigateActionTtlSeconds),
  toolResultPayloadBudget: compatibleRuntimeNumber(z.number().int().min(4 * 1024).max(4 * 1024 * 1024), defaultRuntimeSettings.toolResultPayloadBudget),
  maxCardRepairAttempts: compatibleRuntimeNumber(z.number().int().min(1).max(10), defaultRuntimeSettings.maxCardRepairAttempts),
  contextCompressionTriggerRatio: compatibleRuntimeNumber(z.number().min(0.5).max(0.95), defaultRuntimeSettings.contextCompressionTriggerRatio),
  contextCompressionTargetRatio: compatibleRuntimeNumber(z.number().min(0.1).max(0.8), defaultRuntimeSettings.contextCompressionTargetRatio),
  contextRecentTurnCount: compatibleRuntimeNumber(z.number().int().min(1).max(32), defaultRuntimeSettings.contextRecentTurnCount),
  contextMaxRecentTurnCount: compatibleRuntimeNumber(z.number().int().min(2).max(64), defaultRuntimeSettings.contextMaxRecentTurnCount),
  contextMaxUncompressedTurnCount: compatibleRuntimeNumber(z.number().int().min(4).max(128), defaultRuntimeSettings.contextMaxUncompressedTurnCount),
  contextMaxCompressionTurnsPerCompile: compatibleRuntimeNumber(z.number().int().min(8).max(1024), defaultRuntimeSettings.contextMaxCompressionTurnsPerCompile),
  contextSummaryInputTokenBudget: compatibleRuntimeNumber(z.number().int().min(4 * 1024).max(512 * 1024), defaultRuntimeSettings.contextSummaryInputTokenBudget),
  contextSummaryMaxOutputTokens: compatibleRuntimeNumber(z.number().int().min(200).max(32 * 1024), defaultRuntimeSettings.contextSummaryMaxOutputTokens),
  contextHistoricalToolTokenBudget: compatibleRuntimeNumber(z.number().int().min(1024).max(256 * 1024), defaultRuntimeSettings.contextHistoricalToolTokenBudget),
}).transform((value) => {
  const normalized = { ...value }
  if (normalized.contextCompressionTriggerRatio <= normalized.contextCompressionTargetRatio) {
    normalized.contextCompressionTriggerRatio = defaultRuntimeSettings.contextCompressionTriggerRatio
    normalized.contextCompressionTargetRatio = defaultRuntimeSettings.contextCompressionTargetRatio
  }
  if (normalized.contextRecentTurnCount > normalized.contextMaxRecentTurnCount) {
    normalized.contextRecentTurnCount = defaultRuntimeSettings.contextRecentTurnCount
    normalized.contextMaxRecentTurnCount = defaultRuntimeSettings.contextMaxRecentTurnCount
  }
  return normalized
})

const remoteProviderConfigSchema = z.object({
  version: z.string().min(1),
  provider: z.object({
    baseUrl: z.string(),
    model: z.string(),
    apiKey: z.string(),
    configured: z.boolean(),
    models: z.array(z.object({
      id: z.string().min(1), name: z.string().min(1),
      maxContextTokens: z.number().int().min(4096).max(2097152),
      maxOutputTokens: z.number().int().min(256).max(262144),
      inputCreditsPerMillion: z.string(), outputCreditsPerMillion: z.string(),
      cachedInputCreditsPerMillion: z.string(), cachedOutputCreditsPerMillion: z.string(),
    })).default([]),
  }),
  runtime: runtimeSettingsSchema,
  toolCatalog: z.array(z.record(z.string(), z.unknown())).default([]),
})

export class ProviderConfigClient {
  private currentConfig?: RemoteProviderConfig
  private lastNormalizationFingerprint: string | undefined
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  current(): RemoteProviderConfig | undefined {
    return this.currentConfig
  }
  async get(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    return withSpan("luna_api.provider_config.get", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
    }), async span => {
    let response: Response
    try {
      response = await this.fetchConfig(signal)
    }
    catch (error) {
      if (signal?.aborted) throw error
      telemetryLog("agent.provider_config.failed", "warn", { "error.code": "ai.provider_config_unavailable" })
      throw new Error("ai.provider_config_unavailable")
    }
    span.setAttribute("http.response.status_code", response.status)
    agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: response.ok ? "success" : String(response.status) })
    if (!response.ok) {
      telemetryLog("agent.provider_config.failed", "warn", { "http.response.status_code": response.status })
      throw new Error("ai.provider_config_unavailable")
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
    const config = parsed.data
    const normalizedFields = normalizedRuntimeFields(payload, config.runtime)
    if (normalizedFields.length > 0) {
      const fingerprint = `${config.version}\u0000${normalizedFields.join("\u0000")}`
      if (fingerprint !== this.lastNormalizationFingerprint) {
        telemetryLog("agent.provider_config.normalized", "warn", {
          "luna.provider_config.normalized_fields": normalizedFields,
          "luna.provider_config.normalized_field_count": normalizedFields.length,
        })
      }
      this.lastNormalizationFingerprint = fingerprint
    }
    else {
      this.lastNormalizationFingerprint = undefined
    }
    this.currentConfig = config
    span.setAttribute("luna.provider.config_version", config.version)
    return config
    })
  }

  private async fetchConfig(signal?: AbortSignal): Promise<Response> {
    const maxRetries = this.currentConfig?.runtime.maxRequestRetries ?? defaultRuntimeSettings.maxRequestRetries
    for (let retry = 0; ; retry += 1) {
      try {
        const response = await fetch(new URL("/internal/v1/ai/provider-config", this.baseUrl), {
          headers: { authorization: `Bearer ${this.serviceToken}`, accept: "application/json" },
          ...(signal ? { signal } : {}),
        })
        if (!isRetryableHTTPStatus(response.status) || retry >= maxRetries)
          return response
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: String(response.status) })
        await this.scheduleRetry(retry + 1, maxRetries, signal, parseRetryAfter(response.headers), String(response.status))
      }
      catch (error) {
        if (signal?.aborted || retry >= maxRetries) throw error
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: "network_error" })
        await this.scheduleRetry(retry + 1, maxRetries, signal, undefined, "network_error")
      }
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

function stableConfigIssuePath(path: PropertyKey[]): string {
  if (path.length === 0) return "$"
  return path.map(segment => typeof segment === "number" ? "[]" : String(segment)).join(".")
}

function logInvalidProviderConfig(paths: string[], issueCodes: string[]): void {
  telemetryLog("agent.provider_config.failed", "warn", {
    "error.code": "ai.provider_config_invalid",
    "luna.provider_config.invalid_fields": [...new Set(paths)].slice(0, 20),
    "luna.provider_config.issue_codes": [...new Set(issueCodes)].slice(0, 20),
  })
}

function normalizedRuntimeFields(payload: unknown, runtime: RuntimeSettings): string[] {
  if (!isRecord(payload) || !isRecord(payload.runtime)) return []
  const rawRuntime = payload.runtime
  return Object.entries(runtime)
    .filter(([key, normalized]) => rawRuntime[key] !== normalized)
    .map(([key]) => `runtime.${key}`)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}
