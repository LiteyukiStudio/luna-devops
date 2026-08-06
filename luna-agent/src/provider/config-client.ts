import { z } from "zod"
import { agentMetrics, clientSpanOptions, telemetryLog, withSpan } from "../telemetry.js"

export type RemoteProviderConfig = {
  version: string
  provider: {
    baseUrl: string
    model: string
    apiKey: string
    configured: boolean
  }
  runtime: {
    providerTimeoutMs: number
    runTimeoutMs: number
    agentConcurrentRuns: number
    userConcurrentRuns: number
    contextInputTokenBudget: number
  }
  toolCatalog?: unknown[]
}

const remoteProviderConfigSchema = z.object({
  version: z.string().min(1),
  provider: z.object({
    baseUrl: z.string(),
    model: z.string(),
    apiKey: z.string(),
    configured: z.boolean(),
  }),
  runtime: z.object({
    providerTimeoutMs: z.number().int().min(1_000).max(120_000),
    runTimeoutMs: z.number().int().min(30_000).max(900_000),
    agentConcurrentRuns: z.number().int().min(1).max(100),
    userConcurrentRuns: z.number().int().min(1).max(100),
    contextInputTokenBudget: z.number().int().min(64 * 1024).max(1024 * 1024),
  }),
  toolCatalog: z.array(z.record(z.string(), z.unknown())).default([]),
})

export class ProviderConfigClient {
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  async get(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    return withSpan("luna_api.provider_config.get", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
    }), async span => {
    const response = await fetch(new URL("/internal/v1/ai/provider-config", this.baseUrl), {
      headers: { authorization: `Bearer ${this.serviceToken}`, accept: "application/json" },
      ...(signal ? { signal } : {}),
    })
    span.setAttribute("http.response.status_code", response.status)
    agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "provider_config", outcome: response.ok ? "success" : String(response.status) })
    if (!response.ok) {
      telemetryLog("agent.provider_config.failed", "warn", { "http.response.status_code": response.status })
      throw new Error("ai.provider_config_unavailable")
    }
    const config = remoteProviderConfigSchema.parse(await response.json())
    span.setAttribute("luna.provider.config_version", config.version)
    return config
    })
  }
}
