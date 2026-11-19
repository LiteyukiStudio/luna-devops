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
  | { kind: "failed", effect: ToolEffect, errorCode: string, retryable: boolean, result?: unknown }
  | { kind: "awaiting_approval", effect: ToolEffect, toolCallId: string }
  | { kind: "awaiting_input", effect: ToolEffect, fields: string[] }
  | {
      kind: "running"
      effect: "async"
      result: unknown
      externalOperationId?: string
      verification: RunToolVerification
    }
  | { kind: "canceled", effect: ToolEffect, result?: unknown }

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

  if (effect === "discovery" || effect === "read" || effect === "interactive" || effect === "presentation")
    return { kind: "succeeded", effect, result: call.result, verificationRequired: false }
  if (effect === "execute" && operation.operationId === "runtime_exec_command" && recordValue(call.result).exitCode !== 0)
    return { kind: "failed", effect, errorCode: "ai.tool_execution_failed", retryable: false, result: call.result }
  if (effect === "execute")
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
  const verification = operation.verification?.find(rule => matchesVerificationRule(rule, call))
  if (!verification) return undefined
  const result = recordValue(call.result)
  if (verification.family === "resource")
    return resourceVerificationForCall(verification, call, resolveOperation)

  if (verification.family === "project_order") {
    const expected = Array.isArray(result.projectIds)
      ? result.projectIds
      : Array.isArray(call.arguments.projects)
        ? call.arguments.projects
        : recordValue(call.arguments.body).projectIds
    if (!Array.isArray(expected) || expected.length === 0) return undefined
    const pageSize = boundedReadPageSize(resolveOperation?.(verification.operationId))
    return {
      runId: call.runId,
      sourceToolCallId: call.id,
      sourceOperationId: call.operationId,
      operationId: verification.operationId,
      family: verification.family,
      expectation: "present",
      arguments: { sortBy: "dashboardOrder", sortOrder: "asc", page: 1, pageSize },
      externalOperationId: call.id,
      attempts: 0,
      expected,
    }
  }

  if (verification.family === "billing_rate_rules") {
    const expected = Array.isArray(call.arguments.rules)
      ? call.arguments.rules
      : recordValue(call.arguments.body).rules
    if (!Array.isArray(expected) || expected.length === 0) return undefined
    return {
      runId: call.runId,
      sourceToolCallId: call.id,
      sourceOperationId: call.operationId,
      operationId: verification.operationId,
      family: verification.family,
      expectation: "present",
      arguments: { ...(verification.staticArguments ?? {}) },
      externalOperationId: call.id,
      attempts: 0,
      expected,
    }
  }

  const project = resolveProjectLocator(result, call.arguments)
  const nestedRelease = recordValue(result.release)
  const nestedApplication = recordValue(result.application)
  const nestedDeploymentTarget = recordValue(result.deploymentTarget)
  const externalOperationId = externalOperationIdFor(verification, result, call.arguments, nestedRelease, nestedApplication, nestedDeploymentTarget)
  const application = stringValue(result.applicationId)
    ?? stringValue(result.primaryApplicationId)
    ?? stringValue(nestedApplication.id)
    ?? stringValue(nestedDeploymentTarget.applicationId)
    ?? stringValue(call.arguments.application)
    ?? stringValue(call.arguments.applicationId)
  if (!project || !externalOperationId) return undefined

  let argumentsValue: Record<string, unknown>
  switch (verification.family) {
    case "build_run":
      argumentsValue = { project, buildRunId: externalOperationId }
      break
    case "release":
      argumentsValue = { project, releaseId: externalOperationId }
      break
    case "gateway_route":
      argumentsValue = { project, route: externalOperationId }
      break
    case "application":
      argumentsValue = { project, application: externalOperationId }
      break
    case "deployment_target": {
      if (!application) return undefined
      argumentsValue = {
        project,
        application,
        target: externalOperationId,
      }
      break
    }
    default:
      return undefined
  }
  const secretMutations = extractSecretMutations(call.arguments)
  return {
    runId: call.runId,
    sourceToolCallId: call.id,
    sourceOperationId: call.operationId,
    operationId: verification.operationId,
    family: verification.family,
    expectation: verification.expect ?? "present",
    arguments: { ...(verification.staticArguments ?? {}), ...argumentsValue },
    externalOperationId,
    attempts: 0,
    ...(secretMutations.requiredKeys.length ? { requiredKeys: secretMutations.requiredKeys } : {}),
    ...(secretMutations.absentKeys.length ? { absentKeys: secretMutations.absentKeys } : {}),
  }
}

