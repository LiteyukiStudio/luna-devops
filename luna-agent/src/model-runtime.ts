import type { ContextCompiler, ContextCompilerOptions } from "./context/compiler.js"
import type { AIModelSnapshot, ConversationHistoryEntry, ConversationTitleSource, PromptVersion } from "./domain.js"
import type {
  ModelEvent,
  ModelMessage,
  ModelProvider,
  ModelResponse,
  ModelToolCall,
  ModelToolDefinition,
  ModelToolDetailsResult,
  ModelToolResolver,
  ModelToolSearchResult,
} from "./provider/provider.js"
import { systemPromptFor } from "./prompt/system.js"
import { renameConversationTool } from "./tools/conversation-title.js"
import { isProviderContextLengthError } from "./provider/provider-error.js"
import type { RemoteProviderConfig } from "./provider/config-client.js"

/** ModelRuntime 在 provider 事件之上额外透出的事件。
 *  当前用于让上层在压缩实际发生时向时间线写入一条用户可见的系统提示。 */
export type ModelRuntimeEvent = ModelEvent
  | {
      type: "context.compacted"
      summarizedThroughTurnIndex: number
      sourceTurnCount: number
      trigger: "fixed_threshold" | "context_error"
    }
import { defaultRuntimeSettings, runtimeSettingsSnapshot, type RuntimeSettings } from "./runtime-settings.js"
import { modelVisibleHistory } from "./model-history.js"
import { boundContinuationMessages, boundHistoryMessages, turnPromptMessages } from "./context/model-messages.js"

export type ConversationPromptContext = {
  title: string
  titleSource: ConversationTitleSource
  turnIndex: number
}

export type AssistantModelInput = {
  runId: string
  ownerUserId: string
  conversationId: string
  input: string
  pageContext: Record<string, unknown>
  history: ConversationHistoryEntry[]
  conversation: ConversationPromptContext
  promptVersion: PromptVersion
  reasoningSummary: string
  answer: string
  toolCalls: ModelToolCall[]
  continuationMessages: ModelMessage[]
  loadedOperationIds: string[]
  toolCatalogDigest: string
  model?: AIModelSnapshot
  runtimeSettings?: RuntimeSettings
  providerConfig?: RemoteProviderConfig
}

/**
 * 模型运行时只负责编译上下文、解析可用工具并调用 Provider。
 * Agent 循环、工具执行和暂停/恢复都由 RunExecutor 统一编排。
 */
export class ModelRuntime {
  private readonly resolveTools: (
    pageContext: Record<string, unknown>,
    userInput: string,
    loadedOperationIds: string[],
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ) => ModelToolDefinition[] | Promise<ModelToolDefinition[]>
  private readonly searchTools?: (
    input: { query?: string, page?: number, pageSize?: number },
    pageContext: Record<string, unknown>,
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ) => ModelToolSearchResult | Promise<ModelToolSearchResult>
  private readonly getToolDetails?: (operationIds: string[], toolCatalogDigest?: string) => ModelToolDetailsResult | Promise<ModelToolDetailsResult>

  constructor(
    private readonly provider: ModelProvider,
    tools: ModelToolResolver = [],
    private readonly contextCompiler?: ContextCompiler,
  ) {
    if (Array.isArray(tools)) this.resolveTools = () => tools
    else if (typeof tools === "function") this.resolveTools = tools
    else {
      this.resolveTools = tools.resolve
      this.searchTools = tools.search
      this.getToolDetails = tools.details
    }
  }

  async *stream(input: AssistantModelInput, signal?: AbortSignal): AsyncIterable<ModelRuntimeEvent> {
    const requestInput = modelInputSnapshot(input)
    let forceCompressionTrigger: "context_error" | undefined
    let contextErrorCause: unknown
    for (let attempt = 1; attempt <= 2; attempt += 1) {
      const { request, compaction } = await this.modelRequest(requestInput, signal, forceCompressionTrigger)
      if (forceCompressionTrigger === "context_error" && !compaction)
        throw new Error("ai.model_context_insufficient", { cause: contextErrorCause })
      if (compaction) yield { type: "context.compacted", ...compaction }
      try {
        yield* this.provider.stream(request)
        return
      }
      catch (error) {
        if (!isProviderContextLengthError(error)) throw error
        contextErrorCause = error
        if (attempt >= 2)
          throw new Error("ai.model_context_insufficient", { cause: error })
        forceCompressionTrigger = "context_error"
      }
    }
  }

