import type { ConversationToolInteraction } from "../domain.js"
import type { ModelToolRetrievalState } from "../provider/provider.js"

const terminalToolStatuses = new Set(["succeeded", "failed", "canceled", "skipped"])
const pendingBusinessStatuses = new Set(["pending", "running", "queued", "in_progress", "progressing"])
const terminalBusinessStatuses = new Set([
  "succeeded", "success", "completed", "done", "ready", "active",
  "failed", "canceled", "cancelled", "lost", "timeout", "rejected",
  "unavailable", "not_found", "not_configured", "disabled",
])

/**
 * 只保留可安全进入工具检索查询的稳定工作流状态；资源 ID、原始参数和工具结果不会进入这里。
 */
export class ToolRetrievalStateTracker {
  private readonly resourceContext = new Set<string>()
  private readonly completedOperations = new Set<string>()
  private readonly stableOutcomes = new Set<string>()
  private readonly stableErrorCodes = new Set<string>()
  private pendingState: ModelToolRetrievalState["pendingState"]

  constructor(pageContext: Record<string, unknown>, interactions: ConversationToolInteraction[] = []) {
    for (const resourceType of pageResourceTypes(pageContext)) this.resourceContext.add(resourceType)
    for (const interaction of interactions) {
      if (interaction.type !== "tool_call") continue
      const operationId = exactStringValue(interaction.content.operationId, 120)
      const status = stringValue(interaction.content.status, 80)
      if (!operationId || !status) continue
      this.record(operationId, status, interaction.content.result, stringValue(interaction.content.errorCode, 160))
    }
  }

  record(operationId: string, status: string, result?: unknown, errorCode?: string): void {
    const stableOperationId = exactStringValue(operationId, 120)
    const stableStatus = stringValue(status, 80)
    if (!stableOperationId || !stableStatus) return
    if (terminalToolStatuses.has(stableStatus)) this.completedOperations.add(stableOperationId)
    this.stableOutcomes.add(`${stableOperationId}:${stableStatus}`)
    const normalizedErrorCode = stringValue(errorCode, 160)
    if (normalizedErrorCode) this.stableErrorCodes.add(normalizedErrorCode)

    const verificationStatus = nestedString(result, "lunaVerification", "status")
    const businessStatus = verificationStatus ?? topLevelString(result, "status")
    if (businessStatus && pendingBusinessStatuses.has(businessStatus)) {
      this.pendingState = "async_terminal_check"
    }
    else if (businessStatus && terminalBusinessStatuses.has(businessStatus)) {
      this.pendingState = undefined
    }
    else if (stableStatus === "awaiting_approval") {
      this.pendingState = "approval"
    }
    else if (stableStatus === "awaiting_mfa") {
      this.pendingState = "mfa"
    }
  }

  snapshot(): ModelToolRetrievalState {
    return {
      resourceContext: [...this.resourceContext].sort(),
      completedOperations: [...this.completedOperations].sort(),
      stableOutcomes: [...this.stableOutcomes].sort(),
      ...(this.pendingState ? { pendingState: this.pendingState } : {}),
      stableErrorCodes: [...this.stableErrorCodes].sort(),
    }
  }
}

function pageResourceTypes(pageContext: Record<string, unknown>): string[] {
  const candidates = [
    pageContext.pageKind,
    pageContext.resourceType,
    pageContext.resourceKind,
    pageContext.resourceCategory,
    ...stringArray(pageContext.resourceTypes),
  ]
  return [...new Set(candidates.flatMap(value => {
    const stable = stringValue(value, 120)
    return stable ? [stable] : []
  }))].slice(0, 20)
}

function stringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : []
}

function topLevelString(value: unknown, key: string): string | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined
  return stringValue((value as Record<string, unknown>)[key], 80)
}

function nestedString(value: unknown, parentKey: string, key: string): string | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined
  const nested = (value as Record<string, unknown>)[parentKey]
  return topLevelString(nested, key)
}

function stringValue(value: unknown, limit: number): string | undefined {
  if (typeof value !== "string") return undefined
  const normalized = value.trim().toLowerCase()
  return normalized ? [...normalized].slice(0, limit).join("") : undefined
}

function exactStringValue(value: unknown, limit: number): string | undefined {
  if (typeof value !== "string") return undefined
  const normalized = value.trim()
  return normalized ? [...normalized].slice(0, limit).join("") : undefined
}
