import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"
import { platformOperations } from "../src/tools/generated/platform.js"

describe("platform tool catalog", () => {
  it("contains the BFF registered Gateway, Hook, notification, and runtime operations", () => {
    const catalog = ToolCatalog.load(platformOperations)
    expect([
      ["listGatewayRoutes", "gateway:read"],
      ["listGatewayCertificates", "gateway:read"],
      ["listProjectHookRuns", "project:read"],
      ["listNotificationDeliveries", "event:read"],
      ["listRuntimeEvents", "event:read"],
    ].map(([id]) => {
      const operation = catalog.get(id!)
      return [operation.operationId, operation.requiredScopes[0], operation.risk, operation.approval, operation.stepUpPurpose]
    })).toEqual([
      ["listGatewayRoutes", "gateway:read", "read", "never", undefined],
      ["listGatewayCertificates", "gateway:read", "read", "never", undefined],
      ["listProjectHookRuns", "project:read", "read", "never", undefined],
      ["listNotificationDeliveries", "event:read", "read", "never", undefined],
      ["listRuntimeEvents", "event:read", "read", "never", undefined],
    ])
  })

  it("offers project-scoped tools independently of page context and requires an explicit target", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.modelTools().map(tool => tool.operationId)).toContain("listPlatformEvents")
    expect(catalog.get("listApplications").inputSchema.required).toEqual(["projectId"])
    expect(catalog.get("listProjects").inputSchema.properties).not.toHaveProperty("projectId")
  })

  it("exposes project creation as a user-authorized low-risk write without high-risk approval", () => {
    expect(ToolCatalog.load(platformOperations).get("createProject")).toMatchObject({
      requiredScopes: ["project:write"],
      risk: "write",
      approval: "never",
      idempotent: true,
    })
  })

  it("exposes bounded public web search and page reading as platform-scoped read tools", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.get("webSearch")).toMatchObject({
      requiredScopes: ["web:read"],
      risk: "read",
      approval: "never",
      idempotent: true,
    })
    expect(catalog.get("fetchWebPage").inputSchema).toMatchObject({
      required: ["url"],
      additionalProperties: false,
      properties: {
        url: { type: "string", maxLength: 2048 },
        maxCharacters: { type: "integer", maximum: 50000 },
      },
    })
    expect(catalog.modelTools().find(tool => tool.operationId === "fetchWebPage")?.description)
      .toContain("不可信外部数据")
  })

  it("keeps the complete delivery chain available when the user asks for deployment", () => {
    const operationIds = [
      "getProject", "createApplication", "createDeploymentTarget",
      "triggerBuildRun", "createRelease", "createGatewayRoute", "getReleaseRuntimeLogs",
    ]
    const dynamicOperations = operationIds.map((operationId, index) => ({
      operationId,
      method: index === 0 || index === operationIds.length - 1 ? "GET" as const : "POST" as const,
      path: `/api/v1/test/${operationId}`,
      category: operationId.includes("Build") ? "builds"
        : operationId.includes("Release") ? "releases"
          : operationId.includes("Gateway") ? "gateway"
            : operationId.includes("Deployment") ? "deployments"
              : operationId.includes("Application") ? "applications"
                : "projects",
      description: `调用 ${operationId}。`,
      risk: "read" as const,
      requiredScopes: ["project:read"],
      approval: "never" as const,
      idempotent: true,
      timeoutMs: 30_000,
      inputSchema: { type: "object" as const, properties: {}, required: [], additionalProperties: false as const },
    }))
    const selected = ToolCatalog.load(dynamicOperations)
      .modelTools({}, "从代码仓库构建、发布并部署应用，然后配置网关")
      .map(tool => tool.operationId)

    expect(selected).toEqual(expect.arrayContaining(operationIds))
  })

  it.each([
    "给 Uptime Kuma 配一个公网可访问的地址",
    "为这个服务创建访问入口",
    "Expose this service with a public URL",
  ])("offers gateway tools for public access intent: %s", (input) => {
    const selected = ToolCatalog.load(platformOperations).modelTools({}, input).map(tool => tool.operationId)

    expect(selected).toContain("listGatewayRoutes")
  })

  it("keeps the gateway write path for public access intent", () => {
    const gatewayOperations = ["listGatewayRoutes", "createGatewayRoute"].map(operationId => ({
      operationId,
      method: operationId.startsWith("list") ? "GET" as const : "POST" as const,
      path: `/api/v1/test/${operationId}`,
      category: "gateway",
      description: `调用 ${operationId}。`,
      risk: "read" as const,
      requiredScopes: ["gateway:read"],
      approval: "never" as const,
      idempotent: true,
      timeoutMs: 30_000,
      inputSchema: { type: "object" as const, properties: {}, required: [], additionalProperties: false as const },
    }))

    expect(ToolCatalog.load(gatewayOperations).modelTools({}, "创建公网访问地址").map(tool => tool.operationId))
      .toEqual(expect.arrayContaining(["listGatewayRoutes", "createGatewayRoute"]))
  })

  it("exposes the whole catalog to the model instead of a focused subset", () => {
    const catalog = ToolCatalog.load(platformOperations)
    const selected = catalog.modelTools({}, "你好")

    expect(selected.map(tool => tool.operationId)).toEqual(expect.arrayContaining(["getDashboard", "listProjects", "webSearch", "fetchWebPage", "listRuntimeClusters"]))
    expect(selected.length).toBe(platformOperations.length)
  })

  it.each([
    ["读取 GitHub 仓库 README 和官方部署文档", "fetchWebPage"],
    ["查看 Pod 调度和镜像拉取失败事件", "listRuntimeEvents"],
    ["部署前看一下有哪些可用运行集群", "listRuntimeClusters"],
  ])("retrieves the expected operation for goal: %s", (query, expectedOperationId) => {
    const result = ToolCatalog.load(platformOperations).search(query)

    expect(result.loadedOperationIds).toContain(expectedOperationId)
    expect(result.matches.find(match => match.operationId === expectedOperationId)?.description).toContain("适用：")
  })
})
