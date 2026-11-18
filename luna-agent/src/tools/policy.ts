import type { ToolOperation } from "./catalog.js"
import type { ToolApprovalTier } from "./contracts.js"

export type ToolApprovalGrant = {
  explicitApproved: boolean
  sessionRemembered?: boolean
}

export type ToolDecision =
  | { action: "execute", tier: ToolApprovalTier }
  | { action: "wait_approval", tier: "L1" | "L2", rememberable: boolean, purpose: string }

export class ToolPolicy {
  effectiveTier(
    operation: Pick<ToolOperation, "approvalTier" | "requiresApproval">,
    args?: Record<string, unknown>,
  ): ToolApprovalTier {
    const baseTier: ToolApprovalTier = operation.approvalTier ?? (operation.requiresApproval ? "L2" : "L0")
    if (baseTier === "L1" && targetsProductionStage(args)) {
      return "L2"
    }
    return baseTier
  }

  evaluate(
    operation: Pick<ToolOperation, "operationId" | "approvalTier" | "requiresApproval">,
    grant: boolean | ToolApprovalGrant,
    args?: Record<string, unknown>,
  ): ToolDecision {
    const normalizedGrant: ToolApprovalGrant = typeof grant === "boolean"
      ? { explicitApproved: grant, sessionRemembered: false }
      : grant
    const tier = this.effectiveTier(operation, args)
    if (tier === "L0") {
      return { action: "execute", tier: "L0" }
    }
    if (tier === "L1") {
      if (normalizedGrant.explicitApproved || normalizedGrant.sessionRemembered) {
        return { action: "execute", tier: "L1" }
      }
      return {
        action: "wait_approval",
        tier: "L1",
        rememberable: true,
        purpose: `tool:${operation.operationId}`,
      }
    }
    if (normalizedGrant.explicitApproved) {
      return { action: "execute", tier: "L2" }
    }
    return {
      action: "wait_approval",
      tier: "L2",
      rememberable: false,
      purpose: `tool:${operation.operationId}`,
    }
  }
}

function targetsProductionStage(args: Record<string, unknown> | undefined): boolean {
  if (!args) return false
  if (isProductionStage(args.stage)) return true
  const body = args.body
  if (body && typeof body === "object" && !Array.isArray(body)) {
    return isProductionStage((body as Record<string, unknown>).stage)
  }
  return false
}

function isProductionStage(value: unknown): boolean {
  if (typeof value !== "string") return false
  const normalized = value.trim().toLowerCase()
  return normalized === "prod" || normalized === "production"
}
