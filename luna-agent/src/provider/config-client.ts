import { z } from "zod"
import { trace } from "@opentelemetry/api"
import type { RuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, clientSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"
import { defaultRuntimeSettings } from "../runtime-settings.js"

export type RemoteProviderConfig = {
  version: string
  provider: {
    baseUrl: string
    model: string
    apiKey: string
    configured: boolean
  }
  runtime: RuntimeSettings
  toolCatalog?: unknown[]
}

const runtimeSettingsSchema = z.object({
  providerTimeoutMs: z.number().int().min(1_000).max(120_000),
  maxRequestRetries: z.number().int().min(0).max(10).default(5),
  runTimeoutMs: z.number().int().min(30_000).max(900_000),
  agentConcurrentRuns: z.number().int().min(1).max(100),
  userConcurrentRuns: z.number().int().min(1).max(100),
  contextInputTokenBudget: z.number().int().min(64 * 1024).max(1024 * 1024),
  assistantMaxOutputTokens: z.number().int().min(256).max(16_384).default(8192),
  maxModelSteps: z.number().int().min(1).max(200).default(64),
  maxInputBytes: z.number().int().min(8 * 1024).max(1024 * 1024).default(64_000),
  navigateActionTtlSeconds: z.number().int().min(10).max(600).default(120),
  toolResultPayloadBudget: z.number().int().min(4 * 1024).max(512 * 1024).default(48_000),
  maxCardRepairAttempts: z.number().int().min(1).max(10).default(5),
  contextCompressionTriggerRatio: z.number().min(0.5).max(0.95).default(0.8),
  contextCompressionTargetRatio: z.number().min(0.1).max(0.8).default(0.5),
  contextRecentTurnCount: z.number().int().min(1).max(16).default(6),
  contextMaxRecentTurnCount: z.number().int().min(2).max(32).default(12),
  contextMaxUncompressedTurnCount: z.number().int().min(4).max(200).default(32),
  contextMaxCompressionTurnsPerCompile: z.number().int().min(8).max(500).default(128),
  contextSummaryInputTokenBudget: z.number().int().min(4 * 1024).max(128 * 1024).default(32_000),
  contextSummaryMaxOutputTokens: z.number().int().min(200).max(8_000).default(3_000),
  contextHistoricalToolTokenBudget: z.number().int().min(1024).max(64 * 1024).default(8_000),
}).superRefine((value, context) => {
  if (value.contextCompressionTriggerRatio <= value.contextCompressionTargetRatio) {
    context.addIssue({
      code: "custom",
      path: ["contextCompressionTriggerRatio"],
      message: "ai.runtime.inconsistent_compression_ratios",
    })
  }
  if (value.contextRecentTurnCount > value.contextMaxRecentTurnCount) {
    context.addIssue({
      code: "custom",
      path: ["contextRecentTurnCount"],
      message: "ai.runtime.recent_turn_exceeds_max",
    })
  }
})

const remoteProviderConfigSchema = z.object({
  version: z.string().min(1),
  provider: z.object({
    baseUrl: z.string(),
    model: z.string(),
    apiKey: z.string(),
    configured: z.boolean(),
  }),
  runtime: runtimeSettingsSchema,
  toolCatalog: z.array(z.record(z.string(), z.unknown())).default([]),
})

export class ProviderConfigClient {
  private currentConfig?: RemoteProviderConfig
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  current(): RemoteProviderConfig | undefined {
    return this.currentConfig
  }
  async get(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    return withSpan("luna_api.provider_config.get", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
    }), async span => {
    const response = await this.fetchConfig(signal)
    span.setAttribute("http.response.status_code", response.status)
    agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: response.ok ? "success" : String(response.status) })
    if (!response.ok) {
      telemetryLog("agent.provider_config.failed", "warn", { "http.response.status_code": response.status })
      throw new Error("ai.provider_config_unavailable")
    }
    const config = remoteProviderConfigSchema.parse(await response.json())
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
