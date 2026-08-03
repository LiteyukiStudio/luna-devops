import type { AIToolVisibility } from "@luna-devops/ai-interaction-card-contract"

const internalToolOperationIds = new Set([
  "create_options",
  "rename_conversation",
])

export function toolVisibility(operationId: string): AIToolVisibility {
  return internalToolOperationIds.has(operationId) ? "internal" : "normal"
}
