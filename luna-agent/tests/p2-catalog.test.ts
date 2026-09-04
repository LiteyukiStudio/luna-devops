import { describe, expect, it } from "vitest"
import { ToolCatalog, validateArguments, type ToolOperation } from "../src/tools/catalog.js"
import { getToolDetailsInput } from "../src/tools/tool-details.js"
import { searchToolsInput } from "../src/tools/tool-search.js"
import { testToolOperation } from "./support/tool-catalog.js"

const emptyInputSchema = {
  type: "object" as const,
  properties: {},
  required: [],
  additionalProperties: false as const,
}

function operation(input: Partial<ToolOperation> & Pick<ToolOperation, "operationId" | "name" | "summary">): ToolOperation {
  return {
    operationId: input.operationId,
    name: input.name,
    summary: input.summary,
    category: input.category ?? "volumes",
    tags: input.tags ?? ["project-volume", "storage"],
    aliases: input.aliases ?? { zh: [], en: [] },
    purpose: input.purpose ?? { zh: "", en: "" },
    avoidWhen: input.avoidWhen ?? { zh: "", en: "" },
    preconditions: input.preconditions ?? { zh: [], en: [] },
    successEvidence: input.successEvidence ?? { zh: "", en: "" },
    requiresApproval: input.requiresApproval ?? false,
    idempotent: input.idempotent ?? true,
    method: input.method ?? "GET",
    path: input.path ?? `/api/v1/projects/{projectId}/test/${input.operationId}`,
    requiredScopes: input.requiredScopes ?? ["volume:read"],
    inputSchema: input.inputSchema ?? emptyInputSchema,
    outputSchema: input.outputSchema ?? { type: "object" },
    sensitivePaths: input.sensitivePaths ?? [],
    parameters: input.parameters ?? [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
    requestBody: input.requestBody ?? false,
    requestRequired: input.requestRequired ?? false,
    requestType: input.requestType ?? "",
  }
}

const projectVolumeOperations = [
  operation({
    operationId: "listProjectVolumes",
    name: "列出项目数据卷",
    summary: "分页列出项目空间的数据卷与实时状态。",
    aliases: { zh: ["数据卷", "持久化存储", "项目空间存储"], en: ["ProjectVolume", "persistent storage"] },
    purpose: { zh: "分页列出项目空间数据卷。", en: "List project volumes." },
  }),
  operation({
    operationId: "getProjectVolume",
    name: "读取项目数据卷",
    summary: "读取单个 ProjectVolume 的规格、挂载和实时状态。",
    aliases: { zh: ["数据卷详情", "PVC 详情"], en: ["ProjectVolume details", "persistent volume claim"] },
    purpose: { zh: "查看数据卷详情。", en: "Get volume details." },
    successEvidence: { zh: "返回绑定、传输和实时观察。", en: "Returns bindings, transfers, and live observation." },
    inputSchema: {
      type: "object",
      properties: { projectId: { type: "string" }, volumeId: { type: "string" } },
      required: ["projectId", "volumeId"],
      additionalProperties: false,
    },
  }),
  operation({
    operationId: "updateProjectVolume",
    name: "更新项目数据卷",
    summary: "修改数据卷名称或扩容持久化存储。",
    aliases: { zh: ["扩容数据卷", "扩大 PVC"], en: ["expand ProjectVolume", "resize persistent storage"] },
    purpose: { zh: "扩容或重命名数据卷。", en: "Update or resize a volume." },
    requiresApproval: true,
    method: "PATCH",
    idempotent: false,
    requiredScopes: ["volume:write"],
  }),
  operation({
    operationId: "previewProjectVolumeDeletion",
    name: "预览项目数据卷删除影响",
    summary: "读取挂载、传输任务和底层存储删除影响。",
    aliases: { zh: ["删除数据卷预检", "检查 PVC 挂载"], en: ["preview volume deletion"] },
    purpose: { zh: "预览删除数据卷的影响。", en: "Preview volume deletion impact." },
    requiresApproval: true,
    method: "POST",
    requiredScopes: ["volume:delete"],
  }),
  operation({
    operationId: "deleteProjectVolume",
    name: "删除项目数据卷",
    summary: "删除托管卷或解除外部 PVC 引用。",
    aliases: { zh: ["删除 PVC", "移除持久化存储"], en: ["delete ProjectVolume"] },
    purpose: { zh: "删除数据卷。", en: "Delete a project volume." },
    requiresApproval: true,
    method: "DELETE",
    idempotent: false,
    requiredScopes: ["volume:delete"],
  }),
]

const broadOperations = [
  ...projectVolumeOperations,
  operation({ operationId: "createProject", name: "创建项目空间", summary: "创建新的项目空间。", category: "projects", tags: ["Projects"], aliases: { zh: ["创建项目空间", "新建项目"], en: ["create project", "create project space"] }, purpose: { zh: "创建项目空间。", en: "Create a project." }, method: "POST", idempotent: false, requiredScopes: ["project:write"] }),
  operation({ operationId: "getProject", name: "查看项目空间详情", summary: "按 projectId 读取项目空间。", category: "projects", tags: ["Projects"], aliases: { zh: ["查看项目空间详情"], en: ["get project", "project details"] }, purpose: { zh: "读取项目空间详情。", en: "Get project details." }, inputSchema: { type: "object", properties: { projectId: { type: "string", description: "项目空间标识" } }, required: ["projectId"], additionalProperties: false } }),
  operation({ operationId: "listUsers", name: "列出平台用户", summary: "分页列出平台用户。", category: "users", tags: ["Users"], aliases: { zh: ["列出平台用户", "用户列表"], en: ["list platform users"] }, purpose: { zh: "列出平台用户。", en: "List platform users." } }),
  operation({ operationId: "getBillingSummary", name: "查看平台账单概览", summary: "读取账单费用和用量概览。", category: "billing", tags: ["Billing"], aliases: { zh: ["查看平台账单概览", "费用概览"], en: ["billing summary"] }, purpose: { zh: "查看平台账单概览。", en: "Get billing summary." } }),
  operation({ operationId: "listNotificationChannels", name: "列出通知渠道", summary: "分页列出通知渠道。", category: "notifications", tags: ["Notifications"], aliases: { zh: ["查看通知渠道", "通知渠道列表"], en: ["list notification channels"] }, purpose: { zh: "列出通知渠道。", en: "List notification channels." } }),
  operation({ operationId: "webSearch", name: "搜索互联网", summary: "通过受控出口搜索公开网络。", category: "web", tags: ["Web"], aliases: { zh: ["搜索互联网", "网络搜索"], en: ["web search", "search the internet"] }, purpose: { zh: "搜索互联网。", en: "Search the public web." }, method: "POST", path: "/api/v1/ai-tools/web-search", inputSchema: { type: "object", properties: { query: { type: "string" } }, required: ["query"], additionalProperties: false } }),
  operation({ operationId: "fetchWebPage", name: "读取公开网页正文", summary: "读取公开 URL 的有界正文。", category: "web", tags: ["Web"], aliases: { zh: ["读取公开网页正文", "获取网页正文"], en: ["fetch web page", "read webpage"] }, purpose: { zh: "读取公开网页正文。", en: "Fetch a public webpage." }, method: "POST", path: "/api/v1/ai-tools/fetch-web-page", inputSchema: { type: "object", properties: { url: { type: "string" } }, required: ["url"], additionalProperties: false } }),
]

describe("ToolCatalog", () => {
  it.each([
    "name", "summary", "tags", "aliases", "purpose", "avoidWhen", "preconditions",
    "successEvidence", "requiresApproval", "inputSchema", "outputSchema", "requestBody", "requestRequired",
  ] as const)("rejects a catalog operation without authority-owned %s", (field) => {
    const invalid = structuredClone(testToolOperation("getProject"))
    Reflect.deleteProperty(invalid, field)

    expect(() => ToolCatalog.load([invalid])).toThrow()
  })

  it("normalizes only fields omitted by the Go catalog contract and freezes nested values", () => {
    const input = structuredClone(testToolOperation("getProject"))
    Reflect.deleteProperty(input, "sensitivePaths")
    Reflect.deleteProperty(input, "parameters")
    Reflect.deleteProperty(input, "requestType")

    const operation = ToolCatalog.load([input]).get("getProject")

    expect(operation).toMatchObject({ sensitivePaths: [], parameters: [], requestType: "" })
    expect(Object.isFrozen(operation)).toBe(true)
    expect(Object.isFrozen(operation.aliases.zh)).toBe(true)
    expect(Object.isFrozen(operation.inputSchema)).toBe(true)
  })

  it("browses the complete summary directory with an empty query and stable pagination", () => {
    const catalog = ToolCatalog.load(projectVolumeOperations)
    const first = catalog.search({ page: 1, pageSize: 2 })
    const second = catalog.search({ query: "", page: 2, pageSize: 2 })

    expect(first).toMatchObject({ query: "", page: 1, pageSize: 2, total: 5, totalPages: 3 })
    expect(first.items.map(item => item.operationId)).toEqual(["deleteProjectVolume", "getProjectVolume"])
    expect(second.items.map(item => item.operationId)).toEqual(["listProjectVolumes", "previewProjectVolumeDeletion"])
    expect(first.items[0]).not.toHaveProperty("inputSchema")
    expect(first.items[0]).not.toHaveProperty("outputSchema")
  })

  it.each([
    ["数据卷", "listProjectVolumes"],
    ["ProjectVolume", "listProjectVolumes"],
    ["持久化存储", "listProjectVolumes"],
    ["扩容 PVC", "updateProjectVolume"],
    ["previewProjectVolumeDeletion", "previewProjectVolumeDeletion"],
  ])("searches aliases, resources, actions, names and operationIds: %s", (query, expected) => {
    const result = ToolCatalog.load(projectVolumeOperations).search({ query, page: 1, pageSize: 100 })

    expect(result.items.map(item => item.operationId)).toContain(expected)
  })

  it.each([
    ["扩容数据卷", "updateProjectVolume"],
    ["查看数据卷详情", "getProjectVolume"],
    ["列出项目空间数据卷", "listProjectVolumes"],
    ["预览删除数据卷的影响", "previewProjectVolumeDeletion"],
    ["删除数据卷", "deleteProjectVolume"],
    ["创建项目空间", "createProject"],
    ["查看项目空间详情", "getProject"],
    ["列出平台用户", "listUsers"],
    ["查看平台账单概览", "getBillingSummary"],
    ["查看通知渠道", "listNotificationChannels"],
    ["搜索互联网", "webSearch"],
    ["读取公开网页正文", "fetchWebPage"],
    ["create project", "createProject"],
    ["update volume", "updateProjectVolume"],
    ["delete volume", "deleteProjectVolume"],
    ["preview volume deletion", "previewProjectVolumeDeletion"],
    ["list project volumes", "listProjectVolumes"],
    ["get project", "getProject"],
  ])("ranks the intended read/write operation first: %s", (query, expected) => {
    expect(ToolCatalog.load(broadOperations).search({ query, pageSize: 100 }).items[0]?.operationId).toBe(expected)
  })

  it("indexes parameter and output semantics without exposing schemas in summaries", () => {
    const catalog = ToolCatalog.load(broadOperations)
    expect(catalog.search({ query: "volumeId", pageSize: 8 }).items.map(item => item.operationId)).toContain("getProjectVolume")
    expect(catalog.search({ query: "绑定 传输 实时观察", pageSize: 8 }).items.map(item => item.operationId)).toContain("getProjectVolume")
    expect(catalog.search({ query: "quuxzy-no-such-frobnicator" })).toMatchObject({ items: [], total: 0, totalPages: 0 })
  })

  it("loads full details only for exact selected operationIds", () => {
    const catalog = ToolCatalog.load(projectVolumeOperations)
    const result = catalog.getDetails(["updateProjectVolume", "missingOperation", "updateProjectVolume"])

    expect(result.items).toHaveLength(1)
    expect(result.items[0]).toMatchObject({
      operationId: "updateProjectVolume",
      requiresApproval: true,
      requiredScopes: ["volume:write"],
      inputSchema: emptyInputSchema,
      outputSchema: { type: "object" },
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
    })
    expect(result.missingOperationIds).toEqual(["missingOperation"])
    expect(catalog.modelTools(["updateProjectVolume"]).map(item => item.operationId)).toEqual(["updateProjectVolume"])
    expect(catalog.modelTools([])).toEqual([])
    const semantic = catalog.semanticDetails(["updateProjectVolume"]).items[0]!
    expect(semantic).not.toHaveProperty("inputSchema")
    expect(semantic).not.toHaveProperty("outputSchema")
    expect(semantic).not.toHaveProperty("method")
    expect(semantic).not.toHaveProperty("path")
    expect(semantic).toMatchObject({ operationId: "updateProjectVolume", requiresApproval: true })
  })

  it("keeps the transport DTO small and independent from admission or workflow contracts", () => {
    const value = ToolCatalog.load([{
      ...projectVolumeOperations[0],
      allowed: true,
      risk: "low",
      approval: "never",
      contract: { predecessors: ["getProject"], followups: ["getProjectVolume"] },
    }]).get("listProjectVolumes")

    expect(value).not.toHaveProperty("allowed")
    expect(value).not.toHaveProperty("risk")
    expect(value).not.toHaveProperty("approval")
    expect(value).not.toHaveProperty("contract")
  })

  it("validates discovery inputs and exact detail batches", () => {
    expect(searchToolsInput.parse({})).toEqual({ query: "", page: 1, pageSize: 20 })
    expect(searchToolsInput.parse({ query: "数据卷", page: 2, pageSize: 100 })).toEqual({ query: "数据卷", page: 2, pageSize: 100 })
    expect(getToolDetailsInput.parse({ operationIds: ["listProjectVolumes"] })).toEqual({ operationIds: ["listProjectVolumes"] })
    expect(() => getToolDetailsInput.parse({ operationIds: [] })).toThrow()
    expect(() => getToolDetailsInput.parse({ operationIds: Array.from({ length: 9 }, (_, index) => `getTool${index}`) })).toThrow()
  })

  it("retains strict platform argument validation", () => {
    const schema = ToolCatalog.load(projectVolumeOperations).get("getProjectVolume").inputSchema

    expect(validateArguments(schema, { projectId: "prj_1", volumeId: "vol_1" }))
      .toEqual({ projectId: "prj_1", volumeId: "vol_1" })
    expect(() => validateArguments(schema, { projectId: "prj_1" })).toThrow("ai.tool_arguments_invalid")
  })
})
