import type { RunToolVerification } from "../domain.js"
import type { ToolOrchestrator, ToolCallRecord } from "../tools/orchestrator.js"
import type { ToolOutcome } from "../tools/outcome.js"

export type VerificationExecution =
  | { kind: "verified"; toolCallId: string; result: unknown }
  | { kind: "pending"; toolCallId: string; result: unknown }
  | { kind: "failed"; toolCallId: string; errorCode: string; result?: unknown }

export class ToolVerificationRunner {
  constructor(
    private readonly tools: ToolOrchestrator,
    private readonly initialDelayMs = 1_500,
    private readonly maximumDelayMs = 15_000,
  ) {}

  async verificationForToolCall(toolCallId: string): Promise<RunToolVerification | undefined> {
    const outcome: ToolOutcome = await this.tools.outcome(toolCallId)
    return outcome.kind === "running"
      ? outcome.verification
      : outcome.kind === "succeeded" && outcome.verificationRequired
        ? outcome.verification
        : undefined
  }

  async verify(plan: RunToolVerification, signal: AbortSignal): Promise<VerificationExecution> {
    signal.throwIfAborted()
    const items: unknown[] = []
    let page = 1
    while (true) {
      signal.throwIfAborted()
      const call = await this.tools.propose({
        runId: plan.runId,
        operationId: plan.operationId,
        arguments: { ...plan.arguments, ...("page" in plan.arguments ? { page } : {}) },
        inputMode: "verification",
        verificationForToolCallId: plan.sourceToolCallId,
      }, signal)
      const result = recordValue(call.result)
      if (call.status !== "succeeded" || !Array.isArray(result.items) || !("page" in plan.arguments)
        || result.page === undefined && result.totalPages === undefined)
        return evaluateVerification(plan, call)
      if (!Number.isInteger(result.page) || result.page !== page
        || !Number.isInteger(result.totalPages) || Number(result.totalPages) < 0
        || page > Math.max(1, Number(result.totalPages)))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
      items.push(...result.items as unknown[])
      if (page >= Number(result.totalPages))
        return evaluateVerification(plan, { ...call, result: { ...result, items, page: 1, totalPages: 1 } })
      page += 1
    }
  }

  async wait(attempt: number, signal: AbortSignal): Promise<void> {
    const delayMs = Math.min(this.maximumDelayMs, this.initialDelayMs * 2 ** Math.min(Math.max(0, attempt - 1), 8))
    await abortableDelay(delayMs, signal)
  }
}

