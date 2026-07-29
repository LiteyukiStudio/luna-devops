export type RemoteProviderConfig = {
  version: string
  provider: {
    type: "openai-compatible"
    baseUrl: string
    defaultModel: string
    fallbackModel?: string
    modelPricing: unknown[]
    apiKey: string
    configured: boolean
  }
}

export class ProviderConfigClient {
  constructor(private readonly baseUrl: string, private readonly serviceToken: string) {}
  async get(signal?: AbortSignal): Promise<RemoteProviderConfig> {
    const response = await fetch(new URL("/internal/v1/ai/provider-config", this.baseUrl), {
      headers: { authorization: `Bearer ${this.serviceToken}`, accept: "application/json" },
      ...(signal ? { signal } : {}),
    })
    if (!response.ok) throw new Error("ai.provider_config_unavailable")
    return response.json() as Promise<RemoteProviderConfig>
  }
}