  async complete(input: AssistantModelInput, signal?: AbortSignal): Promise<ModelResponse> {
    const { request } = await this.modelRequest(modelInputSnapshot(input), signal)
    return this.provider.complete(request)
  }

  async searchAvailableTools(
    input: { query?: string, page?: number, pageSize?: number },
    pageContext: Record<string, unknown>,
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ): Promise<ModelToolSearchResult> {
    if (!this.searchTools) {
      return {
        query: input.query?.trim() ?? "",
        items: [],
        page: input.page ?? 1,
        pageSize: input.pageSize ?? 20,
        total: 0,
        totalPages: 0,
        loadedOperationIds: [],
        missingOperationIds: [],
        catalogDigest: toolCatalogDigest ?? "",
        duplicate: false,
        cacheHit: false,
      }
    }
    return this.searchTools(input, pageContext, signal, toolCatalogDigest)
  }

  async getAvailableToolDetails(operationIds: string[], toolCatalogDigest?: string): Promise<ModelToolDetailsResult> {
    if (!this.getToolDetails) return { items: [], loadedOperationIds: [], alreadySelectedOperationIds: [], missingOperationIds: operationIds, catalogDigest: toolCatalogDigest ?? "", duplicate: false, cacheHit: false }
    return this.getToolDetails(operationIds, toolCatalogDigest)
  }

