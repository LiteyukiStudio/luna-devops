import { describe, expect, it } from "vitest"
import { ToolCatalog, type ToolOperation } from "../src/tools/catalog.js"
import type { AgentToolContract } from "../src/tools/contracts.js"
import { buildToolRetrievalDocuments } from "../src/tools/retrieval/documents.js"
import type { EmbeddingProvider } from "../src/tools/retrieval/embeddings.js"
import type { ToolReranker } from "../src/tools/retrieval/pipeline.js"
import { UnicodeLexicalTokenizer } from "../src/tools/retrieval/tokenizer.js"

function contract(overrides: Partial<AgentToolContract> = {}): AgentToolContract {
  return {
    allowed: true,
    resourceTypes: ["application"],
    action: "read",
    sideEffect: "none",
    idempotent: true,
    replaySafe: true,
    risk: "low",
    approval: "never",
    intents: ["读取应用信息"],
    useWhen: ["需要查询应用事实时"],
    avoidWhen: [],
    prerequisites: [],
    parameterSummary: [],
    successEvidence: ["响应返回当前事实"],
    commonErrorCodes: [],
    predecessors: [],
    followups: [],
    verification: { mode: "response", successCodes: [200] },
    ...overrides,
  }
}

function operation(
  operationId: string,
  toolContract: AgentToolContract | undefined,
  category = "applications",
): ToolOperation {
  const writes = toolContract && ["external-write", "platform-write", "destructive"].includes(toolContract.sideEffect)
  const requiresApplicationId = operationId === "getApplication"
  return {
    operationId,
    method: writes ? "POST" : "GET",
    path: `/api/v1/test/${operationId}`,
    category,
    risk: "read",
    requiredScopes: ["project:read"],
    approval: toolContract?.approval ?? "never",
    idempotent: toolContract?.idempotent ?? true,
    timeoutMs: 5000,
    inputSchema: {
      type: "object",
      properties: requiresApplicationId ? { applicationId: { type: "string" } } : {},
      required: requiresApplicationId ? ["applicationId"] : [],
      additionalProperties: false,
    },
    ...(toolContract ? { contract: toolContract } : {}),
  }
}

const applicationOperations = [
  operation("listApplications", contract({
    action: "discover",
    intents: ["列出项目空间内的应用", "查找应用候选"],
    useWhen: ["还没有 applicationId，需要浏览或筛选应用时"],
    avoidWhen: ["已经有唯一 applicationId 且需要详情时"],
  })),
  operation("getApplication", contract({
    intents: ["读取单个应用详情"],
    useWhen: ["已经有唯一 applicationId，需要权威详情时"],
    avoidWhen: ["需要浏览项目空间内全部应用时"],
    parameterSummary: ["applicationId：可信工具结果中的应用标识"],
    predecessors: ["createApplication", "updateApplication"],
  })),
  operation("createApplication", contract({
    action: "create",
    sideEffect: "platform-write",
    intents: ["创建新应用", "建立服务容器"],
    useWhen: ["用户明确要创建应用且名称已经确定时"],
    avoidWhen: ["只是查询、修改或删除已有应用时"],
    prerequisites: ["已取得真实 projectId 和应用名称"],
    parameterSummary: ["projectId：目标项目空间", "name：新应用名称"],
    successEvidence: ["getApplication 回读到新应用"],
    followups: ["getApplication"],
    verification: {
      mode: "readback",
      operationId: "getApplication",
      idSource: "/application/id",
      argumentBindings: { applicationId: "/application/id" },
      completion: { mode: "readback-success" },
    },
  })),
  operation("updateApplication", contract({
    action: "update",
    sideEffect: "platform-write",
    intents: ["修改已有应用", "更新应用名称"],
    useWhen: ["用户明确要修改已存在应用时"],
    avoidWhen: ["需要创建、查询或删除应用时"],
    prerequisites: ["已取得 applicationId 和当前 revision"],
    successEvidence: ["getApplication 回读到新 revision"],
    followups: ["getApplication"],
    verification: {
      mode: "readback",
      operationId: "getApplication",
      idSource: "/application/id",
      argumentBindings: { applicationId: "/application/id" },
      completion: { mode: "readback-success" },
    },
  })),
  operation("previewApplicationDeletion", contract({
    action: "verify",
    intents: ["删除应用前预检影响", "检查应用删除阻断项"],
    useWhen: ["用户提出删除应用，需要先检查关联部署和数据卷时"],
    avoidWhen: ["还没有删除意图，或用户已经确认并进入实际删除阶段时"],
    successEvidence: ["响应返回删除阻断项和影响范围"],
  })),
  operation("deleteApplication", contract({
    action: "delete",
    sideEffect: "destructive",
    idempotent: true,
    replaySafe: false,
    risk: "high",
    approval: "always",
    intents: ["确认后实际删除应用"],
    useWhen: ["删除预检通过且用户明确确认后"],
    avoidWhen: ["尚未完成删除预检或用户只想查看影响时"],
    prerequisites: ["previewApplicationDeletion 已完成且用户已确认"],
    successEvidence: ["应用详情回读为不存在"],
    predecessors: ["previewApplicationDeletion"],
    verification: { mode: "response", successCodes: [204] },
  })),
]

