import { describe, expect, it } from "vitest"
import { ToolCatalog, validateArguments, type ToolOperation } from "../src/tools/catalog.js"
import { getToolDetailsInput } from "../src/tools/tool-details.js"
import { searchToolsInput } from "../src/tools/tool-search.js"

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
  }),
  operation({
    operationId: "getProjectVolume",
    name: "读取项目数据卷",
    summary: "读取单个 ProjectVolume 的规格、挂载和实时状态。",
    aliases: { zh: ["数据卷详情", "PVC 详情"], en: ["ProjectVolume details", "persistent volume claim"] },
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
    requiresApproval: true,
    method: "POST",
    requiredScopes: ["volume:delete"],
  }),
  operation({
    operationId: "deleteProjectVolume",
    name: "删除项目数据卷",
    summary: "删除托管卷或解除外部 PVC 引用。",
    aliases: { zh: ["删除 PVC", "移除持久化存储"], en: ["delete ProjectVolume"] },
    requiresApproval: true,
    method: "DELETE",
    idempotent: false,
    requiredScopes: ["volume:delete"],
  }),
]

describe("ToolCatalog", () => {
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
