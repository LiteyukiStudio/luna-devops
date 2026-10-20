import type { InteractionCardValidationIssue } from "@luna-devops/ai-interaction-card-contract"
import type { ClaimedRun, RunCheckpoint } from "../domain.js"
import { genAIToolCallObject, genAIToolSpanAttributes } from "../genai-semconv.js"
import type { ConversationPromptContext, ModelRuntime } from "../model-runtime.js"
import type { Repository } from "../persistence/repository.js"
import type { ModelMessage, ModelToolCall, ModelToolDetailsResult, ModelToolSearchResult } from "../provider/provider.js"
import { redact } from "../redaction.js"
import type { RuntimeSettings } from "../runtime-settings.js"
import {
  agentMetrics,
  internalSpanOptions,
  recordAIContent,
  stableErrorCode,
  telemetryLog,
  withSpan,
} from "../telemetry.js"
import { businessCardToolInputs, compileBusinessCardToolInput, isBusinessCardToolOperationId } from "../tools/business-card-tools.js"
import { internalToolOperationIds } from "../tools/internal-operation-ids.js"
import { getToolDetailsInput } from "../tools/tool-details.js"
import { ToolLoopStoppedError } from "../tools/loop-guard.js"
import type { ToolOrchestrator } from "../tools/orchestrator.js"
import { searchToolsInput } from "../tools/tool-search.js"
import {
  CardGenerationService,
  cardValidationFailure,
  providerArgumentFailure,
  validationIssues,
  type CardGeneration,
} from "./cards.js"
import { restoredArgumentRepairAttempts } from "./checkpoint.js"
import { InternalToolHandlers } from "./internal-tools.js"
import {
  normalizeToolSearchQuery,
  normalizedDetailRequest,
  reconstructSelectedOperationIds,
  resumedToolMessages,
  restoredInternalToolResults,
} from "./resume.js"
import { toolResultMessage } from "./tool-results.js"

const selectedOperationLimit = 16
const searchAutoLoadLimit = 5

export type RunExecutionInput = NonNullable<Awaited<ReturnType<Repository["getExecutionInput"]>>>

export type ToolStepState = {
  continuationMessages: ModelMessage[]
  selectedOperationIds: string[]
  conversation: ConversationPromptContext
  assistantRenamed: boolean
  cardGeneration?: CardGeneration | undefined
  cardRepairExhausted: boolean
  providerArgumentRepairAttempts: Map<string, number>
  searchedToolQueries: Map<string, ModelToolSearchResult>
  detailedToolRequests: Map<string, ModelToolDetailsResult>
}

export type ToolBatchResult =
  | { kind: "continue" }
  | { kind: "waiting_approval"; toolCallId: string }
  | { kind: "complete" }

export type ToolStepInput = {
  run: ClaimedRun
  executionInput: RunExecutionInput
  runtimeSettings: RuntimeSettings
  answer: string
  calls: Array<ModelToolCall & { id: string }>
  signal: AbortSignal
}

export class ToolStepRunner {
  private readonly cards: CardGenerationService
  private readonly internalTools: InternalToolHandlers

  constructor(
    private readonly repository: Repository,
    private readonly modelRuntime: ModelRuntime,
    private readonly tools: ToolOrchestrator,
  ) {
    this.cards = new CardGenerationService(repository)
    this.internalTools = new InternalToolHandlers(repository)
  }

  async initialState(
    run: ClaimedRun,
    executionInput: RunExecutionInput,
    checkpoint: RunCheckpoint | undefined,
  ): Promise<ToolStepState> {
    const selectedOperationIds = checkpoint
      ? [...run.selectedOperationIds]
      : (await this.repository.touchRunSelectedOperations(
          run.id,
          reconstructSelectedOperationIds(executionInput.toolInteractions),
          selectedOperationLimit,
        )).selectedOperationIds
    return {
      continuationMessages: resumedToolMessages(executionInput.toolInteractions),
      selectedOperationIds,
      conversation: {
        ...executionInput.conversation,
        turnIndex: executionInput.turnIndex,
      },
      assistantRenamed: false,
      cardRepairExhausted: false,
      providerArgumentRepairAttempts: restoredArgumentRepairAttempts(checkpoint),
      searchedToolQueries: restoredInternalToolResults(executionInput.toolInteractions, "search_tools"),
      detailedToolRequests: restoredInternalToolResults(executionInput.toolInteractions, "get_tool_details"),
    }
  }

