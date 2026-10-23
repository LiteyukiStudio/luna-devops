import type { RunExecutionSnapshot } from "../domain.js"
import type { ModelRuntime } from "../model-runtime.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import type { RemoteConfigSource, RemoteProviderConfig } from "../provider/config-client.js"
import type { RunStreamBus } from "../run-stream-bus.js"
import { agentRuntimeInternals, runtimeSettingsFromRemote, runtimeSettingsSnapshot, type RuntimeSettings } from "../runtime-settings.js"
import type { ToolCatalogRegistry } from "../tools/catalog-registry.js"
import type { ToolCatalog } from "../tools/catalog.js"
import type { ToolOrchestrator } from "../tools/orchestrator.js"
import { genAIAgentName, genAIAgentSpanAttributes } from "../genai-semconv.js"
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
import { RunLoop } from "./run-loop.js"

export type RunExecutorDependencies = {
  repository: Repository
  modelRuntime: ModelRuntime
  tools: ToolOrchestrator
  runtimeConfig: RemoteConfigSource
  initialRuntimeSettings: RuntimeSettings
  catalogRegistry: ToolCatalogRegistry
  streamBus: RunStreamBus
}

export class RunExecutor {
  private timer?: NodeJS.Timeout
  private stopping = false
  private readonly active = new Set<Promise<boolean>>()
  private readonly controllers = new Map<string, AbortController>()
  private readonly settlements = new Map<string, Promise<void>>()
  private runtimeSettings: RuntimeSettings
  private providerConfig!: RemoteProviderConfig
  private unsubscribeRemoteConfig: (() => void) | undefined
  private readonly repository: Repository
  private readonly modelRuntime: ModelRuntime
  private readonly tools: ToolOrchestrator
  private readonly runtimeConfig: RemoteConfigSource
  private readonly catalogRegistry: ToolCatalogRegistry
  private readonly streamBus: RunStreamBus
  private readonly runLoop: RunLoop

  constructor(input: RunExecutorDependencies) {
    this.repository = input.repository
    this.modelRuntime = input.modelRuntime
    this.tools = input.tools
    this.runtimeConfig = input.runtimeConfig
    this.catalogRegistry = input.catalogRegistry
    this.streamBus = input.streamBus
    this.runtimeSettings = runtimeSettingsSnapshot(input.initialRuntimeSettings)
    this.runLoop = new RunLoop(input.repository, input.modelRuntime, input.tools, input.streamBus)
    const currentConfig = this.runtimeConfig.current()
    const currentCatalog = this.runtimeConfig.currentCatalog()
    if (!currentConfig || !currentCatalog) throw new Error("ai.provider_config_unavailable")
    this.applyRemoteConfig(currentConfig, currentCatalog)
    this.unsubscribeRemoteConfig = this.runtimeConfig.subscribe((candidate, catalog) => this.applyRemoteConfig(candidate, catalog))
  }

  start(): void {
    const tick = () => {
      if (this.stopping) return
      if (this.active.size < this.runtimeSettings.agentConcurrentRuns) {
        const task = this.claimAndExecute().catch((error) => {
          telemetryLog("agent.run.dispatch_failed", "error", {
            operation: "agent.run.dispatch", outcome: "failed", ...errorDiagnostic(error, stableErrorCode(error)),
          })
          return false
        }).finally(() => this.active.delete(task))
        this.active.add(task)
      }
      this.timer = setTimeout(tick, agentRuntimeInternals.runPollMs)
      this.timer.unref()
    }
    const reconcile = () => {
      if (this.stopping) return
      void this.recoverOrphanedRuns().then((complete) => {
        if (this.stopping) return
        if (complete) {
          tick()
          return
        }
        this.timer = setTimeout(reconcile, agentRuntimeInternals.runPollMs)
        this.timer.unref()
      })
    }
    reconcile()
  }

  async stop(): Promise<void> {
    this.stopping = true
    if (this.timer) clearTimeout(this.timer)
    this.unsubscribeRemoteConfig?.()
    this.unsubscribeRemoteConfig = undefined
    const reason = new Error("ai.agent_stopping")
    this.controllers.forEach(controller => controller.abort(reason))
    this.tools.cancelAll(reason)
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
    return this.claimAndExecute()
  }

