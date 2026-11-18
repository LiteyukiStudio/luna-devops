import type { DocsKnowledge } from "../knowledge/docs-knowledge.js"
import type { ClaimedRun, RunExecutionSnapshot } from "../domain.js"
import type { ModelRuntime } from "../model-runtime.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import { withRunningRunVersionFence } from "../persistence/run-fence.js"
import type { RemoteConfigSource, RemoteProviderConfig } from "../provider/config-client.js"
import type { RunStreamBus } from "../run-stream-bus.js"
import { agentRuntimeInternals, runtimeSettingsFromRemote, runtimeSettingsSnapshot, type RuntimeSettings } from "../runtime-settings.js"
import type { ToolCatalogRegistry } from "../tools/catalog-registry.js"
import type { ToolCatalog } from "../tools/catalog.js"
import type { ToolOrchestrator } from "../tools/orchestrator.js"
import {
  computeWakeBackoffMs,
  formatWaitForTaskResult,
  type TaskStatusClient,
} from "../tools/wait-for-task.js"
import { genAIAgentName, genAIAgentSpanAttributes } from "../genai-semconv.js"
import { redact } from "../redaction.js"
import {
  agentMetrics,
  errorDiagnostic,
  extractTraceContext,
  internalSpanOptions,
  stableErrorCode,
  telemetryLog,
  withSpan,
} from "../telemetry.js"
import { maxRunRecoveries } from "./checkpoint.js"
import { isLeaseLostError } from "./finalize.js"
import { RunLoop } from "./run-loop.js"

export type RunExecutorDependencies = {
  repository: Repository
  modelRuntime: ModelRuntime
  tools: ToolOrchestrator
  runtimeConfig: RemoteConfigSource
  initialRuntimeSettings: RuntimeSettings
  catalogRegistry: ToolCatalogRegistry
  streamBus: RunStreamBus
  docsKnowledge?: DocsKnowledge
  taskStatusClient?: TaskStatusClient
}

const RUN_LEASE_DURATION_MS = 60_000
const RUN_LEASE_RENEW_INTERVAL_MS = 20_000
const LOCAL_LEASE_SAFETY_MARGIN_MS = 2_000
const ORPHAN_LEASE_SWEEP_INTERVAL_MS = 15_000

export class RunExecutor {
  private timer?: NodeJS.Timeout
  private started = false
  private reconciled = false
  private stopping = false
  private lastOrphanSweepAt = 0
  private pumpRequested = false
  private pumpPromise: Promise<void> | undefined
  private readonly stopController = new AbortController()
  private readonly active = new Set<Promise<boolean>>()
  private readonly controllers = new Map<string, AbortController>()
  private readonly settlements = new Map<string, Promise<void>>()
  private readonly baseRuntimeSettings: RuntimeSettings
  private runtimeSettings: RuntimeSettings
  private providerConfig!: RemoteProviderConfig
  private unsubscribeRemoteConfig: (() => void) | undefined
  private readonly repository: Repository
  private readonly modelRuntime: ModelRuntime
  private readonly tools: ToolOrchestrator
  private readonly runtimeConfig: RemoteConfigSource
  private readonly catalogRegistry: ToolCatalogRegistry
  private readonly streamBus: RunStreamBus
  private readonly taskStatusClient: TaskStatusClient | undefined
  private readonly runLoop: RunLoop

  constructor(input: RunExecutorDependencies) {
    this.repository = input.repository
    this.modelRuntime = input.modelRuntime
    this.tools = input.tools
    this.runtimeConfig = input.runtimeConfig
    this.catalogRegistry = input.catalogRegistry
    this.streamBus = input.streamBus
    this.taskStatusClient = input.taskStatusClient
    this.baseRuntimeSettings = runtimeSettingsSnapshot(input.initialRuntimeSettings)
    this.runtimeSettings = this.baseRuntimeSettings
    this.runLoop = new RunLoop(
      input.repository,
      input.modelRuntime,
      input.tools,
      input.streamBus,
      input.docsKnowledge,
      input.taskStatusClient,
    )
    const currentConfig = this.runtimeConfig.current()
    const currentCatalog = this.runtimeConfig.currentCatalog()
    if (!currentConfig || !currentCatalog) throw new Error("ai.provider_config_unavailable")
    this.applyRemoteConfig(currentConfig, currentCatalog)
    this.unsubscribeRemoteConfig = this.runtimeConfig.subscribe((candidate, catalog) => this.applyRemoteConfig(candidate, catalog))
  }

