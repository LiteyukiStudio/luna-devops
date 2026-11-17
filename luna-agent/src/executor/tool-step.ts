import { bundledDocsKnowledge, type DocsKnowledge } from "../knowledge/docs-knowledge.js"
import { listDocsInput, readDocInput, searchDocsInput, type ListDocsInput, type ReadDocInput, type SearchDocsInput } from "../tools/docs-tools.js"
import type { InteractionCardValidationIssue } from "@luna-devops/ai-interaction-card-contract"
import type { ClaimedRun, RunCheckpoint, RunPendingToolCall, RunWaitingTaskState } from "../domain.js"
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
import {
  DEFAULT_SAME_OPERATION_SOFT_WARN_THRESHOLD,
  sameOperationSoftWarnGuidance,
  ToolLoopStoppedError,
  withSameOperationSoftWarning,
} from "../tools/loop-guard.js"
import { ToolInterruption, type ToolOrchestrator } from "../tools/orchestrator.js"
import { searchToolsInput } from "../tools/tool-search.js"
import {
  computeWakeBackoffMs,
  formatWaitForTaskResult,
  waitForTaskInput,
  type TaskStatusClient,
} from "../tools/wait-for-task.js"
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
  reasoningByToolCallItemId,
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
  executedOperationIds: Set<string>
  internalReadCallCounts: Map<string, number>
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
  | { kind: "waiting_approval"; toolCallId: string; batchId?: string; remainingCalls?: RunPendingToolCall[] }
  | { kind: "waiting_external"; waitingTask: RunWaitingTaskState; wakeAt: string; batchId?: string; remainingCalls?: RunPendingToolCall[] }
  | { kind: "complete" }

export type ToolStepInput = {
  run: ClaimedRun
  executionInput: RunExecutionInput
  runtimeSettings: RuntimeSettings
  answer: string
  /** 产生这批工具调用的那一轮推理原文；随 assistant 消息回传。 */
  reasoning: string
  calls: Array<ModelToolCall & { id: string }>
  batchId?: string
  appendToExistingBatch?: boolean
  signal: AbortSignal
}

export class ToolStepRunner {
  private readonly cards: CardGenerationService
  private readonly internalTools: InternalToolHandlers

  constructor(
    private readonly repository: Repository,
    private readonly modelRuntime: ModelRuntime,
    private readonly tools: ToolOrchestrator,
    private readonly docsKnowledge?: DocsKnowledge,
    private readonly taskStatusClient?: TaskStatusClient,
  ) {
    this.cards = new CardGenerationService(
      repository,
      (runId, operationId) => this.tools.isCallableOperation(runId, operationId),
    )
    this.internalTools = new InternalToolHandlers(repository)
  }

  async initialState(
    run: ClaimedRun,
    executionInput: RunExecutionInput,
    checkpoint: RunCheckpoint | undefined,
  ): Promise<ToolStepState> {
    const executedOperationIds = reconstructedExecutedOperationIds(executionInput.toolInteractions)
    const selectedOperationIds = checkpoint
      ? [...run.selectedOperationIds]
      : (await this.repository.touchRunSelectedOperations(
          run.id,
          reconstructSelectedOperationIds(executionInput.toolInteractions),
          selectedOperationLimit,
          executedOperationIds,
        )).selectedOperationIds
    const restoredCardGen = checkpoint?.state.cardGeneration && checkpoint.state.cardGeneration.status === "streaming"
      ? structuredClone(checkpoint.state.cardGeneration)
      : undefined
    return {
      continuationMessages: resumedToolMessages(executionInput.toolInteractions),
      selectedOperationIds,
      executedOperationIds,
      internalReadCallCounts: reconstructedInternalReadCallCounts(executionInput.toolInteractions),
      conversation: {
        ...executionInput.conversation,
        turnIndex: executionInput.turnIndex,
      },
      assistantRenamed: false,
      ...(restoredCardGen ? { cardGeneration: restoredCardGen } : {}),
      cardRepairExhausted: checkpoint?.state.cardRepairExhausted === true,
      providerArgumentRepairAttempts: restoredArgumentRepairAttempts(checkpoint),
      searchedToolQueries: restoredInternalToolResults(executionInput.toolInteractions, "search_tools"),
      detailedToolRequests: restoredInternalToolResults(executionInput.toolInteractions, "get_tool_details"),
    }
  }

