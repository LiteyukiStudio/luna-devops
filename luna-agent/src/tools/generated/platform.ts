const platformListInputSchema = {
  type: "object",
  properties: {
    page: { type: "integer", maximum: 100000 },
    pageSize: { type: "integer", maximum: 100 },
  },
  required: [],
  additionalProperties: false,
} as const

const platformProjectListInputSchema = {
  type: "object",
  properties: {
    page: { type: "integer", minimum: 1, maximum: 100000, default: 1 },
    pageSize: { type: "integer", minimum: 1, maximum: 100, default: 20 },
    scope: { type: "string", enum: ["related", "all"], default: "related" },
  },
  required: [],
  additionalProperties: false,
} as const

const projectListInputSchema = {
  type: "object",
  properties: {
    projectId: { type: "string", maxLength: 64 },
    page: { type: "integer", maximum: 100000 },
    pageSize: { type: "integer", maximum: 100 },
  },
  required: ["projectId"],
  additionalProperties: false,
} as const

const appTemplateListInputSchema = {
  type: "object",
  properties: {
    query: { type: "string", maxLength: 120 },
    category: { type: "string", maxLength: 80 },
  },
  required: [],
  additionalProperties: false,
} as const

const appTemplateDetailInputSchema = {
  type: "object",
  properties: {
    id: { type: "string", maxLength: 120 },
    slug: { type: "string", maxLength: 120 },
  },
  required: [],
  additionalProperties: false,
} as const

const webSearchInputSchema = {
  type: "object",
  properties: {
    query: { type: "string", maxLength: 300 },
    limit: { type: "integer", maximum: 10 },
  },
  required: ["query"],
  additionalProperties: false,
} as const

const fetchWebPageInputSchema = {
  type: "object",
  properties: {
    url: { type: "string", maxLength: 2048 },
    maxCharacters: { type: "integer", maximum: 50000 },
  },
  required: ["url"],
  additionalProperties: false,
} as const

const updateDeploymentTargetRuntimeSecretsInputSchema = {
  type: "object",
  properties: {
    projectId: { type: "string", maxLength: 64 },
    applicationId: { type: "string", maxLength: 64 },
    targetId: { type: "string", maxLength: 64 },
    body: {
      type: "object",
      properties: {
        values: { type: "object", writeOnly: true, "x-luna-sensitive": true, additionalProperties: { type: "string", maxLength: 8192 } },
        generate: {
          type: "object",
          additionalProperties: {
            type: "object",
            properties: {
              length: { type: "integer", minimum: 8, maximum: 256, default: 32 },
              encoding: { type: "string", enum: ["base64", "hex", "alphanumeric", "numeric"], default: "base64" },
            },
            required: [],
            additionalProperties: false,
          },
        },
        clear: { type: "array", items: { type: "string", maxLength: 128 } },
      },
      required: [],
      additionalProperties: false,
    },
  },
  required: ["projectId", "applicationId", "targetId", "body"],
  additionalProperties: false,
} as const

export const platformOperations = [
  operation("getDashboard", "dashboard", "dashboard:read", platformListInputSchema),
  operation("listProjects", "project", "project:read", platformProjectListInputSchema),
  operation("listAppTemplates", "application", "application:read", appTemplateListInputSchema),
  operation("getAppTemplate", "application", "application:read", appTemplateDetailInputSchema),
  operation("webSearch", "web", "web:read", webSearchInputSchema),
  operation("fetchWebPage", "web", "web:read", fetchWebPageInputSchema),
  {
    operationId: "updateDeploymentTargetRuntimeSecrets",
    method: "PUT",
    path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/runtime-secrets",
    category: "deployments",
    risk: "sensitive",
    requiredScopes: ["deployment:update"],
    approval: "always",
    stepUpPurpose: "secret_update",
    idempotent: true,
    timeoutMs: 30000,
    inputSchema: updateDeploymentTargetRuntimeSecretsInputSchema,
    sensitivePaths: ["body.values"],
    resultVerifier: "updateDeploymentTargetRuntimeSecrets_accepted",
  },
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
  operation("listPlatformEvents", "event", "event:read", projectListInputSchema),
  operation("getProject", "project", "project:read", projectListInputSchema),
  operation("listApplications", "application", "application:read", projectListInputSchema),
  operation("listBuildRuns", "build", "build:read", projectListInputSchema),
  operation("listReleases", "deployment", "deployment:read", projectListInputSchema),
  operation("listRuntimeClusters", "runtime", "cluster:read", projectListInputSchema),
  operation("listGatewayRoutes", "gateway", "gateway:read", projectListInputSchema),
  operation("listGatewayCertificates", "gateway", "gateway:read", projectListInputSchema),
  operation("listProjectHookRuns", "project", "project:read", projectListInputSchema),
  operation("listNotificationDeliveries", "event", "event:read", projectListInputSchema),
  operation("listRuntimeEvents", "event", "event:read", projectListInputSchema),
]

function operation(
  operationId: string,
  category: string,
  scope: string,
  inputSchema: typeof platformListInputSchema | typeof platformProjectListInputSchema | typeof projectListInputSchema | typeof appTemplateListInputSchema | typeof appTemplateDetailInputSchema | typeof webSearchInputSchema | typeof fetchWebPageInputSchema,
) {
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
    inputSchema,
    maxItems: 100,
  }
}