  start(): void {
    this.started = true
    const tick = () => {
      if (this.stopping) return
      void this.pump()
      this.timer = setTimeout(tick, agentRuntimeInternals.runPollMs)
      this.timer.unref()
    }
    const reconcile = () => {
      if (this.stopping) return
      void this.recoverOrphanedRuns().then((complete) => {
        if (this.stopping) return
        if (complete) {
          this.reconciled = true
          this.lastOrphanSweepAt = Date.now()
          tick()
          return
        }
        this.timer = setTimeout(reconcile, agentRuntimeInternals.runPollMs)
        this.timer.unref()
      })
    }
    reconcile()
  }

  /** 本地新入队、唤醒或槽位释放时即时触发一轮扫描，无需干等固定轮询周期。 */
  notifyWorkAvailable(): void {
    if (!this.started || !this.reconciled || this.stopping) return
    void this.pump()
  }

  async stop(): Promise<void> {
    this.stopping = true
    if (this.timer) clearTimeout(this.timer)
    this.unsubscribeRemoteConfig?.()
    this.unsubscribeRemoteConfig = undefined
    const reason = new Error("ai.agent_stopping")
    this.stopController.abort(reason)
    this.controllers.forEach(controller => controller.abort(reason))
    this.tools.cancelAll(reason)
    if (this.pumpPromise) await Promise.allSettled([this.pumpPromise])
    await Promise.allSettled([...this.active])
  }

  /**
   * 只在本执行器循环持有 Run 时中止它，并由循环通过流式会话提交 canceled 终态；
   * 未持有时不产生任何副作用并返回 false，由调用方直接持久化取消。
   */
  async cancelExecuting(runId: string): Promise<boolean> {
    if (!this.controllers.has(runId)) return false
    return this.cancel(runId)
  }

  async cancel(runId: string): Promise<boolean> {
    const controller = this.controllers.get(runId)
    const reason = new Error("ai.run_canceled")
    const toolCanceled = this.tools.cancelRun(runId, reason)
    if (!controller && !toolCanceled) return false
    controller?.abort(reason)
    if (controller) {
      const settlement = this.settlements.get(runId)
      if (settlement) await Promise.race([
        settlement,
        new Promise<void>(resolve => setTimeout(resolve, 2_000)),
      ])
    }
    this.tools.clearRunLoopState(runId)
    return true
  }

  async runOnce(): Promise<boolean> {
    await this.pollDueWaitingExternalRuns()
    return this.claimAndExecute()
  }

  /** 扫描到期的 waiting_external Run，推进 wake_at 或在终态/超时时回填结果唤醒回 queued。 */
  async pollDueWaitingExternalRuns(): Promise<number> {
    let dueRuns: ClaimedRun[]
    try {
      dueRuns = await this.repository.claimDueWaitingExternalRuns(16, 15_000)
    }
    catch (error) {
      telemetryLog("agent.run.poll_external_failed", "warn", {
        operation: "agent.run.poll_external",
        outcome: "failed",
        ...errorDiagnostic(error, stableErrorCode(error)),
      })
      return 0
    }
    let resolvedCount = 0
    for (const run of dueRuns) {
      if (this.stopping) break
      try {
        const woke = await this.pollSingleWaitingRun(run)
        if (woke) resolvedCount += 1
      }
      catch (error) {
        telemetryLog("agent.run.poll_external_failed", "warn", {
          "operation": "agent.run.poll_external",
          "outcome": "failed",
          "luna.run.id": run.id,
          ...errorDiagnostic(error, stableErrorCode(error)),
        })
      }
    }
    return resolvedCount
  }

