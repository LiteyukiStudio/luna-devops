import type { Config } from "../config.js"
import type { InteractionCardValidationIssue } from "@luna-devops/ai-interaction-card-contract"
import type { AssistantModelInput, ModelRuntime } from "../model-runtime.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import type { ProviderConfigClient } from "../provider/config-client.js"
import { genAIAgentName, genAIAgentSpanAttributes, genAIInputMessages, genAIOutputMessages, genAIToolCallObject, genAIToolSpanAttributes } from "../genai-semconv.js"
import { agentRuntimeInternals, defaultRuntimeSettings, type RuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, extractTraceContext, internalSpanOptions, recordAIContent, recordSpanError, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { SensitiveInputRejected, ToolInterruption, type ToolOrchestrator } from "../tools/orchestrator.js"
import { searchToolsInput } from "../tools/tool-search.js"
import { createOptionsInput } from "../tools/ui-options.js"
import { CardGenerationService, providerArgumentFailure, setMaxCardRepairAttempts, type CardGeneration } from "./cards.js"
import { InternalToolHandlers } from "./internal-tools.js"
import { internalToolOperationIds, cardToolOperationIds, normalizeToolSearchQuery, resumedOperationIds, resumedToolMessages } from "./resume.js"
import { streamModel } from "./streaming.js"
import { platformToolFailureGuidance, setToolResultPayloadBudget, stableError, toolResultMessage } from "./tool-results.js"

export class RunExecutor {
  private timer?: NodeJS.Timeout
  private stopping = false
  private readonly active = new Set<Promise<boolean>>()
  private readonly controllers = new Map<string, AbortController>()
  private runtimeSettings: RuntimeSettings = defaultRuntimeSettings
  private runtimeRefreshTimer?: NodeJS.Timeout
  private readonly cards: CardGenerationService
  private readonly internalTools: InternalToolHandlers

  constructor(
    private readonly repository: Repository,
    private readonly modelRuntime: ModelRuntime,
    private readonly config: Config,
    private readonly tools?: ToolOrchestrator,
    private readonly runtimeConfig?: Pick<ProviderConfigClient, "get">,
  ) {
    this.cards = new CardGenerationService(repository)
    this.internalTools = new InternalToolHandlers(repository, () => this.runtimeSettings)
  }

  start(): void {
    void this.refreshRuntimeSettings()
    if (this.runtimeConfig) {
      this.runtimeRefreshTimer = setInterval(() => void this.refreshRuntimeSettings(), agentRuntimeInternals.configRefreshMs)
      this.runtimeRefreshTimer.unref()
    }
    const tick = () => {
      if (this.stopping) return
      if (this.active.size < this.runtimeSettings.agentConcurrentRuns) {
        const task = this.claimAndExecute().finally(() => this.active.delete(task))
        this.active.add(task)
      }
      this.timer = setTimeout(tick, agentRuntimeInternals.runPollMs)
      this.timer.unref()
    }
    tick()
  }

  async stop(): Promise<void> {
    this.stopping = true
    if (this.timer) clearTimeout(this.timer)
    if (this.runtimeRefreshTimer) clearInterval(this.runtimeRefreshTimer)
    this.controllers.forEach(controller => controller.abort(new Error("ai.agent_stopping")))
    await Promise.allSettled([...this.active])
  }

  cancel(runId: string): boolean {
    const controller = this.controllers.get(runId)
    if (!controller) return false
    controller.abort(new Error("ai.run_canceled"))
    return true
  }

  async runOnce(): Promise<boolean> {
    return this.claimAndExecute()
  }

  private async claimAndExecute(): Promise<boolean> {
    const run = await this.repository.claimRun(this.config.INSTANCE_ID, agentRuntimeInternals.runLeaseSeconds)
    if (!run) return false

    const activeCount = await this.repository.countActiveUserRuns(run.ownerUserId)
    if (activeCount > this.runtimeSettings.userConcurrentRuns) {
      telemetryLog("agent.run.quota_rejected", "warn", {
        "luna.run.id": run.id,
        "luna.user.id": run.ownerUserId,
        "luna.quota.user_concurrent_runs": this.runtimeSettings.userConcurrentRuns,
        "luna.quota.active_count": activeCount,
      })
      try { await this.repository.updateRun(run.id, "queued", "failed", { completedAt: new Date().toISOString(), errorCode: "ai.quota.user_concurrent_runs_exceeded" }) } catch { /* state may have changed */ }
      return true
    }
    return withSpan(`invoke_agent ${genAIAgentName}`, internalSpanOptions({
      ...genAIAgentSpanAttributes(run.conversationId, run.model?.name, this.runtimeSettings.assistantMaxOutputTokens),
      "luna.run.id": run.id,
      "luna.turn.id": run.turnId,
    }), async span => {
      const startedAt = performance.now()
      let outcome = "completed"
      agentMetrics.activeRuns.add(1)
      const abort = new AbortController()
      this.controllers.set(run.id, abort)
      const timeout = setTimeout(() => abort.abort(new Error("ai.run_timeout")), this.runtimeSettings.runTimeoutMs)
      const heartbeat = setInterval(() => {
        void this.repository.renewLease(run.id, this.config.INSTANCE_ID, agentRuntimeInternals.runLeaseSeconds)
          .then(ok => { if (!ok) abort.abort(new Error("ai.run_lease_lost")) })
      }, Math.max(1000, agentRuntimeInternals.runLeaseSeconds * 333))
      let cardGeneration: CardGeneration | undefined
      const providerArgumentRepairAttempts = new Map<string, number>()
      try {
        telemetryLog("agent.run.started", "info", {
          "luna.run.id": run.id,
        })
        const running = await this.repository.updateRun(run.id, "queued", "running", { startedAt: new Date().toISOString() })
        await this.repository.appendEvent(run.id, "run.started", { state: "running", expectedVersion: running.rowVersion })
        const executionInput = await this.repository.getExecutionInput(run.id)
        if (!executionInput) throw new Error("ai.turn_not_found")
        recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", genAIInputMessages([
          { role: "user", content: executionInput.input },
        ]))
        if (executionInput.pageContext.__lunaDirectToolAction === true) {
          const directTool = executionInput.toolInteractions.find(item => item.type === "tool_call" && ["succeeded", "failed"].includes(String(item.content.status)))
          if (!directTool) throw new Error("ai.direct_tool_not_ready")
          const directStatus = directTool.content.status === "failed" ? "failed" : "completed"
          await this.repository.updateRun(run.id, "running", directStatus, { completedAt: new Date().toISOString() })
          telemetryLog("agent.direct_tool.completed", "info", {
            "luna.run.id": run.id,
            "tool.name": typeof directTool.content.operationId === "string" ? directTool.content.operationId : "unknown",
            "tool.outcome": directStatus,
          })
          return true
        }
        let conversationContext = {
          ...executionInput.conversation,
          turnIndex: executionInput.turnIndex,
        }
        let assistantRenamed = false
        let pendingOptions: unknown
        let interactionCardsCreated = false
        let cardRepairExhausted = false
        let finalAnswer = ""
        let completed = false
        let finalResponseMissing = false
        const continuationMessages = resumedToolMessages(executionInput.toolInteractions)
        // 恢复审批/MFA 后仍保留此前已经使用过的工具，避免动态工具集在断点续跑时漂移。
        const loadedOperationIds = new Set(resumedOperationIds(executionInput.toolInteractions))
        const searchedToolQueries = new Set<string>()
        for (let step = 0; step < this.runtimeSettings.maxModelSteps; step += 1) {
          const result = await this.streamModel(run.id, run.turnId, {
            runId: run.id,
            ownerUserId: run.ownerUserId,
            conversationId: executionInput.conversationId,
            input: executionInput.input,
            pageContext: executionInput.pageContext,
            history: executionInput.history,
            conversation: conversationContext,
            promptVersion: run.promptVersion,
            reasoningSummary: "",
            answer: "",
            toolCalls: [],
            continuationMessages,
            loadedOperationIds: [...loadedOperationIds],
            ...(executionInput.model ? { model: executionInput.model } : {}),
          }, abort.signal)
          finalAnswer = result.answer
          finalResponseMissing = false
          if (cardRepairExhausted) {
            if (result.toolCalls.length > 0) throw new Error("ai.interaction_card_schema_invalid")
            completed = true
            break
          }
          if (!result.toolCalls.length) {
            if (cardGeneration) {
              await this.cards.fail(run.id, cardGeneration, "ai.interaction_card_schema_invalid")
              cardGeneration = undefined
            }
            if (result.answer.trim()) {
              completed = true
              break
            }
            finalResponseMissing = true
            continue
          }

          const toolCalls = result.toolCalls.map((call, index) => ({
            ...call,
            id: call.id ?? `call_${step}_${index}`,
          }))
          continuationMessages.push({ role: "assistant", content: result.answer, toolCalls })
          let platformToolCalled = false
          let createOptionsCalled = false
          let createInteractionCardsCalled = false
          let createdInteractionCardMode: "presentation" | "interactive" | undefined
          let recoverableToolError = false
          const hasPlatformTool = toolCalls.some(call => !internalToolOperationIds.has(call.operationId))
          for (const toolCall of toolCalls) {
            if (toolCall.argumentError) {
              if (toolCall.operationId === "create_interaction_cards" && !hasPlatformTool) {
                cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments)
              }
              const key = cardGeneration?.generationId ?? toolCall.operationId
              let attempt = (providerArgumentRepairAttempts.get(key) ?? 0) + 1
              providerArgumentRepairAttempts.set(key, attempt)
              const issues: InteractionCardValidationIssue[] = [{
                code: toolCall.argumentError.code,
                path: "$",
                message: toolCall.argumentError.message,
                expected: "完整 JSON 对象",
              }]
              if (toolCall.operationId === "create_interaction_cards" && cardGeneration) {
                attempt = await this.cards.recordRepairFailure(run.id, cardGeneration, issues, "ai.tool_arguments_json_invalid")
              }
              const failure = providerArgumentFailure(toolCall.argumentError, attempt, cardGeneration?.generationId)
              recoverableToolError ||= failure.retryable
              cardRepairExhausted ||= !failure.retryable && cardToolOperationIds.has(toolCall.operationId)
              continuationMessages.push(toolResultMessage(toolCall, { ...failure }))
              continue
            }
            if (toolCall.operationId === "create_interaction_cards") {
              if (!hasPlatformTool) {
                cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments)
                const creation = await this.traceInternalTool("create_interaction_cards", run.id, toolCall.arguments, () => this.cards.create(run.id, toolCall.arguments, cardGeneration!), toolCall.id)
                if (!creation.accepted) {
                  recoverableToolError ||= creation.failure.retryable
                  cardRepairExhausted ||= !creation.failure.retryable
                  continuationMessages.push(toolResultMessage(toolCall, { ...creation.failure }))
                  continue
                }
                createInteractionCardsCalled = true
                createdInteractionCardMode = creation.mode
                interactionCardsCreated = true
                cardGeneration = undefined
              }
              continuationMessages.push(toolResultMessage(toolCall, {
                status: hasPlatformTool ? "deferred_until_platform_results" : "succeeded",
                ...(!hasPlatformTool && createdInteractionCardMode
                  ? {
                      workflowState: createdInteractionCardMode === "interactive"
                        ? "awaiting_user_input"
                        : "evidence_presented",
                      completionEvidence: false,
                      guidance: createdInteractionCardMode === "interactive"
                        ? "卡片正在等待用户提交输入或选择；这不代表业务目标已经完成。"
                        : "卡片只完成了结果呈现，不代表任何业务操作已经执行或通过验收。请根据用户目标继续执行、回读验证，或明确说明尚未完成的原因；不要重复生成同一张卡片。",
                    }
                  : {}),
              }))
              continue
            }
            if (toolCall.operationId === "create_options") {
              createOptionsCalled = true
              if (!hasPlatformTool) pendingOptions = toolCall.arguments
              continuationMessages.push(toolResultMessage(toolCall, {
                status: hasPlatformTool ? "deferred_until_final_response" : "accepted",
              }))
              continue
            }
            if (toolCall.operationId === "search_tools") {
              const input = searchToolsInput.parse(toolCall.arguments)
              const normalizedQuery = normalizeToolSearchQuery(input.query)
              const alreadySearched = searchedToolQueries.has(normalizedQuery)
              const searchResult = await this.traceInternalTool("search_tools", run.id, input, async () => {
                if (alreadySearched) return {
                  query: input.query,
                  matches: [],
                  loadedOperationIds: [] as string[],
                  totalMatches: 0,
                }
                searchedToolQueries.add(normalizedQuery)
                return this.modelRuntime.searchAvailableTools(input.query, executionInput.pageContext, input.maxResults)
              }, toolCall.id)
              searchResult.loadedOperationIds.forEach(operationId => loadedOperationIds.add(operationId))
              const searchOutcome = alreadySearched ? "duplicate" : searchResult.matches.length ? "succeeded" : "no_matches"
              agentMetrics.toolSearches.add(1, { outcome: searchOutcome })
              agentMetrics.toolSearchMatches.record(searchResult.matches.length, { outcome: searchOutcome })
              continuationMessages.push(toolResultMessage(toolCall, {
                status: searchResult.matches.length ? "succeeded" : "no_matches",
                matches: searchResult.matches,
                loadedOperationIds: searchResult.loadedOperationIds,
                totalMatches: searchResult.totalMatches,
                guidance: searchResult.matches.length
                  ? "这些工具已加入本轮后续模型步骤。请继续调用最适合的具体工具；不要把检索结果当成业务执行结果。"
                  : "没有新增匹配工具，或同一检索已经执行过。请改用更具体且不同的业务目标检索一次；仍无结果时再如实说明能力缺失。",
              }))
              continue
            }
            if (toolCall.operationId === "rename_conversation") {
              const renamed = await this.traceInternalTool("rename_conversation", run.id, toolCall.arguments, () => this.internalTools.renameConversation(run.id, run.turnId, run.conversationId, toolCall.arguments), toolCall.id)
              if (renamed) {
                assistantRenamed = true
                conversationContext = { ...conversationContext, title: renamed.title, titleSource: renamed.titleSource }
              }
              continuationMessages.push(toolResultMessage(toolCall, {
                status: renamed ? "succeeded" : "skipped",
                ...(renamed ? { title: renamed.title } : {}),
              }))
              continue
            }
            if (toolCall.operationId === "navigate_to_route") {
              const delivery = await this.traceInternalTool("navigate_to_route", run.id, toolCall.arguments, () => this.internalTools.navigateToRoute(run.id, run.turnId, toolCall.arguments), toolCall.id)
              continuationMessages.push(toolResultMessage(toolCall, {
                status: "dispatched",
                actionId: delivery.id,
                expiresAt: delivery.expiresAt,
              }))
              continue
            }
            if (!this.tools) throw new Error("ai.tool_not_available")
            platformToolCalled = true
            let call: Awaited<ReturnType<ToolOrchestrator["propose"]>>
            try {
              call = await this.tools.propose({ runId: run.id, operationId: toolCall.operationId, arguments: toolCall.arguments, inputMode: "model" })
            }
            catch (error) {
              if (!(error instanceof SensitiveInputRejected)) throw error
              recoverableToolError = true
              continuationMessages.push(toolResultMessage(toolCall, {
                status: "failed",
                errorCode: error.message,
                guidance: "敏感输入只能通过安全表单提交；请创建或修复安全表单，不要把密钥写入普通工具参数、聊天消息或回复。",
              }))
              continue
            }
            const failureGuidance = platformToolFailureGuidance(call.operationId, call.errorCode)
            continuationMessages.push(toolResultMessage(toolCall, {
              status: call.status,
              ...(call.result !== undefined ? { result: call.result } : {}),
              ...(call.errorCode ? { errorCode: call.errorCode } : {}),
              ...(failureGuidance ?? {}),
            }))
            if (call.status === "awaiting_approval") {
              outcome = "waiting_approval"
              telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id, "tool.name": call.operationId })
              await this.repository.updateRun(run.id, "running", "waiting_approval")
              return true
            }
            if (call.status === "awaiting_mfa") {
              outcome = "waiting_mfa"
              telemetryLog("agent.run.waiting_mfa", "info", { "luna.run.id": run.id, "tool.name": call.operationId })
              await this.repository.updateRun(run.id, "running", "waiting_mfa")
              return true
            }
          }
          if (recoverableToolError) continue
          if (cardRepairExhausted) continue
          if (platformToolCalled) pendingOptions = undefined
          if (!platformToolCalled && createInteractionCardsCalled) {
            if (createdInteractionCardMode === "presentation") continue
            completed = true
            break
          }
          if (!platformToolCalled && result.answer.trim()) {
            completed = true
            break
          }
          if (!platformToolCalled && createOptionsCalled) finalResponseMissing = true
        }
        if (!completed) throw new Error(finalResponseMissing ? "ai.final_response_missing" : "ai.limit_exceeded")
        if (executionInput.conversation.titleSource === "default" && !assistantRenamed) {
          try {
            const title = await this.modelRuntime.generateConversationTitle(executionInput.input, finalAnswer, { runId: run.id, ownerUserId: run.ownerUserId }, abort.signal, executionInput.model)
            if (title) await this.internalTools.renameConversation(run.id, run.turnId, run.conversationId, { title })
          }
          catch {
            // Title generation is best-effort and must never fail a completed response.
          }
        }
        if (!interactionCardsCreated) {
          await this.ensureOptions(run.id, run.turnId, {
            runId: run.id,
            ownerUserId: run.ownerUserId,
            userInput: executionInput.input,
            answer: finalAnswer,
            pageContext: executionInput.pageContext,
            conversation: conversationContext,
            history: executionInput.history,
            ...(executionInput.model ? { model: executionInput.model } : {}),
          }, pendingOptions, abort.signal)
        }
        recordAIContent(span, "luna.gen_ai.content.output", "gen_ai.output.messages", genAIOutputMessages({
          text: finalAnswer,
          finishReason: "stop",
        }))
        span.setAttribute("gen_ai.response.finish_reasons", ["stop"])
        await this.repository.updateRun(run.id, "running", "completed", { completedAt: new Date().toISOString() })
        telemetryLog("agent.run.completed", "info", { "luna.run.id": run.id })
      }
      catch (error) {
        const message = error instanceof Error ? error.message : "ai.run_failed"
        const errorCode = stableErrorCode(error)
        const canceled = errorCode === "ai.run_canceled"
          || (error instanceof RunStateConflictError && error.actualStatus === "canceled")
        if (canceled) {
          outcome = "canceled"
          span.setAttribute("luna.run.outcome", outcome)
          if (error instanceof RunStateConflictError) {
            span.setAttribute("luna.run.expected_status", error.expectedStatus)
            span.setAttribute("luna.run.actual_status", error.actualStatus ?? "missing")
            span.setAttribute("luna.run.target_status", error.targetStatus)
          }
          telemetryLog("agent.run.canceled", "info", {
            "luna.run.id": run.id,
            "luna.run.cancel_source": error instanceof RunStateConflictError ? "durable_state" : "abort_signal",
          })
          if (cardGeneration) await this.cards.fail(run.id, cardGeneration, "ai.run_canceled")
          return true
        }
        outcome = error instanceof ToolInterruption ? error.state : stableErrorCode(error)
        span.setAttribute("luna.run.outcome", outcome)
        if (error instanceof RunStateConflictError) {
          span.setAttribute("luna.run.expected_status", error.expectedStatus)
          span.setAttribute("luna.run.actual_status", error.actualStatus ?? "missing")
          span.setAttribute("luna.run.target_status", error.targetStatus)
        }
        telemetryLog(error instanceof ToolInterruption ? `agent.run.${error.state}` : "agent.run.failed", error instanceof ToolInterruption ? "info" : "error", {
          "luna.run.id": run.id,
          ...(error instanceof ToolInterruption
            ? {}
            : {
                "error.type": error instanceof Error ? error.name : "UnknownError",
                "error.code": errorCode,
              }),
          ...(error instanceof RunStateConflictError
            ? {
                "luna.run.expected_status": error.expectedStatus,
                "luna.run.actual_status": error.actualStatus ?? "missing",
                "luna.run.target_status": error.targetStatus,
              }
            : {}),
        })
        if (cardGeneration) await this.cards.fail(run.id, cardGeneration, stableError(message))
        if (error instanceof ToolInterruption && error.state === "waiting_input") {
          outcome = "waiting_input"
          await this.repository.appendEvent(run.id, "run.input_required", { fields: error.fields })
          await this.repository.updateRun(run.id, "running", "waiting_input")
          return true
        }
        recordSpanError(span, error)
        try { await this.repository.updateRun(run.id, "running", "failed", { completedAt: new Date().toISOString(), errorCode: stableError(message) }) } catch { /* state was changed by cancellation */ }
      }
      finally {
        clearTimeout(timeout)
        clearInterval(heartbeat)
        this.controllers.delete(run.id)
        await this.repository.finalizeStreamingItems(run.id, "completed")
        await this.repository.releaseLease(run.id, this.config.INSTANCE_ID)
        const metricAttributes = { outcome }
        agentMetrics.activeRuns.add(-1)
        agentMetrics.runs.add(1, metricAttributes)
        agentMetrics.runDuration.record((performance.now() - startedAt) / 1000, metricAttributes)
      }
      return true
    }, extractTraceContext(run.traceContext))
  }

  private async refreshRuntimeSettings(): Promise<void> {
    if (!this.runtimeConfig)
      return
    try {
      const runtimeSettings = (await this.runtimeConfig.get()).runtime
      this.modelRuntime.setContextOptions({
        inputTokenBudget: runtimeSettings.contextInputTokenBudget,
        compressionTriggerRatio: runtimeSettings.contextCompressionTriggerRatio,
        compressionTargetRatio: runtimeSettings.contextCompressionTargetRatio,
        recentTurnCount: runtimeSettings.contextRecentTurnCount,
        maxRecentTurnCount: runtimeSettings.contextMaxRecentTurnCount,
        maxUncompressedTurnCount: runtimeSettings.contextMaxUncompressedTurnCount,
        maxCompressionTurnsPerCompile: runtimeSettings.contextMaxCompressionTurnsPerCompile,
        summaryInputTokenBudget: runtimeSettings.contextSummaryInputTokenBudget,
        summaryMaxOutputTokens: runtimeSettings.contextSummaryMaxOutputTokens,
        historicalToolTokenBudget: runtimeSettings.contextHistoricalToolTokenBudget,
      })
      this.modelRuntime.setAssistantMaxOutputTokens(runtimeSettings.assistantMaxOutputTokens)
      setToolResultPayloadBudget(runtimeSettings.toolResultPayloadBudget)
      setMaxCardRepairAttempts(runtimeSettings.maxCardRepairAttempts)
      this.runtimeSettings = runtimeSettings
    }
    catch {
      // Keep the last validated settings when Luna API is temporarily unavailable.
    }
  }

  private async ensureOptions(
    runId: string,
    turnId: string,
    context: Parameters<ModelRuntime["predictNextSteps"]>[0],
    preferred: unknown,
    signal: AbortSignal,
  ) {
    if (preferred !== undefined) {
      const parsed = createOptionsInput.safeParse(preferred)
      if (parsed.success) {
        await this.traceInternalTool("create_options", runId, parsed.data, () => this.internalTools.createOptions(runId, turnId, parsed.data))
        return
      }
    }
    try {
      const predicted = await this.modelRuntime.predictNextSteps(context, signal)
      const parsed = createOptionsInput.safeParse(predicted)
      if (parsed.success) {
        await this.traceInternalTool("create_options", runId, parsed.data, () => this.internalTools.createOptions(runId, turnId, parsed.data))
        return
      }
    }
    catch {
      if (signal.aborted) throw signal.reason
      // Suggestions are optional. Omitting them is safer than presenting unrelated generic actions.
    }
    if (signal.aborted) throw signal.reason
  }

  private async streamModel(runId: string, turnId: string, input: AssistantModelInput, signal: AbortSignal): Promise<AssistantModelInput> {
    return streamModel(this.repository, this.modelRuntime, runId, turnId, input, signal)
  }

  private async traceInternalTool<T>(operationId: string, runId: string, input: unknown, operation: () => Promise<T>, callId?: string): Promise<T> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    try {
      return await withSpan(`execute_tool ${operationId}`, internalSpanOptions({
        ...genAIToolSpanAttributes({ name: operationId, ...(callId ? { callId } : {}) }),
        "luna.run.id": runId,
      }), async span => {
        recordAIContent(span, "luna.gen_ai.tool.content.input", "gen_ai.tool.call.arguments", genAIToolCallObject(input), {
          "gen_ai.tool.name": operationId,
        })
        let result: T
        try {
          result = await operation()
        }
        catch (error) {
          recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
            error: {
              type: error instanceof Error ? error.name : "UnknownError",
              code: stableErrorCode(error),
            },
          }, {
            "gen_ai.tool.name": operationId,
          })
          throw error
        }
        recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", genAIToolCallObject(result), {
          "gen_ai.tool.name": operationId,
        })
        return result
      })
    }
    catch (error) {
      outcome = stableErrorCode(error)
      throw error
    }
    finally {
      const attributes = { tool: operationId, outcome }
      agentMetrics.toolCalls.add(1, attributes)
      agentMetrics.toolDuration.record((performance.now() - startedAt) / 1000, attributes)
    }
  }
}
