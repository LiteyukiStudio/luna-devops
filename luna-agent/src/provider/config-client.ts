import { z } from "zod"
import { trace } from "@opentelemetry/api"
import type { RemoteRuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, clientSpanOptions, errorDiagnostic, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus, parseRetryAfter, waitForRetry } from "../retry.js"
import { defaultRuntimeSettings } from "../runtime-settings.js"
import type { AIModelSnapshot } from "../domain.js"

export type RemoteAIModel = AIModelSnapshot

export type RemoteProviderConfig = {
  version: string
  provider: {
    baseUrl: string
    apiKey: string
    channelAffinityEnabled: boolean
    configured: boolean
    models: RemoteAIModel[]
  }
  runtime: RemoteRuntimeSettings
  toolCatalog: unknown[]
}

const runtimeSettingsSchema = z.object({
  providerTimeoutMs: z.number().int().min(1_000).max(900_000),
  maxRequestRetries: z.number().int().min(0).max(10),
  runTimeoutMs: z.number().int().min(30_000).max(7_200_000),
  agentConcurrentRuns: z.number().int().min(1).max(100),
  userConcurrentRuns: z.number().int().min(1).max(100),
  assistantMaxOutputTokens: z.number().int().min(256).max(128 * 1024),
  maxModelSteps: z.number().int().min(1).max(1024),
  runMaxToolCalls: z.number().int().min(32).max(2048),
  maxInputBytes: z.number().int().min(8 * 1024).max(8 * 1024 * 1024),
  navigateActionTtlSeconds: z.number().int().min(10).max(600),
  maxCardRepairAttempts: z.number().int().min(1).max(10),
  contextMaxUncompressedTurnCount: z.number().int().min(4).max(128),
  contextMaxCompressionTurnsPerCompile: z.number().int().min(8).max(1024),
  contextSummaryMaxOutputTokens: z.number().int().min(200).max(32 * 1024),
})

const remoteProviderConfigSchema = z.object({
  version: z.string().min(1),
  provider: z.object({
    baseUrl: z.string(),
    apiKey: z.string(),
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

export class ProviderConfigClient {
  private currentConfig?: RemoteProviderConfig
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  current(): RemoteProviderConfig | undefined {
    return this.currentConfig
  }
  async get(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    const config = await this.getCandidate(signal)
    this.commit(config)
    return config
  }

  commit(config: RemoteProviderConfig): void {
    this.currentConfig = config
  }

  async getCandidate(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    return withSpan("luna_api.provider_config.get", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
    }), async span => {
    let response: Response
    try {
      response = await this.fetchConfig(signal)
    }
    catch (error) {
      if (signal?.aborted) throw error
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
    const config = parsed.data
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
    "operation": "agent.provider_config.validate",
    "outcome": "rejected",
    "error.code": "ai.provider_config_invalid",
    "error.type": "AgentProviderConfigError",
    "error.message": "ai.provider_config_invalid",
    "luna.provider_config.invalid_fields": [...new Set(paths)].slice(0, 20),
    "luna.provider_config.issue_codes": [...new Set(issueCodes)].slice(0, 20),
  })
}