  private async pollSingleWaitingRun(run: ClaimedRun): Promise<boolean> {
    const checkpoint = await this.repository.loadRunCheckpoint(run.id)
    const waitingTask = checkpoint?.state.waitingTask
    if (!checkpoint || !waitingTask) {
      await this.repository.updateRun(run.id, "waiting_external", "queued").catch(() => undefined)
      return true
    }
    const nextPhase = checkpoint.state.pendingBatchCalls?.length ? "tool_execution" : "model"
    const restCheckpointState = { ...checkpoint.state }
    delete restCheckpointState.waitingTask
    const completeWaitingTask = async (
      status: "succeeded" | "failed",
      result: Record<string, unknown>,
      durationMs: number,
      errorCode?: string,
    ): Promise<boolean> => {
      const redactedResult = redact(result)
      return this.repository.resolveWaitingExternalRun({
        runId: run.id,
        expectedCheckpointRevision: checkpoint.revision,
        nextPhase,
        nextCheckpointState: restCheckpointState,
        itemId: waitingTask.itemId,
        toolCallId: waitingTask.toolCallId,
        itemStatus: status === "succeeded" ? "completed" : "failed",
        itemContent: {
          toolCallId: waitingTask.toolCallId,
          modelCallId: waitingTask.toolCallId,
          ...(checkpoint.state.pendingBatchId ? { batchId: checkpoint.state.pendingBatchId } : {}),
          operationId: "wait_for_task",
          status,
          arguments: {
            projectId: waitingTask.projectId,
            taskType: waitingTask.taskType,
            taskId: waitingTask.taskId,
            timeoutSeconds: waitingTask.timeoutSeconds,
          },
          result: redactedResult,
          ...(errorCode ? { errorCode } : {}),
          durationMs: Math.max(0, Math.round(durationMs)),
        },
        eventType: status === "succeeded" ? "tool.completed" : "tool.failed",
        eventData: {
          itemId: waitingTask.itemId,
          toolCallId: waitingTask.toolCallId,
          operationId: "wait_for_task",
          status,
          result: redactedResult,
          ...(errorCode ? { errorCode } : {}),
        },
      })
    }

    return withSpan("agent.run.poll_external_task", internalSpanOptions({
      "luna.run.id": run.id,
      "luna.turn.id": run.turnId,
      "luna.task.type": waitingTask.taskType,
    }), async (span) => {
      const nowMs = Date.now()
      const startedMs = Date.parse(waitingTask.startedAt)
      const deadlineMs = Date.parse(waitingTask.deadlineAt)
      const durationMs = Math.max(0, nowMs - (Number.isFinite(startedMs) ? startedMs : nowMs))
      if (!this.taskStatusClient) {
        const errorCode = "ai.tool_not_available"
        const resolved = await completeWaitingTask(
          "failed",
          { status: "failed", errorCode, retryable: false },
          durationMs,
          errorCode,
        )
        span.setAttribute("luna.task.poll_outcome", "client_unavailable")
        return resolved
      }

      const check = await this.taskStatusClient.checkTaskStatus({
        runId: run.id,
        projectId: waitingTask.projectId,
        taskType: waitingTask.taskType,
        taskId: waitingTask.taskId,
        signal: this.stopController.signal,
      })

      if (check.kind === "terminal_error") {
        const resolved = await completeWaitingTask(
          "failed",
          {
            status: "failed",
            errorCode: check.errorCode,
            httpStatus: check.status,
            retryable: false,
            guidance: "无法读取目标后台任务状态（任务不存在或当前用户无访问权限），请向用户如实说明。",
          },
          durationMs,
          check.errorCode,
        )
        span.setAttribute("luna.task.poll_outcome", "terminal_error")
        return resolved
      }

      if (check.kind === "ok" && check.snapshot.terminal) {
        const formatted = formatWaitForTaskResult(check.snapshot, { timedOut: false, durationMs })
        const resolved = await completeWaitingTask("succeeded", formatted, durationMs)
        span.setAttribute("luna.task.poll_outcome", check.snapshot.taskStatus)
        return resolved
      }

      const effectiveDeadlineMs = Number.isFinite(deadlineMs) ? deadlineMs : nowMs
      if (nowMs >= effectiveDeadlineMs) {
        if (check.kind === "transient_error") {
          const resolved = await completeWaitingTask(
            "failed",
            {
              status: "failed",
              projectId: waitingTask.projectId,
              taskType: waitingTask.taskType,
              taskId: waitingTask.taskId,
              errorCode: check.errorCode,
              observationCode: check.errorCode,
              timedOut: true,
              durationMs: Math.max(0, Math.round(durationMs)),
              retryable: false,
              guidance: "等待达到超时上限且后台任务状态接口当前不可达（unavailable），无法确认任务当前状态，请如实告知用户稍后查看或重试，切勿臆测任务仍在运行或已完成。",
            },
            durationMs,
            check.errorCode,
          )
          span.setAttribute("luna.task.poll_outcome", "timed_out_unavailable")
          return resolved
        }
        const formatted = formatWaitForTaskResult(check.snapshot, { timedOut: true, durationMs })
        const resolved = await completeWaitingTask("succeeded", formatted, durationMs)
        span.setAttribute("luna.task.poll_outcome", "timed_out")
        return resolved
      }

      const nextPollCount = waitingTask.pollCount + 1
      const backoffMs = computeWakeBackoffMs(waitingTask.pollCount)
      const nextWakeAt = new Date(Math.min(effectiveDeadlineMs, nowMs + backoffMs)).toISOString()
      await this.repository.rescheduleWaitingExternalRun(run.id, checkpoint.revision, nextPollCount, nextWakeAt)
      span.setAttribute("luna.task.poll_outcome", "rescheduled")
      return false
    }, extractTraceContext(run.traceContext))
  }

