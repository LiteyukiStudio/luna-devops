import { describe, expect, it } from "vitest"
import { ToolCatalog, validateArguments } from "../src/tools/catalog.js"
import type { AgentToolAction, AgentToolContract } from "../src/tools/contracts.js"
import { platformOperations } from "../src/tools/generated/platform.js"

const platformIntents: Record<string, string[]> = {
  getDashboard: ["读取平台概览"],
  listProjects: ["列出项目空间"],
  listAppTemplates: ["搜索应用市场模板"],
  listPlatformEvents: ["列出平台事件"],
  listApplications: ["列出项目空间内应用"],
  webSearch: ["搜索公开互联网和官方资料"],
  fetchWebPage: ["读取 GitHub README 和官方部署文档"],
  updateDeploymentTargetRuntimeSecrets: ["安全更新部署目标运行时密钥"],
  updateProjectRuntimeConfigSetRuntimeSecrets: ["安全更新运行时配置集密钥"],
  listGatewayRoutes: ["列出已有公网访问地址和网关路由", "list public gateway routes and URLs"],
  createGatewayRoute: ["创建公网访问地址和网关路由", "expose a service with a public URL"],
  listRuntimeEvents: ["查看 Pod 调度和镜像拉取失败事件"],
  listRuntimeClusters: ["列出部署可用运行集群"],
}

const contractedPlatformOperations = platformOperations.map(item => platformIntents[item.operationId]
  ? {
      ...item,
      contract: testContract(item.operationId, platformIntents[item.operationId]!, {
        idempotent: item.idempotent,
        approval: item.approval === "always" ? "always" : "never",
        ...("stepUpPurpose" in item && item.stepUpPurpose ? { mfaPurpose: item.stepUpPurpose } : {}),
      }),
    }
  : item)

function testContract(
  operationId: string,
  intents: string[],
  transport: { idempotent?: boolean, approval?: "never" | "always", mfaPurpose?: string } = {},
): AgentToolContract {
  const action = actionFor(operationId)
  const writes = ["create", "update", "delete", "execute"].includes(action)
  return {
    allowed: true,
    resourceTypes: [operationId],
    action,
    sideEffect: action === "delete" ? "destructive" : writes ? "platform-write" : "none",
    idempotent: transport.idempotent ?? !writes,
    replaySafe: !writes,
    risk: action === "delete" ? "high" : writes ? "medium" : "low",
    approval: transport.approval ?? (action === "delete" ? "always" : "never"),
    ...(transport.mfaPurpose ? { mfaPurpose: transport.mfaPurpose } : {}),
    intents,
    useWhen: intents,
    avoidWhen: writes ? ["用户只要求读取或比较候选时"] : [],
    prerequisites: writes ? ["目标资源和必填参数已确定"] : [],
    parameterSummary: [],
    successEvidence: ["响应返回稳定结果"],
    commonErrorCodes: [],
    predecessors: [],
    followups: [],
    verification: { mode: "response", successCodes: [200] },
  }
}

function actionFor(operationId: string): AgentToolAction {
  if (operationId === "webSearch") return "discover"
  if (operationId.startsWith("list") || operationId.startsWith("search")) return "discover"
  if (operationId.startsWith("get") || operationId.startsWith("fetch")) return "read"
  if (operationId.startsWith("preview") || operationId.startsWith("check") || operationId.startsWith("test")) return "verify"
  if (operationId.startsWith("create") || operationId.startsWith("install")) return "create"
  if (operationId.startsWith("update")) return "update"
  if (operationId.startsWith("delete")) return "delete"
  return "execute"
}

