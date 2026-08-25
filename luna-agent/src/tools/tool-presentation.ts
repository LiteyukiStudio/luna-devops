import type { AIToolVisibility } from "@luna-devops/ai-interaction-card-contract"
import { internalToolOperationIds } from "./internal-operation-ids.js"

export function toolVisibility(operationId: string): AIToolVisibility {
  return internalToolOperationIds.has(operationId) ? "internal" : "normal"
}