  private pump(): Promise<void> {
    if (this.stopping) return Promise.resolve()
    if (this.pumpPromise) {
      this.pumpRequested = true
      return this.pumpPromise
    }
    const currentPump = (async () => {
      try {
        do {
          this.pumpRequested = false
          if (this.stopping) break
          const nowMs = Date.now()
          if (this.reconciled && nowMs - this.lastOrphanSweepAt >= ORPHAN_LEASE_SWEEP_INTERVAL_MS) {
            this.lastOrphanSweepAt = nowMs
            await this.recoverOrphanedRuns()
          }
          await this.pollDueWaitingExternalRuns()
          while (!this.stopping && this.active.size < this.runtimeSettings.agentConcurrentRuns) {
            let claimed: { run: ClaimedRun, candidateExecutionSnapshot: RunExecutionSnapshot } | undefined
            try {
              claimed = await this.claimNextRun()
            }
            catch (error) {
              telemetryLog("agent.run.dispatch_failed", "error", {
                operation: "agent.run.dispatch",
                outcome: "failed",
                ...errorDiagnostic(error, stableErrorCode(error)),
              })
              break
            }
            if (!claimed) break
            const { run, candidateExecutionSnapshot } = claimed
            const task = this.executeClaimedRun(run, candidateExecutionSnapshot)
              .catch((error) => {
                telemetryLog("agent.run.dispatch_failed", "error", {
                  operation: "agent.run.dispatch",
                  outcome: "failed",
                  ...errorDiagnostic(error, stableErrorCode(error)),
                })
                return false
              })
              .finally(() => {
                this.active.delete(task)
                this.notifyWorkAvailable()
              })
            this.active.add(task)
          }
        } while (this.pumpRequested && !this.stopping)
      }
      finally {
        this.pumpPromise = undefined
      }
    })()
    this.pumpPromise = currentPump
    return currentPump
  }