function resourceVerificationForCall(
  rule: ToolVerificationRule,
  call: ToolCallRecord,
  resolveOperation?: (operationId: string) => ToolOperation | undefined,
): RunToolVerification | undefined {
  const readOperation = resolveOperation?.(rule.operationId)
  if (!readOperation) return undefined
  const externalOperationId = rule.singleton ? call.id : (expressionValue(rule.idFrom, call) ?? stringValue(recordValue(call.result).id))
  if (!externalOperationId) return undefined
  const readIdArgument = rule.readIdArgument
  const result = recordValue(call.result)
  const properties = recordValue(readOperation.inputSchema.properties)
  const argumentsValue: Record<string, unknown> = {
    ...(rule.staticArguments ?? {}),
  }
  const maxPageSize = boundedReadPageSize(readOperation)

  for (const name of Object.keys(properties)) {
    if (name in argumentsValue) continue
    if (name === readIdArgument) {
      argumentsValue[name] = externalOperationId
      continue
    }
    if (name === "page") {
      argumentsValue[name] = 1
      continue
    }
    if (name === "pageSize") {
      argumentsValue[name] = maxPageSize
      continue
    }
    if (name === "project") {
      const proj = resolveProjectLocator(result, call.arguments)
      if (proj !== undefined) argumentsValue[name] = proj
      continue
    }
    const value = primitiveValue(call.arguments[name]) ?? primitiveValue(result[name])
    if (value !== undefined) argumentsValue[name] = value
  }
  const required = Array.isArray(readOperation.inputSchema.required)
    ? readOperation.inputSchema.required.filter((name): name is string => typeof name === "string")
    : []
  if (required.some(name => argumentsValue[name] === undefined)) return undefined
  const secretMutations = extractSecretMutations(call.arguments)
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
    ...(secretMutations.requiredKeys.length ? { requiredKeys: secretMutations.requiredKeys } : {}),
    ...(secretMutations.absentKeys.length ? { absentKeys: secretMutations.absentKeys } : {}),
    resource: {
      singleton: rule.singleton === true,
      collection: rule.collection === true,
      ...(rule.collectionPath ? { collectionPath: rule.collectionPath } : {}),
      targetField: rule.targetField ?? "id",
      ...(rule.statusField ? { statusField: rule.statusField } : {}),
      ...(rule.pendingStatuses?.length ? { pendingStatuses: rule.pendingStatuses } : {}),
      ...(rule.successStatuses?.length ? { successStatuses: rule.successStatuses } : {}),
      ...(rule.failureStatuses?.length ? { failureStatuses: rule.failureStatuses } : {}),
    },
  }
}

function matchesVerificationRule(rule: ToolVerificationRule, call: ToolCallRecord): boolean {
  if (rule.whenResultHas && !hasResultField(call.result, rule.whenResultHas)) {
    return false
  }
  if (rule.whenArgumentEquals) {
    for (const [key, expected] of Object.entries(rule.whenArgumentEquals)) {
      if (nestedFieldValue(call.arguments, key) !== expected) return false
    }
  }
  if (rule.whenResultEquals) {
    for (const [key, expected] of Object.entries(rule.whenResultEquals)) {
      if (nestedFieldValue(call.result, key) !== expected) return false
    }
  }
  return true
}

