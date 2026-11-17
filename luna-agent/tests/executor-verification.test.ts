import { describe, expect, it } from "vitest"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelMessage, ModelProvider, ModelToolCall } from "../src/provider/provider.js"
import { presentTimeline } from "../src/timeline-presenter.js"
import { businessCardTools } from "../src/tools/business-card-tools.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { waitForTaskTool, type TaskStatusCheckResult, type TaskStatusClient } from "../src/tools/wait-for-task.js"
import { defaultRuntimeSettings } from "../src/runtime-settings.js"
import { TestRepository } from "./support/test-repository.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 12, outputTokens: 6, totalTokens: 18 } }

function provider(steps: Array<{ text?: string, reasoning?: string, toolCalls?: ModelToolCall[] }>) {
  let index = 0
  const requests: ModelMessage[][] = []
  const value: ModelProvider = {
    async *stream(request) {
      requests.push(request.messages)
      const step = steps[Math.min(index, steps.length - 1)] ?? {}
      index += 1
      if (step.reasoning) yield { type: "reasoning_summary_delta", delta: step.reasoning }
      if (step.text) yield { type: "message_delta", delta: step.text }
      if (step.toolCalls?.length) yield { type: "tool_call_delta" }
      yield {
        type: "completed",
        usage: reportedUsage,
        ...(step.toolCalls?.length ? { toolCalls: step.toolCalls } : {}),
      }
    },
    async complete() { return { text: "", toolCalls: [], usage: reportedUsage } },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return { value, calls: () => index, requests }
}

function deliveryCatalog() {
  return ToolCatalog.load([
    testToolOperation("triggerBuildRun", {
      name: "触发构建",
      summary: "提交构建任务。",
      method: "POST",
      path: "/api/v1/projects/{projectId}/build-runs/trigger",
      effect: "async",
      verification: [{ operationId: "getBuildRun", family: "build_run" }],
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getBuildRun", {
      name: "读取构建",
      summary: "读取构建状态。",
      method: "GET",
      path: "/api/v1/projects/{projectId}/build-runs/{runId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "runId", wireName: "runId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, runId: { type: "string" } },
        required: ["projectId", "runId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getBuildJobLogs", {
      name: "读取构建日志",
      summary: "读取失败构建的日志。",
      method: "GET",
      path: "/api/v1/projects/{projectId}/build-runs/{runId}/logs",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "runId", wireName: "runId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, runId: { type: "string" } },
        required: ["projectId", "runId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("createRelease", {
      name: "创建发布",
      summary: "创建应用发布任务。",
      method: "POST",
      path: "/api/v1/projects/{projectId}/releases",
      effect: "async",
      verification: [{ operationId: "getRelease", family: "release" }],
      parameters: [{ inputName: "projectId", wireName: "projectId", in: "path", required: true }],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "body"],
        additionalProperties: false,
      },
    }),
    testToolOperation("getRelease", {
      name: "读取发布",
      summary: "读取发布状态。",
      method: "GET",
      path: "/api/v1/projects/{projectId}/releases/{releaseId}",
      effect: "read",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "releaseId", wireName: "releaseId", in: "path", required: true },
      ],
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, releaseId: { type: "string" } },
        required: ["projectId", "releaseId"],
        additionalProperties: false,
      },
    }),
    testToolOperation("execReleaseRuntimeCommand", {
      name: "执行容器命令",
      summary: "在运行容器内执行诊断命令。",
      method: "POST",
      path: "/api/v1/projects/{projectId}/releases/{releaseId}/exec",
      effect: "execute",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "releaseId", wireName: "releaseId", in: "path", required: true },
      ],
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { projectId: { type: "string" }, releaseId: { type: "string" }, body: { type: "object" } },
        required: ["projectId", "releaseId", "body"],
        additionalProperties: false,
      },
    }),
  ])
}

function unverifiedWriteCatalog() {
  return ToolCatalog.load([
    testToolOperation("updateThing", {
      name: "更新资源",
      summary: "更新普通平台资源。",
      method: "POST",
      path: "/api/v1/things",
      effect: "write",
      requestBody: true,
      requestRequired: true,
      inputSchema: {
        type: "object",
        properties: { body: { type: "object" } },
        required: ["body"],
        additionalProperties: false,
      },
    }),
  ])
}

async function queuedRun(owner: string, key: string, input = "构建应用") {
  const repository = new TestRepository()
  const conversation = await repository.createConversation(owner, key)
  const created = await repository.createTurn(owner, {
    conversationId: conversation.id,
    input,
    pageContext: { projectId: "prj_1" },
    idempotencyKey: key,
    actorSessionId: `ses_${key}`,
  })
  return { repository, conversation, runId: created.run.id }
}

