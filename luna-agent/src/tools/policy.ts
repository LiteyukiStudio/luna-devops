import type { ToolOperation } from "./catalog.js"

export type ToolDecision =
  | { action: "execute" }
  | { action: "wait_approval", purpose: string }

export class ToolPolicy {
  evaluate(operation: ToolOperation, state: { approved: boolean, exempt: boolean }): ToolDecision {
    if (operation.requiresApproval && !state.approved && !state.exempt)
      return { action: "wait_approval", purpose: `tool:${operation.operationId}` }
    return { action: "execute" }
  }
}
