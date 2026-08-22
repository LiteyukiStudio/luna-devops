import { createId } from "../id.js"
import type { ModelBudgetUsage, Repository } from "../persistence/repository.js"
import { errorDiagnostic, internalSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

const reservationLeaseSeconds = 10_800

/**
 * 所有归属 Run 的模型调用必须经过此包装器。它只把数据库批准后的
 * maxOutputTokens 交给真实 Provider，并在 usage 缺失或调用结果未知时
 * 保守确认完整钱包预留，防止用量缺失、重试或进程重启造成少计费。
 */
export class BudgetedModelProvider implements ModelProvider {
  constructor(private readonly inner: ModelProvider, private readonly repository: Repository) {}

  capabilities(): ModelCapabilities { return this.inner.capabilities() }
  health(): Promise<{ ok: boolean, requestId?: string }> { return this.inner.health() }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    if (!request.budget) return this.inner.complete(request)
    return withSpan("agent.model.budgeted", internalSpanOptions({
      "luna.budget.operation": request.budget.operation,
    }), async span => {
      const reservation = await this.reserve(request)
      span.setAttribute("luna.budget.clamped", reservation.maxOutputTokens < request.maxOutputTokens)
      if (request.signal?.aborted) {
        await this.repository.releaseModelBudget(reservation.id)
        throw request.signal.reason ?? new Error("ai.run_canceled")
      }
      try {
        const response = await this.inner.complete({ ...request, maxOutputTokens: reservation.maxOutputTokens })
        await this.confirmReportedUsage(reservation.id, {
          ...response.usage,
          reported: response.usage.reported !== false,
        })
        return { ...response, reservationId: reservation.id }
      }
      catch (error) {
        if (isPreDispatchError(error)) await this.repository.releaseModelBudget(reservation.id)
        else await this.repository.confirmModelBudget(reservation.id)
        throw error
      }
    })
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    if (!request.budget) {
      yield* this.inner.stream(request)
      return
    }
    const reservation = await this.reserve(request)
    if (request.signal?.aborted) {
      await this.repository.releaseModelBudget(reservation.id)
      throw request.signal.reason ?? new Error("ai.run_canceled")
    }
    let confirmed = false
    try {
      for await (const event of this.inner.stream({ ...request, maxOutputTokens: reservation.maxOutputTokens })) {
        if (event.type === "completed") {
          await this.confirmReportedUsage(reservation.id, {
            ...event.usage,
            reported: event.usage.reported !== false,
          })
          confirmed = true
          yield { ...event, reservationId: reservation.id }
        }
        else yield event
      }
      if (!confirmed) await this.repository.confirmModelBudget(reservation.id)
    }
    catch (error) {
      if (!confirmed) {
        if (isPreDispatchError(error)) await this.repository.releaseModelBudget(reservation.id)
        else await this.repository.confirmModelBudget(reservation.id)
      }
      throw error
    }
  }

  private async reserve(request: ModelRequest) {
    const budget = request.budget!
    const estimatedInputTokens = estimateRequestInputTokens(request)
    const reservation = await this.repository.reserveModelBudget({
      id: createId("aibgt"),
      runId: budget.runId,
      ownerUserId: budget.ownerUserId,
      operation: budget.operation,
      estimatedInputTokens,
      requestedOutputTokens: request.maxOutputTokens,
      leaseSeconds: reservationLeaseSeconds,
    })
    telemetryLog("agent.budget.allowed", "info", {
      "luna.budget.operation": budget.operation,
      "luna.budget.clamped": reservation.maxOutputTokens < request.maxOutputTokens,
      "luna.budget.remaining_bucket": remainingBucket(reservation.maxOutputTokens),
    })
    return reservation
  }

  private async confirmReportedUsage(reservationId: string, usage: ModelBudgetUsage): Promise<void> {
    try {
      await this.repository.confirmModelBudget(reservationId, usage)
    }
    catch (error) {
      if (!(error instanceof Error) || error.message !== "ai.provider_usage_invalid") throw error
      // Provider output remains usable, but malformed usage must never lower the
      // authoritative hold. Confirm the full reservation and expose only a stable
      // low-cardinality telemetry code.
      await this.repository.confirmModelBudget(reservationId)
      telemetryLog("agent.budget.usage_invalid", "warn", {
        "operation": "agent.budget.confirm_usage",
        "outcome": "rejected",
        ...errorDiagnostic(error, "ai.provider_usage_invalid"),
      })
    }
  }
}

const preDispatchErrorCodes = new Set([
  "ai.not_configured",
  "ai.model_not_available",
  "ai.provider_config_unavailable",
])

function isPreDispatchError(error: unknown): boolean {
  return error instanceof Error && preDispatchErrorCodes.has(error.message)
}

export function estimateRequestInputTokens(request: Pick<ModelRequest, "messages" | "tools">): number {
  const messageBytes = Buffer.byteLength(JSON.stringify(request.messages), "utf8")
  const toolBytes = Buffer.byteLength(JSON.stringify(request.tools ?? []), "utf8")
  // One token per UTF-8 byte is a tokenizer-independent safety upper bound for
  // ASCII identifiers, code and JSON. ContextCompiler may use a looser packing
  // estimate, but this final gate must never under-estimate the provider input.
  return Math.max(1, messageBytes + toolBytes + request.messages.length * 8 + (request.tools?.length ?? 0) * 12)
}

function remainingBucket(tokens: number): "lt_1k" | "lt_8k" | "lt_64k" | "gte_64k" {
  if (tokens < 1_000) return "lt_1k"
  if (tokens < 8_000) return "lt_8k"
  if (tokens < 64_000) return "lt_64k"
  return "gte_64k"
}
