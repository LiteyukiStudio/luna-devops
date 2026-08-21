import { readFileSync } from "node:fs"
import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"

const platformCatalog = ToolCatalog.load(JSON.parse(readFileSync(
  new URL("./fixtures/platform-catalog.json", import.meta.url),
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

    expect(operations).toHaveLength(207)
    expect(new Set(browsed)).toEqual(new Set(operations.map(operation => operation.operationId)))
    for (const operation of operations) {
      expect(platformCatalog.search({ query: operation.operationId, pageSize: 8 }).items[0]?.operationId)
        .toBe(operation.operationId)
    }
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
})