  private async claimAndExecute(): Promise<boolean> {
    const candidateExecutionSnapshot: RunExecutionSnapshot = Object.freeze({
      runtimeSettings: this.runtimeSettings,
      providerConfig: this.providerConfig,
      toolCatalogDigest: this.catalogRegistry.digest(),
    })
    const run = await this.repository.claimNextQueuedRun(candidateExecutionSnapshot)
    if (!run) return false
    let outcome = "failed"
    let settleRun!: () => void
    const settlement = new Promise<void>((resolve) => { settleRun = resolve })
    this.settlements.set(run.id, settlement)
    const abort = new AbortController()
    this.controllers.set(run.id, abort)
    const spanSettings = run.executionSnapshot?.runtimeSettings ?? this.runtimeSettings
    const timeout = setTimeout(() => abort.abort(new Error("ai.run_timeout")), spanSettings.runTimeoutMs)
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
          const providerConfig = executionSnapshot.providerConfig
          const toolCatalogDigest = executionSnapshot.toolCatalogDigest
          if (!providerConfig || !toolCatalogDigest || toolCatalogDigest !== run.toolCatalogDigest)
            throw new Error("ai.tool_catalog_snapshot_invalid")
          this.catalogRegistry.restore(providerConfig.toolCatalog, toolCatalogDigest)

          const activeCount = await this.repository.countActiveUserRuns(run.ownerUserId)
          if (activeCount > runtimeSettings.userConcurrentRuns) {
            telemetryLog("agent.run.quota_rejected", "warn", {
              "luna.run.id": run.id,
              "luna.user.id": run.ownerUserId,
              "luna.quota.user_concurrent_runs": runtimeSettings.userConcurrentRuns,
              "luna.quota.active_count": activeCount,
              "operation": "agent.run.claim",
              "outcome": "rejected",
              "error.code": "ai.quota.user_concurrent_runs_exceeded",
              "error.type": "AgentQuotaError",
              "error.message": "ai.quota.user_concurrent_runs_exceeded",
            })
            await this.repository.updateRun(run.id, "running", "failed", {
              completedAt: new Date().toISOString(),
              errorCode: "ai.quota.user_concurrent_runs_exceeded",
            })
            return "failed"
          }
          this.tools.setRunMaxToolCallsForRun(run.id, runtimeSettings.runMaxToolCalls)
          return await this.runLoop.execute({
            run,
            runtimeSettings,
            providerConfig,
            toolCatalogDigest,
            signal: abort.signal,
            span,
          })
        }
        catch (error) {
          outcome = error instanceof RunStateConflictError ? "canceled" : stableErrorCode(error)
          telemetryLog("agent.run.dispatch_failed", "error", {
            operation: "agent.run.execute",
            outcome: "failed",
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
        operation: "agent.run.execute",
        outcome: "failed",
        "luna.run.id": run.id,
        ...errorDiagnostic(error, outcome),
      })
    }
    finally {
      clearTimeout(timeout)
      this.controllers.delete(run.id)
      if (!["waiting_approval", "waiting_input"].includes(outcome))
        this.tools.clearRunLoopState(run.id)
      const metricAttributes = { outcome }
      agentMetrics.activeRuns.add(-1)
      agentMetrics.runs.add(1, metricAttributes)
      agentMetrics.runDuration.record((performance.now() - startedAt) / 1000, metricAttributes)
      const finalRun = await this.repository.getRun(run.ownerUserId, run.id).catch(() => undefined)
      if (finalRun && !["queued", "running"].includes(finalRun.status))
        await this.streamBus.cleanup(run.id).catch(() => undefined)
      this.settlements.delete(run.id)
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
        const recovery = await this.repository.recoverOrphanedRun(run.id, run.rowVersion, maxRunRecoveries)
        if (recovery === "requeued") {
          telemetryLog("agent.run.recovered", "warn", {
            operation: "agent.run.startup_reconcile",
            outcome: "requeued",
            "luna.run.id": run.id,
          })
        }
        else if (recovery === "interrupted") {
          telemetryLog("agent.run.orphan_interrupted", "warn", {
            operation: "agent.run.startup_reconcile", outcome: "interrupted", "error.code": "ai.agent_restarted",
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
    const runtimeSettings = runtimeSettingsFromRemote(candidate.runtime)
    this.tools.setRunMaxToolCalls(runtimeSettings.runMaxToolCalls)
    this.runtimeSettings = runtimeSettingsSnapshot(runtimeSettings)
    this.providerConfig = candidate
  }
}
