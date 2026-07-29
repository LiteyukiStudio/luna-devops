const listInputSchema = {
  type: "object",
  properties: {
    projectId: { type: "string", maxLength: 64 },
    page: { type: "integer", maximum: 100000 },
    pageSize: { type: "integer", maximum: 100 },
  },
  required: [],
  additionalProperties: false,
} as const

export const p2ReadonlyOperations = [
  operation("listGatewayRoutes", "gateway", "gateway:read"),
  operation("listGatewayCertificates", "gateway", "gateway:read"),
  operation("listProjectHookRuns", "project", "project:read"),
  operation("listNotificationDeliveries", "event", "event:read"),
  operation("listRuntimeEvents", "event", "event:read"),
]

function operation(operationId: string, category: string, scope: string) {
  return {
    operationId,
    method: "GET",
    path: `/api/v1/ai-tools/${operationId}`,
    category,
    risk: "read",
    requiredScopes: [scope],
    approval: "never",
    idempotent: true,
    timeoutMs: 15000,
    inputSchema: listInputSchema,
    maxItems: 100,
  }
}
