import { readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, inject, it } from "vitest"
import { ToolCatalog, validateArguments } from "../src/tools/catalog.js"
import { ToolArgumentsInvalidError } from "../src/tools/argument-validator.js"
import { businessCardTools } from "../src/tools/business-card-tools.js"
import { renameConversationTool } from "../src/tools/conversation-title.js"
import { getToolDetailsTool } from "../src/tools/tool-details.js"
import { searchToolsTool } from "../src/tools/tool-search.js"
import { navigateToRouteTool } from "../src/tools/ui-route.js"

const platformCatalog = ToolCatalog.load(JSON.parse(readFileSync(
  inject("platformCatalogFixturePath"),
  "utf8",
)))

describe("real PlatformCatalog retrieval", () => {
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
    ["查看集群压力", "observeRuntimeClusterPressure"],
    ["查看通知渠道", "listNotificationChannels"],
    ["搜索互联网", "webSearch"],
    ["读取公开网页正文", "fetchWebPage"],
    ["create project", "createProject"],
    ["update project volume", "updateProjectVolume"],
    ["delete project volume", "deleteProjectVolume"],
    ["preview project volume deletion", "previewProjectVolumeDeletion"],
    ["list project volumes", "listProjectVolumes"],
    ["get project volume", "getProjectVolume"],
    ["查看删除影响", "previewProjectVolumeDeletion"],
    ["在应用市场搜索 Dify 模板", "listAppTemplates"],
    ["读取已选应用模板的完整安装参数", "getAppTemplate"],
    ["创建数据卷前选择存储类", "listProjectVolumeStorageClasses"],
  ])("ranks the intended real operation first for %s", (query, operationId) => {
    const result = platformCatalog.search({ query, pageSize: 20 })
    expect(result.items[0]?.operationId).toBe(operationId)
  })

  it("keeps preview/read/write boundaries and approval flags distinct", () => {
    const preview = platformCatalog.search({ query: "预览删除数据卷的影响", pageSize: 8 })
    const read = platformCatalog.search({ query: "查看数据卷详情", pageSize: 8 })
    const write = platformCatalog.search({ query: "扩容数据卷", pageSize: 8 })

    expect(preview.items[0]).toMatchObject({ operationId: "previewProjectVolumeDeletion", requiresApproval: true })
    expect(preview.items[0]?.operationId).not.toBe("deleteProjectVolume")
    expect(read.items[0]).toMatchObject({ operationId: "getProjectVolume", requiresApproval: false })
    expect(write.items[0]).toMatchObject({ operationId: "updateProjectVolume", requiresApproval: true })
  })

  it("indexes real parameter names and output semantics", () => {
    expect(platformCatalog.search({ query: "volumeId", pageSize: 8 }).items.map(item => item.operationId))
      .toContain("getProjectVolume")
    expect(platformCatalog.search({ query: "clusterId", pageSize: 8 }).items.some(item => item.operationId.includes("RuntimeCluster")))
      .toBe(true)
    expect(platformCatalog.search({ query: "projectId", pageSize: 8 }).items.some(item => item.operationId.includes("Project")))
      .toBe(true)
  })

  it("discovers every real operation exactly and paginates the complete directory", () => {
    const operations = platformCatalog.all()
    const browsed = Array.from({ length: Math.ceil(operations.length / 100) }, (_, index) =>
      platformCatalog.search({ page: index + 1, pageSize: 100 }).items.map(item => item.operationId)).flat()

    expect(operations).toHaveLength(209)
    expect(new Set(browsed)).toEqual(new Set(operations.map(operation => operation.operationId)))
    for (const operation of operations) {
      expect(platformCatalog.search({ query: operation.operationId, pageSize: 8 }).items[0]?.operationId)
        .toBe(operation.operationId)
    }
  })

  it("keeps every operation discoverable in the first eight results by its human-readable intent", () => {
    for (const operation of platformCatalog.all()) {
      const matches = platformCatalog.search({ query: operation.summary, pageSize: 8 }).items
        .map(item => item.operationId)
      expect(matches, `${operation.operationId}: ${operation.summary}`)
        .toContain(operation.operationId)
    }
  })

  it("accepts app-template filters and every route placeholder required by the HTTP transport", () => {
    const list = platformCatalog.get("listAppTemplates")
    const detail = platformCatalog.get("getAppTemplate")
    expect(validateArguments(list.inputSchema, { query: "Dify", category: "ai" }))
      .toEqual({ query: "Dify", category: "ai" })
    expect(validateArguments(detail.inputSchema, { templateId: "postgresql" }))
      .toEqual({ templateId: "postgresql" })

    for (const operation of platformCatalog.all()) {
      for (const placeholder of operation.path.matchAll(/\{([^}]+)\}/g)) {
        expect(operation.parameters, `${operation.operationId}:${placeholder[1]}`)
          .toContainEqual(expect.objectContaining({ in: "path", wireName: placeholder[1], required: true }))
        expect(operation.inputSchema.properties, `${operation.operationId}:${placeholder[1]}`)
          .toHaveProperty(placeholder[1]!)
      }
    }
  })

  it("rejects incomplete conditional volume requests before platform execution", () => {
    const create = platformCatalog.get("createProjectVolume")
    const incompleteBlank = {
      projectId: "prj_1",
      body: {
        displayName: "postgresql-data",
        clusterId: "clu_1",
        capacity: "10Gi",
        accessMode: "ReadWriteOnce",
        volumeMode: "Filesystem",
        source: { type: "blank" },
      },
    }
    expect(() => validateArguments(create.inputSchema, incompleteBlank))
      .toThrow("ai.tool_arguments_invalid")

    const completeBlank = {
      ...incompleteBlank,
      body: { ...incompleteBlank.body, storageClassName: "standard" },
    }
    expect(validateArguments(create.inputSchema, completeBlank)).toEqual(completeBlank)

    const existingClaim = {
      projectId: "prj_1",
      body: {
        displayName: "existing-data",
        clusterId: "clu_1",
        source: { type: "existingClaim", claimName: "postgresql-data", ownershipMode: "referenced" },
      },
    }
    expect(validateArguments(create.inputSchema, existingClaim)).toEqual(existingClaim)

    const update = platformCatalog.get("updateProjectVolume")
    expect(() => validateArguments(update.inputSchema, {
      projectId: "prj_1",
      volumeId: "pvol_1",
      revision: 1,
      body: {},
    })).toThrow("ai.tool_arguments_invalid")
  })

  it("rejects a non-canonical app-template stage before platform execution", () => {
    const install = platformCatalog.get("installAppTemplate")
    const input = {
      projectId: "prj_1",
      templateId: "redis",
      body: {
        applicationName: "Redis",
        applicationIdentifier: "redis",
        deploymentName: "Redis dev",
        stage: "default",
        clusterId: "clu_1",
      },
    }
    try {
      validateArguments(install.inputSchema, input)
      throw new Error("expected installAppTemplate arguments to be rejected")
    }
    catch (error) {
      expect(error).toBeInstanceOf(ToolArgumentsInvalidError)
      expect((error as ToolArgumentsInvalidError).issues).toContainEqual(expect.objectContaining({
        path: "/body/stage",
        code: "enum",
        allowedValues: ["dev", "test", "staging", "prod"],
      }))
    }
    expect(validateArguments(install.inputSchema, {
      ...input,
      body: { ...input.body, stage: "dev" },
    })).toEqual({ ...input, body: { ...input.body, stage: "dev" } })
  })

  it("loads both storage-class discovery and creation for a managed-volume goal", () => {
    const matches = platformCatalog.search({
      query: "创建空白数据卷并选择目标集群 StorageClass",
      pageSize: 8,
    }).items.map(item => item.operationId)
    expect(matches).toContain("listProjectVolumeStorageClasses")
    expect(matches).toContain("createProjectVolume")
  })

  it("covers every real operation through search, semantic details, and model-tool loading", () => {
    const operations = platformCatalog.all()
    for (const operation of operations) {
      const search = platformCatalog.search({ query: operation.operationId, pageSize: 8 })
      const details = platformCatalog.semanticDetails([operation.operationId])
      const modelTools = platformCatalog.modelTools([operation.operationId])

      expect(search.items[0]?.operationId, operation.operationId).toBe(operation.operationId)
      expect(details.missingOperationIds, operation.operationId).toEqual([])
      expect(details.items, operation.operationId).toHaveLength(1)
      expect(details.items[0], operation.operationId).toMatchObject({
        operationId: operation.operationId,
        requiresApproval: operation.requiresApproval,
        requiredScopes: operation.requiredScopes,
      })
      expect(modelTools, operation.operationId).toEqual([expect.objectContaining({
        operationId: operation.operationId,
        inputSchema: operation.inputSchema,
      })])
    }
  })

  it("is deterministic and returns no fabricated match", () => {
    const first = platformCatalog.search({ query: "查看项目空间详情", pageSize: 20 })
    const second = platformCatalog.search({ query: "查看项目空间详情", pageSize: 20 })
    expect(second).toEqual(first)
    expect(platformCatalog.search({ query: "zzqxwvplm" }))
      .toMatchObject({ items: [], total: 0, totalPages: 0 })
  })

  it("keeps every platform operation named by a Skill available in the real catalog", () => {
    const known = new Set([
      ...platformCatalog.all().map(operation => operation.operationId),
      searchToolsTool.operationId,
      getToolDetailsTool.operationId,
      navigateToRouteTool.operationId,
      renameConversationTool.operationId,
      ...businessCardTools.map(tool => tool.operationId),
    ])
    const operationLike = /^(?:get|list|create|update|delete|preview|retry|install|trigger|check|test|read|fetch|search|rotate|revoke|unbind|pin|unpin|rollback|cancel|cleanup|export|import|resolve|set|start|complete|authorize|reconfigure)[A-Z_]/
    const missing = skillMarkdownFiles(new URL("../skills", import.meta.url).pathname)
      .flatMap(file => [...readFileSync(file, "utf8").matchAll(/`([A-Za-z][A-Za-z0-9_]+)`/g)].map(match => match[1]!))
      .filter((operationId, index, values) => operationLike.test(operationId) && !known.has(operationId) && values.indexOf(operationId) === index)
    expect(missing).toEqual([])
  })
})

function skillMarkdownFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry =>
    entry.isDirectory()
      ? skillMarkdownFiles(join(directory, entry.name))
      : entry.name.endsWith(".md") ? [join(directory, entry.name)] : [])
}
