import type { ToolOperation } from "./catalog.js"

export type ToolDecision =
  | { action: "execute" }
  | { action: "wait_approval", purpose: string }

export class ToolPolicy {
  evaluate(operation: ToolOperation, state: { approved: boolean, mfaPurpose?: string }): ToolDecision {
    const approval = operation.contract?.approval ?? operation.approval
    const risk = operation.contract?.risk ?? operation.risk
    const approvalRequired = approval === "always"
      || (approval === "risk_based" && ["high", "critical", "sensitive", "destructive"].includes(risk))
    if (approvalRequired && !state.approved) return { action: "wait_approval", purpose: `tool:${operation.operationId}` }
    // Step-up 是否仍有效只能由持有 Session 与 Assertion 的 Luna API 权威判断。
    // Agent 先执行委托交换；缺失时 API 返回 mfa_required，再进入可恢复状态。
    return { action: "execute" }
  }
}
