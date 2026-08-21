import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"
import { RunExecutor } from "../src/executor.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { TestRepository } from "./support/test-repository.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import type { ModelProvider } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { getToolDetailsTool } from "../src/tools/tool-details.js"
import { searchToolsTool } from "../src/tools/tool-search.js"

describe("RunExecutor slim lifecycle", () => {
  it("atomically claims and completes a queued Run without a lease heartbeat", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "hello")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "请简要回复",
      pageContext: {},
      idempotencyKey: "run-complete",
      actorSessionId: "ses_a",
    })
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(new DeterministicProvider()),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "worker-a" }),
    )

    expect(await executor.runOnce()).toBe(true)
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(await executor.runOnce()).toBe(false)
  })

  it("marks an in-flight Run interrupted when the Agent stops", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "stop")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "等待",
      pageContext: {},
      idempotencyKey: "run-stop",
      actorSessionId: "ses_a",
    })
    let notifyStarted!: () => void
    const started = new Promise<void>(resolve => { notifyStarted = resolve })
    const provider: ModelProvider = {
      async *stream(request) {
        notifyStarted()
        await new Promise<void>((_resolve, reject) => {
          const signal = request.signal
          const abortError = () => signal?.reason instanceof Error ? signal.reason : new Error("ai.run_canceled")
          if (signal?.aborted) reject(abortError())
          signal?.addEventListener("abort", () => reject(abortError()), { once: true })
        })
        yield { type: "completed", usage: { inputTokens: 0, outputTokens: 0 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "worker-stop" }))
    const executing = executor.runOnce()
    await started
    await executor.stop()
    await executing

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("interrupted")
  })

  it("resumes the same Run after one-call approval and preserves the Tool result", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "restart")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "重启发布",
      pageContext: {},
      idempotencyKey: "run-approval",
      actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "restartRelease",
      name: "重启发布",
      summary: "重启指定发布。",
      method: "POST",
      path: "/api/v1/releases/{releaseId}/restart",
      category: "deployment",
      requiredScopes: ["deployment:write"],
      requiresApproval: true,
      idempotent: true,
      parameters: [{ inputName: "releaseId", wireName: "releaseId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
    }])
    let step = 0
    const provider: ModelProvider = {
      async *stream() {
        if (step++ === 0) {
          yield { type: "completed", usage: { inputTokens: 4, outputTokens: 2 }, toolCalls: [{ id: "restart", operationId: "restartRelease", arguments: { releaseId: "rel_a" } }] }
          return
        }
        yield { type: "message_delta", delta: "发布已重启。" }
        yield { type: "completed", usage: { inputTokens: 6, outputTokens: 3 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } })),
      new ProjectingToolCallStore(store, repository),
      undefined,
      repository,
    )
    const executor = new RunExecutor(repository, new ModelRuntime(provider, catalog.modelTools(["restartRelease"])), loadConfig({ NODE_ENV: "test" }), tools)

    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("waiting_approval")
    expect((await repository.getRunToolState(created.run.id))?.selectedOperationIds).toEqual(["restartRelease"])
    const pending = [...store.records.values()][0]!
    await tools.resolveApproval(pending.id, "approve")
    await repository.updateRun(created.run.id, "waiting_approval", "queued")
    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(store.records.get(pending.id)).toMatchObject({ status: "succeeded", approvalDecision: "approve" })
    expect((await repository.getExecutionInput(created.run.id))?.toolInteractions.some(item => item.type === "tool_result")).toBe(true)
    const completedEvent = (await repository.getEvents("usr_a", created.run.id, 0))
      .find(event => event.type === "tool.completed")
    expect(completedEvent?.data.resultItem).toMatchObject({
      type: "tool_result",
      status: "completed",
      content: { relatedItemId: `${pending.id}:item`, result: { restarted: true } },
    })
  })

  it("keeps an exact platform tool available for the rest of the Run after get_tool_details", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "details")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "查看项目空间",
      pageContext: {},
      idempotencyKey: "run-details",
      actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject",
      name: "读取项目空间",
      summary: "读取指定项目空间。",
      method: "GET",
      path: "/api/v1/projects/{projectId}",
      category: "project",
      requiredScopes: ["project:read"],
      requiresApproval: false,
      idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
    }])
    const observedTools: string[][] = []
    let step = 0
    const provider: ModelProvider = {
      async *stream(request) {
        observedTools.push((request.tools ?? []).map(tool => tool.operationId))
        if (step++ === 0) {
          yield { type: "completed", usage: { inputTokens: 2, outputTokens: 1 }, toolCalls: [{ id: "details", operationId: "get_tool_details", arguments: { operationIds: ["getProject"] } }] }
          return
        }
        if (step === 2) {
          yield { type: "completed", usage: { inputTokens: 3, outputTokens: 1 }, toolCalls: [{ id: "get", operationId: "getProject", arguments: { projectId: "prj_a" } }] }
          return
        }
        yield { type: "message_delta", delta: "项目空间已读取。" }
        yield { type: "completed", usage: { inputTokens: 4, outputTokens: 2 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (_pageContext, _userInput, operationIds) => [...catalog.modelTools(operationIds), getToolDetailsTool],
      search: input => ({
        ...catalog.search(input), loadedOperationIds: [], missingOperationIds: [], catalogDigest: catalog.digest,
        duplicate: false, cacheHit: false,
      }),
      details: operationIds => {
        const result = catalog.getDetails(operationIds)
        return {
          ...result, loadedOperationIds: result.items.map(item => item.operationId),
          alreadySelectedOperationIds: [], catalogDigest: catalog.digest, duplicate: false, cacheHit: false,
        }
      },
    })
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "prj_a" } })),
      new ProjectingToolCallStore(new MemoryToolCallStore(), repository),
    )
    const executor = new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test" }), tools)

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(observedTools[0]).not.toContain("getProject")
    expect(observedTools[1]).toContain("getProject")
    expect(observedTools[2]).toContain("getProject")
    expect((await repository.getRunToolState(created.run.id))?.selectedOperationIds).toEqual(["getProject"])
  })

  it("auto-loads search candidates, keeps them visible, permits different arguments, and replays duplicate search results", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "search")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "分别查看两个项目空间",
      pageContext: {},
      idempotencyKey: "run-search-autoload",
      actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject",
      name: "读取项目空间",
      summary: "按 projectId 读取项目空间详情。",
      purpose: { zh: "读取单个项目空间详情。", en: "Get one project." },
      aliases: { zh: ["查看项目空间详情"], en: ["get project"] },
      avoidWhen: { zh: "需要列表时不要使用。", en: "Do not use for lists." },
      preconditions: { zh: ["提供 projectId"], en: ["Provide projectId"] },
      successEvidence: { zh: "返回项目空间。", en: "Returns the project." },
      method: "GET",
      path: "/api/v1/projects/{projectId}",
      category: "projects",
      tags: ["Projects"],
      requiredScopes: ["project:read"],
      requiresApproval: false,
      idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
      outputSchema: { type: "object", properties: { id: { type: "string" } } },
    }])
    const observedTools: string[][] = []
    const executed: string[] = []
    let step = 0
    const provider: ModelProvider = {
      async *stream(request) {
        observedTools.push((request.tools ?? []).map(tool => tool.operationId))
        if (step === 0 || step === 3) {
          step += 1
          yield { type: "completed", usage: { inputTokens: 2, outputTokens: 1 }, toolCalls: [{ id: `search-${step}`, operationId: "search_tools", arguments: { query: "查看项目空间详情" } }] }
          return
        }
        if (step === 1 || step === 2) {
          const projectId = step === 1 ? "prj_a" : "prj_b"
          step += 1
          yield { type: "completed", usage: { inputTokens: 2, outputTokens: 1 }, toolCalls: [{ id: `get-${projectId}`, operationId: "getProject", arguments: { projectId } }] }
          return
        }
        yield { type: "message_delta", delta: "两个项目空间都已读取。" }
        yield { type: "completed", usage: { inputTokens: 2, outputTokens: 1 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (_pageContext, _userInput, operationIds) => [...catalog.modelTools(operationIds), searchToolsTool],
      search: input => ({
        ...catalog.search(input), loadedOperationIds: [], missingOperationIds: [], catalogDigest: catalog.digest,
        duplicate: false, cacheHit: false,
      }),
      details: operationIds => {
        const result = catalog.semanticDetails(operationIds)
        return {
          ...result, loadedOperationIds: result.items.map(item => item.operationId), alreadySelectedOperationIds: [],
          catalogDigest: catalog.digest, duplicate: false, cacheHit: false,
        }
      },
    })
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(({ arguments: args }) => {
        executed.push(String(args.projectId))
        return { status: 200, body: { id: args.projectId } }
      }),
      new ProjectingToolCallStore(new MemoryToolCallStore(), repository),
    )

    await new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test" }), tools).runOnce()

    expect(executed).toEqual(["prj_a", "prj_b"])
    expect(observedTools[0]).not.toContain("getProject")
    expect(observedTools.slice(1).every(tools => tools.includes("getProject"))).toBe(true)
    expect((await repository.getRunToolState(created.run.id))?.selectedOperationIds).toEqual(["getProject"])
    const searchRecords = (await repository.getExecutionInput(created.run.id))!.toolInteractions
      .filter(item => item.content.operationId === "search_tools")
    expect(searchRecords).toHaveLength(2)
    expect(searchRecords[0]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], duplicate: false, cacheHit: false })
    expect(searchRecords[1]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], duplicate: true, cacheHit: true })
    expect((searchRecords[1]?.content.result as { items: unknown[] }).items).toHaveLength(1)
  })

  it("replays repeated mixed-order detail requests and reports already selected tools", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_a", "details-cache")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "确认项目工具", pageContext: {}, idempotencyKey: "details-cache-run", actorSessionId: "ses_a",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getProject", name: "查看项目空间", summary: "读取项目空间", category: "projects", tags: ["Projects"],
      aliases: { zh: ["项目空间详情"], en: ["project details"] }, purpose: { zh: "读取项目空间。", en: "Get project." },
      avoidWhen: { zh: "", en: "" }, preconditions: { zh: [], en: [] }, successEvidence: { zh: "返回项目空间。", en: "Returns project." },
      method: "GET", path: "/api/v1/projects/{projectId}", requiredScopes: ["project:read"], requiresApproval: false, idempotent: true,
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
      outputSchema: { type: "object" },
    }])
    let step = 0
    const provider: ModelProvider = {
      async *stream() {
        if (step++ < 2) {
          yield {
            type: "completed", usage: { inputTokens: 1, outputTokens: 1 },
            toolCalls: [{ id: `details-${step}`, operationId: "get_tool_details", arguments: { operationIds: step === 1 ? ["getProject", "missingTool"] : ["missingTool", "getProject", "getProject"] } }],
          }
          return
        }
        yield { type: "message_delta", delta: "详情已确认。" }
        yield { type: "completed", usage: { inputTokens: 1, outputTokens: 1 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }), health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (_page, _input, operationIds) => [...catalog.modelTools(operationIds), getToolDetailsTool],
      search: input => ({ ...catalog.search(input), loadedOperationIds: [], missingOperationIds: [], catalogDigest: catalog.digest, duplicate: false, cacheHit: false }),
      details: operationIds => {
        const result = catalog.semanticDetails(operationIds)
        return { ...result, loadedOperationIds: result.items.map(item => item.operationId), alreadySelectedOperationIds: [], catalogDigest: catalog.digest, duplicate: false, cacheHit: false }
      },
    })

    await new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test" })).runOnce()

    const records = (await repository.getExecutionInput(created.run.id))!.toolInteractions.filter(item => item.content.operationId === "get_tool_details")
    expect(records).toHaveLength(2)
    expect(records[0]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], missingOperationIds: ["missingTool"], duplicate: false })
    expect(records[1]?.content.result).toMatchObject({ loadedOperationIds: ["getProject"], alreadySelectedOperationIds: ["getProject"], duplicate: true, cacheHit: true })
    expect((records[1]?.content.result as { items: unknown[] }).items).toHaveLength(1)
  })
})
