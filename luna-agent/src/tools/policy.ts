import type { ConversationApprovalMode } from "../domain.js"
import type { ToolOperation } from "./catalog.js"
import type { ToolApprovalTier } from "./contracts.js"

export type ToolApprovalGrant = {
  explicitApproved: boolean
  sessionRemembered?: boolean
  approvalMode?: ConversationApprovalMode
  modeApproved?: "mode_auto" | "mode_full_access"
}

export type ToolDecision =
  | { action: "execute", tier: ToolApprovalTier, rememberable: boolean, autoDecision?: "mode_auto" | "mode_full_access" }
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

  isAutoModeExempt(
    operation: Pick<ToolOperation, "operationId" | "domain" | "adminOnly" | "approvalTier" | "requiresApproval" | "autoModeExempt">,
    args?: Record<string, unknown>,
  ): boolean {
    const tier = this.effectiveTier(operation, args)
    if (tier === "L0") return true
    if (targetsProductionStage(args)) return false
    if (typeof operation.autoModeExempt === "boolean") {
      return operation.autoModeExempt
    }
    if (operation.adminOnly || operation.domain === "admin") {
      return false
    }
    const lower = operation.operationId.trim().toLowerCase()
    for (const marker of ["_delete_", "_remove_", "_unbind_", "_revoke_"]) {
      if (lower.includes(marker)) return false
    }
    if (
      lower === "runtime_exec_command"
      || lower === "runtime_save_cluster"
      || lower === "runtime_save_cluster_egress_exception"
      || lower === "runtime_approve_cluster_network_share"
      || lower === "workspace_request_billing_transfer"
    ) {
      return false
    }
    return true
  }

  evaluate(
    operation: Pick<ToolOperation, "operationId" | "domain" | "adminOnly" | "approvalTier" | "requiresApproval" | "autoModeExempt">,
    grant: boolean | ToolApprovalGrant,
    args?: Record<string, unknown>,
  ): ToolDecision {
    const normalizedGrant: ToolApprovalGrant = typeof grant === "boolean"
      ? { explicitApproved: grant, sessionRemembered: false }
      : grant
    const mode: ConversationApprovalMode = normalizedGrant.approvalMode ?? "default"
    const tier = this.effectiveTier(operation, args)
    if (tier === "L0") {
      return { action: "execute", tier: "L0", rememberable: false }
    }
    if (mode === "full_access" || normalizedGrant.modeApproved === "mode_full_access") {
      return { action: "execute", tier, rememberable: false, autoDecision: "mode_full_access" }
    }
    if (
      (mode === "auto" || normalizedGrant.modeApproved === "mode_auto")
      && this.isAutoModeExempt(operation, args)
    ) {
      return { action: "execute", tier, rememberable: false, autoDecision: "mode_auto" }
    }
    if (tier === "L1") {
      const rememberable = mode !== "strict"
      if (normalizedGrant.explicitApproved || (mode !== "strict" && normalizedGrant.sessionRemembered)) {
        return { action: "execute", tier: "L1", rememberable }
      }
      return {
        action: "wait_approval",
        tier: "L1",
        rememberable,
        purpose: `tool:${operation.operationId}`,
      }
    }
    if (normalizedGrant.explicitApproved) {
      return { action: "execute", tier: "L2", rememberable: false }
    }
    return {
      action: "wait_approval",
      tier: "L2",
      rememberable: false,
      purpose: `tool:${operation.operationId}`,
    }
  }
}

export function targetsProductionStage(args: Record<string, unknown> | undefined): boolean {
  if (!args) return false
  if (isProductionStage(args.stage) || isProductionStage(args.target)) return true
  const body = args.body
  if (body && typeof body === "object" && !Array.isArray(body)) {
    const bodyRecord = body as Record<string, unknown>
    return isProductionStage(bodyRecord.stage) || isProductionStage(bodyRecord.target)
  }
  return false
}

function isProductionStage(value: unknown): boolean {
  if (typeof value !== "string") return false
  const normalized = value.trim().toLowerCase()
  return normalized === "prod" || normalized === "production"
}