  private async claimNextRun(): Promise<{ run: ClaimedRun, candidateExecutionSnapshot: RunExecutionSnapshot } | undefined> {
    const candidateExecutionSnapshot: RunExecutionSnapshot = Object.freeze({
      runtimeSettings: this.runtimeSettings,
      toolCatalogDigest: this.catalogRegistry.digest(),
    })
    const run = await this.repository.claimNextQueuedRun(
      candidateExecutionSnapshot,
      this.runtimeSettings.userConcurrentRuns,
    )
    if (!run) return undefined
    return { run, candidateExecutionSnapshot }
  }

  private async claimAndExecute(): Promise<boolean> {
    const claimed = await this.claimNextRun()
    if (!claimed) return false
    return this.executeClaimedRun(claimed.run, claimed.candidateExecutionSnapshot)
  }

  private async executeClaimedRun(
    run: ClaimedRun,
    candidateExecutionSnapshot: RunExecutionSnapshot,
  ): Promise<boolean> {
    const priorSettlement = this.settlements.get(run.id)
    if (priorSettlement) {
      this.controllers.get(run.id)?.abort(new Error("ai.run_lease_lost"))
      await priorSettlement
    }
    let outcome = "failed"
    let settleRun!: () => void
    const settlement = new Promise<void>((resolve) => { settleRun = resolve })
    this.settlements.set(run.id, settlement)
    const abort = new AbortController()
    this.controllers.set(run.id, abort)
    const spanSettings = run.executionSnapshot?.runtimeSettings ?? this.runtimeSettings
    const timeout = setTimeout(() => abort.abort(new Error("ai.run_timeout")), spanSettings.runTimeoutMs)
    let leaseStopped = false
    let leaseDeadlineTimer: NodeJS.Timeout | undefined
    let stopLeaseHeartbeat = () => { leaseStopped = true }
    const scheduleLeaseDeadline = (durationMs: number) => {
      if (leaseDeadlineTimer) clearTimeout(leaseDeadlineTimer)
      if (leaseStopped || abort.signal.aborted) return
      leaseDeadlineTimer = setTimeout(() => {
        if (leaseStopped || abort.signal.aborted) return
        stopLeaseHeartbeat()
        abort.abort(new Error("ai.run_lease_lost"))
      }, Math.max(1_000, durationMs - LOCAL_LEASE_SAFETY_MARGIN_MS))
      leaseDeadlineTimer.unref()
    }
    scheduleLeaseDeadline(RUN_LEASE_DURATION_MS)
    const leaseTimer = setInterval(() => {
      if (this.stopping || leaseStopped || abort.signal.aborted) return
      void this.repository.renewRunLease(run.id, run.rowVersion, RUN_LEASE_DURATION_MS)
        .then((renewal) => {
          if (leaseStopped || abort.signal.aborted) return
          if (renewal === "renewed") {
            scheduleLeaseDeadline(RUN_LEASE_DURATION_MS)
            return
          }
          if (renewal === "canceled") {
            stopLeaseHeartbeat()
            abort.abort(new Error("ai.run_canceled"))
          }
          else if (renewal === "lost") {
            stopLeaseHeartbeat()
            abort.abort(new Error("ai.run_lease_lost"))
          }
        })
        .catch((error) => {
          telemetryLog("agent.run.lease_renew_failed", "warn", {
            "operation": "agent.run.lease_renew",
            "outcome": "failed",
            "luna.run.id": run.id,
            ...errorDiagnostic(error, "ai.persistence_unavailable"),
          })
        })
    }, RUN_LEASE_RENEW_INTERVAL_MS)
    leaseTimer.unref()
    stopLeaseHeartbeat = () => {
      if (leaseStopped) return
      leaseStopped = true
      clearInterval(leaseTimer)
      if (leaseDeadlineTimer) {
        clearTimeout(leaseDeadlineTimer)
        leaseDeadlineTimer = undefined
      }
    }
    const startedAt = performance.now()
    agentMetrics.activeRuns.add(1)
    try {
      outcome = await withSpan(`invoke_agent ${genAIAgentName}`, internalSpanOptions({
        ...genAIAgentSpanAttributes(run.conversationId, run.model?.name, spanSettings.assistantMaxOutputTokens),
        "luna.run.id": run.id,
        "luna.turn.id": run.turnId,
      }), async (span) => {
        try {
          const executionSnapshot = run.executionSnapshot ?? candidateExecutionSnapshot
          const runtimeSettings = executionSnapshot.runtimeSettings
          const providerConfig = this.providerConfig
          if (!providerConfig) throw new Error("ai.provider_config_unavailable")
          let toolCatalogDigest = executionSnapshot.toolCatalogDigest ?? run.toolCatalogDigest
          let effectiveRun = run
          if (!toolCatalogDigest || !this.catalogRegistry.has(toolCatalogDigest)) {
            toolCatalogDigest = this.catalogRegistry.current().digest
            await this.repository.updateRunToolCatalogDigest(run.id, toolCatalogDigest, run.rowVersion)
            effectiveRun = { ...run, toolCatalogDigest }
          }

          this.tools.setRunMaxToolCallsForRun(run.id, runtimeSettings.runMaxToolCalls)
          return await withRunningRunVersionFence(run.id, run.rowVersion, () => this.runLoop.execute({
            run: effectiveRun,
            runtimeSettings,
            providerConfig,
            toolCatalogDigest,
            onStopLeaseHeartbeat: stopLeaseHeartbeat,
            signal: abort.signal,
            span,
          }))
        }
        catch (error) {
          stopLeaseHeartbeat()
          if (isLeaseLostError(error, run.rowVersion) || (abort.signal.aborted && isLeaseLostError(abort.signal.reason, run.rowVersion))) {
            telemetryLog("agent.run.lease_lost", "warn", {
              "operation": "agent.run.execute",
              "outcome": "abandoned",
              "error.code": "ai.run_lease_lost",
              "luna.run.id": run.id,
            })
            span.setAttribute("luna.run.outcome", "lease_lost")
            return "lease_lost"
          }
          outcome = error instanceof RunStateConflictError ? "canceled" : stableErrorCode(error)
          telemetryLog("agent.run.dispatch_failed", "error", {
            "operation": "agent.run.execute",
            "outcome": "failed",
            "luna.run.id": run.id,
            ...errorDiagnostic(error, outcome),
          })
          return await this.runLoop.failBeforeExecution(run, error, span)
        }
      }, extractTraceContext(run.traceContext))
    }
    catch (error) {
      outcome = error instanceof RunStateConflictError ? "canceled" : stableErrorCode(error)
      telemetryLog("agent.run.dispatch_failed", "error", {
        "operation": "agent.run.execute",
        "outcome": "failed",
        "luna.run.id": run.id,
        ...errorDiagnostic(error, outcome),
      })
    }
    finally {
      stopLeaseHeartbeat()
      clearTimeout(timeout)
      if (this.controllers.get(run.id) === abort) this.controllers.delete(run.id)
      if (!["waiting_approval", "waiting_input", "lease_lost"].includes(outcome))
        this.tools.clearRunLoopState(run.id)
      const metricAttributes = { outcome }
      agentMetrics.activeRuns.add(-1)
      agentMetrics.runs.add(1, metricAttributes)
      agentMetrics.runDuration.record((performance.now() - startedAt) / 1000, metricAttributes)
      if (outcome !== "lease_lost") {
        const finalRun = await this.repository.getRun(run.ownerUserId, run.id).catch(() => undefined)
        if (finalRun && !["queued", "running"].includes(finalRun.status))
          await this.streamBus.cleanup(run.id).catch(() => undefined)
      }
      if (this.settlements.get(run.id) === settlement) this.settlements.delete(run.id)
      settleRun()
    }
    return true
  }