function resolveProjectLocator(result: Record<string, unknown>, args: Record<string, unknown>): string | undefined {
  return stringValue(result.projectId)
    ?? stringValue(args.project)
    ?? stringValue(args.projectId)
    ?? firstStringElement(result.projectIds)
    ?? firstStringElement(args.addProjects)
    ?? firstStringElement(args.projects)
}

function boundedReadPageSize(readOperation: ToolOperation | undefined): number {
  const properties = readOperation ? recordValue(readOperation.inputSchema.properties) : {}
  const pageSizeProp = recordValue(properties.pageSize)
  const maxVal = typeof pageSizeProp.maximum === "number" && Number.isFinite(pageSizeProp.maximum)
    ? Math.floor(pageSizeProp.maximum)
    : 50
  return Math.max(1, Math.min(50, maxVal))
}

function expressionValue(expression: string | undefined, call: ToolCallRecord): string | undefined {
  if (!expression) return undefined
  const value = expression.split(".").reduce<unknown>((current, segment) => {
    const record = recordValue(current)
    return record[segment]
  }, { result: call.result, argument: call.arguments })
  return stringValue(value)
}

function nestedFieldValue(source: unknown, path: string): unknown {
  let current = source
  for (const segment of path.split(".").filter(Boolean)) {
    const record = recordValue(current)
    current = record[segment]
  }
  return current
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
    return stringValue(result.applicationId)
      ?? stringValue(result.primaryApplicationId)
      ?? stringValue(nestedApplication.id)
      ?? stringValue(result.id)
      ?? stringValue(argumentsValue.application)
      ?? stringValue(argumentsValue.applicationId)
  if (verification.family === "deployment_target")
    return stringValue(result.deploymentTargetId)
      ?? stringValue(result.primaryDeploymentTargetId)
      ?? stringValue(nestedDeploymentTarget.id)
      ?? stringValue(result.id)
      ?? stringValue(argumentsValue.target)
      ?? stringValue(argumentsValue.targetId)
  if (verification.family === "build_run")
    return stringValue(result.buildRunId)
      ?? stringValue(result.id)
      ?? stringValue(argumentsValue.buildRunId)
      ?? stringValue(argumentsValue.runId)
  if (verification.family === "gateway_route")
    return stringValue(result.routeId)
      ?? stringValue(result.id)
      ?? stringValue(argumentsValue.route)
      ?? stringValue(argumentsValue.routeId)
  return stringValue(result.releaseId)
    ?? stringValue(result.primaryReleaseId)
    ?? stringValue(nestedRelease.id)
    ?? stringValue(result.id)
    ?? stringValue(argumentsValue.releaseId)
}

function extractSecretMutations(argumentsValue: Record<string, unknown>): { requiredKeys: string[], absentKeys: string[] } {
  const required = new Set<string>()
  const absent = new Set<string>()

  const actions = argumentsValue.secretActions ?? argumentsValue.secrets
  if (Array.isArray(actions)) {
    for (const raw of actions) {
      const item = recordValue(raw)
      const key = stringValue(item.key)
      if (!key) continue
      const op = stringValue(item.operation) ?? stringValue(item.action)
      if (op === "clear" || op === "delete" || op === "remove") absent.add(key)
      else if (op === "set" || op === "generate" || item.value !== undefined) required.add(key)
    }
  }
  for (const mapKey of ["upsertSecrets", "secretValues", "secretConfigs"] as const) {
    const dict = recordValue(argumentsValue[mapKey])
    for (const key of Object.keys(dict)) {
      if (key.trim()) required.add(key.trim())
    }
  }
  for (const arrKey of ["removeSecretKeys", "clearSecretKeys"] as const) {
    const arr = argumentsValue[arrKey]
    if (Array.isArray(arr)) {
      for (const k of arr) {
        const s = stringValue(k)
        if (s) absent.add(s)
      }
    }
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

function firstStringElement(value: unknown): string | undefined {
  if (!Array.isArray(value)) return undefined
  for (const item of value) {
    const s = stringValue(item)
    if (s) return s
  }
  return undefined
}
