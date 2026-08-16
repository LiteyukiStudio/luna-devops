import { describe, expect, it } from "vitest"
import { ToolCatalog, validateArguments } from "../src/tools/catalog.js"
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

  it("exposes runtime secret updates with a Direct Tool Action boundary", () => {
    const catalog = ToolCatalog.load(platformOperations)
    const operation = catalog.get("updateDeploymentTargetRuntimeSecrets")
    const body = operation.inputSchema.properties.body as Record<string, unknown>
    const values = (body.properties as Record<string, unknown>).values

    expect(operation).toMatchObject({
      risk: "sensitive",
      approval: "always",
      stepUpPurpose: "secret_update",
      sensitivePaths: ["body.values"],
    })
    expect(values).toMatchObject({ writeOnly: true, "x-luna-sensitive": true })
    expect(catalog.modelTools().map(tool => tool.operationId)).not.toContain("generateSecret")
    expect(catalog.modelTools().find(tool => tool.operationId === operation.operationId)?.description)
      .toContain("安全表单")
  })

  it("offers project-scoped tools independently of page context and requires an explicit target", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.modelTools().map(tool => tool.operationId)).toContain("listPlatformEvents")
    expect(catalog.get("listApplications").inputSchema.required).toEqual(["projectId"])
    const listProjects = catalog.get("listProjects")
    expect(listProjects.inputSchema.properties).not.toHaveProperty("projectId")
    expect(listProjects.inputSchema.properties.scope).toMatchObject({ enum: ["related", "all"], default: "related" })
    expect(validateArguments(listProjects.inputSchema, {})).toEqual({})
    expect(validateArguments(listProjects.inputSchema, { scope: "related", page: 1, pageSize: 100 })).toEqual({ scope: "related", page: 1, pageSize: 100 })
    expect(() => validateArguments(listProjects.inputSchema, { scope: "mine" })).toThrow("ai.tool_arguments_invalid")
    expect(() => validateArguments(listProjects.inputSchema, { page: 0 })).toThrow("ai.tool_arguments_invalid")
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

  it("requires a usable push credential before triggering a source build", () => {
    const catalog = ToolCatalog.load([
      {
        operationId: "listRegistryCredentials",
        method: "GET",
        path: "/api/v1/registries/{registryId}/credentials",
        category: "registries",
        risk: "read",
        requiredScopes: ["registry:read"],
        approval: "never",
        idempotent: true,
        timeoutMs: 30_000,
        inputSchema: {
          type: "object",
          properties: { registryId: { type: "string" }, projectId: { type: "string" } },
          required: ["registryId"],
          additionalProperties: false,
        },
      },
      {
        operationId: "triggerBuildRun",
        method: "POST",
        path: "/api/v1/projects/{projectId}/build-runs/trigger",
        category: "builds",
        risk: "write",
        requiredScopes: ["build:write"],
        approval: "never",
        idempotent: false,
        timeoutMs: 30_000,
        inputSchema: {
          type: "object",
          properties: {
            projectId: { type: "string" },
            body: { type: "object" },
          },
          required: ["projectId", "body"],
          additionalProperties: false,
        },
      },
      {
        operationId: "retryBuildRun",
        method: "POST",
        path: "/api/v1/projects/{projectId}/build-runs/{runId}/retry",
        category: "builds",
        risk: "write",
        requiredScopes: ["build:write"],
        approval: "never",
        idempotent: false,
        timeoutMs: 30_000,
        inputSchema: {
          type: "object",
          properties: {
            projectId: { type: "string" },
            runId: { type: "string" },
          },
          required: ["projectId", "runId"],
          additionalProperties: false,
        },
      },
    ])

    const credentials = catalog.modelTools().find(tool => tool.operationId === "listRegistryCredentials")?.description ?? ""
    const trigger = catalog.modelTools().find(tool => tool.operationId === "triggerBuildRun")?.description ?? ""
    const retry = catalog.modelTools().find(tool => tool.operationId === "retryBuildRun")?.description ?? ""

    expect(credentials).toContain("usage 为 push 或 push-pull")
    expect(credentials).toContain("构建的 projectId 与目标 registryId")
    expect(credentials).toContain("禁止跨项目复用查询结果")
    expect(trigger).toContain("相同 projectId 和 targetRegistryId")
    expect(trigger).toContain("build.registry_push_credential_required")
    expect(trigger).toContain("停止修改分支、Dockerfile、构建上下文或镜像引用")
    expect(retry).toContain("retryBuildRun.projectId")
    expect(retry).toContain("停止重复调用 retryBuildRun")
    expect(retry).toContain("不要改用 triggerBuildRun")
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

  it.each([
    ["列出这个项目空间里当前可挂载的数据卷", "listProjectVolumes"],
    ["把项目数据卷扩容到 100Gi", "updateProjectVolume"],
    ["删除这个 PVC 前先确认有哪些挂载", "previewProjectVolumeDeletion"],
    ["把数据库卷导出成备份归档", "createVolumeExport"],
    ["查看数据卷导入任务为什么失败", "getVolumeTransfer"],
  ])("retrieves the project-volume operation within the first eight results: %s", (query, expectedOperationId) => {
    const operationIds = [
      "listProjectVolumes", "getProjectVolume", "createProjectVolume", "updateProjectVolume",
      "previewProjectVolumeDeletion", "deleteProjectVolume", "createVolumeExport",
      "listVolumeTransfers", "getVolumeTransfer", "retryVolumeTransfer", "cancelVolumeTransfer",
      "createDeploymentTarget", "listRuntimeClusters",
    ]
    const operations = operationIds.map(operationId => ({
      operationId,
      method: /^(list|get)/.test(operationId) || operationId.startsWith("preview") ? "GET" as const : "POST" as const,
      path: `/api/v1/test/${operationId}`,
      category: operationId.includes("Transfer") ? "Volume Transfers" : operationId.includes("Volume") ? "Project Volumes" : "Deployments",
      description: `调用 Luna DevOps 的 ${operationId} 平台操作。`,
      risk: operationId.startsWith("delete") ? "destructive" as const : "read" as const,
      requiredScopes: ["volume:read"],
      approval: operationId.startsWith("delete") ? "always" as const : "never" as const,
      idempotent: true,
      timeoutMs: 30_000,
      inputSchema: { type: "object" as const, properties: {}, required: [], additionalProperties: false as const },
    }))

    const result = ToolCatalog.load(operations).search(query, {}, 8)
    expect(result.loadedOperationIds).toContain(expectedOperationId)
    expect(result.matches.find(match => match.operationId === expectedOperationId)?.description).toContain("适用：")
  })

  it("tells the model to use Web or CLI for local archive upload", () => {
    const importOperation = {
      operationId: "createVolumeImport",
      method: "POST" as const,
      path: "/api/v1/test/createVolumeImport",
      category: "Volume Transfers",
      risk: "sensitive" as const,
      requiredScopes: ["volume:import"],
      approval: "always" as const,
      idempotent: false,
      timeoutMs: 30_000,
      inputSchema: { type: "object" as const, properties: {}, required: [], additionalProperties: false as const },
    }
    const description = ToolCatalog.load([importOperation]).modelTools()[0]?.description ?? ""
    expect(description).toContain("Web 或 Luna CLI")
    expect(description).toContain("不能把文件内容编码进参数")
  })
})
