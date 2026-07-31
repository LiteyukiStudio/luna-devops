import { z } from "zod"

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
    agentConcurrentRuns: z.number().int().min(1).max(10),
  }),
  toolCatalog: z.array(z.record(z.string(), z.unknown())).default([]),
})

export class ProviderConfigClient {
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  async get(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    const response = await fetch(new URL("/internal/v1/ai/provider-config", this.baseUrl), {
      headers: { authorization: `Bearer ${this.serviceToken}`, accept: "application/json" },
      ...(signal ? { signal } : {}),
    })
    if (!response.ok) throw new Error("ai.provider_config_unavailable")
    return remoteProviderConfigSchema.parse(await response.json())
  }
}
