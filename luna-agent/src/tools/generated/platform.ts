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

export const platformOperations = [
  operation("getDashboard", "dashboard", "dashboard:read"),
  operation("listProjects", "project", "project:read"),
  {
    operationId: "createProject",
    method: "POST",
    path: "/api/v1/ai-tools/createProject",
    category: "project",
    risk: "write",
    requiredScopes: ["project:write"],
    approval: "never",
    idempotent: true,
    timeoutMs: 15000,
    inputSchema: {
      type: "object",
      properties: {
        identifier: { type: "string", maxLength: 63 },
        name: { type: "string", maxLength: 120 },
        description: { type: "string", maxLength: 500 },
        namespaceStrategy: { type: "string", enum: ["project"] },
        maxConcurrentBuilds: { type: "integer", maximum: 100 },
        webConsoleEnabled: { type: "boolean" },
      },
      required: ["identifier", "name"],
      additionalProperties: false,
    },
    resultVerifier: "project_created",
  },
  operation("listPlatformEvents", "event", "event:read"),
  operation("getProject", "project", "project:read"),
  operation("listApplications", "application", "application:read"),
  operation("listBuildRuns", "build", "build:read"),
  operation("listReleases", "deployment", "deployment:read"),
  operation("listRuntimeClusters", "runtime", "cluster:read"),
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
