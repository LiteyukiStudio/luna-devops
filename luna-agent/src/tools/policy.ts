import type { ToolOperation } from "./catalog.js"

export type ToolDecision =
  | { action: "execute" }
  | { action: "wait_approval", purpose: string }

export class ToolPolicy {
  evaluate(operation: ToolOperation, approved: boolean): ToolDecision {
    if (operation.requiresApproval && !approved)
      return { action: "wait_approval", purpose: `tool:${operation.operationId}` }
    return { action: "execute" }
  }
}
