import type { AIToolVisibility } from "@luna-devops/ai-interaction-card-contract"
import { internalToolOperationIds } from "./internal-operation-ids.js"

export function toolVisibility(operationId: string): AIToolVisibility {
  if (operationId === "wait_for_task") return "normal"
  return internalToolOperationIds.has(operationId) ? "internal" : "normal"
}
