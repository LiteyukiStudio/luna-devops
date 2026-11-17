import type { Repository } from "../persistence/repository.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "./provider.js"

/**
 * 纯事后权威用量落库包装器（无事前锁表或 CreditHold 预冻结）：
 * 1) 强制校验每次模型完成必须携带合法 reported Usage，缺失即抛 ai.provider_usage_unavailable；
 * 2) 当请求附带 budget 上下文时，将不可变 Usage 写入 ai.model_usages 供异步计费与上下文水位使用。
 */
export class UsageRecordingModelProvider implements ModelProvider {
  constructor(
    private readonly inner: ModelProvider,
    private readonly repository: Pick<Repository, "recordReportedModelUsage">,
  ) {}

  capabilities(): ModelCapabilities {
    return this.inner.capabilities()
  }

  health(): Promise<{ ok: boolean, requestId?: string }> {
    return this.inner.health()
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    const maxOutputTokens = boundedMaxOutputTokens(request)
    if (request.signal?.aborted) throw request.signal.reason ?? new Error("ai.run_canceled")
    const response = await this.inner.complete({ ...request, maxOutputTokens })
    if (response.usage.status !== "reported") throw new Error("ai.provider_usage_unavailable")
    if (!request.budget) return response
    const recorded = await this.repository.recordReportedModelUsage({
      runId: request.budget.runId,
      ownerUserId: request.budget.ownerUserId,
      operation: request.budget.operation,
      usage: response.usage.value,
      callType: "complete",
      ...(request.modelPricing ? { model: request.modelPricing } : {}),
      ...(response.providerRequestId ? { providerRequestId: response.providerRequestId } : {}),
      ...(response.responseId ? { responseId: response.responseId } : {}),
      ...(response.responseModel ? { responseModel: response.responseModel } : {}),
      ...(response.finishReason ? { finishReason: response.finishReason } : {}),
    })
    return { ...response, usageId: recorded.id }
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    const maxOutputTokens = boundedMaxOutputTokens(request)
    if (request.signal?.aborted) throw request.signal.reason ?? new Error("ai.run_canceled")
    let completed = false
    for await (const event of this.inner.stream({ ...request, maxOutputTokens })) {
      if (event.type !== "completed") {
        yield event
        continue
      }
      if (event.usage.status !== "reported") throw new Error("ai.provider_usage_unavailable")
      completed = true
      if (!request.budget) {
        yield event
        continue
      }
      const recorded = await this.repository.recordReportedModelUsage({
        runId: request.budget.runId,
        ownerUserId: request.budget.ownerUserId,
        operation: request.budget.operation,
        usage: event.usage.value,
        callType: "stream",
        ...(request.modelPricing ? { model: request.modelPricing } : {}),
        ...(event.providerRequestId ? { providerRequestId: event.providerRequestId } : {}),
        ...(event.responseId ? { responseId: event.responseId } : {}),
        ...(event.responseModel ? { responseModel: event.responseModel } : {}),
        ...(event.finishReason ? { finishReason: event.finishReason } : {}),
      })
      yield { ...event, usageId: recorded.id }
    }
    if (!completed) throw new Error("ai.provider_usage_unavailable")
  }
}

function boundedMaxOutputTokens(request: ModelRequest): number {
  if (!Number.isSafeInteger(request.maxOutputTokens) || request.maxOutputTokens < 1) {
    throw new Error("ai.model_output_limit_invalid")
  }
  if (!request.modelPricing) return request.maxOutputTokens
  if (!Number.isSafeInteger(request.modelPricing.maxOutputTokens) || request.modelPricing.maxOutputTokens < 1) {
    throw new Error("ai.model_output_limit_invalid")
  }
  return Math.min(request.maxOutputTokens, request.modelPricing.maxOutputTokens)
}