  async generateConversationTitle(
    input: string,
    answer: string,
    budget: { runId: string, ownerUserId: string },
    signal?: AbortSignal,
    model?: AIModelSnapshot,
    providerConfig?: RemoteProviderConfig,
  ): Promise<string | undefined> {
    const response = await this.provider.complete({
      messages: [
        { role: "system", content: "根据会话内容生成一个简洁标题，并使用用户当前语言。只返回标题，不要添加引号、Markdown、句末标点或解释；标题不超过 30 个字符。" },
        { role: "user", content: `用户消息：${input}\n助手回复：${answer.slice(0, 600)}` },
      ],
      maxOutputTokens: 48,
      budget: { ...budget, operation: "title" },
      ...(signal ? { signal } : {}),
      ...(model ? { modelId: model.id, modelName: model.name, modelPricing: model } : {}),
      ...(providerConfig ? { providerConfig } : {}),
    })
    const title = response.text.trim().split(/\r?\n/, 1)[0]?.replace(/^["'“”‘’]+|["'“”‘’。.！!？?]+$/g, "").trim()
    return title ? [...title].slice(0, 60).join("") : undefined
  }

  private async modelRequest(input: AssistantModelInput, signal?: AbortSignal, forceCompressionTrigger?: "context_error") {
    const runtimeSettings = input.runtimeSettings ?? defaultRuntimeSettings
    const maxOutputTokens = Math.min(
      runtimeSettings.assistantMaxOutputTokens,
      input.model?.maxOutputTokens ?? runtimeSettings.assistantMaxOutputTokens,
    )
    const tools = await this.modelTools(
      input.pageContext,
      input.conversation,
      input.input,
      input.loadedOperationIds,
      signal,
      input.toolCatalogDigest,
    )
    const base = modelMessageParts(
      input.promptVersion,
      input.input,
      input.pageContext,
      input.conversation,
      tools.map(tool => tool.operationId),
      input.history,
    )
    const history = modelVisibleHistory(input.history)
    const compiled = this.contextCompiler
      ? await this.contextCompiler.compile({
          conversationId: input.conversationId,
          beforeTurnIndex: input.conversation.turnIndex,
          systemMessages: base.systemMessages,
          currentMessages: base.currentMessages,
          history,
          continuationMessages: input.continuationMessages,
          tools,
          budget: { runId: input.runId, ownerUserId: input.ownerUserId },
          maxOutputTokens,
          options: contextCompilerOptions(runtimeSettings),
          ...(input.providerConfig ? { providerConfig: input.providerConfig } : {}),
          ...(input.model ? { model: input.model } : {}),
          ...(signal ? { signal } : {}),
          ...(forceCompressionTrigger ? { forceCompressionTrigger } : {}),
        })
      : undefined
    const messages = compiled
      ? compiled.messages
      : modelMessages(base.systemMessages, base.currentMessages, history.slice(-4), input.continuationMessages, runtimeSettings)
    const conversationCompacted = compiled
      ? compiled.compressionOutcome === "compressed"
        || compiled.compressionOutcome === "reused"
      : undefined
    // 仅“本轮实际发生了压缩”才需要透传事件；
    // reused 已在之前轮次通知过，避免重复提醒。
    const compaction = compiled?.compaction
    return {
      request: {
        messages,
        maxOutputTokens,
        budget: { runId: input.runId, ownerUserId: input.ownerUserId, operation: "assistant" as const },
        tools,
        conversationId: input.conversationId,
        ...(conversationCompacted !== undefined ? { conversationCompacted } : {}),
        ...(signal ? { signal } : {}),
        ...(input.model ? { modelId: input.model.id, modelName: input.model.name, modelPricing: input.model } : {}),
        ...(input.providerConfig ? { providerConfig: input.providerConfig } : {}),
      },
      compaction,
    }
  }

  private async modelTools(
    pageContext: Record<string, unknown>,
    conversation: ConversationPromptContext,
    userInput: string,
    loadedOperationIds: string[] = [],
    signal?: AbortSignal,
    toolCatalogDigest?: string,
  ) {
    return [
      ...await this.resolveTools(pageContext, userInput, loadedOperationIds, signal, toolCatalogDigest),
      ...(conversation.titleSource === "user" ? [] : [renameConversationTool]),
    ].sort((left, right) => left.operationId < right.operationId ? -1 : left.operationId > right.operationId ? 1 : 0)
  }
}

function modelMessageParts(
  promptVersion: PromptVersion,
  input: string,
  pageContext: Record<string, unknown>,
  conversation: ConversationPromptContext,
  loadedOperationIds: string[],
  history: ConversationHistoryEntry[],
) {
  return {
    systemMessages: [{
      role: "system" as const,
      content: systemPromptFor(promptVersion),
    }],
    currentMessages: turnPromptMessages(input, pageContext, conversation.turnIndex, loadedOperationIds, history),
  }
}

function modelMessages(
  systemMessages: ModelMessage[],
  currentMessages: ModelMessage[],
  history: ConversationHistoryEntry[],
  continuationMessages: ModelMessage[],
  runtimeSettings: RuntimeSettings,
) {
  return [
    ...systemMessages,
    ...boundHistoryMessages(history, runtimeSettings.contextMaxHistoryPayloadBytes),
    ...currentMessages,
    ...boundContinuationMessages(continuationMessages, runtimeSettings.contextMaxContinuationPayloadBytes),
  ]
}

function modelInputSnapshot(input: AssistantModelInput): AssistantModelInput {
  return {
    ...input,
    runtimeSettings: runtimeSettingsSnapshot(input.runtimeSettings ?? defaultRuntimeSettings),
  }
}

function contextCompilerOptions(runtimeSettings: RuntimeSettings): ContextCompilerOptions {
  return Object.freeze({
    recentTurnCount: runtimeSettings.contextRecentTurnCount,
    maxUncompressedTurnCount: runtimeSettings.contextMaxUncompressedTurnCount,
    maxCompressionTurnsPerCompile: runtimeSettings.contextMaxCompressionTurnsPerCompile,
    summaryMaxOutputTokens: runtimeSettings.contextSummaryMaxOutputTokens,
    maxHistoryPayloadBytes: runtimeSettings.contextMaxHistoryPayloadBytes,
    maxSummaryPayloadBytes: runtimeSettings.contextMaxSummaryPayloadBytes,
    maxContinuationPayloadBytes: runtimeSettings.contextMaxContinuationPayloadBytes,
  })
}
