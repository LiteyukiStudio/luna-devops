import type { AIToolVisibility } from "@luna-devops/ai-interaction-card-contract"

const internalToolOperationIds = new Set([
  "search_tools",
	"get_tool_details",
	"present_card",
	"request_input",
	"request_choice",
  "rename_conversation",
])

export function toolVisibility(operationId: string): AIToolVisibility {
  return internalToolOperationIds.has(operationId) ? "internal" : "normal"
}
