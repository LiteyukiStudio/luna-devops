import type { ToolOperation } from "./catalog.js"

export type ToolDecision =
  | { action: "execute" }
  | { action: "wait_approval", purpose: string }
  | { action: "wait_mfa", purpose: string }

export class ToolPolicy {
  evaluate(operation: ToolOperation, state: { approved: boolean, mfaPurpose?: string }): ToolDecision {
    const approval = operation.contract?.approval ?? operation.approval
    const risk = operation.contract?.risk ?? operation.risk
    const approvalRequired = approval === "always"
      || (approval === "risk_based" && ["high", "critical", "sensitive", "destructive"].includes(risk))
    if (approvalRequired && !state.approved) return { action: "wait_approval", purpose: `tool:${operation.operationId}` }
    const mfaPurpose = operation.contract?.mfaPurpose ?? operation.stepUpPurpose
    if (mfaPurpose && state.mfaPurpose !== mfaPurpose) return { action: "wait_mfa", purpose: mfaPurpose }
    return { action: "execute" }
  }
}
