import type { Config } from "../config.js"
import type { InteractionCardValidationIssue } from "@luna-devops/ai-interaction-card-contract"
import type { AssistantModelInput, ModelRuntime } from "../model-runtime.js"
import type { ConversationToolInteraction } from "../domain.js"
import type { ModelToolDetailsResult, ModelToolSearchResult } from "../provider/provider.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import type { ProviderConfigClient } from "../provider/config-client.js"
import type { ToolCatalogRegistry } from "../tools/catalog-registry.js"
import { genAIAgentName, genAIAgentSpanAttributes, genAIInputMessages, genAIOutputMessages, genAIToolCallObject, genAIToolSpanAttributes } from "../genai-semconv.js"
import { agentRuntimeInternals, defaultRuntimeSettings, type RuntimeSettings } from "../runtime-settings.js"
import { agentMetrics, errorDiagnostic, extractTraceContext, internalSpanOptions, recordAIContent, recordSpanError, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { ToolInterruption, type ToolOrchestrator } from "../tools/orchestrator.js"
import { ToolLoopStoppedError } from "../tools/loop-guard.js"
import { businessCardToolInputs, compileBusinessCardToolInput, isBusinessCardToolOperationId } from "../tools/business-card-tools.js"
import { searchToolsInput } from "../tools/tool-search.js"
import { getToolDetailsInput } from "../tools/tool-details.js"
import { CardGenerationService, cardValidationFailure, providerArgumentFailure, setMaxCardRepairAttempts, validationIssues, type CardGeneration } from "./cards.js"
import { InternalToolHandlers } from "./internal-tools.js"
import { internalToolOperationIds, cardToolOperationIds, normalizeToolSearchQuery, resumedToolMessages } from "./resume.js"
import { streamModel } from "./streaming.js"
import { setToolResultPayloadBudget, stableError, toolResultMessage } from "./tool-results.js"

const selectedOperationLimit = 16
const searchAutoLoadLimit = 5
type RuntimeConfigSource = Pick<ProviderConfigClient, "get"> & Partial<Pick<ProviderConfigClient, "getCandidate" | "commit">>

export class RunExecutor {
  private timer?: NodeJS.Timeout
  private stopping = false
  private readonly active = new Set<Promise<boolean>>()
  private readonly controllers = new Map<string, AbortController>()
  private runtimeSettings: RuntimeSettings
  private runtimeRefreshTimer?: NodeJS.Timeout
  private readonly cards: CardGenerationService
  private readonly internalTools: InternalToolHandlers

  constructor(
    private readonly repository: Repository,
    private readonly modelRuntime: ModelRuntime,
    private readonly config: Config,
    private readonly tools?: ToolOrchestrator,
    private readonly runtimeConfig?: RuntimeConfigSource,
    initialRuntimeSettings: RuntimeSettings = defaultRuntimeSettings,
    private readonly catalogRegistry?: ToolCatalogRegistry,
  ) {
    this.runtimeSettings = initialRuntimeSettings
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
    const run = await this.repository.claimNextQueuedRun()
    if (!run) return false

    const activeCount = await this.repository.countActiveUserRuns(run.ownerUserId)
    if (activeCount > this.runtimeSettings.userConcurrentRuns) {
      telemetryLog("agent.run.quota_rejected", "warn", {
        "luna.run.id": run.id,
        "luna.user.id": run.ownerUserId,
        "luna.quota.user_concurrent_runs": this.runtimeSettings.userConcurrentRuns,
        "luna.quota.active_count": activeCount,
        "operation": "agent.run.claim",
        "outcome": "rejected",
        "error.code": "ai.quota.user_concurrent_runs_exceeded",
        "error.type": "AgentQuotaError",
        "error.message": "ai.quota.user_concurrent_runs_exceeded",
      })
      try { await this.repository.updateRun(run.id, "running", "failed", { completedAt: new Date().toISOString(), errorCode: "ai.quota.user_concurrent_runs_exceeded" }) } catch { /* state may have changed */ }
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
      let cardGeneration: CardGeneration | undefined
      const providerArgumentRepairAttempts = new Map<string, number>()
      try {
        telemetryLog("agent.run.started", "info", {
          "luna.run.id": run.id,
        })
        await this.repository.appendEvent(run.id, "run.started", {
          state: "running",
        })
        const executionInput = await this.repository.getExecutionInput(run.id)
        if (!executionInput) throw new Error("ai.turn_not_found")
        let selectedOperationIds = (await this.repository.touchRunSelectedOperations(
          run.id,
          reconstructSelectedOperationIds(executionInput.toolInteractions),
          selectedOperationLimit,
        )).selectedOperationIds
        recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", genAIInputMessages([
          { role: "user", content: executionInput.input },
        ]))
        if (executionInput.pageContext.__lunaDirectToolAction === true) {
          const directTool = executionInput.toolInteractions.find(item => item.type === "tool_call" && ["succeeded", "failed", "rejected"].includes(String(item.content.status)))
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
        let cardRepairExhausted = false
        let finalAnswer = ""
        let completed = false
        let finalResponseMissing = false
        const continuationMessages = resumedToolMessages(executionInput.toolInteractions)
        const searchedToolQueries = restoredInternalToolResults(executionInput.toolInteractions, "search_tools")
        const detailedToolRequests = restoredInternalToolResults(executionInput.toolInteractions, "get_tool_details")
        for (let step = 0; step < this.runtimeSettings.maxModelSteps; step += 1) {
          const loadedOperationIds = [...selectedOperationIds]
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
            loadedOperationIds,
            toolCatalogDigest: run.toolCatalogDigest,
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
          let toolDetailsCalled = false
          let searchToolsCalled = false
          let createInteractionCardsCalled = false
          let createdInteractionCardMode: "presentation" | "interactive" | undefined
          let recoverableToolError = false
          const hasPlatformTool = toolCalls.some(call => !internalToolOperationIds.has(call.operationId))
          for (const toolCall of toolCalls) {
            if (toolCall.argumentError) {
              if (cardToolOperationIds.has(toolCall.operationId) && !hasPlatformTool) {
                if (cardGeneration && cardGeneration.operationId !== toolCall.operationId) {
                  await this.cards.fail(run.id, cardGeneration, "ai.interaction_card_schema_invalid")
                  cardGeneration = undefined
                }
                cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments, toolCall.operationId)
              }
              const key = toolCall.operationId
              let attempt = (providerArgumentRepairAttempts.get(key) ?? 0) + 1
              providerArgumentRepairAttempts.set(key, attempt)
              const issues: InteractionCardValidationIssue[] = [{
                code: toolCall.argumentError.code,
                path: "$",
                message: toolCall.argumentError.message,
                expected: "完整 JSON 对象",
              }]
              if (cardToolOperationIds.has(toolCall.operationId) && cardGeneration?.operationId === toolCall.operationId) {
                attempt = await this.cards.recordRepairFailure(run.id, cardGeneration, issues, "ai.tool_arguments_json_invalid")
              }
              const failure = providerArgumentFailure(toolCall.argumentError, attempt, cardGeneration?.generationId, toolCall.operationId)
              recoverableToolError ||= failure.retryable
              cardRepairExhausted ||= !failure.retryable && cardToolOperationIds.has(toolCall.operationId)
              continuationMessages.push(toolResultMessage(toolCall, { ...failure }))
              continue
            }
            if (cardToolOperationIds.has(toolCall.operationId)) {
              if (!hasPlatformTool) {
                if (cardGeneration && cardGeneration.operationId !== toolCall.operationId) {
                  await this.cards.fail(run.id, cardGeneration, "ai.interaction_card_schema_invalid")
                  cardGeneration = undefined
                }
                let cardInput: unknown = toolCall.arguments
                if (isBusinessCardToolOperationId(toolCall.operationId)) {
                  const parsed = businessCardToolInputs[toolCall.operationId].safeParse(toolCall.arguments)
                  if (!parsed.success) {
                    cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments, toolCall.operationId)
                    const issues = validationIssues(parsed.error.issues)
                    const attempt = await this.cards.recordRepairFailure(run.id, cardGeneration, issues, "ai.interaction_card_schema_invalid")
                    const failure = cardValidationFailure(
                      "create",
                      issues,
                      attempt,
                      cardGeneration.generationId,
                      "ai.interaction_card_schema_invalid",
                      toolCall.operationId,
                    )
                    recoverableToolError ||= failure.retryable
                    cardRepairExhausted ||= !failure.retryable
                    continuationMessages.push(toolResultMessage(toolCall, { ...failure }))
                    continue
                  }
                  cardInput = compileBusinessCardToolInput(toolCall.operationId, parsed.data)
                }
                cardGeneration ??= await this.cards.start(run.id, run.turnId, cardInput, toolCall.operationId)
                const creation = await this.traceInternalTool(toolCall.operationId, run.id, toolCall.arguments, () => this.cards.create(run.id, cardInput, cardGeneration!), toolCall.id)
                if (!creation.accepted) {
                  recoverableToolError ||= creation.failure.retryable
                  cardRepairExhausted ||= !creation.failure.retryable
                  continuationMessages.push(toolResultMessage(toolCall, { ...creation.failure }))
                  continue
                }
                createInteractionCardsCalled = true
                createdInteractionCardMode = creation.mode
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
            if (toolCall.operationId === "get_tool_details") {
              toolDetailsCalled = true
              const parsedInput = getToolDetailsInput.safeParse(toolCall.arguments)
              if (!parsedInput.success) {
                const errorCode = "ai.tool_arguments_invalid"
                await this.internalTools.recordToolCall(
                  run.id,
                  run.turnId,
                  "get_tool_details",
                  toolCall.arguments,
                  "failed",
                  { issues: parsedInput.error.issues.map((issue: { path: PropertyKey[], code: string }) => ({ path: issue.path, code: issue.code })) },
                  errorCode,
                  toolCall.id,
                )
                recoverableToolError = true
                continuationMessages.push(toolResultMessage(toolCall, {
                  status: "failed",
                  errorCode,
                  retryable: true,
                  guidance: "请提供一到八个精确 operationId 后，在当前 Run 内重试一次 get_tool_details。",
                }))
                continue
              }
              const input = parsedInput.data
              const requestKey = normalizedDetailRequest(input.operationIds)
              const cached = detailedToolRequests.get(requestKey)
              const alreadyLoaded = cached !== undefined
              const detailsStartedAt = performance.now()
              let detailsResult: Awaited<ReturnType<ModelRuntime["getAvailableToolDetails"]>>
              try {
                detailsResult = await this.traceInternalTool("get_tool_details", run.id, input, async () => {
                  const base = cached ?? await this.modelRuntime.getAvailableToolDetails(input.operationIds, run.toolCatalogDigest)
                  if (!cached) detailedToolRequests.set(requestKey, base)
                  const selection = await this.repository.touchRunSelectedOperations(run.id, base.loadedOperationIds, selectedOperationLimit)
                  selectedOperationIds = selection.selectedOperationIds
                  return {
                    ...base,
                    alreadySelectedOperationIds: selection.alreadySelectedOperationIds,
                    catalogDigest: run.toolCatalogDigest,
                    duplicate: alreadyLoaded,
                    cacheHit: alreadyLoaded,
                  }
                }, toolCall.id)
                await this.internalTools.recordToolCall(
                  run.id,
                  run.turnId,
                  "get_tool_details",
                  input,
                  "succeeded",
                  detailsResult,
                  undefined,
                  toolCall.id,
                  performance.now() - detailsStartedAt,
                )
              }
              catch (error) {
                const errorCode = stableErrorCode(error)
                await this.internalTools.recordToolCall(
                  run.id,
                  run.turnId,
                  "get_tool_details",
                  input,
                  "failed",
                  undefined,
                  errorCode,
                  toolCall.id,
                  performance.now() - detailsStartedAt,
                )
                throw error
              }
              const itemCount = detailsResult.items.length
              const detailsOutcome = alreadyLoaded ? "duplicate" : itemCount ? "succeeded" : "empty"
              agentMetrics.toolDetailLoads.add(1, { outcome: detailsOutcome })
              agentMetrics.toolDetailItems.record(itemCount, { outcome: detailsOutcome })
              continuationMessages.push(toolResultMessage(toolCall, {
                status: itemCount ? "succeeded" : "empty",
                ...detailsResult,
                guidance: detailsResult.loadedOperationIds.length
                  ? "这些工具已加入本轮后续模型步骤。请继续调用最适合的具体工具；目录详情不代表业务执行结果。"
                  : "没有加载到已准入工具。请核对 operationId，或先用 search_tools 检索权威目录。",
              }))
              continue
            }
            if (toolCall.operationId === "search_tools") {
              searchToolsCalled = true
              const parsedInput = searchToolsInput.safeParse(toolCall.arguments)
              if (!parsedInput.success) {
                const errorCode = "ai.tool_arguments_invalid"
                await this.internalTools.recordToolCall(
                  run.id,
                  run.turnId,
                  "search_tools",
                  toolCall.arguments,
                  "failed",
                  { issues: parsedInput.error.issues.map(issue => ({ path: issue.path, code: issue.code })) },
                  errorCode,
                  toolCall.id,
                )
                recoverableToolError = true
                continuationMessages.push(toolResultMessage(toolCall, {
                  status: "failed",
                  errorCode,
                  retryable: true,
                  guidance: "请修正 query、page 或 pageSize 后，在当前 Run 内重试一次 search_tools。",
                }))
                continue
              }
              const input = parsedInput.data
              const normalizedQuery = `${normalizeToolSearchQuery(input.query)}\u0000${input.page}\u0000${input.pageSize}`
              const cached = searchedToolQueries.get(normalizedQuery)
              const alreadySearched = cached !== undefined
              const searchStartedAt = performance.now()
              let searchResult: Awaited<ReturnType<ModelRuntime["searchAvailableTools"]>>
              try {
                searchResult = await this.traceInternalTool("search_tools", run.id, {
                  queryPresent: input.query.length > 0,
                  page: input.page,
                  pageSize: input.pageSize,
                }, async () => {
                  const base = cached ?? await this.modelRuntime.searchAvailableTools(
                    input,
                    executionInput.pageContext,
                    abort.signal,
                    run.toolCatalogDigest,
                  )
                  const candidates = input.query ? base.items.slice(0, Math.min(searchAutoLoadLimit, 8)).map(item => item.operationId) : []
                  const selection = await this.repository.touchRunSelectedOperations(run.id, candidates, selectedOperationLimit)
                  selectedOperationIds = selection.selectedOperationIds
                  const value = {
                    ...base,
                    loadedOperationIds: candidates,
                    missingOperationIds: base.missingOperationIds ?? [],
                    catalogDigest: run.toolCatalogDigest,
                    duplicate: alreadySearched,
                    cacheHit: alreadySearched,
                  }
                  if (!cached) searchedToolQueries.set(normalizedQuery, { ...value, duplicate: false, cacheHit: false })
                  return value
                }, toolCall.id, false)
                await this.internalTools.recordToolCall(
                  run.id,
                  run.turnId,
                  "search_tools",
                  input,
                  "succeeded",
                  searchResult,
                  undefined,
                  toolCall.id,
                  performance.now() - searchStartedAt,
                )
              }
              catch (error) {
                const errorCode = stableErrorCode(error)
                await this.internalTools.recordToolCall(
                  run.id,
                  run.turnId,
                  "search_tools",
                  input,
                  "failed",
                  undefined,
                  errorCode,
                  toolCall.id,
                  performance.now() - searchStartedAt,
                )
                throw error
              }
              const searchOutcome = alreadySearched ? "duplicate" : searchResult.items.length ? "succeeded" : "no_matches"
              agentMetrics.toolSearches.add(1, { outcome: searchOutcome })
              agentMetrics.toolSearchMatches.record(searchResult.items.length, { outcome: searchOutcome })
              continuationMessages.push(toolResultMessage(toolCall, {
                status: searchResult.items.length ? "succeeded" : "no_matches",
                ...searchResult,
                guidance: searchResult.loadedOperationIds.length
                  ? "最相关候选已经加入当前 Run，下一步请直接调用最适合的具体工具。只有需要参数语义、相似能力消歧或风险确认时才调用 get_tool_details；目录结果不代表业务已执行。"
                  : searchResult.items.length
                    ? "这是只浏览不自动加载的目录页；需要执行时请用非空 query 精确检索，或调用 get_tool_details 选择精确 operationId。"
                    : "没有匹配工具。请改用更具体的业务目标检索一次；仍无结果时再如实说明能力缺失。",
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
            selectedOperationIds = (await this.repository.touchRunSelectedOperations(run.id, [toolCall.operationId], selectedOperationLimit)).selectedOperationIds
            let call: Awaited<ReturnType<ToolOrchestrator["propose"]>>
            try {
              call = await this.tools.propose({ runId: run.id, operationId: toolCall.operationId, arguments: toolCall.arguments, inputMode: "model" })
            }
            catch (error) {
              if (!(error instanceof ToolLoopStoppedError)) throw error
              recoverableToolError = true
              continuationMessages.push(toolResultMessage(toolCall, {
                status: "failed",
                ...error.toJSON(),
                guidance: "工具循环保护已停止这次调用；不要原样重试。请基于现有结果回答，或改用参数和信息来源都不同的下一步。",
              }))
              continue
            }
            continuationMessages.push(toolResultMessage(toolCall, {
              status: call.status,
              ...(call.result !== undefined ? { result: call.result } : {}),
              ...(call.errorCode ? { errorCode: call.errorCode } : {}),
            }))
            if (call.status === "awaiting_approval") {
              outcome = "waiting_approval"
              telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id, "tool.name": call.operationId })
              await this.repository.updateRun(run.id, "running", "waiting_approval")
              return true
            }
          }
          if (recoverableToolError) continue
          if (cardRepairExhausted) continue
          // 目录浏览和语义检索都只完成能力发现；无论模型是否附带文本，都必须把结果送入下一模型步。
          if (toolDetailsCalled || searchToolsCalled) continue
          if (!platformToolCalled && createInteractionCardsCalled) {
            if (createdInteractionCardMode === "presentation") continue
            completed = true
            break
          }
          if (!platformToolCalled && result.answer.trim()) {
            completed = true
            break
          }
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
        if (errorCode === "ai.agent_stopping") {
          outcome = "interrupted"
          span.setAttribute("luna.run.outcome", outcome)
          telemetryLog("agent.run.interrupted", "info", { "luna.run.id": run.id })
          if (cardGeneration) await this.cards.fail(run.id, cardGeneration, "ai.agent_stopping")
          try {
            await this.repository.updateRun(run.id, "running", "interrupted", {
              completedAt: new Date().toISOString(),
              errorCode: "ai.agent_stopping",
            })
          }
          catch { /* cancellation or completion won the state transition */ }
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
		  "operation": "agent.run",
		  "outcome": error instanceof ToolInterruption ? error.state : "failed",
          ...(error instanceof ToolInterruption
            ? {}
            : errorDiagnostic(error, errorCode)),
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
        this.controllers.delete(run.id)
        await this.repository.finalizeStreamingItems(run.id, "completed")
		if (!["waiting_approval", "waiting_input"].includes(outcome))
          this.tools?.clearRunLoopState(run.id)
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
      const candidate = this.catalogRegistry && this.runtimeConfig.getCandidate
        ? await this.runtimeConfig.getCandidate()
        : await this.runtimeConfig.get()
      if (this.catalogRegistry && this.runtimeConfig.commit) {
        await withSpan("agent.tool_catalog.refresh", internalSpanOptions(), async (span) => {
          const refresh = this.catalogRegistry!.refresh(candidate.toolCatalog, candidate.version)
          this.runtimeConfig!.commit!(candidate)
          const outcome = refresh.changed ? "updated" : "unchanged"
          span.setAttributes({
            "luna.tool_catalog.refresh.outcome": outcome,
            "luna.tool_catalog.operation_count": this.catalogRegistry!.current().all().length,
          })
          agentMetrics.toolCatalogRefreshes.add(1, { outcome })
          telemetryLog("agent.tool_catalog.refreshed", "info", {
            "luna.tool_catalog.refresh.outcome": outcome,
            "luna.tool_catalog.operation_count": this.catalogRegistry!.current().all().length,
          })
          this.catalogRegistry!.retain(await this.repository.listActiveToolCatalogDigests())
        })
      }
      const runtimeSettings = {
        ...candidate.runtime,
        // 这些策略只在进程启动时从环境变量读取，不随平台配置热更新。
        contextCompressionTriggerRatio: this.runtimeSettings.contextCompressionTriggerRatio,
        contextRecentTurnCount: this.runtimeSettings.contextRecentTurnCount,
        contextMaxHistoryPayloadBytes: this.runtimeSettings.contextMaxHistoryPayloadBytes,
        contextMaxSummaryPayloadBytes: this.runtimeSettings.contextMaxSummaryPayloadBytes,
        contextMaxContinuationPayloadBytes: this.runtimeSettings.contextMaxContinuationPayloadBytes,
        toolResultPayloadBudget: this.runtimeSettings.toolResultPayloadBudget,
      }
      this.modelRuntime.setContextOptions({
        compressionTriggerRatio: runtimeSettings.contextCompressionTriggerRatio,
        recentTurnCount: runtimeSettings.contextRecentTurnCount,
        maxUncompressedTurnCount: runtimeSettings.contextMaxUncompressedTurnCount,
        maxCompressionTurnsPerCompile: runtimeSettings.contextMaxCompressionTurnsPerCompile,
        summaryMaxOutputTokens: runtimeSettings.contextSummaryMaxOutputTokens,
        maxHistoryPayloadBytes: runtimeSettings.contextMaxHistoryPayloadBytes,
        maxSummaryPayloadBytes: runtimeSettings.contextMaxSummaryPayloadBytes,
        maxContinuationPayloadBytes: runtimeSettings.contextMaxContinuationPayloadBytes,
      })
      this.modelRuntime.setAssistantMaxOutputTokens(runtimeSettings.assistantMaxOutputTokens)
      setToolResultPayloadBudget(runtimeSettings.toolResultPayloadBudget)
      setMaxCardRepairAttempts(runtimeSettings.maxCardRepairAttempts)
      this.tools?.setRunMaxToolCalls(runtimeSettings.runMaxToolCalls)
      this.runtimeSettings = runtimeSettings
    }
    catch (error) {
      agentMetrics.toolCatalogRefreshes.add(1, { outcome: "failed" })
      telemetryLog("agent.tool_catalog.refresh_failed", "warn", {
        "operation": "agent.tool_catalog.refresh",
        "outcome": "failed",
        ...errorDiagnostic(error, stableErrorCode(error)),
      })
      // Keep the last validated settings when Luna API is temporarily unavailable.
    }
  }

  private async streamModel(runId: string, turnId: string, input: AssistantModelInput, signal: AbortSignal): Promise<AssistantModelInput> {
    return streamModel(this.repository, this.modelRuntime, runId, turnId, input, signal)
  }

  private async traceInternalTool<T>(operationId: string, runId: string, input: unknown, operation: () => Promise<T>, callId?: string, captureContent = true): Promise<T> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    try {
      return await withSpan(`execute_tool ${operationId}`, internalSpanOptions({
        ...genAIToolSpanAttributes({ name: operationId, ...(callId ? { callId } : {}) }),
        "luna.run.id": runId,
      }), async span => {
        if (captureContent) {
          recordAIContent(span, "luna.gen_ai.tool.content.input", "gen_ai.tool.call.arguments", genAIToolCallObject(input), {
            "gen_ai.tool.name": operationId,
          })
        }
        let result: T
        try {
          result = await operation()
        }
        catch (error) {
          if (captureContent) {
            recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
              error: {
                type: error instanceof Error ? error.name : "UnknownError",
                code: stableErrorCode(error),
              },
            }, {
              "gen_ai.tool.name": operationId,
            })
          }
          throw error
        }
        if (captureContent) {
          recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", genAIToolCallObject(result), {
            "gen_ai.tool.name": operationId,
          })
        }
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

function reconstructSelectedOperationIds(interactions: ConversationToolInteraction[]): string[] {
  const selected: string[] = []
  for (const interaction of interactions) {
    if (interaction.type !== "tool_call") continue
    const operationId = typeof interaction.content.operationId === "string" ? interaction.content.operationId : ""
    if (operationId && !internalToolOperationIds.has(operationId)) selected.push(operationId)
    const result = recordValue(interaction.content.result)
    const loaded = Array.isArray(result?.loadedOperationIds)
      ? result.loadedOperationIds.filter((item): item is string => typeof item === "string")
      : []
    selected.push(...loaded)
  }
  return [...new Set(selected)]
}

function restoredInternalToolResults(interactions: ConversationToolInteraction[], operationId: "search_tools"): Map<string, ModelToolSearchResult>
function restoredInternalToolResults(interactions: ConversationToolInteraction[], operationId: "get_tool_details"): Map<string, ModelToolDetailsResult>
function restoredInternalToolResults(interactions: ConversationToolInteraction[], operationId: "search_tools" | "get_tool_details") {
  const output = new Map<string, ModelToolSearchResult | ModelToolDetailsResult>()
  for (const interaction of interactions) {
    if (interaction.type !== "tool_call" || interaction.content.operationId !== operationId) continue
    const args = recordValue(interaction.content.arguments)
    const result = recordValue(interaction.content.result)
    if (!args || !result) continue
    if (operationId === "search_tools") {
      const parsed = searchToolsInput.safeParse(args)
      if (parsed.success && Array.isArray(result.items)) {
        const key = `${normalizeToolSearchQuery(parsed.data.query)}\u0000${parsed.data.page}\u0000${parsed.data.pageSize}`
        output.set(key, result as unknown as ModelToolSearchResult)
      }
    }
    else {
      const parsed = getToolDetailsInput.safeParse(args)
      if (parsed.success && Array.isArray(result.items)) output.set(normalizedDetailRequest(parsed.data.operationIds), result as unknown as ModelToolDetailsResult)
    }
  }
  return output
}

function normalizedDetailRequest(operationIds: string[]): string {
  return JSON.stringify([...new Set(operationIds)].sort())
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}
