import type { RunToolVerification } from "../domain.js"
import type { ToolOperation, ToolVerificationRule } from "./catalog.js"
import type { ToolCallRecord } from "./orchestrator.js"

export type ToolEffect = "discovery" | "read" | "write" | "async" | "execute" | "interactive" | "presentation"

export type ToolOutcome =
  | {
      kind: "succeeded"
      effect: ToolEffect
      result: unknown
      verificationRequired: boolean
      verification?: RunToolVerification
    }
  | { kind: "failed"; effect: ToolEffect; errorCode: string; retryable: boolean; result?: unknown }
  | { kind: "awaiting_approval"; effect: ToolEffect; toolCallId: string }
  | { kind: "awaiting_input"; effect: ToolEffect; fields: string[] }
  | {
      kind: "running"
      effect: "async"
      result: unknown
      externalOperationId?: string
      verification: RunToolVerification
    }
  | { kind: "canceled"; effect: ToolEffect; result?: unknown }

export function toolEffect(operation: ToolOperation): ToolEffect {
  if (operation.effect) return operation.effect
  return operation.method === "GET" ? "read" : "write"
}

export function toolOutcome(
  operation: ToolOperation,
  call: ToolCallRecord,
  resolveOperation?: (operationId: string) => ToolOperation | undefined,
): ToolOutcome {
  const effect = toolEffect(operation)
  if (call.status === "awaiting_approval")
    return { kind: "awaiting_approval", effect, toolCallId: call.id }
  if (call.status === "canceled")
    return { kind: "canceled", effect, ...(call.result === undefined ? {} : { result: call.result }) }
  if (call.status === "failed" || call.status === "rejected")
    return {
      kind: "failed",
      effect,
      errorCode: call.errorCode ?? "ai.tool_execution_failed",
      retryable: call.status === "failed" && isRetryable(call.result),
      ...(call.result === undefined ? {} : { result: call.result }),
    }
  if (call.status !== "succeeded")
    return { kind: "failed", effect, errorCode: "ai.tool_outcome_unknown", retryable: false }

  if (effect === "discovery")
    return { kind: "succeeded", effect, result: call.result, verificationRequired: false }
  if (effect === "read")
    return { kind: "succeeded", effect, result: call.result, verificationRequired: false }
  if (effect === "execute" && operation.operationId === "execReleaseRuntimeCommand" && recordValue(call.result).exitCode !== 0)
    return { kind: "failed", effect, errorCode: "ai.tool_execution_failed", retryable: false, result: call.result }
  if (effect === "execute")
    return { kind: "succeeded", effect, result: call.result, verificationRequired: false }
  if (effect === "interactive")
    return { kind: "succeeded", effect, result: call.result, verificationRequired: false }
  if (effect === "presentation")
    return { kind: "succeeded", effect, result: call.result, verificationRequired: false }

  const verification = verificationForCall(operation, call, resolveOperation)
  if (effect === "async") {
    if (!verification)
      return { kind: "failed", effect, errorCode: "ai.verification_inconclusive", retryable: false, result: call.result }
    return {
      kind: "running",
      effect,
      result: call.result,
      ...(verification.externalOperationId ? { externalOperationId: verification.externalOperationId } : {}),
      verification,
    }
  }

  return {
    kind: "succeeded",
    effect,
    result: call.result,
    verificationRequired: true,
    ...(verification ? { verification } : {}),
  }
}

