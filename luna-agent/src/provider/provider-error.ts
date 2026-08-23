import { z } from "zod"

const providerErrorBodySchema = z.object({
  error: z.object({
    code: z.union([z.string(), z.number()]).optional(),
    type: z.string().optional(),
    param: z.string().nullable().optional(),
    message: z.string().optional(),
  }).passthrough(),
}).passthrough()

export type ProviderFailureStage = "dispatch" | "response_headers" | "response_body" | "stream"

export class ProviderRequestError extends Error {
  override readonly name = "ProviderRequestError"

  constructor(
    message: string,
    readonly options: {
      status?: number
      providerCode?: string
      providerType?: string
      providerParam?: string
      providerRequestId?: string
      responseId?: string
      responseModel?: string
      stage: ProviderFailureStage
      requestOutcome: "not_dispatched" | "rejected" | "unknown"
    },
  ) {
    super(message)
  }
}

const contextCodes = new Set(["context_length_exceeded", "model_context_window_exceeded", "max_context_length_exceeded"])
const authCodes = new Set(["invalid_api_key", "authentication_error", "unauthorized"])
const quotaCodes = new Set(["insufficient_quota", "billing_hard_limit_reached", "quota_exceeded"])
const rateLimitCodes = new Set(["rate_limit_exceeded", "requests_rate_limit_exceeded", "tokens_rate_limit_exceeded"])
const timeoutCodes = new Set(["request_timeout", "timeout"])
const unavailableCodes = new Set(["server_error", "service_unavailable", "overloaded"])

export function parseProviderErrorBody(value: unknown): { code?: string, type?: string, param?: string } {
  const parsed = providerErrorBodySchema.safeParse(value)
  if (!parsed.success) return {}
  const error = parsed.data.error
  return {
    ...(error.code !== undefined ? { code: String(error.code) } : {}),
    ...(error.type ? { type: error.type } : {}),
    ...(error.param ? { param: error.param } : {}),
  }
}

export function mapProviderError(status: number, detail: { code?: string, type?: string, param?: string }): string {
  const code = detail.code?.toLowerCase()
  const type = detail.type?.toLowerCase()
  if (code && contextCodes.has(code)) return "ai.provider_context_length_exceeded"
  if (code && authCodes.has(code)) return "ai.provider_auth_failed"
  if (code && quotaCodes.has(code)) return "ai.provider_quota_exhausted"
  if (code && rateLimitCodes.has(code)) return "ai.provider_rate_limited"
  if (code && timeoutCodes.has(code)) return "ai.provider_timeout"
  if (code && unavailableCodes.has(code)) return "ai.provider_unavailable"
  if (status === 401 || status === 403 || type === "authentication_error") return "ai.provider_auth_failed"
  if (status === 402) return "ai.provider_quota_exhausted"
  if (status === 408 || status === 504) return "ai.provider_timeout"
  if (status === 429 || type === "rate_limit_error") return "ai.provider_rate_limited"
  if (status >= 500) return "ai.provider_unavailable"
  return "ai.provider_request_failed"
}

export function isProviderContextLengthError(error: unknown): boolean {
  return error instanceof ProviderRequestError && error.message === "ai.provider_context_length_exceeded"
}
