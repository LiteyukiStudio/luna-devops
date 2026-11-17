import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import { agentMetrics, clientSpanOptions, recordSpanError, withSpan } from "../telemetry.js"

export const waitForTaskTypes = ["build_run", "release"] as const
export type WaitForTaskType = typeof waitForTaskTypes[number]

export const waitForTaskInput = z.object({
  projectId: z.string().trim().min(1).max(64),
  taskType: z.enum(waitForTaskTypes),
  taskId: z.string().trim().min(1).max(64),
  timeoutSeconds: z.number().int().min(15).max(1800).default(900),
}).strict()

export type WaitForTaskInput = z.infer<typeof waitForTaskInput>

export const waitForTaskTool: ModelToolDefinition = {
  operationId: "wait_for_task",
  description: "在多步串行编排中持久化挂起当前执行并等待指定构建（build_run）或发布（release）到达终态。挂起期间释放并发槽位且零 Token 轮询；无论目标任务 succeeded、failed 还是 canceled（或超时），都会唤醒并将结构化终态结果返回给下一步推理。单步触发异步任务时请勿调用本工具，应直接发 live_task 实时卡片结束本轮。",
  inputSchema: {
    type: "object",
    properties: {
      projectId: { type: "string", minLength: 1, maxLength: 64, description: "目标任务所属项目空间 ID。" },
      taskType: { type: "string", enum: ["build_run", "release"], description: "任务类型：build_run（构建记录）或 release（发布记录）。" },
      taskId: { type: "string", minLength: 1, maxLength: 64, description: "目标构建记录 ID 或发布记录 ID。" },
      timeoutSeconds: { type: "integer", minimum: 15, maximum: 1800, default: 900, description: "最长等待秒数（默认 900，允许范围 15..1800）。" },
    },
    required: ["projectId", "taskType", "taskId"],
    additionalProperties: false,
  },
}

const taskStatusSnapshotSchema = z.object({
  taskType: z.enum(waitForTaskTypes),
  taskId: z.string().min(1),
  projectId: z.string().min(1),
  applicationId: z.string().optional(),
  deploymentTargetId: z.string().optional(),
  rawStatus: z.string(),
  taskStatus: z.enum(["pending", "running", "succeeded", "failed", "canceled"]),
  terminal: z.boolean(),
  failureCode: z.string().optional(),
  imageRef: z.string().optional(),
  imageDigest: z.string().optional(),
  sourceBranch: z.string().optional(),
  sourceTag: z.string().optional(),
  sourceCommit: z.string().optional(),
  buildRunId: z.string().optional(),
  revision: z.number().int().optional(),
  releaseType: z.string().optional(),
  startedAt: z.string().optional(),
  finishedAt: z.string().optional(),
  updatedAt: z.string(),
})

export type TaskStatusSnapshot = z.infer<typeof taskStatusSnapshotSchema>

export type TaskStatusCheckRequest = {
  runId: string
  projectId: string
  taskType: WaitForTaskType
  taskId: string
  signal?: AbortSignal
}

export type TaskStatusCheckResult =
  | { kind: "ok"; snapshot: TaskStatusSnapshot }
  | { kind: "terminal_error"; errorCode: string; status: number }
  | { kind: "transient_error"; errorCode: string }

export interface TaskStatusClient {
  checkTaskStatus(request: TaskStatusCheckRequest): Promise<TaskStatusCheckResult>
}

export class HttpLunaApiTaskStatusClient implements TaskStatusClient {
  constructor(
    private readonly baseUrl: string,
    private readonly serviceToken: string,
    private readonly requestTimeoutMs = 5_000,
  ) {}