  /**
   * 启动对账：有 checkpoint 的孤儿 Run 重新排队续跑，没有 checkpoint 的按原语义置为
   * interrupted。单副本部署下 running 只可能来自本进程崩溃，因此恢复不与执行者竞争。
   */
  private async recoverOrphanedRuns(): Promise<boolean> {
    let complete = true
    let running: Array<{ id: string, rowVersion: number }>
    try {
      running = await this.repository.listRunningRuns()
    }
    catch (error) {
      telemetryLog("agent.run.startup_reconcile_failed", "warn", {
        operation: "agent.run.startup_reconcile", outcome: "failed", ...errorDiagnostic(error, "ai.persistence_unavailable"),
      })
      return false
    }
    for (const run of running) {
      try {
        const localController = this.controllers.get(run.id)
        const localSettlement = this.settlements.get(run.id)
        if (localController || localSettlement) {
          localController?.abort(new Error("ai.run_lease_lost"))
          if (localSettlement) await localSettlement
        }
        const recovery = await this.repository.recoverOrphanedRun(run.id, run.rowVersion, maxRunRecoveries)
        if (recovery === "requeued") {
          telemetryLog("agent.run.recovered", "warn", {
            "operation": "agent.run.startup_reconcile",
            "outcome": "requeued",
            "luna.run.id": run.id,
          })
        }
        else if (recovery === "interrupted") {
          telemetryLog("agent.run.orphan_interrupted", "warn", {
            "operation": "agent.run.startup_reconcile", "outcome": "interrupted", "error.code": "ai.agent_restarted",
          })
          await this.cleanupTerminalStream(run.id)
        }
        else complete = false
      }
      catch (error) {
        complete = false
        telemetryLog("agent.run.startup_reconcile_failed", "warn", {
          operation: "agent.run.startup_reconcile", outcome: "failed", ...errorDiagnostic(error, "ai.persistence_unavailable"),
        })
      }
    }
    return complete
  }