export function evaluateVerification(plan: RunToolVerification, call: ToolCallRecord): VerificationExecution {
  if (call.status === "canceled")
    return { kind: "failed", toolCallId: call.id, errorCode: "ai.run_canceled", result: call.result }
  if (plan.expectation === "absent" && call.status === "failed" && isResourceNotFound(call))
    return { kind: "verified", toolCallId: call.id, result: call.result }
  if (call.status !== "succeeded")
    return {
      kind: "failed",
      toolCallId: call.id,
      errorCode: call.errorCode ?? "ai.tool_execution_failed",
      ...(call.result === undefined ? {} : { result: call.result }),
    }
  const result = recordValue(call.result)
  if ((plan.family === "deployment_target" || plan.family === "project_order" || plan.resource?.collection)
    && (Number(result.page) > 1 || Number(result.totalPages) > 1))
    return { kind: "pending", toolCallId: call.id, result: call.result }
  switch (plan.family) {
    case "build_run": {
      const status = normalizedStatus(result.status)
      if (plan.expectation === "absent")
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (plan.expectation === "canceled") {
        if (["canceled", "cancelled"].includes(status))
          return { kind: "verified", toolCallId: call.id, result: call.result }
        if (["queued", "pending", "running", "progressing"].includes(status))
          return { kind: "pending", toolCallId: call.id, result: call.result }
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      }
      if (status === "succeeded") return { kind: "verified", toolCallId: call.id, result: call.result }
      if (["queued", "pending", "running", "progressing"].includes(status))
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (["failed", "lost", "timeout", "canceled", "cancelled"].includes(status))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      break
    }
    case "release": {
      const status = normalizedStatus(result.status)
      if (status === "succeeded") return { kind: "verified", toolCallId: call.id, result: call.result }
      if (["pending", "queued", "running", "progressing", "deploying"].includes(status))
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (["failed", "canceled", "cancelled"].includes(status))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      break
    }
    case "gateway_route": {
      if (plan.expectation === "absent")
        return { kind: "pending", toolCallId: call.id, result: call.result }
      const status = normalizedStatus(result.status)
      const tlsMode = normalizedStatus(result.tlsMode)
      const certificateStatus = normalizedStatus(result.certificateStatus)
      if (["degraded", "failed", "rejected"].includes(status))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      if (["failed", "invalid", "denied", "expired", "error"].includes(certificateStatus))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      if (result.enabled === false && status === "disabled")
        return { kind: "verified", toolCallId: call.id, result: call.result }
      if (!["ready", "active"].includes(status))
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (tlsMode === "manual-cert" && certificateStatus === "manual")
        return { kind: "verified", toolCallId: call.id, result: call.result }
      if (tlsMode === "http-only" && certificateStatus === "disabled")
        return { kind: "verified", toolCallId: call.id, result: call.result }
      if (tlsMode === "http-challenge" && certificateStatus === "issued")
        return { kind: "verified", toolCallId: call.id, result: call.result }
      return { kind: "pending", toolCallId: call.id, result: call.result }
    }
    case "application": {
      const applicationId = stringValue(result.id) ?? stringValue(result.applicationId)
      if (applicationId !== plan.externalOperationId)
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (plan.expectation === "absent") {
        if (normalizedStatus(result.deleteStatus) === "delete_failed")
          return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
        return { kind: "pending", toolCallId: call.id, result: call.result }
      }
      return { kind: "verified", toolCallId: call.id, result: call.result }
    }
    case "deployment_target": {
      const items = Array.isArray(result.items)
        ? result.items.map(recordValue).filter(item => Object.keys(item).length > 0)
        : [result]
      const target = items.find(item => stringValue(item.id) === plan.externalOperationId)
      if (!target) return plan.expectation === "absent"
        ? { kind: "verified", toolCallId: call.id, result: call.result }
        : { kind: "pending", toolCallId: call.id, result: call.result }
      const deleteStatus = normalizedStatus(target.deleteStatus)
      if (plan.expectation === "absent") {
        if (["delete_failed", "failed"].includes(deleteStatus))
          return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
        return { kind: "pending", toolCallId: call.id, result: call.result }
      }
      if (["deleting", "delete_failed", "failed"].includes(deleteStatus))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      return { kind: "verified", toolCallId: call.id, result: call.result }
    }
    case "runtime_secrets":
    case "runtime_config_secrets": {
      const variables = Array.isArray(result.environmentVariables)
        ? result.environmentVariables.map(recordValue)
        : []
      const configuredSecretKeys = new Set(variables
        .filter(item => normalizedStatus(item.valueMode) === "secret" && item.configured === true)
        .map(item => stringValue(item.key))
        .filter((key): key is string => key !== undefined))
      const requiredPresent = (plan.requiredKeys ?? []).every(key => configuredSecretKeys.has(key))
      const clearedAbsent = (plan.absentKeys ?? []).every(key => !configuredSecretKeys.has(key))
      return requiredPresent && clearedAbsent
        ? { kind: "verified", toolCallId: call.id, result: call.result }
        : { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
    }
    case "project_order": {
      const expected = Array.isArray(plan.expected)
        ? plan.expected.filter((value): value is string => typeof value === "string")
        : []
      const items = Array.isArray(result.items)
        ? result.items.map(recordValue)
        : []
      const byId = new Map(items.map(item => [stringValue(item.id), item]))
      const verified = expected.length > 0 && expected.every((projectId, index) =>
        byId.get(projectId)?.dashboardOrder === index + 1)
      return verified
        ? { kind: "verified", toolCallId: call.id, result: call.result }
        : { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
    }
    case "billing_rate_rules": {
      const expected = Array.isArray(plan.expected)
        ? plan.expected.map(recordValue).filter(item => Object.keys(item).length > 0)
        : []
      const rules = (Array.isArray(call.result) ? call.result : []).map(recordValue)
      const byMeter = new Map(rules.map(rule => [stringValue(rule.meter), rule]))
      const verified = expected.length > 0 && expected.every((expectedRule) => {
        const meter = stringValue(expectedRule.meter)
        const actual = meter ? byMeter.get(meter) : undefined
        return actual !== undefined
          && normalizedDecimal(actual.creditsPerUnit) === normalizedDecimal(expectedRule.creditsPerUnit)
          && actual.enabled === expectedRule.enabled
      })
      return verified
        ? { kind: "verified", toolCallId: call.id, result: call.result }
        : { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
    }
    case "resource": {
      const resource = plan.resource
      if (!resource)
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
      const values = resource.collection
        ? Array.isArray(result.items)
          ? result.items.map(recordValue)
          : Array.isArray(call.result)
            ? call.result.map(recordValue)
            : [result]
        : [result]
      const target = resource.singleton
        ? result
        : values.find(item => stringValue(item[resource.targetField]) === plan.externalOperationId)
      if (!target) {
        return plan.expectation === "absent"
          ? { kind: "verified", toolCallId: call.id, result: call.result }
          : { kind: "pending", toolCallId: call.id, result: call.result }
      }
      if (!resource.statusField)
        return plan.expectation === "absent"
          ? { kind: "pending", toolCallId: call.id, result: call.result }
          : { kind: "verified", toolCallId: call.id, result: call.result }
      const status = normalizedStatus(target[resource.statusField])
      if ((resource.failureStatuses ?? []).some(value => normalizedStatus(value) === status))
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.tool_execution_failed", result: call.result }
      if (plan.expectation === "absent")
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (plan.expectation === "canceled") {
        if ((resource.successStatuses ?? []).some(value => normalizedStatus(value) === status))
          return { kind: "verified", toolCallId: call.id, result: call.result }
        if ((resource.pendingStatuses ?? []).some(value => normalizedStatus(value) === status))
          return { kind: "pending", toolCallId: call.id, result: call.result }
        return { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
      }
      if ((resource.pendingStatuses ?? []).some(value => normalizedStatus(value) === status))
        return { kind: "pending", toolCallId: call.id, result: call.result }
      if (!resource.successStatuses?.length || resource.successStatuses.some(value => normalizedStatus(value) === status))
        return { kind: "verified", toolCallId: call.id, result: call.result }
      return { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
    }
  }
  return { kind: "failed", toolCallId: call.id, errorCode: "ai.verification_inconclusive", result: call.result }
}

function normalizedStatus(value: unknown): string {
  return typeof value === "string" ? value.trim().toLowerCase() : ""
}

function normalizedDecimal(value: unknown): string | undefined {
  const text = stringValue(value)
  if (!text || !/^\d+(?:\.\d+)?$/.test(text)) return text
  const [integerPart, fractionalPart = ""] = text.split(".")
  const integer = integerPart!.replace(/^0+(?=\d)/, "")
  const fractional = fractionalPart.replace(/0+$/, "")
  return fractional ? `${integer}.${fractional}` : integer
}

function isResourceNotFound(call: ToolCallRecord): boolean {
  if (call.errorCode === "resource.not_found") return true
  const result = recordValue(call.result)
  return stringValue(result.code) === "resource.not_found"
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function abortableDelay(delayMs: number, signal: AbortSignal): Promise<void> {
  if (delayMs <= 0) return Promise.resolve()
  return new Promise((resolve, reject) => {
    const finish = () => {
      signal.removeEventListener("abort", abort)
      resolve()
    }
    const timer = setTimeout(finish, delayMs)
    const abort = () => {
      clearTimeout(timer)
      signal.removeEventListener("abort", abort)
      reject(signal.reason instanceof Error ? signal.reason : new Error("ai.run_canceled"))
    }
    signal.addEventListener("abort", abort, { once: true })
    timer.unref?.()
  })
}
