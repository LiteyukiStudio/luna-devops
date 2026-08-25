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
import { recordAvailableTools } from "./telemetry.js"
import { renameConversationTool } from "./tools/conversation-title.js"
import { isProviderContextLengthError } from "./provider/provider-error.js"

/** ModelRuntime 在 provider 事件之上额外透出的事件。
 *  当前用于让上层在压缩实际发生时向时间线写入一条用户可见的系统提示。 */
export type ModelRuntimeEvent = ModelEvent
  | {
      type: "context.compacted"
      summarizedThroughTurnIndex: number
      sourceTurnCount: number
      trigger: "provider_usage" | "context_error" | "turn_backlog"
      priorPromptTokens?: number
    }
import { defaultRuntimeSettings } from "./runtime-settings.js"
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
}

/**
 * 模型运行时只负责编译上下文、解析可用工具并调用 Provider。
 * Agent 循环、工具执行和暂停/恢复都由 RunExecutor 统一编排。
 */
export class ModelRuntime {
  private assistantMaxOutputTokens = defaultRuntimeSettings.assistantMaxOutputTokens
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

  setContextOptions(options: Partial<ContextCompilerOptions>): void {
    this.contextCompiler?.setOptions(options)
  }

  setAssistantMaxOutputTokens(tokens: number): void {
    if (!Number.isSafeInteger(tokens) || tokens < 1)
      throw new Error("ai.max_output_tokens_invalid")
    this.assistantMaxOutputTokens = tokens
  }

  async *stream(input: AssistantModelInput, signal?: AbortSignal): AsyncIterable<ModelRuntimeEvent> {
    let forceCompressionTrigger: "context_error" | undefined
    let contextErrorCause: unknown
    for (let attempt = 1; attempt <= 3; attempt += 1) {
      const { request, compaction } = await this.modelRequest(input, signal, forceCompressionTrigger)
      if (forceCompressionTrigger === "context_error" && !compaction)
        throw new Error("ai.model_context_insufficient", { cause: contextErrorCause })
      recordAvailableTools(request.tools ?? [])
      if (compaction) yield { type: "context.compacted", ...compaction }
      try {
        yield* this.provider.stream(request)
        return
      }
      catch (error) {
        if (!isProviderContextLengthError(error)) throw error
        contextErrorCause = error
        if (attempt >= 3)
          throw new Error("ai.model_context_insufficient", { cause: error })
        forceCompressionTrigger = "context_error"
      }
    }
  }

  async complete(input: AssistantModelInput, signal?: AbortSignal): Promise<ModelResponse> {
    const { request } = await this.modelRequest(input, signal)
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

  async generateConversationTitle(input: string, answer: string, budget: { runId: string, ownerUserId: string }, signal?: AbortSignal, model?: AIModelSnapshot): Promise<string | undefined> {
    const response = await this.provider.complete({
      messages: [
        { role: "system", content: "根据会话内容生成一个简洁标题，并使用用户当前语言。只返回标题，不要添加引号、Markdown、句末标点或解释；标题不超过 30 个字符。" },
        { role: "user", content: `用户消息：${input}\n助手回复：${answer.slice(0, 600)}` },
      ],
      maxOutputTokens: 48,
      budget: { ...budget, operation: "title" },
      ...(signal ? { signal } : {}),
      ...(model ? { modelId: model.id, modelName: model.name, modelPricing: model } : {}),
    })
    const title = response.text.trim().split(/\r?\n/, 1)[0]?.replace(/^["'“”‘’]+|["'“”‘’。.！!？?]+$/g, "").trim()
    return title ? [...title].slice(0, 60).join("") : undefined
  }

  private async modelRequest(input: AssistantModelInput, signal?: AbortSignal, forceCompressionTrigger?: "context_error") {
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
          maxOutputTokens: this.assistantMaxOutputTokens,
          ...(input.model ? { model: input.model } : {}),
          ...(signal ? { signal } : {}),
          ...(forceCompressionTrigger ? { forceCompressionTrigger } : {}),
        })
      : undefined
    const messages = compiled
      ? compiled.messages
      : modelMessages(base.systemMessages, base.currentMessages, history.slice(-4), input.continuationMessages)
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
        maxOutputTokens: this.assistantMaxOutputTokens,
        budget: { runId: input.runId, ownerUserId: input.ownerUserId, operation: "assistant" as const },
        tools,
        conversationId: input.conversationId,
        ...(conversationCompacted !== undefined ? { conversationCompacted } : {}),
        ...(signal ? { signal } : {}),
        ...(input.model ? { modelId: input.model.id, modelName: input.model.name, modelPricing: input.model } : {}),
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
) {
  return [
    ...systemMessages,
    ...boundHistoryMessages(history, defaultRuntimeSettings.contextMaxHistoryPayloadBytes),
    ...currentMessages,
    ...boundContinuationMessages(continuationMessages, defaultRuntimeSettings.contextMaxContinuationPayloadBytes),
  ]
}