export function verificationForCall(
  operation: ToolOperation,
  call: ToolCallRecord,
  resolveOperation?: (operationId: string) => ToolOperation | undefined,
): RunToolVerification | undefined {
  const verification = operation.verification?.find(rule =>
    !rule.whenResultHas || hasResultField(call.result, rule.whenResultHas))
  if (!verification) return undefined
  const result = recordValue(call.result)
  if (verification.family === "resource")
    return resourceVerificationForCall(verification, call, resolveOperation)
  const body = recordValue(call.arguments.body)
  if (verification.family === "project_order") {
    const expected = body.projectIds
    if (!Array.isArray(expected) || expected.length === 0) return undefined
    return {
      runId: call.runId,
      sourceToolCallId: call.id,
      sourceOperationId: call.operationId,
      operationId: verification.operationId,
      family: verification.family,
      expectation: "present",
      arguments: { page: 1, pageSize: 100 },
      externalOperationId: call.id,
      attempts: 0,
      expected,
    }
  }
  if (verification.family === "billing_rate_rules") {
    const expected = body.rules
    if (!Array.isArray(expected) || expected.length === 0) return undefined
    return {
      runId: call.runId,
      sourceToolCallId: call.id,
      sourceOperationId: call.operationId,
      operationId: verification.operationId,
      family: verification.family,
      expectation: "present",
      arguments: {},
      externalOperationId: call.id,
      attempts: 0,
      expected,
    }
  }
  const projectId = stringValue(call.arguments.projectId)
  const nestedRelease = recordValue(result.release)
  const nestedApplication = recordValue(result.application)
  const nestedDeploymentTarget = recordValue(result.deploymentTarget)
  const externalOperationId = externalOperationIdFor(verification, result, call.arguments, nestedRelease, nestedApplication, nestedDeploymentTarget)
  const applicationId = stringValue(call.arguments.applicationId)
    ?? stringValue(result.applicationId)
    ?? stringValue(nestedApplication.id)
    ?? stringValue(nestedDeploymentTarget.applicationId)
  if (!projectId || !externalOperationId) return undefined

  let argumentsValue: Record<string, unknown>
  switch (verification.family) {
    case "build_run":
      argumentsValue = { projectId, runId: externalOperationId }
      break
    case "release":
      argumentsValue = { projectId, releaseId: externalOperationId }
      break
    case "gateway_route":
      argumentsValue = { projectId, routeId: externalOperationId }
      break
    case "application":
      argumentsValue = { projectId, applicationId: externalOperationId }
      break
    case "deployment_target": {
      if (!applicationId) return undefined
      const stage = stringValue(result.stage) ?? stringValue(recordValue(call.arguments.body).stage)
      argumentsValue = {
        projectId,
        applicationId,
        ...(stage ? { stage } : {}),
        page: 1,
        pageSize: 100,
      }
      break
    }
    case "runtime_secrets": {
      const targetId = stringValue(result.id) ?? stringValue(call.arguments.targetId)
      if (!applicationId || !targetId) return undefined
      argumentsValue = { projectId, applicationId, targetId }
      break
    }
    case "runtime_config_secrets": {
      const setId = stringValue(call.arguments.setId) ?? stringValue(result.id)
      if (!setId) return undefined
      argumentsValue = { projectId, setId }
      break
    }
    default:
      return undefined
  }
  const secretMutations = ["runtime_secrets", "runtime_config_secrets"].includes(verification.family)
    ? runtimeSecretMutations(call.arguments)
    : undefined
  return {
    runId: call.runId,
    sourceToolCallId: call.id,
    sourceOperationId: call.operationId,
    operationId: verification.operationId,
    family: verification.family,
    expectation: verification.expect ?? "present",
    arguments: argumentsValue,
    externalOperationId,
    attempts: 0,
    ...(secretMutations?.requiredKeys.length ? { requiredKeys: secretMutations.requiredKeys } : {}),
    ...(secretMutations?.absentKeys.length ? { absentKeys: secretMutations.absentKeys } : {}),
  }
}

