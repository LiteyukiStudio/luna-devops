import { hashCanonicalJSON } from "../canonical-json.js"
import type { CreateTurn } from "../domain.js"

const excludedClientContextKeys = new Set(["timestamp", "utcOffsetMinutes", "locale"])

function sanitizePageContextForIdempotency(pageContext: Record<string, unknown>): Record<string, unknown> {
  const sanitized: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(pageContext)) {
    if (key === "server" || key === "locale") continue
    if (key === "client" && value && typeof value === "object" && !Array.isArray(value)) {
      const clientEntries = Object.entries(value as Record<string, unknown>)
        .filter(([clientKey]) => !excludedClientContextKeys.has(clientKey))
      if (clientEntries.length > 0) {
        sanitized.client = Object.fromEntries(clientEntries)
      }
      continue
    }
    sanitized[key] = value
  }
  return sanitized
}

export function createTurnRequestHash(input: CreateTurn): string {
  return hashCanonicalJSON({
    conversationId: input.conversationId,
    input: input.input,
    ...(input.modelId ? { modelId: input.modelId } : {}),
    pageContext: sanitizePageContextForIdempotency(input.pageContext),
    ...(input.directToolAction
      ? {
          directToolAction: {
            operationId: input.directToolAction.operationId,
            arguments: input.directToolAction.arguments,
          },
        }
      : {}),
  })
}
