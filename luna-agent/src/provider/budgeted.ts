import { createId } from "../id.js"
import type { ModelAttemptMetadata, Repository } from "../persistence/repository.js"
import { internalSpanOptions, telemetryLog, withSpan } from "../telemetry.js"
import { ProviderRequestError } from "./provider-error.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse, ModelUsage } from "./provider.js"

const creditHoldLeaseSeconds = 10_800

/** 每次 Provider attempt 独立创建信用风险 hold；hold 从不包含或推导实际 Token。 */
export class BudgetedModelProvider implements ModelProvider {
  constructor(private readonly inner: ModelProvider, private readonly repository: Repository) {}

  capabilities(): ModelCapabilities { return this.inner.capabilities() }
  health(): Promise<{ ok: boolean, requestId?: string }> { return this.inner.health() }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    if (!request.budget) return this.inner.complete(request)
    return withSpan("agent.model.credit_hold", internalSpanOptions({ "luna.credit_hold.operation": request.budget.operation }), async span => {
      const hold = await this.hold(request)
      span.setAttribute("luna.credit_hold.attempt", hold.attempt)
      if (request.signal?.aborted) {
        await this.repository.releaseModelCreditHold(hold.id)
        throw request.signal.reason ?? new Error("ai.run_canceled")
      }
      try {
        const response = await this.inner.complete({ ...request, maxOutputTokens: hold.maxOutputTokens })
        const reconciliationRequired = await this.finalizeSuccess(hold.id, response.usage, {
          callType: "complete",
          ...(response.providerRequestId ? { providerRequestId: response.providerRequestId } : {}),
          ...(response.responseId ? { responseId: response.responseId } : {}),
          ...(response.responseModel ? { responseModel: response.responseModel } : {}),
          ...(response.finishReason ? { finishReason: response.finishReason } : {}),
        })
        return { ...response, creditHoldId: hold.id, ...(reconciliationRequired ? { reconciliationRequired: true } : {}) }
      }
      catch (error) {
        await this.finalizeFailure(hold.id, error)
        throw error
      }
    })
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    if (!request.budget) { yield* this.inner.stream(request); return }
    const hold = await this.hold(request)
    if (request.signal?.aborted) {
      await this.repository.releaseModelCreditHold(hold.id)
      throw request.signal.reason ?? new Error("ai.run_canceled")
    }
    let finalized = false
    try {
      for await (const event of this.inner.stream({ ...request, maxOutputTokens: hold.maxOutputTokens })) {
        if (event.type !== "completed") { yield event; continue }
        const reconciliationRequired = await this.finalizeSuccess(hold.id, event.usage, {
          callType: "stream",
          ...(event.providerRequestId ? { providerRequestId: event.providerRequestId } : {}),
          ...(event.responseId ? { responseId: event.responseId } : {}),
          ...(event.responseModel ? { responseModel: event.responseModel } : {}),
          ...(event.finishReason ? { finishReason: event.finishReason } : {}),
        })
        finalized = true
        yield { ...event, creditHoldId: hold.id, ...(reconciliationRequired ? { reconciliationRequired: true } : {}) }
      }
      if (!finalized) await this.repository.markModelUsageUnavailable(hold.id, "request_outcome_unknown", { failureStage: "stream" })
    }
    catch (error) {
      if (!finalized) await this.finalizeFailure(hold.id, error)
      throw error
    }
  }

  private async hold(request: ModelRequest) {
    const budget = request.budget!
    const hold = await this.repository.createModelCreditHold({
      id: createId("aihold"), runId: budget.runId, ownerUserId: budget.ownerUserId,
      operation: budget.operation, requestedOutputTokens: request.maxOutputTokens,
      leaseSeconds: creditHoldLeaseSeconds,
    })
    telemetryLog("agent.credit_hold.created", "info", {
      "luna.credit_hold.operation": budget.operation,
      "luna.credit_hold.attempt": hold.attempt,
    })
    return hold
  }

  private async finalizeSuccess(
    holdId: string,
    usage: ModelUsage,
    metadata: ModelAttemptMetadata & { callType: "stream" | "complete" },
  ): Promise<boolean> {
    if (usage.status === "reported") {
      return (await this.repository.recordReportedModelUsage(holdId, usage.value, metadata)).reconciliationRequired
    }
    await this.repository.markModelUsageUnavailable(holdId, usage.reason, metadata)
    return true
  }

  private async finalizeFailure(holdId: string, error: unknown): Promise<void> {
    if (error instanceof ProviderRequestError
      && (error.options.requestOutcome === "not_dispatched" || error.options.requestOutcome === "rejected")) {
      await this.repository.releaseModelCreditHold(holdId)
      return
    }
    const provider = error instanceof ProviderRequestError ? error.options : undefined
    await this.repository.markModelUsageUnavailable(holdId, "request_outcome_unknown", {
      ...(provider?.providerRequestId ? { providerRequestId: provider.providerRequestId } : {}),
      ...(provider?.responseId ? { responseId: provider.responseId } : {}),
      ...(provider?.responseModel ? { responseModel: provider.responseModel } : {}),
      ...(provider?.stage ? { failureStage: provider.stage } : {}),
    })
  }
}
