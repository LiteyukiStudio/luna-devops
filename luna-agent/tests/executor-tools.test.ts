import { describe, expect, it, vi } from "vitest"
import { loadConfig } from "../src/config.js"
import { RunExecutor } from "../src/executor.js"
import { ModelRuntime } from "../src/model-runtime.js"
import { MemoryRepository } from "../src/persistence/memory.js"
import type { RunStateConflictError } from "../src/persistence/repository.js"
import { DeterministicProvider } from "../src/provider/deterministic.js"
import type { ModelProvider, ModelRequest } from "../src/provider/provider.js"
import { presentTimeline } from "../src/timeline-presenter.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { createOptionsInput, createOptionsTool } from "../src/tools/ui-options.js"
import { navigateToRouteTool } from "../src/tools/ui-route.js"
import { searchToolsTool } from "../src/tools/tool-search.js"

describe("provider to tool to subsequent model invocation", () => {
  it("reports the authoritative state when a run transition loses a race", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "conflict")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "state-conflict",
    })

    await repository.cancelRun("usr_a", created.run.id)

    await expect(repository.updateRun(created.run.id, "queued", "running")).rejects.toMatchObject({
      name: "RunStateConflictError",
      message: "ai.run_state_conflict",
      expectedStatus: "queued",
      targetStatus: "running",
      actualStatus: "canceled",
    } satisfies Partial<RunStateConflictError>)
  })

  it("treats a durable cancellation won during completion as canceled instead of failed", async () => {
    class CancelBeforeCompletionRepository extends MemoryRepository {
      override async updateRun(runId: string, from: Parameters<MemoryRepository["updateRun"]>[1], to: Parameters<MemoryRepository["updateRun"]>[2], fields: Parameters<MemoryRepository["updateRun"]>[3] = {}) {
        if (from === "running" && to === "completed") await this.cancelRun("usr_a", runId)
        return super.updateRun(runId, from, to, fields)
      }
    }
    const repository = new CancelBeforeCompletionRepository()
    const conversation = await repository.createConversation("usr_a", "completion race")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "hello", pageContext: {}, idempotencyKey: "completion-race",
    })
    const executor = new RunExecutor(repository, new ModelRuntime(new DeterministicProvider()), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "completion-race-worker" }))

    await expect(executor.runOnce()).resolves.toBe(true)

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("canceled")
    const events = await repository.getEvents("usr_a", created.run.id, 0)
    expect(events.some(event => event.type === "run.canceled")).toBe(true)
    expect(events.some(event => event.type === "run.failed")).toBe(false)
  })

  it("aborts an active model stream immediately after the run is canceled", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "cancel")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "keep working", pageContext: {}, idempotencyKey: "cancel-run",
    })
    const provider: ModelProvider = {
      async *stream(request: ModelRequest) {
        yield { type: "message_delta", delta: "partial" } as const
        await new Promise<void>((resolve, reject) => {
          if (request.signal?.aborted) {
            reject(request.signal.reason instanceof Error ? request.signal.reason : new Error("ai.run_canceled"))
            return
          }
          request.signal?.addEventListener("abort", () => reject(request.signal?.reason instanceof Error ? request.signal.reason : new Error("ai.run_canceled")), { once: true })
        })
        resolveNever()
      },
      async complete() {
        return { text: "unused", usage: { inputTokens: 0, outputTokens: 0 } }
      },
      capabilities: () => ({ streaming: true, toolCalling: false, structuredOutput: false }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "cancel-worker" }))
    const execution = executor.runOnce()
    await vi.waitFor(async () => {
      expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("running")
    })
    await repository.cancelRun("usr_a", created.run.id)
    expect(executor.cancel(created.run.id)).toBe(true)
    await execution
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("canceled")
    expect((await repository.getTimeline("usr_a", conversation.id))?.turns[0]?.items.at(-1)?.status).toBe("completed")
  })

  it("automatically titles the first completed conversation without replacing manual titles", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "新会话")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "检查今天失败的构建", pageContext: {}, idempotencyKey: "auto-title",
    })
    const executor = new RunExecutor(repository, new ModelRuntime(new DeterministicProvider()), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "title-worker" }))
    await executor.runOnce()
    expect(await repository.getConversation("usr_a", conversation.id)).toMatchObject({
      titleSource: "assistant",
    })
    expect((await repository.getConversation("usr_a", conversation.id))?.title).not.toBe("新会话")
    const events = await repository.getEvents("usr_a", (await repository.getTimeline("usr_a", conversation.id))!.turns[0]!.run!.id, 0)
    expect(events.filter(event => event.type === "content.delta").length).toBeGreaterThan(1)
    expect(events.some(event => event.type === "thinking.started")).toBe(true)
    expect(events.some(event => event.type === "thinking.completed")).toBe(true)
    const eventCursors = (await presentTimeline(repository, "usr_a", conversation.id))?.eventCursors
    expect(eventCursors).toHaveLength(1)
    expect(eventCursors?.[0]?.runId).toBe((await repository.getTimeline("usr_a", conversation.id))?.turns[0]?.run?.id)
    expect(typeof eventCursors?.[0]?.after).toBe("number")
  })

  it("passes the current assistant title as context and accepts a rename tool when the topic drifts", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "旧的部署话题", undefined, "assistant")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "现在改为排查账单扣费", pageContext: {}, idempotencyKey: "retitle-topic",
    })
    const requests: ModelRequest[] = []
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        yield { type: "message_delta", delta: "我们来排查账单扣费。" }
        yield {
          type: "completed",
          usage: { inputTokens: 10, outputTokens: 10 },
          toolCalls: [{ operationId: "rename_conversation", arguments: { title: "账单扣费排查" } }],
        }
      },
      async complete() {
        return { text: "unused", usage: { inputTokens: 0, outputTokens: 0 } }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "retitle-worker" }))

    await executor.runOnce()

    expect(requests[0]?.messages.at(-1)?.content).toContain('"title":"旧的部署话题"')
    expect(requests[0]?.messages.at(-1)?.content).toContain('"titleSource":"assistant"')
    expect(requests[0]?.tools?.some(tool => tool.operationId === "rename_conversation")).toBe(true)
    expect(await repository.getConversation("usr_a", conversation.id)).toMatchObject({
      title: "账单扣费排查",
      titleSource: "assistant",
    })
  })

  it("never exposes or applies the rename tool after the user manually names a conversation", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "用户固定标题")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "换一个话题", pageContext: {}, idempotencyKey: "locked-title",
    })
    const requests: ModelRequest[] = []
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        yield { type: "message_delta", delta: "标题保持不变。" }
        yield {
          type: "completed",
          usage: { inputTokens: 10, outputTokens: 10 },
          toolCalls: [{ operationId: "rename_conversation", arguments: { title: "恶意覆盖" } }],
        }
      },
      async complete() {
        return { text: "unused", usage: { inputTokens: 0, outputTokens: 0 } }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "locked-title-worker" }))

    await executor.runOnce()

    expect(requests[0]?.tools?.some(tool => tool.operationId === "rename_conversation")).toBe(false)
    expect(await repository.getConversation("usr_a", conversation.id)).toMatchObject({
      title: "用户固定标题",
      titleSource: "user",
    })
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const timelineItems = timeline?.turns[0]?.selectedRun?.items as Array<{
      toolCall?: { operationId: string, status?: string }
    }> | undefined
    const renameCall = timelineItems?.find(item => item.toolCall?.operationId === "rename_conversation")
    expect(renameCall?.toolCall?.status).toBe("skipped")
  })

  it("persists create_options arguments and returns visible UI actions without a business API call", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "options")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "What should I do next?", pageContext: {}, idempotencyKey: "options-tool",
    })
    const optionArguments = {
      title: "Choose a next step",
      options: [
        { id: "projects", label: "Open projects", action: { type: "navigate", routeName: "projects" } },
        { id: "continue", label: "Continue diagnosis", action: { type: "send_message", message: "Continue the diagnosis" } },
      ],
    }
    const provider: ModelProvider = {
      async *stream() {
        yield {
          type: "completed",
          usage: { inputTokens: 10, outputTokens: 10 },
          toolCalls: [{ operationId: "create_options", arguments: optionArguments }],
        }
      },
      async complete() {
        return { text: "Next steps", usage: { inputTokens: 1, outputTokens: 1 } }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "options-worker" }))

    await executor.runOnce()

    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const items = timeline?.turns[0]?.selectedRun?.items as Array<{
      toolCall?: { operationId: string, arguments: Record<string, unknown>, uiActions?: Array<{ type: string, label?: string }> }
    }> | undefined
    const optionsItem = items?.find(item => item.toolCall?.operationId === "create_options")
    expect(optionsItem?.toolCall?.arguments).toEqual(createOptionsInput.parse(optionArguments))
    expect(optionsItem?.toolCall?.uiActions).toEqual([
      expect.objectContaining({ type: "navigate", label: "Open projects" }),
      expect.objectContaining({ type: "send_message", label: "Continue diagnosis" }),
    ])
  })

  it("returns invalid interaction-card arguments to the model for a bounded self-correction", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "Redis 配置", undefined, "user")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "生成 Redis 配置表单",
      pageContext: {},
      idempotencyKey: "repair-interaction-card",
    })
    const requests: ModelRequest[] = []
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        if (modelStep++ === 0) {
          yield { type: "message_delta", delta: "正在生成配置表单。" }
          yield {
            type: "completed",
            usage: { inputTokens: 10, outputTokens: 10 },
            toolCalls: [{
              id: "invalid_card",
              operationId: "create_interaction_cards",
              arguments: {
                schemaVersion: 1,
                placement: "turn_end",
                title: "Redis 配置",
                mode: "interactive",
                template: "form",
                cards: [],
              },
            }],
          }
          return
        }
        yield { type: "message_delta", delta: "请填写 Redis 配置。" }
        yield {
          type: "completed",
          usage: { inputTokens: 20, outputTokens: 20 },
          toolCalls: [{
            id: "valid_card",
            operationId: "create_interaction_cards",
            arguments: {
              schemaVersion: 1,
              placement: "turn_end",
              title: "Redis 配置",
              mode: "interactive",
              template: "form",
              cards: [{
                id: "redis",
                presentation: { variant: "form", title: "Redis" },
                form: {
                  sections: [{
                    id: "basic",
                    fields: [{ id: "name", type: "text", label: "实例名称", required: true }],
                  }],
                },
                actions: [{
                  id: "continue",
                  type: "send_message",
                  label: "继续",
                  message: "继续配置 {{name}}",
                }],
              }],
            },
          }],
        }
      },
      async complete() {
        return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "repair-card-worker" }),
    )

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(requests).toHaveLength(2)
    const retryMessage = requests[1]?.messages.find(message => message.role === "tool" && message.toolCallId === "invalid_card")
    expect(retryMessage).toMatchObject({ role: "tool", toolCallId: "invalid_card" })
    expect(retryMessage?.content).toContain("ai.interaction_card_schema_invalid")
    expect(retryMessage?.content).toContain('"path":"cards"')
    expect(retryMessage?.content).toContain('"attempt":1')
    expect(retryMessage?.content).toContain('"retryable":true')
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    expect(JSON.stringify(timeline)).toContain("Redis 配置")
    expect(JSON.stringify(timeline)).not.toContain("invalid_card")
    expect(timeline?.turns[0]?.selectedRun?.items.some(item =>
      "toolCall" in item && item.toolCall.operationId === "create_options",
    )).toBe(false)
  })

  it("feeds presentation-card completion back to the model before ending the workflow", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "部署状态", undefined, "user")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "展示部署摘要并继续判断是否完成",
      pageContext: {},
      idempotencyKey: "presentation-card-continuation",
    })
    const requests: ModelRequest[] = []
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        if (modelStep++ === 0) {
          yield {
            type: "completed",
            usage: { inputTokens: 10, outputTokens: 5 },
            toolCalls: [{
              id: "create_summary",
              operationId: "create_interaction_cards",
              arguments: {
                schemaVersion: 1,
                title: "部署摘要",
                mode: "presentation",
                template: "result",
                cards: [{
                  id: "summary",
                  presentation: { variant: "summary", title: "部署参数已整理" },
                }],
              },
            }],
          }
          return
        }
        yield { type: "message_delta", delta: "这里只完成了参数呈现，部署尚未执行。" }
        yield { type: "completed", usage: { inputTokens: 10, outputTokens: 10 }, toolCalls: [] }
      },
      async complete() {
        return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "presentation-continuation-worker" }),
    )

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(requests).toHaveLength(2)
    const cardResult = requests[1]?.messages.find(message =>
      message.role === "tool" && message.toolCallId === "create_summary",
    )
    expect(cardResult?.content).toContain('"workflowState":"evidence_presented"')
    expect(cardResult?.content).toContain('"completionEvidence":false')
    expect(JSON.stringify(await presentTimeline(repository, "usr_a", conversation.id))).toContain("部署尚未执行")
  })

  it("keeps the last interaction-card schema issues when bounded repair fails", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "卡片校验", undefined, "user")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "生成配置卡片",
      pageContext: {},
      idempotencyKey: "failed-interaction-card-repair",
    })
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream() {
        modelStep += 1
        yield {
          type: "completed",
          usage: { inputTokens: 10, outputTokens: 5 },
          toolCalls: [{
            id: `invalid_card_${modelStep}`,
              operationId: "create_interaction_cards",
              arguments: {
                schemaVersion: 1,
                title: "无效卡片",
              mode: "interactive",
              template: "form",
              cards: [],
            },
          }],
        }
      },
      async complete() {
        return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "failed-card-repair-worker" }),
    )

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("failed")
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const generation = timeline?.turns[0]?.selectedRun?.items.find(item =>
      "toolCall" in item && item.toolCall.operationId === "create_interaction_cards",
    )
    expect(generation && "toolCall" in generation ? generation.toolCall : undefined).toMatchObject({
      status: "failed",
      errorCode: "ai.interaction_card_schema_invalid",
      result: {
        summaryKey: "aiAssistant.cards.failed",
        errorCode: "ai.interaction_card_schema_invalid",
        attempt: 5,
        maxAttempts: 5,
      },
    })
    expect(JSON.stringify(generation)).toContain('"path":"cards"')
  })

  it("continues through multiple platform tool rounds before completing the run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "安装 PostgreSQL", undefined, "user")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "帮我在轻雪项目空间v2安装 PostgreSQL",
      pageContext: { routeName: "projects" },
      idempotencyKey: "multi-step-tool-loop",
      runActorGrantCiphertext: "encrypted-test-grant",
    })
    const catalog = ToolCatalog.load([
      {
        operationId: "listProjects", method: "GET", path: "/api/v1/projects", category: "project",
        risk: "read", requiredScopes: ["project:read"], approval: "never", idempotent: true, timeoutMs: 5000,
        inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
      },
      {
        operationId: "listApplications", method: "GET", path: "/api/v1/applications", category: "application",
        risk: "read", requiredScopes: ["application:read"], approval: "never", idempotent: true, timeoutMs: 5000,
        inputSchema: { type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"], additionalProperties: false },
      },
    ])
    const client = new DeterministicLunaApiClient(request => request.operation.operationId === "listProjects"
      ? { status: 200, body: { items: [{ id: "prj_liteyuki", name: "轻雪项目空间v2" }] } }
      : { status: 200, body: { items: [] } })
    const requests: ModelRequest[] = []
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        if (modelStep++ === 0) {
          yield { type: "message_delta", delta: "我先查找目标项目空间。" }
          yield {
            type: "completed",
            usage: { inputTokens: 10, outputTokens: 5 },
            toolCalls: [{ id: "call_projects", operationId: "listProjects", arguments: {} }],
          }
          return
        }
        if (modelStep === 2) {
          yield { type: "message_delta", delta: "已经找到项目空间，继续检查现有应用。" }
          yield {
            type: "completed",
            usage: { inputTokens: 20, outputTokens: 8 },
            toolCalls: [{ id: "call_apps", operationId: "listApplications", arguments: { projectId: "prj_liteyuki" } }],
          }
          return
        }
        yield { type: "message_delta", delta: "目标项目空间中没有现有 PostgreSQL 应用，可以继续选择模板和配置参数。" }
        yield { type: "completed", usage: { inputTokens: 30, outputTokens: 15 } }
      },
      async complete() {
        return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const tools = new ToolOrchestrator(
      catalog,
      client,
      new ProjectingToolCallStore(new MemoryToolCallStore(), repository),
      undefined,
      undefined,
      async () => "opaque-grant",
    )
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider, catalog.modelTools()),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "multi-step-worker" }),
      tools,
    )

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["listProjects", "listApplications"])
    expect(requests).toHaveLength(3)
    expect(requests[1]?.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({ role: "assistant", toolCalls: [expect.objectContaining({ id: "call_projects", operationId: "listProjects" })] }),
      expect.objectContaining({ role: "tool", toolCallId: "call_projects" }),
    ]))
    expect(requests[2]?.messages).toEqual(expect.arrayContaining([
      expect.objectContaining({ role: "tool", toolCallId: "call_projects" }),
      expect.objectContaining({ role: "tool", toolCallId: "call_apps" }),
    ]))
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    expect(timeline?.turns[0]?.selectedRun?.items.filter(item => item.type === "tool_call")).toHaveLength(2)
    expect(JSON.stringify(timeline)).toContain("目标项目空间中没有现有 PostgreSQL 应用")
  })

  it.each(["triggerBuildRun", "retryBuildRun"] as const)("returns actionable non-retry guidance when %s lacks a push credential", async (operationId) => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "源码构建", undefined, "user")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "从源码构建这个应用",
      pageContext: {},
      idempotencyKey: `build-missing-push-credential-${operationId}`,
      runActorGrantCiphertext: "encrypted-test-grant",
    })
    const toolArguments = operationId === "triggerBuildRun"
      ? {
          projectId: "prj_test",
          body: { targetRegistryId: "reg_test", sourceBranch: "main", dockerfilePath: "Dockerfile" },
        }
      : { projectId: "prj_test", runId: "build_original" }
    const catalog = ToolCatalog.load([{
      operationId,
      method: "POST",
      path: operationId === "triggerBuildRun"
        ? "/api/v1/projects/{projectId}/build-runs/trigger"
        : "/api/v1/projects/{projectId}/build-runs/{runId}/retry",
      category: "builds",
      risk: "write",
      requiredScopes: ["build:write"],
      approval: "never",
      idempotent: false,
      timeoutMs: 5000,
      inputSchema: {
        type: "object",
        properties: {
          projectId: { type: "string" },
          body: { type: "object", additionalProperties: true },
          runId: { type: "string" },
        },
        required: operationId === "triggerBuildRun" ? ["projectId", "body"] : ["projectId", "runId"],
        additionalProperties: false,
      },
    }])
    const client = new DeterministicLunaApiClient(() => ({
      status: 409,
      body: {
        code: "build.registry_push_credential_required",
        error: "Registry push credential required",
      },
    }))
    const requests: ModelRequest[] = []
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        if (modelStep++ === 0) {
          yield {
            type: "completed",
            usage: { inputTokens: 10, outputTokens: 5 },
            toolCalls: [{
              id: "build_action",
              operationId,
              arguments: toolArguments,
            }],
          }
          return
        }
        yield { type: "message_delta", delta: "目标镜像站缺少可用推送凭据，请先完成凭据配置。" }
        yield { type: "completed", usage: { inputTokens: 15, outputTokens: 8 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const tools = new ToolOrchestrator(
      catalog,
      client,
      new ProjectingToolCallStore(new MemoryToolCallStore(), repository),
      undefined,
      undefined,
      async () => "opaque-grant",
    )
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider, catalog.modelTools()),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "build-preflight-worker" }),
      tools,
    )

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(client.calls).toHaveLength(1)
    expect(client.calls[0]?.operation.operationId).toBe(operationId)
    const toolMessage = requests[1]?.messages.find(message => message.role === "tool" && message.toolCallId === "build_action")
    expect(toolMessage?.content).toContain('"errorCode":"build.registry_push_credential_required"')
    expect(toolMessage?.content).toContain('"retryable":false')
    expect(toolMessage?.content).toContain('"blocked":true')
    expect(toolMessage?.content).toContain('"workflowState":"blocked_on_registry_push_credential"')
    expect(toolMessage?.content).toContain('"requiredPreflightOperationId":"listRegistryCredentials"')
    expect(toolMessage?.content).toContain("同时传入本次构建的 projectId 与目标 registryId")
    expect(toolMessage?.content).toContain("不得复用其他项目空间的结果")
    expect(toolMessage?.content).toContain("不要再次调用 triggerBuildRun 或 retryBuildRun")
    expect(toolMessage?.content).toContain("不要修改分支、Dockerfile、构建上下文、镜像引用或 Tag")
  })

  it("exposes the full catalog from the first model step and executes the requested tool", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "公网入口", undefined, "user")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "继续处理刚才的事情",
      pageContext: { routeName: "applications" },
      idempotencyKey: "full-catalog-tool",
      runActorGrantCiphertext: "encrypted-test-grant",
    })
    const catalog = ToolCatalog.load([{
      operationId: "createGatewayRoute", method: "POST", path: "/api/v1/gateway-routes", category: "gateway",
      description: "创建公网网关路由。", risk: "write", requiredScopes: ["gateway:write"], approval: "never",
      idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    }])
    const requests: ModelRequest[] = []
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        if (modelStep++ === 0) {
          yield { type: "completed", usage: { inputTokens: 5, outputTokens: 2 }, toolCalls: [{ id: "gateway", operationId: "createGatewayRoute", arguments: {} }] }
          return
        }
        yield { type: "message_delta", delta: "公网入口已创建并完成回读。" }
        yield { type: "completed", usage: { inputTokens: 9, outputTokens: 5 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "gwr_test" } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(new MemoryToolCallStore(), repository), undefined, undefined, async () => "opaque-grant")
    const runtime = new ModelRuntime(provider, {
      resolve: (pageContext, input, loaded) => [...catalog.resolve(pageContext, input, loaded), searchToolsTool],
      search: (query, pageContext, limit) => catalog.search(query, pageContext, limit),
    })
    const executor = new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test", INSTANCE_ID: "full-catalog-worker" }), tools)

    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    expect(requests[0]?.tools?.map(tool => tool.operationId)).toContain("createGatewayRoute")
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["createGatewayRoute"])
  })

  it("persists an automatic registered-route action without invoking the business tool orchestrator", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "route", undefined, "user")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "Open the projects page",
      pageContext: { routeName: "dashboard" },
      idempotencyKey: "automatic-route",
      clientInstanceId: "executor-client-instance",
    })
    const requests: ModelRequest[] = []
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        yield { type: "message_delta", delta: "Opening the projects page." }
        yield {
          type: "completed",
          usage: { inputTokens: 10, outputTokens: 10 },
          toolCalls: [
            { operationId: "navigate_to_route", arguments: { routeName: "projects" } },
            {
              operationId: "create_options",
              arguments: {
                title: "Next",
                options: [
                  { id: "dashboard", label: "Dashboard", action: { type: "navigate", routeName: "dashboard" } },
                  { id: "explain", label: "Explain projects", action: { type: "send_message", message: "Explain project spaces" } },
                ],
              },
            },
          ],
        }
      },
      async complete() {
        return { text: "", usage: { inputTokens: 1, outputTokens: 1 } }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(
      repository,
      new ModelRuntime(provider, [createOptionsTool, navigateToRouteTool]),
      loadConfig({ NODE_ENV: "test", INSTANCE_ID: "route-worker" }),
    )

    await executor.runOnce()

    expect(requests[0]?.tools?.map(tool => tool.operationId)).toContain("navigate_to_route")
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const routeItem = timeline?.turns[0]?.selectedRun?.items.find(item => "toolCall" in item && item.toolCall.operationId === "navigate_to_route")
    expect(routeItem && "toolCall" in routeItem ? routeItem.toolCall.uiActions : undefined).toEqual([
      expect.objectContaining({
        type: "navigate",
        activation: "automatic",
        repeatable: false,
        payload: { routeName: "projects", params: {}, query: {} },
      }),
    ])
    const pendingActions = await repository.listPendingUIActions("usr_a", "executor-client-instance")
    expect(pendingActions).toHaveLength(1)
    expect(pendingActions[0]).toMatchObject({
      runId: timeline?.turns[0]?.selectedRun?.id,
      status: "pending",
    })
    expect(pendingActions[0]?.action).toMatchObject({
      type: "navigate",
      activation: "automatic",
    })
  })

  it("runs a required prediction phase when the main response omits create_options", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "Locked title", undefined, "user")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "你好",
      pageContext: { locale: "zh-CN", routeName: "dashboard" },
      idempotencyKey: "required-options",
    })
    const requests: ModelRequest[] = []
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        yield { type: "message_delta", delta: "你好，我可以帮你查看平台状态。" }
        yield { type: "completed", usage: { inputTokens: 10, outputTokens: 10 } }
      },
      async complete(request) {
        requests.push(request)
        return {
          text: "",
          usage: { inputTokens: 5, outputTokens: 5 },
          toolCalls: [{
            operationId: "create_options",
            arguments: {
              title: "你接下来可能想做",
              options: [
                { id: "projects", label: "查看项目空间", action: { type: "navigate", routeName: "projects" } },
                { id: "events", label: "检查平台事件", action: { type: "navigate", routeName: "events" } },
              ],
            },
          }],
        }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "required-options-worker" }))

    await executor.runOnce()

    expect(requests[1]?.toolChoice).toEqual({ operationId: "create_options" })
    expect(requests[1]?.tools?.map(tool => tool.operationId)).toEqual(["create_options"])
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const options = timeline?.turns[0]?.selectedRun?.items.find(item => "toolCall" in item && item.toolCall.operationId === "create_options")
    expect(options && "toolCall" in options ? options.toolCall.uiActions : undefined).toHaveLength(2)
  })

  it("omits suggestions when the model cannot produce context-specific options", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "Locked title", undefined, "user")
    await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "在轻雪项目空间v2部署 PostgreSQL",
      pageContext: {
        locale: "zh-CN",
        routeName: "project.workspace",
        projectId: "prj_liteyuki",
      },
      idempotencyKey: "invalid-options",
    })
    const provider: ModelProvider = {
      async *stream() {
        yield { type: "message_delta", delta: "已找到目标项目空间。" }
        yield { type: "completed", usage: { inputTokens: 10, outputTokens: 10 } }
      },
      async complete() {
        return {
          text: "",
          usage: { inputTokens: 5, outputTokens: 5 },
          toolCalls: [],
        }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const executor = new RunExecutor(repository, new ModelRuntime(provider), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "invalid-options-worker" }))

    await executor.runOnce()

    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    const options = timeline?.turns[0]?.selectedRun?.items.find(item => "toolCall" in item && item.toolCall.operationId === "create_options")
    expect(options).toBeUndefined()
  })

  it("executes a deterministic read tool and completes the same durable run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "tool loop")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: 'tool:getBuildRun {"buildId":"build_a"}',
      pageContext: {}, idempotencyKey: "tool-loop-request", runActorGrantCiphertext: "encrypted-test-grant",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getBuildRun", method: "GET", path: "/api/v1/builds", category: "build",
      risk: "read", requiredScopes: ["build:read"], approval: "never", idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: { buildId: { type: "string" } }, required: ["buildId"], additionalProperties: false },
    }])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "build_a", status: "failed" } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(new MemoryToolCallStore(), repository), undefined, undefined, async () => "opaque-grant")
    const config = loadConfig({ NODE_ENV: "test", INSTANCE_ID: "test-worker" })
    const executor = new RunExecutor(repository, new ModelRuntime(new DeterministicProvider()), config, tools)
    expect(await executor.runOnce()).toBe(true)
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    const timeline = await repository.getTimeline("usr_a", conversation.id)
    expect(timeline?.turns[0]?.items.some(item => item.type === "tool_result")).toBe(true)
    expect(timeline?.turns[0]?.items.some(item => item.type === "assistant_message")).toBe(true)
    const presented = await presentTimeline(repository, "usr_a", conversation.id)
    const toolItem = presented?.turns[0]?.selectedRun?.items.find(item => "toolCall" in item && item.toolCall.operationId === "getBuildRun")
    expect(toolItem && "toolCall" in toolItem ? toolItem.toolCall.durationMs : undefined).toEqual(expect.any(Number))
    expect(client.calls[0]?.runActorGrant).toBe("opaque-grant")
  })
  it("persists approval and MFA interruptions, then resumes the same run", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "restart")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: 'tool:restartRelease {"releaseId":"rel_a"}',
      pageContext: {}, idempotencyKey: "approval-loop", runActorGrantCiphertext: "encrypted",
    })
    const catalog = ToolCatalog.load([{
      operationId: "restartRelease", method: "POST", path: "/api/v1/releases/restart", category: "deployment",
      risk: "destructive", requiredScopes: ["deployment:write"], approval: "always", stepUpPurpose: "deployment_restart",
      idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: { releaseId: { type: "string" } }, required: ["releaseId"], additionalProperties: false },
    }])
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } })), new ProjectingToolCallStore(store, repository), undefined, undefined, async () => "grant")
    const executor = new RunExecutor(repository, new ModelRuntime(new DeterministicProvider()), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "approval-worker" }), tools)
    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("waiting_approval")
    const pending = [...store.records.values()][0]!
    const mfa = await tools.approve(pending.id, pending.argumentsHash, pending.rowVersion)
    await repository.updateRun(created.run.id, "waiting_approval", "waiting_mfa")
    expect(mfa.status).toBe("awaiting_mfa")
    await tools.resumeMfa(mfa.id, "deployment_restart", mfa.rowVersion, "mfa_assertion_1")
    await repository.updateRun(created.run.id, "waiting_mfa", "queued")
    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
  })

  it("moves missing arguments to waiting_input and resumes with supplied input", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "input")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "tool:getBuildRun {}", pageContext: {}, idempotencyKey: "input-loop",
    })
    const catalog = ToolCatalog.load([{
      operationId: "getBuildRun", method: "GET", path: "/api/v1/builds", category: "build",
      risk: "read", requiredScopes: ["build:read"], approval: "never", idempotent: true, timeoutMs: 5000,
      inputSchema: { type: "object", properties: { buildId: { type: "string" } }, required: ["buildId"], additionalProperties: false },
    }])
    const tools = new ToolOrchestrator(catalog, new DeterministicLunaApiClient(() => ({ status: 200, body: { id: "build_a" } })), new ProjectingToolCallStore(new MemoryToolCallStore(), repository), undefined, undefined, async () => "grant")
    const executor = new RunExecutor(repository, new ModelRuntime(new DeterministicProvider()), loadConfig({ NODE_ENV: "test", INSTANCE_ID: "input-worker" }), tools)
    await executor.runOnce()
    const waiting = await repository.getRun("usr_a", created.run.id)
    expect(waiting?.status).toBe("waiting_input")
    await repository.appendRunInput(created.run.id, 'tool:getBuildRun {"buildId":"build_a"}')
    await repository.updateRun(created.run.id, "waiting_input", "queued")
    await executor.runOnce()
    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
  })

  it("feeds generated secrets to the model while persisting masked results", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "secret")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id, input: "部署应用并生成 JWT 密钥", pageContext: {}, idempotencyKey: "secret-loop",
    })
    const catalog = ToolCatalog.load([{
      operationId: "generateSecret", method: "GET", path: "/api/v1/ai-tools/generateSecret", category: "secret",
      risk: "read", requiredScopes: ["secret:generate"], approval: "never", idempotent: true, timeoutMs: 5000,
      inputSchema: {
        type: "object",
        properties: { length: { type: "integer" }, encoding: { type: "string" }, count: { type: "integer" } },
        required: [], additionalProperties: false,
      },
    }])
    const generated = "s3cretValu3_forJwt"
    const client = new DeterministicLunaApiClient(() => ({
      status: 200,
      body: { secrets: [generated], encoding: "alphanumeric", length: generated.length },
    }))
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(store, repository), undefined, undefined, async () => "grant")
    const requests: ModelRequest[] = []
    let modelStep = 0
    const provider: ModelProvider = {
      async *stream(request) {
        requests.push(request)
        if (modelStep++ === 0) {
          yield { type: "completed", usage: { inputTokens: 5, outputTokens: 2 }, toolCalls: [{ id: "secret", operationId: "generateSecret", arguments: {} }] }
          return
        }
        yield { type: "message_delta", delta: "已生成密钥并继续部署。" }
        yield { type: "completed", usage: { inputTokens: 9, outputTokens: 5 } }
      },
      async complete() { return { text: "", usage: { inputTokens: 1, outputTokens: 0 }, toolCalls: [] } },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }
    const runtime = new ModelRuntime(provider, {
      resolve: (pageContext, input, loaded) => [...catalog.resolve(pageContext, input, loaded), searchToolsTool],
      search: (query, pageContext, limit) => catalog.search(query, pageContext, limit),
    })
    const executor = new RunExecutor(repository, runtime, loadConfig({ NODE_ENV: "test", INSTANCE_ID: "secret-worker" }), tools)
    await executor.runOnce()

    expect((await repository.getRun("usr_a", created.run.id))?.status).toBe("completed")
    // 模型可见的工具结果包含明文生成值
    const toolMessage = requests[1]?.messages.find(message => message.role === "tool" && message.toolCallId === "secret")
    expect(String(toolMessage?.content)).toContain(generated)
    // 持久化投影不包含明文，遥测 redact 后按等长 * 掩码
    const persisted = [...store.records.values()][0]!
    expect(JSON.stringify(persisted.result)).not.toContain(generated)
    expect(JSON.stringify(persisted.result)).toContain("*".repeat(generated.length))
    const timeline = await presentTimeline(repository, "usr_a", conversation.id)
    expect(JSON.stringify(timeline)).not.toContain(generated)
  })

  it("passes a submitted secret only to the real tool client and redacts projections", async () => {
    const repository = new MemoryRepository()
    const conversation = await repository.createConversation("usr_a", "配置密码")
    const created = await repository.createTurn("usr_a", {
      conversationId: conversation.id,
      input: "提交配置",
      pageContext: {},
      idempotencyKey: "submitted-secret-tool",
      runActorGrantCiphertext: "encrypted-test-grant",
    })
    const catalog = ToolCatalog.load([{
      operationId: "saveConfig", method: "POST", path: "/api/v1/config", category: "application",
      risk: "write", requiredScopes: ["application:write"], approval: "never", idempotent: true, timeoutMs: 5000,
      inputSchema: {
        type: "object",
        properties: { environment: { type: "array" } },
        required: ["environment"], additionalProperties: false,
      },
    }])
    const submitted = "database-password"
    const client = new DeterministicLunaApiClient(() => ({
      status: 200,
      body: { accepted: true },
    }))
    const store = new MemoryToolCallStore()
    const tools = new ToolOrchestrator(
      catalog,
      client,
      new ProjectingToolCallStore(store, repository),
      undefined,
      undefined,
      async () => "opaque-grant",
    )

    const call = await tools.propose({
      runId: created.run.id,
      operationId: "saveConfig",
      arguments: { environment: [{ key: "DATABASE_PASSWORD", value: submitted }] },
    })

    expect(call.status).toBe("succeeded")
    expect(client.calls[0]?.arguments).toEqual({ environment: [{ key: "DATABASE_PASSWORD", value: submitted }] })
    expect(JSON.stringify(store.events)).not.toContain(submitted)
    expect(JSON.stringify(await presentTimeline(repository, "usr_a", conversation.id))).not.toContain(submitted)
  })
})

function resolveNever(): never {
  throw new Error("unreachable")
}