  async execute(input: ToolStepInput, state: ToolStepState): Promise<ToolBatchResult> {
    const { run, executionInput, runtimeSettings, signal } = input
    const batchId = input.batchId
    const assistantMessage = this.ensureBatchAssistantMessage(state, input.answer, input.reasoning, input.appendToExistingBatch === true)
    const commitCallResult = (toolCall: ModelToolCall & { id: string }, resultPayload: Record<string, unknown>) => {
      assistantMessage.toolCalls ??= []
      if (!assistantMessage.toolCalls.some(existing => existing.id === toolCall.id)) {
        assistantMessage.toolCalls.push(toolCall)
      }
      const nextToolMessage = this.toolResult(toolCall, resultPayload)
      const existingIndex = state.continuationMessages.findIndex(
        message => message.role === "tool" && message.toolCallId === toolCall.id,
      )
      if (existingIndex >= 0) {
        state.continuationMessages[existingIndex] = nextToolMessage
      }
      else {
        state.continuationMessages.push(nextToolMessage)
      }
    }

    let platformToolCalled = false
    let toolDetailsCalled = false
    let searchToolsCalled = false
    let businessCardCalled = false
    let hasInteractiveCard = false
    let cardRepairExhaustedInBatch = false
    let recoverableToolError = false
    const hasPlatformTool = input.calls.some(call => !internalToolOperationIds.has(call.operationId))
    const bumpInternalReadCount = (operationId: string): number => {
      const next = (state.internalReadCallCounts.get(operationId) ?? 0) + 1
      state.internalReadCallCounts.set(operationId, next)
      return next
    }

    for (let callIndex = 0; callIndex < input.calls.length; callIndex += 1) {
      const toolCall = input.calls[callIndex]!
      if (state.cardRepairExhausted && isBusinessCardToolOperationId(toolCall.operationId)) {
        commitCallResult(toolCall, {
          status: "failed",
          errorCode: "ai.interaction_card_schema_invalid",
          retryable: false,
          guidance: "当前回合交互卡片自动修正次数已用尽，禁止继续调用卡片工具；请直接改用纯文本向用户说明结果或确认所需信息。",
        })
        continue
      }
      if (toolCall.argumentError) {
        if (isBusinessCardToolOperationId(toolCall.operationId) && !hasPlatformTool) {
          if (state.cardGeneration && state.cardGeneration.operationId !== toolCall.operationId) {
            await this.cards.fail(run.id, state.cardGeneration, "ai.interaction_card_schema_invalid")
            state.cardGeneration = undefined
          }
          state.cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments, runtimeSettings.maxCardRepairAttempts, toolCall.operationId, toolCall.id, batchId)
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
        if (!failure.retryable && isBusinessCardToolOperationId(toolCall.operationId)) {
          state.cardRepairExhausted = true
          cardRepairExhaustedInBatch = true
          state.cardGeneration = undefined
        }
        commitCallResult(toolCall, { ...failure })
        continue
      }
      if (isBusinessCardToolOperationId(toolCall.operationId)) {
        if (hasPlatformTool) {
          commitCallResult(toolCall, {
            status: "deferred_until_platform_results",
            guidance: "当前批次包含待执行或待审批的平台工具，交互卡片已延后；请等待平台工具返回真实结果后再调用卡片工具展示或请求输入。",
          })
          continue
        }
        if (state.cardGeneration && state.cardGeneration.operationId !== toolCall.operationId) {
          await this.cards.fail(run.id, state.cardGeneration, "ai.interaction_card_schema_invalid")
          state.cardGeneration = undefined
        }
        const parsed = businessCardToolInputs[toolCall.operationId].safeParse(toolCall.arguments)
        if (!parsed.success) {
          state.cardGeneration ??= await this.cards.start(run.id, run.turnId, toolCall.arguments, runtimeSettings.maxCardRepairAttempts, toolCall.operationId, toolCall.id, batchId)
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
          if (!failure.retryable) {
            state.cardRepairExhausted = true
            cardRepairExhaustedInBatch = true
            state.cardGeneration = undefined
          }
          commitCallResult(toolCall, { ...failure })
          continue
        }
        const cardInput = compileBusinessCardToolInput(toolCall.operationId, parsed.data)
        state.cardGeneration ??= await this.cards.start(run.id, run.turnId, cardInput, runtimeSettings.maxCardRepairAttempts, toolCall.operationId, toolCall.id, batchId)
        const creation = await this.traceInternalTool(toolCall.operationId, run.id, toolCall.arguments, () => this.cards.create(run.id, cardInput, state.cardGeneration!), toolCall.id)
        if (!creation.accepted) {
          recoverableToolError ||= creation.failure.retryable
          if (!creation.failure.retryable) {
            state.cardRepairExhausted = true
            cardRepairExhaustedInBatch = true
            state.cardGeneration = undefined
          }
          commitCallResult(toolCall, { ...creation.failure })
          continue
        }
        businessCardCalled = true
        if (creation.mode === "interactive") hasInteractiveCard = true
        state.cardGeneration = undefined
        commitCallResult(toolCall, {
          status: "succeeded",
          workflowState: creation.mode === "interactive"
            ? "awaiting_user_input"
            : "evidence_presented",
          completionEvidence: false,
          guidance: creation.mode === "interactive"
            ? "卡片正在等待用户提交输入或选择；这不代表业务目标已经完成。"
            : "卡片已完成结果或实时进度呈现。单步异步任务在生成 live_task 卡片后即可直接给出简要说明收口本轮；若后续仍有依赖步骤则请按需继续执行，不要重复生成同一张卡片。",
        })
        continue
      }
      if (toolCall.operationId === "wait_for_task") {
        const parsedWait = waitForTaskInput.safeParse(toolCall.arguments)
        if (!parsedWait.success) {
          const errorCode = "ai.tool_arguments_invalid"
          const issues = parsedWait.error.issues.map(issue => ({ path: issue.path, code: issue.code }))
          await this.internalTools.recordToolCall(run.id, run.turnId, "wait_for_task", toolCall.arguments, "failed", { issues }, errorCode, toolCall.id, undefined, batchId)
          recoverableToolError = true
          commitCallResult(toolCall, {
            status: "failed",
            errorCode,
            issues,
            retryable: true,
            guidance: "请提供合法的 projectId、taskType（build_run 或 release）、taskId 与可选 timeoutSeconds（15..1800）后重试。",
          })
          continue
        }
        const waitArgs = parsedWait.data
        const fastCheckStartedAt = performance.now()
        if (!this.taskStatusClient) {
          const errorCode = "ai.tool_not_available"
          await this.internalTools.recordToolCall(run.id, run.turnId, "wait_for_task", waitArgs, "failed", { errorCode }, errorCode, toolCall.id, performance.now() - fastCheckStartedAt, batchId)
          commitCallResult(toolCall, {
            status: "failed",
            errorCode,
            retryable: false,
            guidance: "当前执行环境未启用后台任务状态客户端。",
          })
          continue
        }
        const check = await this.traceInternalTool(
          "wait_for_task",
          run.id,
          waitArgs,
          () => this.taskStatusClient!.checkTaskStatus({
            runId: run.id,
            projectId: waitArgs.projectId,
            taskType: waitArgs.taskType,
            taskId: waitArgs.taskId,
            signal,
          }),
          toolCall.id,
        )
        const fastCheckDurationMs = performance.now() - fastCheckStartedAt
        if (check.kind === "terminal_error") {
          await this.internalTools.recordToolCall(
            run.id,
            run.turnId,
            "wait_for_task",
            waitArgs,
            "failed",
            { errorCode: check.errorCode, httpStatus: check.status },
            check.errorCode,
            toolCall.id,
            fastCheckDurationMs,
            batchId,
          )
          commitCallResult(toolCall, {
            status: "failed",
            errorCode: check.errorCode,
            retryable: false,
            guidance: "无法读取目标后台任务状态（任务不存在或当前用户无访问权限），请核对参数并向用户如实说明。",
          })
          continue
        }
        if (check.kind === "ok" && check.snapshot.terminal) {
          const formatted = formatWaitForTaskResult(check.snapshot, { timedOut: false, durationMs: fastCheckDurationMs })
          await this.internalTools.recordToolCall(
            run.id,
            run.turnId,
            "wait_for_task",
            waitArgs,
            "succeeded",
            formatted,
            undefined,
            toolCall.id,
            fastCheckDurationMs,
            batchId,
          )
          commitCallResult(toolCall, formatted)
          continue
        }
        const initialSummary = check.kind === "ok"
          ? {
              projectId: check.snapshot.projectId,
              taskType: check.snapshot.taskType,
              taskId: check.snapshot.taskId,
              taskStatus: check.snapshot.taskStatus,
              ...(check.snapshot.applicationId ? { applicationId: check.snapshot.applicationId } : {}),
              ...(check.snapshot.deploymentTargetId ? { deploymentTargetId: check.snapshot.deploymentTargetId } : {}),
            }
          : {
              projectId: waitArgs.projectId,
              taskType: waitArgs.taskType,
              taskId: waitArgs.taskId,
              observationCode: check.errorCode,
            }
        const started = await this.internalTools.startWaitForTask(run.id, run.turnId, waitArgs, initialSummary, toolCall.id, batchId)
        const nowMs = Date.now()
        const startedAt = new Date(nowMs).toISOString()
        const deadlineMs = nowMs + waitArgs.timeoutSeconds * 1_000
        const deadlineAt = new Date(deadlineMs).toISOString()
        const wakeAt = new Date(Math.min(deadlineMs, nowMs + computeWakeBackoffMs(0))).toISOString()
        const remainingCalls = toPendingToolCalls(input.calls.slice(callIndex + 1))
        if ((assistantMessage.toolCalls?.length ?? 0) === 0 && !assistantMessage.content.trim() && !assistantMessage.reasoning) {
          this.removeTrailingEmptyAssistantMessage(state, assistantMessage)
        }
        return {
          kind: "waiting_external",
          waitingTask: {
            itemId: started.itemId,
            toolCallId: toolCall.id,
            projectId: waitArgs.projectId,
            taskType: waitArgs.taskType,
            taskId: waitArgs.taskId,
            timeoutSeconds: waitArgs.timeoutSeconds,
            startedAt,
            deadlineAt,
            pollCount: 0,
          },
          wakeAt,
          ...(batchId ? { batchId } : {}),
          ...(remainingCalls.length ? { remainingCalls } : {}),
        }
      }
      if (toolCall.operationId === "get_tool_details") {
        toolDetailsCalled = true
        const readCallCount = bumpInternalReadCount("get_tool_details")
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
            undefined,
            batchId,
          )
          recoverableToolError = true
          commitCallResult(toolCall, {
            status: "failed",
            errorCode,
            retryable: true,
            guidance: withSameOperationSoftWarning(
              "请提供一到八个精确 operationId 后，在当前 Run 内重试一次 get_tool_details。",
              "get_tool_details",
              readCallCount,
            ),
          })
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
            const selection = await this.repository.touchRunSelectedOperations(
              run.id,
              base.loadedOperationIds,
              selectedOperationLimit,
              state.executedOperationIds,
            )
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
            batchId,
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
            batchId,
          )
          throw error
        }
        const itemCount = detailsResult.items.length
        const detailsOutcome = alreadyLoaded ? "duplicate" : itemCount ? "succeeded" : "empty"
        agentMetrics.toolDetailLoads.add(1, { outcome: detailsOutcome })
        agentMetrics.toolDetailItems.record(itemCount, { outcome: detailsOutcome })
        commitCallResult(toolCall, {
          status: itemCount ? "succeeded" : "empty",
          ...detailsResult,
          guidance: withSameOperationSoftWarning(
            detailsResult.loadedOperationIds.length
              ? "这些工具已加入本轮后续模型步骤。请继续调用最适合的具体工具；目录详情不代表业务执行结果。"
              : "没有加载到已准入工具。请核对 operationId，或先用 search_tools 检索权威目录。",
            "get_tool_details",
            readCallCount,
          ),
        })
        continue
      }
      if (toolCall.operationId === "search_tools") {
        searchToolsCalled = true
        const readCallCount = bumpInternalReadCount("search_tools")
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
            undefined,
            batchId,
          )
          recoverableToolError = true
          commitCallResult(toolCall, {
            status: "failed",
            errorCode,
            retryable: true,
            guidance: withSameOperationSoftWarning(
              "请修正 query、page 或 pageSize 后，在当前 Run 内重试一次 search_tools。",
              "search_tools",
              readCallCount,
            ),
          })
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
            const selection = await this.repository.touchRunSelectedOperations(
              run.id,
              candidates,
              selectedOperationLimit,
              state.executedOperationIds,
            )
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
            batchId,
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
            batchId,
          )
          throw error
        }
        const searchOutcome = alreadySearched ? "duplicate" : searchResult.items.length ? "succeeded" : "no_matches"
        agentMetrics.toolSearches.add(1, { outcome: searchOutcome })
        agentMetrics.toolSearchMatches.record(searchResult.items.length, { outcome: searchOutcome })
        commitCallResult(toolCall, {
          status: searchResult.items.length ? "succeeded" : "no_matches",
          ...searchResult,
          guidance: withSameOperationSoftWarning(
            searchResult.loadedOperationIds.length
              ? "最相关候选已经加入当前 Run，下一步请直接调用最适合的具体工具。只有需要参数语义、相似能力消歧或风险确认时才调用 get_tool_details；目录结果不代表业务已执行。"
              : searchResult.items.length
                ? "这是只浏览不自动加载的目录页；需要执行时请用非空 query 精确检索，或调用 get_tool_details 选择精确 operationId。"
                : "没有匹配工具。请改用更具体的业务目标检索一次；仍无结果时再如实说明能力缺失。",
            "search_tools",
            readCallCount,
          ),
        })
        continue
      }
      if (toolCall.operationId === "search_docs" || toolCall.operationId === "list_docs" || toolCall.operationId === "read_doc") {
        const readCallCount = bumpInternalReadCount(toolCall.operationId)
        const docsResult = await this.runDocsTool(run, toolCall, batchId)
        if (docsResult.status === "failed") recoverableToolError = true
        const mergedGuidance = withSameOperationSoftWarning(
          typeof docsResult.guidance === "string" ? docsResult.guidance : undefined,
          toolCall.operationId,
          readCallCount,
        )
        commitCallResult(toolCall, mergedGuidance ? { ...docsResult, guidance: mergedGuidance } : docsResult)
        continue
      }
      if (toolCall.operationId === "rename_conversation") {
        const outcome = await this.traceInternalTool("rename_conversation", run.id, toolCall.arguments, () => this.internalTools.renameConversation(run.id, run.turnId, run.conversationId, toolCall.arguments, toolCall.id, batchId), toolCall.id)
        if (!outcome.ok) {
          recoverableToolError = true
          commitCallResult(toolCall, {
            status: "failed",
            code: outcome.errorCode,
            errorCode: outcome.errorCode,
            retryable: true,
            issues: outcome.issues,
            guidance: "rename_conversation 参数校验失败；请提供 1..120 字符的非空 title，或直接继续回答用户问题。",
          })
          continue
        }
        if (outcome.renamed) {
          state.assistantRenamed = true
          state.conversation = { ...state.conversation, title: outcome.renamed.title, titleSource: outcome.renamed.titleSource }
        }
        commitCallResult(toolCall, {
          status: outcome.renamed ? "succeeded" : "skipped",
          ...(outcome.renamed ? { title: outcome.renamed.title } : {}),
        })
        continue
      }
      if (toolCall.operationId === "navigate_to_route") {
        const outcome = await this.traceInternalTool("navigate_to_route", run.id, toolCall.arguments, () => this.internalTools.navigateToRoute(run.id, run.turnId, toolCall.arguments, toolCall.id, batchId), toolCall.id)
        if (!outcome.ok) {
          recoverableToolError = true
          commitCallResult(toolCall, {
            status: "failed",
            code: outcome.errorCode,
            errorCode: outcome.errorCode,
            retryable: true,
            issues: outcome.issues,
            guidance: "navigate_to_route 参数校验失败；请仅使用已注册内部路由名（routeName）与合法标识符参数，或直接以文本回答。",
          })
          continue
        }
        commitCallResult(toolCall, {
          status: "succeeded",
          guidance: "已在时间线生成可点击的内部导航按钮；页面不会自动切换。",
        })
        continue
      }
      platformToolCalled = true
      state.executedOperationIds.add(toolCall.operationId)
      state.selectedOperationIds = (await this.repository.touchRunSelectedOperations(
        run.id,
        [toolCall.operationId],
        selectedOperationLimit,
        state.executedOperationIds,
      )).selectedOperationIds
      let call: Awaited<ReturnType<ToolOrchestrator["propose"]>>
      try {
        call = await this.tools.propose({
          runId: run.id,
          operationId: toolCall.operationId,
          arguments: toolCall.arguments,
          inputMode: "model",
          modelCallId: toolCall.id,
          ...(batchId ? { batchId } : {}),
        }, signal)
      }
      catch (error) {
        if (error instanceof ToolInterruption) throw error
        if (!(error instanceof ToolLoopStoppedError)) throw error
        recoverableToolError = true
        commitCallResult(toolCall, {
          status: "failed",
          ...error.toJSON(),
          guidance: "工具循环保护已停止这次调用；不要原样重试。请基于现有结果回答，或改用参数和信息来源都不同的下一步。",
        })
        continue
      }
      if (call.status === "awaiting_approval") {
        telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id, "tool.name": call.operationId })
        const remainingCalls = toPendingToolCalls(input.calls.slice(callIndex + 1))
        if ((assistantMessage.toolCalls?.length ?? 0) === 0 && !assistantMessage.content.trim() && !assistantMessage.reasoning) {
          this.removeTrailingEmptyAssistantMessage(state, assistantMessage)
        }
        return {
          kind: "waiting_approval",
          toolCallId: call.id,
          ...(batchId ? { batchId } : {}),
          ...(remainingCalls.length ? { remainingCalls } : {}),
        }
      }
      const isReadOrDiscovery = await this.tools.isReadOrDiscoveryOperation(run.id, toolCall.operationId)
      const platformCallCount = isReadOrDiscovery
        ? this.tools.operationCallCount(run.id, toolCall.operationId)
        : 0
      const spinWarning = platformCallCount >= DEFAULT_SAME_OPERATION_SOFT_WARN_THRESHOLD
        ? sameOperationSoftWarnGuidance(toolCall.operationId)
        : undefined
      const outcome = await this.tools.outcome(call.id)
      if (outcome.kind === "failed") {
        recoverableToolError ||= outcome.retryable
        commitCallResult(toolCall, {
          status: "failed",
          errorCode: outcome.errorCode,
          retryable: outcome.retryable,
          ...(outcome.result !== undefined
            ? { result: outcome.result }
            : call.result !== undefined
              ? { result: call.result }
              : {}),
          ...(spinWarning ? { guidance: spinWarning } : {}),
        })
        continue
      }
      commitCallResult(toolCall, {
        status: call.status,
        ...(call.result !== undefined ? { result: call.result } : {}),
        ...(call.errorCode ? { errorCode: call.errorCode } : {}),
        ...(spinWarning ? { guidance: spinWarning } : {}),
      })
    }
    if (!platformToolCalled && businessCardCalled && hasInteractiveCard && !recoverableToolError && !toolDetailsCalled && !searchToolsCalled)
      return { kind: "complete" }
    if (recoverableToolError || cardRepairExhaustedInBatch || toolDetailsCalled || searchToolsCalled)
      return { kind: "continue" }
    if (!platformToolCalled && businessCardCalled)
      return { kind: "continue" }
    return { kind: "continue" }
  }

  async recoverInterruptedBatch(
    runId: string,
    state: ToolStepState,
  ): Promise<{ awaitingToolCallId?: string }> {
    const calls = await this.tools.recoverToolBatch(runId)
    const modelCalls = calls.filter(call => call.inputMode !== "verification")
    const awaitingApproval = modelCalls.find(call => call.status === "awaiting_approval")
    if (modelCalls.some(call => call.status === "running")) throw new Error("ai.tool_outcome_unknown")

    const execution = await this.repository.getExecutionInput(runId)
    if (!execution) throw new Error("ai.turn_not_found")
    const settledToolCallIds = new Set(state.continuationMessages.flatMap(message =>
      message.role === "tool" ? [message.toolCallId] : []))
    const reasoningByCallItem = reasoningByToolCallItemId(execution.toolInteractions)
    for (const call of modelCalls) {
      const itemId = `${call.id}:item`
      const projected = execution.toolInteractions.find(item => item.itemId === itemId)
      if (call.status === "skipped" && call.executionCount === 0) {
        if (projected && projected.status === "streaming") {
          await this.repository.updateItemWithEvent(itemId, "completed", redact({
            ...projected.content,
            status: "skipped",
            errorCode: call.errorCode ?? "ai.agent_restarted",
          }), "item.finalized", { itemId, toolCallId: call.id })
        }
        continue
      }
      if (call.status === "proposed" || call.status === "awaiting_approval") continue
      const projectedModelCallId = typeof projected?.content.modelCallId === "string" && projected.content.modelCallId
        ? projected.content.modelCallId
        : undefined
      const projectedBatchId = typeof projected?.content.batchId === "string" && projected.content.batchId
        ? projected.content.batchId
        : undefined
      const resolvedModelCallId = call.modelCallId ?? projectedModelCallId
      const resolvedBatchId = call.batchId ?? projectedBatchId
      const result = execution.toolInteractions.find(item => item.type === "tool_result" && item.content.relatedItemId === itemId)
      const content = redact({
        ...projected?.content,
        toolCallId: call.id,
        ...(resolvedModelCallId ? { modelCallId: resolvedModelCallId } : {}),
        ...(resolvedBatchId ? { batchId: resolvedBatchId } : {}),
        operationId: call.operationId,
        status: call.status,
        arguments: call.arguments,
        result: call.result,
        errorCode: call.errorCode,
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
      const outcome = await this.tools.outcome(call.id)
      const effectiveCallId = resolvedModelCallId ?? call.id
      if (settledToolCallIds.has(effectiveCallId) || settledToolCallIds.has(call.id)) continue
      const toolCall = { id: effectiveCallId, operationId: call.operationId, arguments: call.arguments }
      const reasoning = reasoningByCallItem.get(itemId)
      state.continuationMessages.push({ role: "assistant", content: "", toolCalls: [toolCall], ...(reasoning ? { reasoning } : {}) })
      state.continuationMessages.push(this.toolResult(toolCall, {
        status: outcome.kind === "failed" ? "failed" : call.status,
        ...(call.result !== undefined ? { result: call.result } : {}),
        ...(outcome.kind === "failed" ? { errorCode: outcome.errorCode } : call.errorCode ? { errorCode: call.errorCode } : {}),
      }))
      settledToolCallIds.add(effectiveCallId)
      settledToolCallIds.add(call.id)
    }
    return {
      ...(awaitingApproval ? { awaitingToolCallId: awaitingApproval.id } : {}),
    }
  }

  async failCard(runId: string, state: ToolStepState, errorCode: string): Promise<void> {
    if (!state.cardGeneration) return
    await this.cards.fail(runId, state.cardGeneration, errorCode)
    state.cardGeneration = undefined
  }

  private ensureBatchAssistantMessage(
    state: ToolStepState,
    answer: string,
    reasoning: string,
    appendToExistingBatch: boolean,
  ): Extract<ModelMessage, { role: "assistant" }> {
    if (appendToExistingBatch) {
      for (let index = state.continuationMessages.length - 1; index >= 0; index -= 1) {
        const candidate = state.continuationMessages[index]!
        if (candidate.role === "assistant" && Array.isArray(candidate.toolCalls)) {
          return candidate
        }
        if (candidate.role === "user") break
      }
    }
    const created: Extract<ModelMessage, { role: "assistant" }> = {
      role: "assistant",
      content: answer,
      toolCalls: [],
      ...(reasoning ? { reasoning } : {}),
    }
    state.continuationMessages.push(created)
    return created
  }

  private removeTrailingEmptyAssistantMessage(
    state: ToolStepState,
    target: Extract<ModelMessage, { role: "assistant" }>,
  ): void {
    const lastIndex = state.continuationMessages.lastIndexOf(target)
    if (lastIndex >= 0) {
      state.continuationMessages.splice(lastIndex, 1)
    }
  }

  private toolResult(toolCall: ModelToolCall & { id: string }, result: Record<string, unknown>): ModelMessage {
    return toolResultMessage(toolCall, result)
  }

  /** 公开文档检索与读取：只读、无平台副作用，结果按普通内部工具记录到时间线。 */
  private async runDocsTool(run: ClaimedRun, toolCall: ModelToolCall & { id: string }, batchId?: string): Promise<Record<string, unknown>> {
    const operationId = toolCall.operationId as "search_docs" | "list_docs" | "read_doc"
    const parsed = operationId === "search_docs"
      ? searchDocsInput.safeParse(toolCall.arguments)
      : operationId === "list_docs" ? listDocsInput.safeParse(toolCall.arguments) : readDocInput.safeParse(toolCall.arguments)
    if (!parsed.success) {
      const errorCode = "ai.tool_arguments_invalid"
      const issues = parsed.error.issues.map(issue => ({ path: issue.path, code: issue.code }))
      await this.internalTools.recordToolCall(run.id, run.turnId, operationId, toolCall.arguments, "failed", { issues }, errorCode, toolCall.id, undefined, batchId)
      return { status: "failed", errorCode, issues, retryable: true, guidance: `请按 issues 修正参数后，在当前 Run 内重试一次 ${operationId}。` }
    }
    const startedAt = performance.now()
    const knowledge = this.docsKnowledge ?? bundledDocsKnowledge()
    const result = await this.traceInternalTool(operationId, run.id, parsed.data, async () => {
      if (operationId === "search_docs") {
        const input = parsed.data as SearchDocsInput
        const items = knowledge.search(input.query, input.locale, input.limit)
        return items.length
          ? { status: "succeeded", docsVersion: knowledge.digest, items, guidance: "这些是与问题最相关的文档章节摘要。回答配置语法、变量或限制前，用 read_doc 读取对应章节原文核对；引用时附上文档标题或链接。" }
          : { status: "no_matches", docsVersion: knowledge.digest, items, guidance: "公开文档中没有匹配章节。可换用更具体的名词检索一次；仍无结果时如实说明文档未覆盖，不要编造。" }
      }
      if (operationId === "list_docs") {
        const input = parsed.data as ListDocsInput
        const listing = knowledge.list(input.locale, input.path, input.depth)
        return listing
          ? { status: "succeeded", docsVersion: knowledge.digest, ...listing, guidance: "目录树只包含标题和简短描述。选出相关文档后用 read_doc 读取正文；需要更深一层时把目录的 path 传回 list_docs。" }
          : { status: "not_found", errorCode: "ai.docs_not_found", guidance: "目录不存在。请从 path 为空的根目录开始逐级浏览，不要猜测路径。" }
      }
      const input = parsed.data as ReadDocInput
      const document = knowledge.read(input.docId, input.sectionId)
      return document
        ? { status: "succeeded", docsVersion: knowledge.digest, ...document }
        : { status: "not_found", errorCode: "ai.docs_not_found", guidance: "docId 或 sectionId 不存在。请使用 search_docs 返回的值，不要猜测。" }
    }, toolCall.id)
    const status = result.status === "not_found" ? "failed" : "succeeded"
    await this.internalTools.recordToolCall(run.id, run.turnId, operationId, parsed.data, status, result, status === "failed" ? "ai.docs_not_found" : undefined, toolCall.id, performance.now() - startedAt, batchId)
    return result
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

function toPendingToolCalls(calls: Array<ModelToolCall & { id: string }>): RunPendingToolCall[] {
  return calls.map(call => ({
    id: call.id,
    operationId: call.operationId,
    arguments: call.arguments,
    ...(call.argumentError ? { argumentError: call.argumentError } : {}),
  }))
}

const internalReadToolOperationIds = new Set([
  "search_tools",
  "get_tool_details",
  "search_docs",
  "list_docs",
  "read_doc",
])

function reconstructedExecutedOperationIds(interactions: RunExecutionInput["toolInteractions"]): Set<string> {
  const executed = new Set<string>()
  for (const item of interactions) {
    if (item.type !== "tool_call") continue
    const operationId = typeof item.content.operationId === "string" ? item.content.operationId : undefined
    if (!operationId || internalToolOperationIds.has(operationId) || item.content.status === "skipped") continue
    executed.add(operationId)
  }
  return executed
}

function reconstructedInternalReadCallCounts(interactions: RunExecutionInput["toolInteractions"]): Map<string, number> {
  const counts = new Map<string, number>()
  for (const item of interactions) {
    if (item.type !== "tool_call") continue
    const operationId = typeof item.content.operationId === "string" ? item.content.operationId : undefined
    if (!operationId || !internalReadToolOperationIds.has(operationId)) continue
    counts.set(operationId, (counts.get(operationId) ?? 0) + 1)
  }
  return counts
}