function resourceVerificationForCall(
  rule: ToolVerificationRule,
  call: ToolCallRecord,
  resolveOperation?: (operationId: string) => ToolOperation | undefined,
): RunToolVerification | undefined {
  const readOperation = resolveOperation?.(rule.operationId)
  if (!readOperation) return undefined
  const externalOperationId = rule.singleton ? call.id : expressionValue(rule.idFrom, call)
  if (!externalOperationId) return undefined
  const readIdArgument = rule.readIdArgument
  const body = recordValue(call.arguments.body)
  const result = recordValue(call.result)
  const properties = recordValue(readOperation.inputSchema.properties)
  const argumentsValue: Record<string, unknown> = {}
  for (const name of Object.keys(properties)) {
    if (name === readIdArgument) {
      argumentsValue[name] = externalOperationId
      continue
    }
    if (name === "page") {
      argumentsValue[name] = 1
      continue
    }
    if (name === "pageSize") {
      argumentsValue[name] = 100
      continue
    }
    const value = primitiveValue(call.arguments[name])
      ?? primitiveValue(body[name])
      ?? primitiveValue(result[name])
    if (value !== undefined) argumentsValue[name] = value
  }
  const required = Array.isArray(readOperation.inputSchema.required)
    ? readOperation.inputSchema.required.filter((name): name is string => typeof name === "string")
    : []
  if (required.some(name => argumentsValue[name] === undefined)) return undefined
  return {
    runId: call.runId,
    sourceToolCallId: call.id,
    sourceOperationId: call.operationId,
    operationId: rule.operationId,
    family: "resource",
    expectation: rule.expect ?? "present",
    arguments: argumentsValue,
    externalOperationId,
    attempts: 0,
    resource: {
      singleton: rule.singleton === true,
      collection: rule.collection === true,
      targetField: rule.targetField ?? "id",
      ...(rule.statusField ? { statusField: rule.statusField } : {}),
      ...(rule.pendingStatuses?.length ? { pendingStatuses: rule.pendingStatuses } : {}),
      ...(rule.successStatuses?.length ? { successStatuses: rule.successStatuses } : {}),
      ...(rule.failureStatuses?.length ? { failureStatuses: rule.failureStatuses } : {}),
    },
  }
}

function expressionValue(expression: string | undefined, call: ToolCallRecord): string | undefined {
  if (!expression) return undefined
  const value = expression.split(".").reduce<unknown>((current, segment) => {
    const record = recordValue(current)
    return record[segment]
  }, { result: call.result, argument: call.arguments })
  return stringValue(value)
}

function primitiveValue(value: unknown): string | number | boolean | undefined {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean"
    ? value
    : undefined
}

function externalOperationIdFor(
  verification: ToolVerificationRule,
  result: Record<string, unknown>,
  argumentsValue: Record<string, unknown>,
  nestedRelease: Record<string, unknown>,
  nestedApplication: Record<string, unknown>,
  nestedDeploymentTarget: Record<string, unknown>,
): string | undefined {
  if (verification.family === "application")
    return stringValue(nestedApplication.id)
      ?? stringValue(result.id)
      ?? stringValue(result.applicationId)
      ?? stringValue(argumentsValue.applicationId)
  if (verification.family === "deployment_target")
    return stringValue(nestedDeploymentTarget.id)
      ?? stringValue(result.id)
      ?? stringValue(argumentsValue.targetId)
  if (verification.family === "runtime_secrets")
    return stringValue(result.id) ?? stringValue(argumentsValue.targetId)
  if (verification.family === "runtime_config_secrets")
    return stringValue(result.id) ?? stringValue(argumentsValue.setId)
  if (verification.family === "build_run")
    return stringValue(result.id)
      ?? stringValue(argumentsValue.runId)
  return stringValue(result.id)
    ?? stringValue(nestedRelease.id)
    ?? stringValue(result.releaseId)
    ?? (verification.family === "gateway_route" ? stringValue(argumentsValue.routeId) : undefined)
}

function runtimeSecretMutations(argumentsValue: Record<string, unknown>): { requiredKeys: string[], absentKeys: string[] } {
  const items = recordValue(argumentsValue.body).items
  const required = new Set<string>()
  const absent = new Set<string>()
  for (const value of Array.isArray(items) ? items : []) {
    const item = recordValue(value)
    if (stringValue(item.valueMode) !== "secret") continue
    const key = stringValue(item.key)
    if (!key) continue
    if (stringValue(item.operation) === "clear") absent.add(key)
    else if (["set", "generate"].includes(stringValue(item.operation) ?? "")) required.add(key)
  }
  return { requiredKeys: [...required], absentKeys: [...absent] }
}

function hasResultField(value: unknown, path: string): boolean {
  let current = value
  for (const segment of path.split(".").filter(Boolean)) {
    const record = recordValue(current)
    if (!(segment in record)) return false
    current = record[segment]
  }
  return current !== undefined && current !== null
}

function isRetryable(result: unknown): boolean {
  return recordValue(result).retryable === true
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined
}