  private async cleanupTerminalStream(runId: string): Promise<void> {
    try {
      await this.streamBus.cleanup(runId)
    }
    catch (error) {
      agentMetrics.streamCleanups.add(1, { outcome: "failed" })
      telemetryLog("agent.stream.cleanup_failed", "warn", {
        operation: "agent.stream.cleanup",
        outcome: "failed",
        ...errorDiagnostic(error, "ai.stream_cleanup_failed"),
      })
    }
  }

  private applyRemoteConfig(candidate: RemoteProviderConfig, catalog: ToolCatalog): void {
    const refresh = this.catalogRegistry.refresh(catalog, candidate.version)
    const outcome = refresh.changed ? "updated" : "unchanged"
    agentMetrics.toolCatalogRefreshes.add(1, { outcome })
    telemetryLog("agent.tool_catalog.refreshed", "info", {
      "luna.tool_catalog.refresh.outcome": outcome,
      "luna.tool_catalog.operation_count": this.catalogRegistry.current().all().length,
    })
    void this.repository.listActiveToolCatalogDigests()
      .then(digests => this.catalogRegistry.retain(digests))
      .catch(error => telemetryLog("agent.tool_catalog.retain_failed", "warn", {
        operation: "agent.tool_catalog.retain", outcome: "failed",
        ...errorDiagnostic(error, "ai.persistence_unavailable"),
      }))
    const runtimeSettings = runtimeSettingsSnapshot({
      ...this.baseRuntimeSettings,
      ...runtimeSettingsFromRemote(candidate.runtime, {
        maxModelSteps: this.baseRuntimeSettings.maxModelSteps,
        runMaxToolCalls: this.baseRuntimeSettings.runMaxToolCalls,
      }),
      assistantMaxOutputTokens: this.baseRuntimeSettings.assistantMaxOutputTokens,
      maxCardRepairAttempts: this.baseRuntimeSettings.maxCardRepairAttempts,
      contextRecentTurnCount: this.baseRuntimeSettings.contextRecentTurnCount,
      contextMaxCompressionTurnsPerCompile: this.baseRuntimeSettings.contextMaxCompressionTurnsPerCompile,
      contextSummaryMaxOutputTokens: this.baseRuntimeSettings.contextSummaryMaxOutputTokens,
    })
    this.tools.setRunMaxToolCalls(runtimeSettings.runMaxToolCalls)
    this.runtimeSettings = runtimeSettings
    this.providerConfig = candidate
  }
}
