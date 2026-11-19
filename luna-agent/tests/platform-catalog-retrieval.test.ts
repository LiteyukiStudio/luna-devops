import { listDocsTool, readDocTool, searchDocsTool } from "../src/tools/docs-tools.js"
import { readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, inject, it } from "vitest"
import { ToolCatalog, validateArguments } from "../src/tools/catalog.js"
import { ToolArgumentsInvalidError } from "../src/tools/argument-validator.js"
import { businessCardTools } from "../src/tools/business-card-tools.js"
import { renameConversationTool } from "../src/tools/conversation-title.js"
import { getToolDetailsTool } from "../src/tools/tool-details.js"
import { loadSkillTool } from "../src/tools/skill-loader.js"
import { searchToolsTool } from "../src/tools/tool-search.js"
import { navigateToRouteTool } from "../src/tools/ui-route.js"
import { waitForTaskTool } from "../src/tools/wait-for-task.js"

const adminCatalog = ToolCatalog.load(JSON.parse(readFileSync(
  inject("platformCatalogFixturePath"),
  "utf8",
)))
const tenantCatalog = adminCatalog.withoutAdminOperations()

describe("real 166-tool AI Domain Facade Catalog retrieval", () => {
  it("enforces exact 166 admin / 133 tenant dual-snapshot role isolation", () => {
    expect(adminCatalog.all()).toHaveLength(166)
    expect(tenantCatalog.all()).toHaveLength(133)
    expect(tenantCatalog.all().some(op => op.adminOnly === true || op.operationId.startsWith("admin_"))).toBe(false)
    expect(adminCatalog.all().filter(op => op.adminOnly === true)).toHaveLength(33)
    expect(tenantCatalog.digest).not.toBe(adminCatalog.digest)
  })

  it.each([
    ["扩容数据卷", "runtime_update_volume"],
    ["查看数据卷详情", "runtime_get_volume"],
    ["删除数据卷", "runtime_delete_volume"],
    ["创建项目空间", "workspace_create_project"],
    ["查看项目空间详情", "workspace_get_project"],
    ["搜索互联网", "ops_web_search"],
    ["读取公开网页正文", "ops_web_fetch"],
    ["查看部署诊断快照", "runtime_get_target_diagnostics"],
    ["扩缩容部署目标副本数", "delivery_scale_target"],
    ["回滚生产环境版本", "delivery_rollback_target"],
    ["在应用市场搜索模板", "delivery_list_templates"],
    ["查看应用模板安装参数", "delivery_get_template"],
    ["清理数据保留策略", "admin_execute_data_retention_cleanup"],
  ])("ranks the intended real facade operation in top results for %s", (query, operationId) => {
    const result = adminCatalog.search({ query, pageSize: 5 })
    expect(result.items.map(item => item.operationId)).toContain(operationId)
  })

  it("keeps read/write boundaries and approval flags distinct", () => {
    expect(tenantCatalog.get("runtime_get_volume")).toMatchObject({
      operationId: "runtime_get_volume",
      approvalTier: "L0",
      requiresApproval: false,
    })
    expect(tenantCatalog.get("runtime_update_volume")).toMatchObject({
      operationId: "runtime_update_volume",
      approvalTier: "L2",
      requiresApproval: true,
    })
    expect(tenantCatalog.get("workspace_create_project")).toMatchObject({
      operationId: "workspace_create_project",
      approvalTier: "L1",
      requiresApproval: true,
    })
  })

  it("indexes flattened parameter names and domain semantics", () => {
    expect(tenantCatalog.search({ query: "volume", pageSize: 8 }).items.map(item => item.operationId))
      .toContain("runtime_get_volume")
    expect(tenantCatalog.search({ query: "cluster", pageSize: 8 }).items.some(item => item.operationId.startsWith("runtime_")))
      .toBe(true)
    expect(tenantCatalog.search({ query: "project", pageSize: 8 }).items.some(item => item.operationId.startsWith("workspace_")))
      .toBe(true)
  })

  it("discovers every real operation exactly and paginates the complete directory", () => {
    for (const catalog of [tenantCatalog, adminCatalog]) {
      const operations = catalog.discoverableOperations()
      const browsed = Array.from({ length: Math.ceil(operations.length / 100) }, (_, index) =>
        catalog.search({ page: index + 1, pageSize: 100 }).items.map(item => item.operationId)).flat()

      expect(new Set(browsed)).toEqual(new Set(operations.map(operation => operation.operationId)))
      for (const operation of operations) {
        expect(catalog.search({ query: operation.operationId, pageSize: 8 }).items[0]?.operationId)
          .toBe(operation.operationId)
      }
    }
  })

  it("keeps every operation discoverable in the first eight results by its human-readable summary", () => {
    for (const operation of adminCatalog.discoverableOperations()) {
      const matches = adminCatalog.search({ query: operation.summary, pageSize: 8 }).items
        .map(item => item.operationId)
      expect(matches, `${operation.operationId}: ${operation.summary}`)
        .toContain(operation.operationId)
    }
  })

  it("validates flattened top-level input schemas and sensitivePaths without body wrapper", () => {
    for (const operation of adminCatalog.all()) {
      expect(operation.inputSchema).toMatchObject({
        type: "object",
        additionalProperties: false,
      })
      expect(operation.inputSchema.properties).not.toHaveProperty("body")
    }

    const install = tenantCatalog.get("delivery_install_template")
    try {
      validateArguments(install.inputSchema, {
        project: "prj_1",
        template: "redis",
        applicationName: "Redis",
        applicationIdentifier: "redis",
        stage: "default",
      })
      throw new Error("expected delivery_install_template stage=default to be rejected")
    }
    catch (error) {
      expect(error).toBeInstanceOf(ToolArgumentsInvalidError)
      expect((error as ToolArgumentsInvalidError).issues).toContainEqual(expect.objectContaining({
        path: "/stage",
        code: "enum",
        allowedValues: ["dev", "test", "staging", "prod"],
      }))
    }

    const validInstall = {
      project: "prj_1",
      template: "redis",
      applicationName: "Redis",
      applicationIdentifier: "redis",
      stage: "dev",
      secretValues: { password: "redis-test-password" },
    }
    expect(validateArguments(install.inputSchema, validInstall)).toEqual(validInstall)
    expect(install.sensitivePaths).toContain("secretValues.*")
  })

  it("covers every real operation through search, semantic details, and model-tool loading", () => {
    const operations = adminCatalog.discoverableOperations()
    for (const operation of operations) {
      const search = adminCatalog.search({ query: operation.operationId, pageSize: 8 })
      const details = adminCatalog.semanticDetails([operation.operationId])
      const modelTools = adminCatalog.modelTools([operation.operationId])

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

  it("exposes writes only when they declare same-snapshot authoritative L0 verification", () => {
    for (const operation of adminCatalog.all()) {
      const effect = operation.effect ?? "read"
      if (effect !== "write" && effect !== "async") continue
      expect(Boolean(operation.verification?.length), operation.operationId).toBe(true)
      for (const rule of operation.verification ?? []) {
        const verifyOp = adminCatalog.get(rule.operationId)
        expect(verifyOp.approvalTier, `${operation.operationId} -> ${rule.operationId}`).toBe("L0")
        if (!operation.adminOnly) {
          expect(verifyOp.adminOnly ?? false, `tenant tool ${operation.operationId} references adminOnly ${rule.operationId}`).toBe(false)
        }
      }
    }
  })

  it("is deterministic and returns no fabricated match", () => {
    const first = tenantCatalog.search({ query: "查看项目空间详情", pageSize: 20 })
    const second = tenantCatalog.search({ query: "查看项目空间详情", pageSize: 20 })
    expect(second).toEqual(first)
    expect(tenantCatalog.search({ query: "zzqxwvplm" }))
      .toMatchObject({ items: [], total: 0, totalPages: 0 })
  })

  it("keeps every platform facade operation named by a Skill available in the real catalog", () => {
    const known = new Set([
      ...adminCatalog.all().map(operation => operation.operationId),
      searchToolsTool.operationId,
      getToolDetailsTool.operationId,
      loadSkillTool.operationId,
      navigateToRouteTool.operationId,
      renameConversationTool.operationId,
      waitForTaskTool.operationId,
      ...businessCardTools.map(tool => tool.operationId),
      ...[searchDocsTool, listDocsTool, readDocTool].map(tool => tool.operationId),
    ])
    const facadeOperationPattern = /^(?:delivery|build|runtime|network|workspace|ops|admin)_[a-z0-9]+_[a-z0-9_]+$/
    const missing = skillMarkdownFiles(new URL("../skills", import.meta.url).pathname)
      .flatMap(file => [...readFileSync(file, "utf8").matchAll(/`([a-z][a-z0-9_]+)`/g)].map(match => match[1]!))
      .filter((operationId, index, values) =>
        facadeOperationPattern.test(operationId)
        && !operationId.endsWith("_*")
        && !known.has(operationId)
        && values.indexOf(operationId) === index,
      )
    expect(missing).toEqual([])
  })

  it("retrieves runtime_get_target_diagnostics and workspace_get_project as L0 auto read tools in Chinese diagnostic queries", () => {
    const projectDiag = tenantCatalog.get("workspace_get_project")
    const targetDiag = tenantCatalog.get("runtime_get_target_diagnostics")
    expect(projectDiag.approvalTier).toBe("L0")
    expect(projectDiag.requiresApproval).toBe(false)
    expect(targetDiag.approvalTier).toBe("L0")
    expect(targetDiag.requiresApproval).toBe(false)

    const targetHits = tenantCatalog.search({ query: "查看部署诊断快照排查服务为什么挂了", pageSize: 5 })
    expect(targetHits.items.map(item => item.operationId)).toContain("runtime_get_target_diagnostics")
  })
})

function skillMarkdownFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry =>
    entry.isDirectory()
      ? skillMarkdownFiles(join(directory, entry.name))
      : entry.name.endsWith(".md") ? [join(directory, entry.name)] : [])
}
