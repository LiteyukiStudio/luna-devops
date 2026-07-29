import type { ToolOperation } from "./catalog.js"

export type ToolDecision =
  | { action: "execute" }
  | { action: "wait_approval", purpose: string }
  | { action: "wait_mfa", purpose: string }

export class ToolPolicy {
  evaluate(operation: ToolOperation, state: { approved: boolean, mfaPurpose?: string }): ToolDecision {
    const approvalRequired = operation.approval === "always"
      || (operation.approval === "risk_based" && ["sensitive", "destructive"].includes(operation.risk))
    if (approvalRequired && !state.approved) return { action: "wait_approval", purpose: `tool:${operation.operationId}` }
    if (operation.stepUpPurpose && state.mfaPurpose !== operation.stepUpPurpose) return { action: "wait_mfa", purpose: operation.stepUpPurpose }
    return { action: "execute" }
  }
}