function liveTaskCardArguments(projectId: string, buildRunId: string) {
  return {
    schemaVersion: 1,
    title: "已提交构建任务",
    mode: "presentation",
    template: "live_task",
    cards: [{
      id: "build_progress",
      presentation: { variant: "panel", title: "构建实时进度" },
      blocks: [{
        id: "progress_1",
        type: "live_progress",
        label: `构建 ${buildRunId}`,
        binding: {
          projectId,
          operationType: "build_run",
          operationId: buildRunId,
        },
      }],
    }],
  }
}

describe("Dual-mode async tasks & completionBlockers removal", () => {
  it("completes single-step triggerBuildRun + live_task card immediately without blocking polling", async () => {
    const owner = "usr_single_step_build"
    const { repository, conversation, runId } = await queuedRun(owner, "single-step-build", "帮我触发构建")
    const catalog = deliveryCatalog()
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun") {
        return { status: 201, body: { id: "bldr_live_1", status: "queued" } }
      }
      throw new Error(`unexpected blocking poll: ${request.operation.operationId}`)
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      {
        toolCalls: [{
          id: "call_trigger",
          operationId: "triggerBuildRun",
          arguments: { projectId: "prj_1", body: { applicationId: "app_1" } },
        }],
      },
      {
        toolCalls: [{
          id: "call_card",
          operationId: "present_card",
          arguments: liveTaskCardArguments("prj_1", "bldr_live_1"),
        }],
      },
      { text: "构建已提交，可在上方实时进度卡片查看状态喵～" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: () => [...catalog.modelTools(["triggerBuildRun"]), ...businessCardTools, waitForTaskTool],
    })), { catalog, tools })

    await executor.runOnce()

    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["triggerBuildRun"])
    const timeline = await presentTimeline(repository, owner, conversation.id)
    expect(timeline?.turns[0]?.selectedRun?.status).toBe("completed")
  })

  it("suspends multi-step wait_for_task into waiting_external, releases concurrency slots, avoids timeline spam, and resumes on succeeded", async () => {
    const owner = "usr_wait_succeeded"
    const { repository, conversation, runId } = await queuedRun(owner, "wait-succeeded", "构建成功后自动部署到 dev")
    const catalog = deliveryCatalog()
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun") {
        return { status: 201, body: { id: "bldr_chain_1", status: "queued" } }
      }
      if (request.operation.operationId === "createRelease") {
        return { status: 201, body: { id: "rel_chain_1", status: "pending" } }
      }
      return { status: 200, body: {} }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))

    const pollResponses: TaskStatusCheckResult[] = [
      {
        kind: "ok",
        snapshot: {
          taskType: "build_run",
          taskId: "bldr_chain_1",
          projectId: "prj_1",
          applicationId: "app_1",
          rawStatus: "running",
          taskStatus: "running",
          terminal: false,
          updatedAt: "2026-11-17T08:00:01Z",
        },
      },
      {
        kind: "ok",
        snapshot: {
          taskType: "build_run",
          taskId: "bldr_chain_1",
          projectId: "prj_1",
          applicationId: "app_1",
          rawStatus: "running",
          taskStatus: "running",
          terminal: false,
          updatedAt: "2026-11-17T08:00:05Z",
        },
      },
      {
        kind: "ok",
        snapshot: {
          taskType: "build_run",
          taskId: "bldr_chain_1",
          projectId: "prj_1",
          applicationId: "app_1",
          rawStatus: "succeeded",
          taskStatus: "succeeded",
          terminal: true,
          imageRef: "registry.example.com/app:sha-123",
          updatedAt: "2026-11-17T08:00:12Z",
        },
      },
    ]
    let statusChecks = 0
    const taskStatusClient: TaskStatusClient = {
      async checkTaskStatus() {
        const next = pollResponses[Math.min(statusChecks, pollResponses.length - 1)]!
        statusChecks += 1
        return next
      },
    }

    const model = provider([
      {
        reasoning: "先触发构建，再挂起等待构建完成。",
        toolCalls: [
          { id: "call_build", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } },
        ],
      },
      {
        reasoning: "构建已创建，进入持久化等待。",
        toolCalls: [
          { id: "call_wait", operationId: "wait_for_task", arguments: { projectId: "prj_1", taskType: "build_run", taskId: "bldr_chain_1", timeoutSeconds: 600 } },
        ],
      },
      {
        reasoning: "构建已成功拿到镜像，继续创建 dev 发布。",
        toolCalls: [
          { id: "call_release", operationId: "createRelease", arguments: { projectId: "prj_1", body: { deploymentTargetId: "dplt_dev", image: "registry.example.com/app:sha-123" } } },
        ],
      },
      { text: "构建已完成并已提交 dev 发布（rel_chain_1）。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: () => [...catalog.modelTools(["triggerBuildRun", "createRelease"]), waitForTaskTool],
    })), {
      catalog,
      tools,
      taskStatusClient,
      initialRuntimeSettings: { ...defaultRuntimeSettings, userConcurrentRuns: 1 },
    })

    // 第一阶段：触发构建并调用 wait_for_task，立即挂起退出释放槽位
    await executor.runOnce()
    const suspendedRun = await repository.getRun(owner, runId)
    expect(suspendedRun?.status).toBe("waiting_external")
    expect(suspendedRun?.wakeAt).toBeDefined()
    expect(await repository.countActiveUserRuns(owner)).toBe(0)

    // 验证 wait_for_task 在时间线上默认对用户可见（visibility: "normal"）
    const timelineWhileWaiting = await presentTimeline(repository, owner, conversation.id)
    expect(timelineWhileWaiting?.turns[0]?.selectedRun?.status).toBe("waiting_external")
    const waitItem = timelineWhileWaiting?.turns[0]?.selectedRun?.items.find(
      (item): item is Extract<typeof item, { toolCall: unknown }> => "toolCall" in item && item.toolCall?.operationId === "wait_for_task",
    )
    expect(waitItem?.toolCall).toMatchObject({
      operationId: "wait_for_task",
      visibility: "normal",
      status: "running",
      arguments: { projectId: "prj_1", taskType: "build_run", taskId: "bldr_chain_1", timeoutSeconds: 600 },
    })

    // 同一用户在 userConcurrentRuns=1 下发起的新会话 Run 可立即借用释放出的槽位执行完毕
    const secondConv = await repository.createConversation(owner, "other-conv")
    const secondTurn = await repository.createTurn(owner, {
      conversationId: secondConv.id,
      input: "顺便回答一个简单问题",
      pageContext: {},
      idempotencyKey: "concurrent-while-waiting",
      actorSessionId: "ses_concurrent",
    })
    const itemsBeforePoll = (await repository.getTimeline(owner, conversation.id))?.turns[0]?.items.length ?? 0

    // 推进第一轮到期检查（仍在 running）：只更新 wake_at，零新增时间线条目，且顺便完成第二个并发会话！
    const firstCheckpoint = await repository.loadRunCheckpoint(runId)
    await repository.rescheduleWaitingExternalRun(runId, firstCheckpoint!.revision, 0, new Date(Date.now() - 1_000).toISOString())
    await executor.runOnce()
    expect((await repository.getRun(owner, secondTurn.run.id))?.status).toBe("completed")
    expect((await repository.getRun(owner, runId))?.status).toBe("waiting_external")
    const itemsAfterRunningPoll = (await repository.getTimeline(owner, conversation.id))?.turns[0]?.items.length ?? 0
    expect(itemsAfterRunningPoll).toBe(itemsBeforePoll)

    // 推进第二轮到期检查（到达 succeeded 终态）：唤醒回 queued 并自动执行后续 createRelease 收口
    const secondCheckpoint = await repository.loadRunCheckpoint(runId)
    await repository.rescheduleWaitingExternalRun(runId, secondCheckpoint!.revision, 1, new Date(Date.now() - 1_000).toISOString())
    await executor.runOnce()

    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["triggerBuildRun", "createRelease"])
    const finalTimeline = await presentTimeline(repository, owner, conversation.id)
    const completedWaitItem = finalTimeline?.turns[0]?.selectedRun?.items.find(
      (item): item is Extract<typeof item, { toolCall: unknown }> => "toolCall" in item && item.toolCall?.operationId === "wait_for_task",
    )
    expect(completedWaitItem?.toolCall?.status).toBe("succeeded")
    expect(completedWaitItem?.toolCall?.result?.data).toMatchObject({
      taskStatus: "succeeded",
      imageRef: "registry.example.com/app:sha-123",
      timedOut: false,
    })
  })

  it("wakes Run normally when target build reaches failed so the model can read logs and diagnose without crashing the Run", async () => {
    const owner = "usr_wait_failed_diagnose"
    const { repository, runId } = await queuedRun(owner, "wait-failed-diagnose", "触发构建，如果失败直接帮我查日志分析原因")
    const catalog = deliveryCatalog()
    const client = new DeterministicLunaApiClient((request) => {
      if (request.operation.operationId === "triggerBuildRun") {
        return { status: 201, body: { id: "bldr_fail_1", status: "queued" } }
      }
      if (request.operation.operationId === "getBuildJobLogs") {
        return { status: 200, body: { log: "ERROR: npm ERR! Missing script: build" } }
      }
      return { status: 200, body: {} }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))

    let checks = 0
    const taskStatusClient: TaskStatusClient = {
      async checkTaskStatus() {
        checks += 1
        if (checks === 1) {
          return {
            kind: "ok",
            snapshot: {
              taskType: "build_run",
              taskId: "bldr_fail_1",
              projectId: "prj_1",
              rawStatus: "running",
              taskStatus: "running",
              terminal: false,
              updatedAt: "2026-11-17T08:10:00Z",
            },
          }
        }
        return {
          kind: "ok",
          snapshot: {
            taskType: "build_run",
            taskId: "bldr_fail_1",
            projectId: "prj_1",
            rawStatus: "failed",
            taskStatus: "failed",
            terminal: true,
            failureCode: "build.dockerfile_step_failed",
            updatedAt: "2026-11-17T08:10:20Z",
          },
        }
      },
    }

    const model = provider([
      {
        toolCalls: [
          { id: "c1", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } },
          { id: "c2", operationId: "wait_for_task", arguments: { projectId: "prj_1", taskType: "build_run", taskId: "bldr_fail_1" } },
        ],
      },
      {
        toolCalls: [
          { id: "c3", operationId: "getBuildJobLogs", arguments: { projectId: "prj_1", runId: "bldr_fail_1" } },
        ],
      },
      { text: "构建失败，日志显示缺少 npm build 脚本（Missing script: build），请检查 package.json。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: () => [...catalog.modelTools(["triggerBuildRun", "getBuildJobLogs"]), waitForTaskTool],
    })), { catalog, tools, taskStatusClient })

    await executor.runOnce()
    expect((await repository.getRun(owner, runId))?.status).toBe("waiting_external")

    const cp = await repository.loadRunCheckpoint(runId)
    await repository.rescheduleWaitingExternalRun(runId, cp!.revision, 0, new Date(Date.now() - 1_000).toISOString())
    await executor.runOnce()

    // 关键断言：构建失败时绝不炸毁 Run，模型收到 taskStatus: "failed" 后成功调用日志工具并正常 completed！
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    expect(client.calls.map(call => call.operation.operationId)).toEqual(["triggerBuildRun", "getBuildJobLogs"])
    const waitResultMsg = model.requests[1]?.find(msg => msg.role === "tool" && msg.toolCallId === "c2")
    expect(waitResultMsg?.content).toContain('"taskStatus":"failed"')
    expect(waitResultMsg?.content).toContain("build.dockerfile_step_failed")
  })

  it("cancels immediately when user clicks Stop during waiting_external", async () => {
    const owner = "usr_wait_cancel"
    const { repository, conversation, runId } = await queuedRun(owner, "wait-cancel", "构建完成后部署")
    const catalog = deliveryCatalog()
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 201, body: { id: "bldr_cancel_wait", status: "queued" } })),
      new ProjectingToolCallStore(repository.toolCallStore, repository),
    )
    const taskStatusClient: TaskStatusClient = {
      async checkTaskStatus() {
        return {
          kind: "ok",
          snapshot: {
            taskType: "build_run",
            taskId: "bldr_cancel_wait",
            projectId: "prj_1",
            rawStatus: "running",
            taskStatus: "running",
            terminal: false,
            updatedAt: "2026-11-17T08:20:00Z",
          },
        }
      },
    }
    const model = provider([
      {
        toolCalls: [
          { id: "w1", operationId: "wait_for_task", arguments: { projectId: "prj_1", taskType: "build_run", taskId: "bldr_cancel_wait" } },
        ],
      },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: () => [waitForTaskTool],
    })), { catalog, tools, taskStatusClient })

    await executor.runOnce()
    expect((await repository.getRun(owner, runId))?.status).toBe("waiting_external")

    const canceled = await repository.cancelRun(owner, runId)
    expect(canceled?.status).toBe("canceled")
    expect(canceled?.wakeAt).toBeUndefined()
    const timeline = await presentTimeline(repository, owner, conversation.id)
    const waitItem = timeline?.turns[0]?.selectedRun?.items.find(
      (item): item is Extract<typeof item, { toolCall: unknown }> => "toolCall" in item && item.toolCall?.operationId === "wait_for_task",
    )
    expect(waitItem?.toolCall?.status).toBe("canceled")
  })

  it("allows honest completion after 403/409 write errors, 400 retry with fixed args, and non-zero exec command exit", async () => {
    const catalog = deliveryCatalog()

    // 1) 403 / 409 错误后模型如实向用户汇报并正常 completed（不触发 completionBlockers 死锁）
    {
      const owner = "usr_409_honest"
      const { repository, runId } = await queuedRun(owner, "conflict-honest")
      const client = new DeterministicLunaApiClient(() => ({
        status: 409,
        body: { code: "resource.conflict", retryable: false },
      }))
      const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
      const model = provider([
        { toolCalls: [{ id: "call_409", operationId: "createRelease", arguments: { projectId: "prj_1", body: { target: "dev" } } }] },
        { text: "当前部署目标已有进行中的发布（409 冲突），请稍后再试。" },
      ])
      const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
        resolve: () => catalog.modelTools(["createRelease"]),
      })), { catalog, tools })
      await executor.runOnce()
      expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    }

    // 2) 首次 400 参数错误，模型修正参数重试成功后正常 completed
    {
      const owner = "usr_400_retry_fixed"
      const { repository, runId } = await queuedRun(owner, "bad-request-fixed")
      const client = new DeterministicLunaApiClient((request) => {
        const body = request.arguments.body as Record<string, unknown> | undefined
        if (body?.tag === "bad tag!") {
          return { status: 400, body: { code: "request.invalid", retryable: false } }
        }
        return { status: 201, body: { id: "bldr_fixed", status: "queued" } }
      })
      const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
      const model = provider([
        { toolCalls: [{ id: "try_1", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { tag: "bad tag!" } } }] },
        { toolCalls: [{ id: "try_2", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { tag: "v1.0.0" } } }] },
        { text: "已修正镜像标签并成功触发构建（bldr_fixed）。" },
      ])
      const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
        resolve: () => catalog.modelTools(["triggerBuildRun"]),
      })), { catalog, tools })
      await executor.runOnce()
      expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
      expect(client.calls).toHaveLength(2)
    }

    // 3) 容器命令（effect: "execute"）返回非 0 退出码 / 错误状态时可正常总结完成
    {
      const owner = "usr_exec_nonzero"
      const { repository, runId } = await queuedRun(owner, "exec-nonzero")
      const client = new DeterministicLunaApiClient(() => ({
        status: 422,
        body: { code: "runtime.command_exit_nonzero", exitCode: 1, stdout: "", stderr: "no match" },
      }))
      const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
      const model = provider([
        { toolCalls: [{ id: "exec_1", operationId: "execReleaseRuntimeCommand", arguments: { projectId: "prj_1", releaseId: "rel_1", body: { command: ["grep", "ERROR", "/var/log/app.log"] } } }] },
        { text: "日志中未检索到 ERROR 关键字（grep 退出码为 1）。" },
      ])
      const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
        resolve: () => catalog.modelTools(["execReleaseRuntimeCommand"]),
      })), { catalog, tools })
      await executor.runOnce()
      expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    }
  })

  it("reuses one logical idempotency key when retrying an identical transient-failed write and rejects unverified writes", async () => {
    const catalog = deliveryCatalog()
    const { repository, runId } = await queuedRun("usr_write_model_retry", "write-model-retry")
    let triggerAttempts = 0
    const client = new DeterministicLunaApiClient(() => {
      triggerAttempts += 1
      return triggerAttempts === 1
        ? { status: 500, body: { code: "service.unavailable", retryable: true } }
        : { status: 201, body: { id: "bldr_retry", status: "queued" } }
    })
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const model = provider([
      { toolCalls: [{ id: "build-retry-1", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { toolCalls: [{ id: "build-retry-2", operationId: "triggerBuildRun", arguments: { projectId: "prj_1", body: { applicationId: "app_1" } } }] },
      { text: "重试后已成功提交构建。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: () => catalog.modelTools(["triggerBuildRun"]),
    })), { catalog, tools })

    await executor.runOnce()
    expect(await repository.getRun("usr_write_model_retry", runId)).toMatchObject({ status: "completed" })
    expect(client.calls).toHaveLength(2)
    expect(client.calls[1]?.idempotencyKey).toBe(client.calls[0]?.idempotencyKey)

    // 未声明 verification 元数据的未准入写操作仍由目录守卫拦截
    const unverified = await queuedRun("usr_write_unverified", "write-unverified")
    const unverifiedCatalog = unverifiedWriteCatalog()
    const unverifiedTools = new ToolOrchestrator(
      unverifiedCatalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: {} })),
      new ProjectingToolCallStore(unverified.repository.toolCallStore, unverified.repository),
    )
    const unverifiedModel = provider([
      { toolCalls: [{ id: "u1", operationId: "updateThing", arguments: { body: { name: "next" } } }] },
    ])
    const unverifiedExecutor = testExecutor(unverified.repository, new ModelRuntime(unverifiedModel.value, testRegistry()), {
      catalog: unverifiedCatalog,
      tools: unverifiedTools,
    })
    await unverifiedExecutor.runOnce()
    expect(await unverified.repository.getRun("usr_write_unverified", unverified.runId)).toMatchObject({
      status: "failed",
      errorCode: "ai.tool_not_available",
    })
  })

  it("[P1-1] fails wait_for_task with ai.progress.unavailable on deadline transient_error without fabricating running status", async () => {
    const catalog = deliveryCatalog()
    const owner = "usr_wait_unavailable"
    const { repository, conversation, runId } = await queuedRun(owner, "wait-unavailable")
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: {} })),
      new ProjectingToolCallStore(repository.toolCallStore, repository),
    )
    const taskStatusClient: TaskStatusClient = {
      async checkTaskStatus(): Promise<TaskStatusCheckResult> {
        return { kind: "transient_error", errorCode: "ai.progress.unavailable" }
      },
    }
    const model = provider([
      {
        toolCalls: [{
          id: "wait_unavail_1",
          operationId: "wait_for_task",
          arguments: { projectId: "prj_1", taskType: "build_run", taskId: "bldr_unavail", timeoutSeconds: 15 },
        }],
      },
      { text: "后台任务状态暂时不可达（unavailable），请稍后查看构建详情。" },
    ])
    const executor = testExecutor(repository, new ModelRuntime(model.value, testRegistry({
      resolve: () => [waitForTaskTool],
    })), { catalog, tools, taskStatusClient })

    await executor.runOnce()
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "waiting_external" })
    // 首次快检 transient_error 时不得写入伪造的 taskStatus: "running"
    const initialTimeline = await presentTimeline(repository, owner, conversation.id)
    const initialWaitCall = initialTimeline?.turns[0]?.selectedRun?.items.flatMap(item => "toolCall" in item ? [item.toolCall] : [])[0]
    expect(initialWaitCall?.result?.data).toMatchObject({ observationCode: "ai.progress.unavailable" })
    expect((initialWaitCall?.result?.data as Record<string, unknown> | undefined)?.taskStatus).toBeUndefined()

    // 推进 checkpoint deadline 使其立即到期，再次遇到 transient_error 时必须 failed 且不含伪造 running
    const cp = await repository.loadRunCheckpoint(runId)
    if (!cp?.state.waitingTask) throw new Error("missing waitingTask checkpoint")
    await repository.saveRunCheckpoint(runId, {
      expectedRevision: cp.revision,
      phase: "waiting_external",
      modelStep: cp.modelStep,
      toolCallCount: cp.toolCallCount,
      state: {
        ...cp.state,
        waitingTask: {
          ...cp.state.waitingTask,
          deadlineAt: new Date(Date.now() - 1_000).toISOString(),
        },
      },
    })
    await repository.updateRun(runId, "waiting_external", "waiting_external", {
      wakeAt: new Date(Date.now() - 1_000).toISOString(),
    })

    await executor.runOnce()
    expect(await repository.getRun(owner, runId)).toMatchObject({ status: "completed" })
    const finalTimeline = await presentTimeline(repository, owner, conversation.id)
    const finalWaitCall = finalTimeline?.turns[0]?.selectedRun?.items.flatMap(item => "toolCall" in item ? [item.toolCall] : [])[0]
    expect(finalWaitCall).toMatchObject({
      status: "failed",
      errorCode: "ai.progress.unavailable",
    })
    expect((finalWaitCall?.result?.data as Record<string, unknown> | undefined)?.taskStatus).toBeUndefined()
    expect(model.requests[1]?.some(m => m.role === "tool" && m.content.includes("\"errorCode\":\"ai.progress.unavailable\"") && !m.content.includes("\"taskStatus\":\"running\""))).toBe(true)
  })
})