  async execute(input: ToolStepInput, state: ToolStepState): Promise<ToolBatchResult> {
    const { run, executionInput, runtimeSettings, signal } = input
    state.continuationMessages.push({ role: "assistant", content: input.answer, toolCalls: input.calls })
    let platformToolCalled = false
    let toolDetailsCalled = false
    let searchToolsCalled = false
    let businessCardCalled = false
    let createdInteractionCardMode: "presentation" | "interactive" | undefined
    let recoverableToolError = false
    const hasPlatformTool = input.calls.some(call => !internalToolOperationIds.has(call.operationId))
    for (const toolCall of input.calls) {
      if (toolCall.argumentError) {
        if (isBusinessCardToolOperationId(toolCall.operationId) && !hasPlatformTool) {
          if (state.cardGeneration && state.cardGeneration.operationId !== toolCall.operationId) {
            await this.cards.fail(run.id, state.cardGeneration, "ai.interaction_card_schema_invalid")
            state.cardGeneration = undefined
          }
          state.cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments, runtimeSettings.maxCardRepairAttempts, toolCall.operationId)
        }
        const key = toolCall.operationId
        let attempt = (state.providerArgumentRepairAttempts.get(key) ?? 0) + 1
        state.providerArgumentRepairAttempts.set(key, attempt)
        const issues: InteractionCardValidationIssue[] = [{
          code: toolCall.argumentError.code,
          path: "$",
          message: toolCall.argumentError.message,
          expected: "完整 JSON 对象",
        }]
        if (isBusinessCardToolOperationId(toolCall.operationId) && state.cardGeneration?.operationId === toolCall.operationId) {
          attempt = await this.cards.recordRepairFailure(run.id, state.cardGeneration, issues, "ai.tool_arguments_json_invalid")
        }
        const failure = providerArgumentFailure(toolCall.argumentError, attempt, runtimeSettings.maxCardRepairAttempts, state.cardGeneration?.generationId, toolCall.operationId)
        recoverableToolError ||= failure.retryable
        state.cardRepairExhausted ||= !failure.retryable && isBusinessCardToolOperationId(toolCall.operationId)
        state.continuationMessages.push(this.toolResult(toolCall, { ...failure }, runtimeSettings))
        continue
      }
      if (isBusinessCardToolOperationId(toolCall.operationId)) {
        if (!hasPlatformTool) {
          if (state.cardGeneration && state.cardGeneration.operationId !== toolCall.operationId) {
            await this.cards.fail(run.id, state.cardGeneration, "ai.interaction_card_schema_invalid")
            state.cardGeneration = undefined
          }
          let cardInput: unknown = toolCall.arguments
          const parsed = businessCardToolInputs[toolCall.operationId].safeParse(toolCall.arguments)
          if (!parsed.success) {
            state.cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments, runtimeSettings.maxCardRepairAttempts, toolCall.operationId)
            const issues = validationIssues(parsed.error.issues)
            const attempt = await this.cards.recordRepairFailure(run.id, state.cardGeneration, issues, "ai.interaction_card_schema_invalid")
            const failure = cardValidationFailure(
              "create",
              issues,
              attempt,
              runtimeSettings.maxCardRepairAttempts,
              state.cardGeneration.generationId,
              "ai.interaction_card_schema_invalid",
              toolCall.operationId,
            )
            recoverableToolError ||= failure.retryable
            state.cardRepairExhausted ||= !failure.retryable
            state.continuationMessages.push(this.toolResult(toolCall, { ...failure }, runtimeSettings))
            continue
          }
          cardInput = compileBusinessCardToolInput(toolCall.operationId, parsed.data)
          state.cardGeneration ??= await this.cards.start(run.id, run.turnId, cardInput, runtimeSettings.maxCardRepairAttempts, toolCall.operationId)
          const creation = await this.traceInternalTool(toolCall.operationId, run.id, toolCall.arguments, () => this.cards.create(run.id, cardInput, state.cardGeneration!), toolCall.id)
          if (!creation.accepted) {
            recoverableToolError ||= creation.failure.retryable
            state.cardRepairExhausted ||= !creation.failure.retryable
            state.continuationMessages.push(this.toolResult(toolCall, { ...creation.failure }, runtimeSettings))
            continue
          }
          businessCardCalled = true
          createdInteractionCardMode = creation.mode
          state.cardGeneration = undefined
        }
        state.continuationMessages.push(this.toolResult(toolCall, {
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
        }, runtimeSettings))
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
          state.continuationMessages.push(this.toolResult(toolCall, {
            status: "failed",
            errorCode,
            retryable: true,
            guidance: "请提供一到八个精确 operationId 后，在当前 Run 内重试一次 get_tool_details。",
          }, runtimeSettings))
          continue
        }
        const detailInput = parsedInput.data
        const requestKey = normalizedDetailRequest(detailInput.operationIds)
        const cached = state.detailedToolRequests.get(requestKey)
        const alreadyLoaded = cached !== undefined
        const detailsStartedAt = performance.now()
        let detailsResult: Awaited<ReturnType<ModelRuntime["getAvailableToolDetails"]>>
        try {
          detailsResult = await this.traceInternalTool("get_tool_details", run.id, detailInput, async () => {
            const base = cached ?? await this.modelRuntime.getAvailableToolDetails(detailInput.operationIds, run.toolCatalogDigest)
            if (!cached) state.detailedToolRequests.set(requestKey, base)
            const selection = await this.repository.touchRunSelectedOperations(run.id, base.loadedOperationIds, selectedOperationLimit)
            state.selectedOperationIds = selection.selectedOperationIds
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
            detailInput,
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
            detailInput,
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
        state.continuationMessages.push(this.toolResult(toolCall, {
          status: itemCount ? "succeeded" : "empty",
          ...detailsResult,
          guidance: detailsResult.loadedOperationIds.length
            ? "这些工具已加入本轮后续模型步骤。请继续调用最适合的具体工具；目录详情不代表业务执行结果。"
            : "没有加载到已准入工具。请核对 operationId，或先用 search_tools 检索权威目录。",
        }, runtimeSettings))
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
          state.continuationMessages.push(this.toolResult(toolCall, {
            status: "failed",
            errorCode,
            retryable: true,
            guidance: "请修正 query、page 或 pageSize 后，在当前 Run 内重试一次 search_tools。",
          }, runtimeSettings))
          continue
        }
        const searchInput = parsedInput.data
        const normalizedQuery = `${normalizeToolSearchQuery(searchInput.query)}\u0000${searchInput.page}\u0000${searchInput.pageSize}`
        const cached = state.searchedToolQueries.get(normalizedQuery)
        const alreadySearched = cached !== undefined
        const searchStartedAt = performance.now()
        let searchResult: Awaited<ReturnType<ModelRuntime["searchAvailableTools"]>>
        try {
          searchResult = await this.traceInternalTool("search_tools", run.id, {
            queryPresent: searchInput.query.length > 0,
            page: searchInput.page,
            pageSize: searchInput.pageSize,
          }, async () => {
            const base = cached ?? await this.modelRuntime.searchAvailableTools(
              searchInput,
              executionInput.pageContext,
              signal,
              run.toolCatalogDigest,
            )
            const candidates = searchInput.query ? base.items.slice(0, Math.min(searchAutoLoadLimit, 8)).map(item => item.operationId) : []
            const selection = await this.repository.touchRunSelectedOperations(run.id, candidates, selectedOperationLimit)
            state.selectedOperationIds = selection.selectedOperationIds
            const value = {
              ...base,
              loadedOperationIds: candidates,
              missingOperationIds: base.missingOperationIds ?? [],
              catalogDigest: run.toolCatalogDigest,
              duplicate: alreadySearched,
              cacheHit: alreadySearched,
            }
            if (!cached) state.searchedToolQueries.set(normalizedQuery, { ...value, duplicate: false, cacheHit: false })
            return value
          }, toolCall.id, false)
          await this.internalTools.recordToolCall(
            run.id,
            run.turnId,
            "search_tools",
            searchInput,
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
            searchInput,
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
        state.continuationMessages.push(this.toolResult(toolCall, {
          status: searchResult.items.length ? "succeeded" : "no_matches",
          ...searchResult,
          guidance: searchResult.loadedOperationIds.length
            ? "最相关候选已经加入当前 Run，下一步请直接调用最适合的具体工具。只有需要参数语义、相似能力消歧或风险确认时才调用 get_tool_details；目录结果不代表业务已执行。"
            : searchResult.items.length
              ? "这是只浏览不自动加载的目录页；需要执行时请用非空 query 精确检索，或调用 get_tool_details 选择精确 operationId。"
              : "没有匹配工具。请改用更具体的业务目标检索一次；仍无结果时再如实说明能力缺失。",
        }, runtimeSettings))
        continue
      }
      if (toolCall.operationId === "rename_conversation") {
        const renamed = await this.traceInternalTool("rename_conversation", run.id, toolCall.arguments, () => this.internalTools.renameConversation(run.id, run.turnId, run.conversationId, toolCall.arguments), toolCall.id)
        if (renamed) {
          state.assistantRenamed = true
          state.conversation = { ...state.conversation, title: renamed.title, titleSource: renamed.titleSource }
        }
        state.continuationMessages.push(this.toolResult(toolCall, {
          status: renamed ? "succeeded" : "skipped",
          ...(renamed ? { title: renamed.title } : {}),
        }, runtimeSettings))
        continue
      }
      if (toolCall.operationId === "navigate_to_route") {
        await this.traceInternalTool("navigate_to_route", run.id, toolCall.arguments, () => this.internalTools.navigateToRoute(run.id, run.turnId, toolCall.arguments), toolCall.id)
        state.continuationMessages.push(this.toolResult(toolCall, {
          status: "succeeded",
          guidance: "已在时间线生成可点击的内部导航按钮；页面不会自动切换。",
        }, runtimeSettings))
        continue
      }
      platformToolCalled = true
      state.selectedOperationIds = (await this.repository.touchRunSelectedOperations(run.id, [toolCall.operationId], selectedOperationLimit)).selectedOperationIds
      let call: Awaited<ReturnType<ToolOrchestrator["propose"]>>
      try {
        call = await this.tools.propose({ runId: run.id, operationId: toolCall.operationId, arguments: toolCall.arguments, inputMode: "model" }, signal)
      }
      catch (error) {
        if (!(error instanceof ToolLoopStoppedError)) throw error
        recoverableToolError = true
        state.continuationMessages.push(this.toolResult(toolCall, {
          status: "failed",
          ...error.toJSON(),
          guidance: "工具循环保护已停止这次调用；不要原样重试。请基于现有结果回答，或改用参数和信息来源都不同的下一步。",
        }, runtimeSettings))
        continue
      }
      state.continuationMessages.push(this.toolResult(toolCall, {
        status: call.status,
        ...(call.result !== undefined ? { result: call.result } : {}),
        ...(call.errorCode ? { errorCode: call.errorCode } : {}),
      }, runtimeSettings))
      if (call.status === "awaiting_approval") {
        telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id, "tool.name": call.operationId })
        return { kind: "waiting_approval", toolCallId: call.id }
      }
    }
    if (recoverableToolError || state.cardRepairExhausted || toolDetailsCalled || searchToolsCalled)
      return { kind: "continue" }
    if (!platformToolCalled && businessCardCalled)
      return createdInteractionCardMode === "presentation" ? { kind: "continue" } : { kind: "complete" }
    return { kind: "continue" }
  }

  async recoverInterruptedBatch(
    runId: string,
    state: ToolStepState,
    runtimeSettings: RuntimeSettings,
  ): Promise<string | undefined> {
    const calls = await this.tools.recoverToolBatch(runId)
    const awaitingApproval = calls.find(call => call.status === "awaiting_approval")
    if (calls.some(call => call.status === "running")) throw new Error("ai.tool_outcome_unknown")

    const execution = await this.repository.getExecutionInput(runId)
    if (!execution) throw new Error("ai.turn_not_found")
    const settledToolCallIds = new Set(state.continuationMessages.flatMap(message =>
      message.role === "tool" ? [message.toolCallId] : []))
    for (const call of calls) {
      if (call.status === "proposed" || call.status === "awaiting_approval") continue
      const itemId = `${call.id}:item`
      const projected = execution.toolInteractions.find(item => item.itemId === itemId)
      const result = execution.toolInteractions.find(item => item.type === "tool_result" && item.content.relatedItemId === itemId)
      const content = redact({
        ...projected?.content,
        toolCallId: call.id, operationId: call.operationId, status: call.status,
        arguments: call.arguments, result: call.result, errorCode: call.errorCode,
      })
      if (!projected) {
        await this.repository.appendItemWithEvent({
          id: itemId, runId, turnId: execution.turnId, type: "tool_call", status: "streaming", content,
        }, "tool.started", { itemId, toolCallId: call.id })
      }
      if (!result) {
        const status = call.status === "failed" ? "failed" : "completed"
        const event = call.status === "succeeded" ? "tool.completed" : `tool.${call.status}`
        await this.repository.completeToolItemWithEvent(itemId, status, content, {
          id: `${call.id}:result`, runId, turnId: execution.turnId, type: "tool_result", status,
          content: redact({ relatedItemId: itemId, result: call.result, errorCode: call.errorCode }),
        }, event, { itemId, toolCallId: call.id })
      }
      if (settledToolCallIds.has(call.id)) continue
      const toolCall = { id: call.id, operationId: call.operationId, arguments: call.arguments }
      state.continuationMessages.push({ role: "assistant", content: "", toolCalls: [toolCall] })
      state.continuationMessages.push(this.toolResult(toolCall, {
        status: call.status,
        ...(call.result !== undefined ? { result: call.result } : {}),
        ...(call.errorCode ? { errorCode: call.errorCode } : {}),
      }, runtimeSettings))
    }
    return awaitingApproval?.id
  }

  async failCard(runId: string, state: ToolStepState, errorCode: string): Promise<void> {
    if (!state.cardGeneration) return
    await this.cards.fail(runId, state.cardGeneration, errorCode)
    state.cardGeneration = undefined
  }

  private toolResult(toolCall: ModelToolCall & { id: string }, result: Record<string, unknown>, runtimeSettings: RuntimeSettings): ModelMessage {
    return toolResultMessage(toolCall, result, runtimeSettings.toolResultPayloadBudget)
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