function volumeIntent(operationId: string): string {
  const intents: Record<string, string> = {
    listProjectVolumes: "列出项目空间可挂载数据卷",
    getProjectVolume: "读取单个项目数据卷详情",
    createProjectVolume: "创建项目数据卷",
    updateProjectVolume: "扩容或修改项目数据卷",
    previewProjectVolumeDeletion: "删除 PVC 前检查挂载影响",
    deleteProjectVolume: "确认后删除项目数据卷",
    createVolumeExport: "把数据卷导出为备份归档",
    listVolumeTransfers: "列出数据卷导入导出任务",
    getVolumeTransfer: "查看数据卷导入导出失败详情",
    retryVolumeTransfer: "重试数据卷传输",
    cancelVolumeTransfer: "取消数据卷传输",
    createDeploymentTarget: "创建部署配置并挂载数据卷",
    listRuntimeClusters: "列出运行集群",
  }
  return intents[operationId] ?? operationId
}

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
    const catalog = ToolCatalog.load(contractedPlatformOperations)
    const operation = catalog.get("updateDeploymentTargetRuntimeSecrets")
    const body = operation.inputSchema.properties.body as Record<string, unknown>
    const bodyProperties = body.properties as Record<string, unknown>
    const items = bodyProperties.items as { items: { properties: Record<string, unknown> } }
    const value = items.items.properties.value

    expect(operation).toMatchObject({
      risk: "sensitive",
      approval: "always",
      stepUpPurpose: "secret_update",
      sensitivePaths: ["body.items.*.value"],
    })
    expect(value).toMatchObject({ writeOnly: true, "x-luna-sensitive": true })
    const tools = catalog.modelTools({}, "安全更新部署目标运行时密钥")
    expect(tools.map(tool => tool.operationId)).not.toContain("generateSecret")
    expect(tools.find(tool => tool.operationId === operation.operationId)?.description)
      .toContain("安全表单")
  })

  it("keeps project schemas available without exposing unrelated tools for an empty goal", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.modelTools().map(tool => tool.operationId)).not.toContain("listPlatformEvents")
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
    const catalog = ToolCatalog.load(contractedPlatformOperations)

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
    expect(catalog.modelTools({}, "读取 GitHub README").find(tool => tool.operationId === "fetchWebPage")?.description)
      .toContain("不可信外部数据")
  })

  it("explicitly admits every executable handwritten discovery tool", () => {
    const catalog = ToolCatalog.load(platformOperations)
    const admitted = catalog.allowedModelTools().map(tool => tool.operationId)

    expect(admitted).toEqual(expect.arrayContaining(["webSearch", "fetchWebPage", "getAppTemplate"]))
    expect(catalog.get("webSearch")).toMatchObject({ requiredScopes: ["web:read"], contract: { allowed: true } })
    expect(catalog.get("fetchWebPage")).toMatchObject({ requiredScopes: ["web:read"], contract: { allowed: true } })
    expect(catalog.get("getAppTemplate")).toMatchObject({ requiredScopes: ["application:read"], contract: { allowed: true } })
  })

  it("loads the relevant delivery stage instead of injecting an entire workflow", () => {
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
      contract: testContract(operationId, [
        operationId === "getProject" ? "读取项目空间"
          : operationId === "createApplication" ? "创建应用"
            : operationId === "createDeploymentTarget" ? "创建部署配置"
              : operationId === "triggerBuildRun" ? "从代码仓库构建镜像"
                : operationId === "createRelease" ? "发布镜像"
                  : operationId === "createGatewayRoute" ? "配置网关公网入口"
                    : "读取发布运行日志",
      ], { idempotent: true }),
    }))
    const selected = ToolCatalog.load(dynamicOperations)
      .modelTools({}, "从代码仓库构建、发布并部署应用，然后配置网关")
      .map(tool => tool.operationId)

    expect(selected.length).toBeLessThanOrEqual(operationIds.length)
    expect(selected).toContain("triggerBuildRun")
    expect(selected).toContain("createRelease")
    expect(selected).toContain("createGatewayRoute")
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
        contract: testContract("listRegistryCredentials", ["查询源码构建镜像推送凭据"]),
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
        contract: {
          ...testContract("triggerBuildRun", ["从源码触发构建并推送镜像"]),
          avoidWhen: ["出现推送凭据缺失时，停止修改分支、Dockerfile、构建上下文或镜像引用"],
          commonErrorCodes: ["build.registry_push_credential_required"],
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
        contract: {
          ...testContract("retryBuildRun", ["重试失败的源码构建"]),
          prerequisites: ["使用 retryBuildRun.projectId 检查原构建的镜像站凭据"],
          avoidWhen: ["凭据缺失时停止重复调用 retryBuildRun，不要改用 triggerBuildRun"],
        },
      },
    ])

    const modelTools = catalog.modelTools({}, "源码构建前检查镜像推送凭据，失败后重试")
    const credentials = modelTools.find(tool => tool.operationId === "listRegistryCredentials")?.description ?? ""
    const trigger = modelTools.find(tool => tool.operationId === "triggerBuildRun")?.description ?? ""
    const retry = modelTools.find(tool => tool.operationId === "retryBuildRun")?.description ?? ""

    expect(credentials).toContain("usage 为 push 或 push-pull")
    expect(credentials).toContain("构建的 projectId 与目标 registryId")
    expect(credentials).toContain("不得复用另一个项目空间的查询结果")
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
    const selected = ToolCatalog.load(contractedPlatformOperations).modelTools({}, input).map(tool => tool.operationId)

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
      contract: testContract(operationId, [operationId.startsWith("list") ? "列出已有公网访问入口" : "创建公网访问入口"], { idempotent: true }),
    }))

    expect(ToolCatalog.load(gatewayOperations).modelTools({}, "创建公网访问地址").map(tool => tool.operationId))
      .toEqual(expect.arrayContaining(["listGatewayRoutes", "createGatewayRoute"]))
  })

  it("only exposes an allowed focused subset and keeps the platform cap", () => {
    const catalog = ToolCatalog.load(contractedPlatformOperations)
    const selected = catalog.modelTools({}, "查看 Pod 调度失败事件")

    expect(selected.map(tool => tool.operationId)).toContain("listRuntimeEvents")
    expect(selected.map(tool => tool.operationId)).not.toContain("generateSecret")
    expect(selected.length).toBeLessThanOrEqual(12)
    expect(selected.length).toBeLessThan(platformOperations.length)
  })

  it.each([
    ["读取 GitHub 仓库 README 和官方部署文档", "fetchWebPage"],
    ["查看 Pod 调度和镜像拉取失败事件", "listRuntimeEvents"],
    ["部署前看一下有哪些可用运行集群", "listRuntimeClusters"],
  ])("retrieves the expected operation for goal: %s", (query, expectedOperationId) => {
    const result = ToolCatalog.load(contractedPlatformOperations).search(query)

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
      contract: testContract(operationId, [volumeIntent(operationId)], { idempotent: true }),
    }))

    const result = ToolCatalog.load(operations).search(query, {}, 8)
    expect(result.loadedOperationIds).toContain(expectedOperationId)
    expect(result.matches.find(match => match.operationId === expectedOperationId)?.description).toContain("适用：")
  })

  it("lists a compact deterministic directory and loads exact tool details", () => {
    const catalog = ToolCatalog.load(contractedPlatformOperations)
    const directory = catalog.browse({ mode: "list", page: 1, pageSize: 200 })

    expect(directory.mode).toBe("list")
    expect(directory.entries.map(item => item.operationId)).toEqual(
      [...directory.entries.map(item => item.operationId)].sort(),
    )
    expect(directory.entries.find(item => item.operationId === "getDashboard")).toMatchObject({
      category: "dashboard",
      action: "read",
      risk: "low",
    })
    expect(directory.loadedOperationIds).toEqual([])

    const details = catalog.browse({
      mode: "details",
      operationIds: ["getDashboard", "missingOperation", "getDashboard"],
    })
    expect(details.loadedOperationIds).toEqual(["getDashboard"])
    expect(details.missingOperationIds).toEqual(["missingOperation"])
    expect(details.details[0]?.description).toContain("适用：")
  })

  it("keeps the unsupported local archive upload operation out of the model directory", () => {
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
      contract: { ...testContract("createVolumeImport", ["导入本地数据卷归档"]), allowed: false },
    }
    const catalog = ToolCatalog.load([importOperation])
    expect(catalog.get("createVolumeImport").operationId).toBe("createVolumeImport")
    expect(catalog.modelTools({}, "导入本地数据卷归档")).toEqual([])
  })
})