describe("hybrid tool retrieval", () => {
  it("tokenizes Chinese, camelCase and stable error codes without business classifiers", () => {
    const tokens = new UnicodeLexicalTokenizer().tokenize("读取 resourceCategory，错误 cluster.resource_category_invalid")

    expect(tokens.some(token => token === "读取" || token === "读")).toBe(true)
    expect(tokens).toContain("resourcecategory")
    expect(tokens).toContain("resource")
    expect(tokens).toContain("category")
    expect(tokens).toContain("cluster.resource_category_invalid")
    expect(tokens).toContain("invalid")
  })

  it("keeps avoidWhen out of all positive retrieval documents", () => {
    const toolContract = contract({ avoidWhen: ["绝对不要使用这个负向边界词"] })
    const documents = buildToolRetrievalDocuments({
      operationId: "inspectApplication",
      category: "applications",
      inputSchema: { properties: {} },
      contract: toolContract,
    }, new UnicodeLexicalTokenizer())

    expect(JSON.stringify(documents)).not.toContain("负向边界词")
  })

  it("only exposes operations with an explicit allowed contract", () => {
    const hidden = operation("hiddenApplication", contract({ allowed: false }))
    const legacy = operation("legacyApplication", undefined)
    const catalog = ToolCatalog.load([...applicationOperations, hidden, legacy])

    expect(catalog.get("legacyApplication").operationId).toBe("legacyApplication")
    expect(catalog.resolve({}, "列出应用").map(item => item.operationId)).not.toContain("legacyApplication")
    expect(catalog.resolve({}, "列出应用").map(item => item.operationId)).not.toContain("hiddenApplication")
    expect(catalog.search("legacyApplication").loadedOperationIds).not.toContain("legacyApplication")
    expect(catalog.search("hiddenApplication").loadedOperationIds).not.toContain("hiddenApplication")
  })

  it.each([
    ["列出项目空间里所有应用候选", "listApplications", "createApplication"],
    ["读取这个 applicationId 对应的单个应用详情", "getApplication", "listApplications"],
    ["创建一个名为 api-server 的新应用", "createApplication", "listApplications"],
    ["修改已有应用的名称", "updateApplication", "createApplication"],
    ["删除应用前先检查影响和阻断项", "previewApplicationDeletion", "deleteApplication"],
  ])("keeps hard-negative operation boundaries: %s", (query, expected, forbiddenFirst) => {
    const result = ToolCatalog.load(applicationOperations).search(query)
    expect(result.loadedOperationIds).toContain(expected)
    expect(result.loadedOperationIds[0]).toBe(expected)
    expect(result.loadedOperationIds[0]).not.toBe(forbiddenFirst)
  })

  it("does not turn a verifier's reverse workflow relation into a required write predecessor", () => {
    const result = ToolCatalog.load(applicationOperations).search("读取这个 applicationId 对应的单个应用详情")

    expect(result.retrieval.matches.find(item => item.operationId === "createApplication")?.reasonCode).not.toBe("required_predecessor")
    expect(result.loadedOperationIds[0]).toBe("getApplication")
  })

  it("returns stable Top 8 results and caps the model-visible platform set at 12", () => {
    const operations = Array.from({ length: 20 }, (_, index) => operation(`inspectResource${index}`, contract({
      resourceTypes: [`resource-${index}`],
      intents: [`检查资源 ${index} 的唯一状态`],
      useWhen: [`需要读取资源 ${index} 时`],
    }), "runtime"))
    const catalog = ToolCatalog.load(operations)
    const first = catalog.search("检查资源状态", {}, 8)
    const second = catalog.search("检查资源状态", {}, 8)

    expect(first.loadedOperationIds).toHaveLength(8)
    expect(second.loadedOperationIds).toEqual(first.loadedOperationIds)
    const visible = catalog.resolve({}, "检查资源状态", operations.slice(10).map(item => item.operationId))
    expect(visible).toHaveLength(12)
    expect(visible.map(item => item.operationId)).toEqual(expect.arrayContaining(operations.slice(10).map(item => item.operationId)))
  })

  it("retains sticky operations and their verifier while allowing search results to expand the loaded set", () => {
    const catalog = ToolCatalog.load(applicationOperations)
    const discovered = catalog.search("创建一个新应用")
    const resolved = catalog.resolve({}, "继续验收刚才创建的应用", discovered.loadedOperationIds)
      .map(item => item.operationId)

    expect(discovered.loadedOperationIds).toContain("createApplication")
    expect(resolved).toContain("createApplication")
    expect(resolved).toContain("getApplication")
  })

  it("keeps a required verifier ahead of a full sticky-tool budget", () => {
    const fillers = Array.from({ length: 12 }, (_, index) => operation(`inspectSticky${index}`, contract({
      resourceTypes: [`sticky-${index}`],
      intents: [`检查 sticky ${index}`],
      useWhen: [`需要 sticky ${index} 时`],
    })))
    const newGoal = operation("inspectNewGoal", contract({
      resourceTypes: ["new-goal"],
      intents: ["检查唯一新目标 zeta"],
      useWhen: ["用户要求检查唯一新目标 zeta 时"],
    }))
    const catalog = ToolCatalog.load([...applicationOperations, ...fillers, newGoal])
    const resolved = catalog.resolve({
      completedOperations: ["createApplication"],
    }, "检查唯一新目标 zeta", fillers.map(item => item.operationId))
      .map(item => item.operationId)

    expect(resolved).toHaveLength(12)
    expect(resolved[0]).toBe("getApplication")
    expect(resolved).toContain("inspectNewGoal")
  })

  it("uses controlled run state and does not introduce unrelated writes while waiting", () => {
    const catalog = ToolCatalog.load(applicationOperations)
    const result = catalog.search("创建一个新的应用", {
      resourceTypes: ["application"],
      completedOperations: ["listApplications"],
      stableOutcomes: ["application.list.succeeded"],
      pendingState: "approval",
      stableErrorCodes: ["application.revision_conflict"],
    })

    expect(result.retrieval.query).toMatchObject({
      resourceContext: ["application"],
      completedOperations: ["listApplications"],
      pendingState: "approval",
      stableErrorCodes: ["application.revision_conflict"],
    })
    expect(result.loadedOperationIds).not.toContain("createApplication")
  })

  it("redacts common credentials before constructing the retrieval query", () => {
    const catalog = ToolCatalog.load(applicationOperations)
    const result = catalog.search("检查应用失败，apiKey=super-secret-value")

    expect(result.retrieval.query.currentGoal).toContain("apiKey=[REDACTED]")
    expect(result.retrieval.query.currentGoal).not.toContain("super-secret-value")
  })

  it("does not keep a completed write callable while its async terminal state is pending", () => {
    const catalog = ToolCatalog.load(applicationOperations)
    const resolved = catalog.resolve({
      completedOperations: ["createApplication"],
      pendingState: "async_terminal_check",
    }, "继续检查创建结果", ["createApplication"])
      .map(item => item.operationId)

    expect(resolved).not.toContain("createApplication")
    expect(resolved).toContain("getApplication")
  })

  it("uses all three vector documents and RRF for semantic recall", async () => {
    const catalog = ToolCatalog.load(applicationOperations, {
      embeddingProvider: new SemanticEmbeddingProvider(),
      reranker: stableReranker,
    })

    const intent = await catalog.searchAsync("make a brand new service container")
    const parameters = await catalog.searchAsync("applicationId field lookup")
    const workflow = await catalog.searchAsync("read back the newly created resource")

    expect(intent).toMatchObject({ strategy: "hybrid", outcome: "succeeded" })
    expect(intent.loadedOperationIds).toContain("createApplication")
    expect(parameters.loadedOperationIds).toContain("getApplication")
    expect(workflow.loadedOperationIds).toContain("createApplication")
    expect(intent.retrieval.matches.find(item => item.operationId === "createApplication")?.ranks.intent).toBeDefined()
    expect(parameters.retrieval.matches.find(item => item.operationId === "getApplication")?.ranks.parameters).toBeDefined()
    expect(workflow.retrieval.matches.find(item => item.operationId === "createApplication")?.ranks.workflow).toBeDefined()
  })

  it("degrades safely and explicitly when embedding and reranking are unavailable", async () => {
    const catalog = ToolCatalog.load(applicationOperations, {
      embeddingProvider: new FailingEmbeddingProvider(),
      reranker: { rerank: async () => { throw new Error("provider offline") } },
    })
    const result = await catalog.searchAsync("列出项目空间内的应用")
    const automatic = await catalog.resolveDetailedAsync({}, "列出项目空间内的应用")

    expect(result.outcome).toBe("degraded")
    expect(result.degradedReason).toBe("embedding_unavailable,rerank_unavailable")
    expect(result.loadedOperationIds).toContain("listApplications")
    expect(automatic.retrieval.outcome).toBe("degraded")
    expect(automatic.tools.map(tool => tool.operationId)).toContain("listApplications")
  })
})