  async checkTaskStatus(request: TaskStatusCheckRequest): Promise<TaskStatusCheckResult> {
    const startedAt = performance.now()
    return withSpan("luna_api.task_status.get", clientSpanOptions({
      "server.address": new URL(this.baseUrl).hostname,
      "luna.run.id": request.runId,
      "luna.task.type": request.taskType,
    }), async (span) => {
      const url = new URL(
        `/internal/v1/ai/runs/${encodeURIComponent(request.runId)}/tasks/${encodeURIComponent(request.projectId)}/${encodeURIComponent(request.taskType)}/${encodeURIComponent(request.taskId)}`,
        this.baseUrl,
      )
      const timeoutSignal = AbortSignal.timeout(this.requestTimeoutMs)
      const combinedSignal = request.signal
        ? AbortSignal.any([request.signal, timeoutSignal])
        : timeoutSignal
      let response: Response
      try {
        response = await fetch(url, {
          method: "GET",
          headers: {
            authorization: `Bearer ${this.serviceToken}`,
            accept: "application/json",
          },
          signal: combinedSignal,
        })
      }
      catch (error) {
        if (request.signal?.aborted) throw error
        const outcome = timeoutSignal.aborted ? "timeout" : "network_error"
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "task_status", outcome })
        recordSpanError(span, new Error("ai.progress.unavailable", { cause: error }))
        return { kind: "transient_error", errorCode: "ai.progress.unavailable" }
      }
      span.setAttribute("http.response.status_code", response.status)
      span.setAttribute("luna.external.duration_ms", performance.now() - startedAt)
      let body: unknown
      let bodyParseError: unknown
      try {
        body = await response.json()
      }
      catch (error) {
        body = undefined
        bodyParseError = error
      }
      if (response.ok) {
        const parsed = taskStatusSnapshotSchema.safeParse(body)
        if (!parsed.success) {
          agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "task_status", outcome: "invalid_response" })
          recordSpanError(span, new Error("ai.progress.unavailable", { cause: bodyParseError ?? parsed.error }))
          return { kind: "transient_error", errorCode: "ai.progress.unavailable" }
        }
        agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "task_status", outcome: "success" })
        return { kind: "ok", snapshot: parsed.data }
      }
      agentMetrics.externalRequests.add(1, { target: "luna_api", operation: "task_status", outcome: String(response.status) })
      const code = extractErrorCode(body)
      if (response.status === 400 || response.status === 401 || response.status === 403 || response.status === 404 || response.status === 409) {
        const fallbackCode = response.status === 404
          ? "resource.not_found"
          : response.status === 401 || response.status === 403
            ? "ai.tool_permission_denied"
            : "ai.tool_execution_failed"
        const resolvedCode = code ?? fallbackCode
        recordSpanError(span, new Error(resolvedCode))
        return { kind: "terminal_error", errorCode: resolvedCode, status: response.status }
      }
      const transientCode = code ?? "ai.progress.unavailable"
      recordSpanError(span, new Error(transientCode))
      return { kind: "transient_error", errorCode: transientCode }
    })
  }
}

/** 轻量到期轮询退避：2s -> 4s -> 8s -> 12s（严格封顶 12s，落在 10~15s 退避上限区间内）。 */
export function computeWakeBackoffMs(pollCount: number): number {
  const step = Math.max(0, Math.floor(pollCount))
  return Math.min(12_000, 2_000 * 2 ** Math.min(step, 3))
}

export function formatWaitForTaskResult(
  snapshot: TaskStatusSnapshot,
  options: { timedOut: boolean; durationMs: number },
): Record<string, unknown> {
  const { timedOut, durationMs } = options
  const guidance = snapshot.taskStatus === "failed"
    ? `目标${snapshot.taskType === "build_run" ? "构建" : "发布"}已结束，终态为 failed${snapshot.failureCode ? `（failureCode: ${snapshot.failureCode}）` : ""}。请调用对应日志或详情工具排查失败原因并向用户清晰说明，不要宣称后续步骤已完成。`
    : snapshot.taskStatus === "canceled"
      ? `目标${snapshot.taskType === "build_run" ? "构建" : "发布"}已被取消（canceled）。请停止依赖它的后续写操作并如实告知用户。`
      : timedOut
        ? `等待达到 ${Math.round(durationMs / 1000)} 秒超时上限，目标任务当前状态仍为 ${snapshot.taskStatus}（未失败）。请向用户说明任务仍在后台执行，并可提供 live_task 实时进度卡片。`
        : `目标${snapshot.taskType === "build_run" ? "构建" : "发布"}已成功完成（succeeded），请继续执行后续串联步骤或向用户总结结果。`
  return {
    status: "succeeded",
    projectId: snapshot.projectId,
    taskType: snapshot.taskType,
    taskId: snapshot.taskId,
    rawStatus: snapshot.rawStatus,
    taskStatus: snapshot.taskStatus,
    terminal: snapshot.terminal,
    timedOut,
    durationMs: Math.max(0, Math.round(durationMs)),
    ...(snapshot.applicationId ? { applicationId: snapshot.applicationId } : {}),
    ...(snapshot.deploymentTargetId ? { deploymentTargetId: snapshot.deploymentTargetId } : {}),
    ...(snapshot.imageRef ? { imageRef: snapshot.imageRef } : {}),
    ...(snapshot.imageDigest ? { imageDigest: snapshot.imageDigest } : {}),
    ...(snapshot.failureCode ? { failureCode: snapshot.failureCode } : {}),
    ...(snapshot.buildRunId ? { buildRunId: snapshot.buildRunId } : {}),
    ...(snapshot.revision !== undefined ? { revision: snapshot.revision } : {}),
    ...(snapshot.releaseType ? { releaseType: snapshot.releaseType } : {}),
    ...(snapshot.startedAt ? { startedAt: snapshot.startedAt } : {}),
    ...(snapshot.finishedAt ? { finishedAt: snapshot.finishedAt } : {}),
    updatedAt: snapshot.updatedAt,
    guidance,
  }
}

function extractErrorCode(body: unknown): string | undefined {
  if (!body || typeof body !== "object" || Array.isArray(body)) return undefined
  const record = body as Record<string, unknown>
  if (typeof record.code === "string" && record.code.trim()) return record.code.trim()
  if (record.error && typeof record.error === "object" && !Array.isArray(record.error)) {
    const nested = (record.error as Record<string, unknown>).code
    if (typeof nested === "string" && nested.trim()) return nested.trim()
  }
  return undefined
}
