import { describe, expect, it } from "vitest"
import { ToolCatalog, type ToolOperation } from "../src/tools/catalog.js"
import { UnicodeLexicalTokenizer } from "../src/tools/retrieval/tokenizer.js"

function operation(index: number): ToolOperation {
  return {
    operationId: `inspectProjectVolume${index}`,
    name: `检查项目数据卷 ${index}`,
    summary: `读取持久化存储 ${index} 的当前状态。`,
    category: "volumes",
    tags: ["storage", `volume-${index}`],
    aliases: { zh: [`数据卷 ${index}`], en: [`ProjectVolume ${index}`] },
    purpose: { zh: "", en: "" },
    avoidWhen: { zh: "", en: "" },
    preconditions: { zh: [], en: [] },
    successEvidence: { zh: "", en: "" },
    requiresApproval: false,
    idempotent: true,
    method: "GET",
    path: `/api/v1/projects/{projectId}/volumes/${index}`,
    requiredScopes: ["volume:read"],
    inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    outputSchema: { type: "object" },
    sensitivePaths: [],
    parameters: [],
    requestBody: false,
    requestRequired: false,
    requestType: "",
  }
}

describe("simple BM25 tool retrieval", () => {
  it("tokenizes Chinese, camelCase and stable identifiers", () => {
    const tokens = new UnicodeLexicalTokenizer().tokenize("读取 ProjectVolume，调用 inspectProjectVolume")

    expect(tokens.some(token => token === "读取" || token === "读")).toBe(true)
    expect(tokens).toContain("projectvolume")
    expect(tokens).toContain("project")
    expect(tokens).toContain("volume")
    expect(tokens).toContain("inspectprojectvolume")
  })

  it("does not impose a fixed Top 8 existence boundary", () => {
    const operations = Array.from({ length: 24 }, (_, index) => operation(index))
    const result = ToolCatalog.load(operations).search({ query: "项目数据卷 持久化存储", page: 1, pageSize: 100 })

    expect(result.total).toBe(24)
    expect(result.items).toHaveLength(24)
  })

  it("ranks an exact operationId match first deterministically", () => {
    const catalog = ToolCatalog.load(Array.from({ length: 12 }, (_, index) => operation(index)))
    const first = catalog.search({ query: "inspectProjectVolume7", pageSize: 100 })
    const second = catalog.search({ query: "inspectProjectVolume7", pageSize: 100 })

    expect(first.items[0]?.operationId).toBe("inspectProjectVolume7")
    expect(second.items).toEqual(first.items)
  })
})