const stableReranker: ToolReranker = {
  rerank: async (_query, candidates) => candidates.map((candidate, index) => ({
    operationId: candidate.operationId,
    score: candidates.length - index,
  })),
}

class SemanticEmbeddingProvider implements EmbeddingProvider {
  identity() {
    return { provider: "test", model: "semantic-fixture", dimensions: 4 }
  }

  async embedDocuments(input: string[]): Promise<number[][]> {
    return input.map(value => semanticVector(value))
  }

  async embedQuery(input: string): Promise<number[]> {
    return semanticVector(input)
  }
}

class FailingEmbeddingProvider implements EmbeddingProvider {
  identity() {
    return { provider: "test", model: "offline", dimensions: 4 }
  }

  async embedDocuments(): Promise<number[][]> {
    throw new Error("provider offline")
  }

  async embedQuery(): Promise<number[]> {
    throw new Error("provider offline")
  }
}

function semanticVector(input: string): number[] {
  const value = input.toLocaleLowerCase()
  if (value.includes("brand new service") || value.includes("创建新应用") || value.includes("建立服务容器")) return [1, 0, 0, 0]
  if (value.includes("read back") || value.includes("回读") || value.includes("成功证据")) return [0, 0, 1, 0]
  if (value.includes("applicationid") || value.includes("应用标识")) return [0, 1, 0, 0]
  return [0, 0, 0, 1]
}
