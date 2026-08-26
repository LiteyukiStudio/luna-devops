import type {
  InteractionCardGroup,
  InteractionCardValidationFailure,
  InteractionCardValidationIssue,
} from "@luna-devops/ai-interaction-card-contract"
import { createId } from "../id.js"
import type { Repository } from "../persistence/repository.js"
import type { ModelToolCall } from "../provider/provider.js"
import { agentMetrics, telemetryLog } from "../telemetry.js"
import { createInteractionCardsInput } from "../tools/ui-cards.js"

const CARD_MAX_ATTEMPTS = 2
let maxCardRepairAttempts = CARD_MAX_ATTEMPTS

export function setMaxCardRepairAttempts(attempts: number): void {
  // 初次失败后只允许一次定向修复；第二次失败立即转为文字 fallback。
  void attempts
  maxCardRepairAttempts = CARD_MAX_ATTEMPTS
}

export type CardGeneration = {
  operationId: string
  itemId: string
  toolCallId: string
  timelineIndex: number
  generationId: string
  placeholderArguments: {
    schemaVersion: 1
    generationId: string
    title?: string
    description?: string
    placement: "inline" | "turn_end"
  }
  attempt: number
  status: "streaming" | "completed" | "failed"
  issues?: InteractionCardValidationIssue[]
}

// 交互卡片生成器：窄模型工具在这里统一投影为 create_interaction_cards
// 稳定 Timeline 协议，并管理占位、schema 校验、修复重试与终态。
// 每次失败都按真实模型 operationId 隔离，在同一占位项推进 attempt。
export class CardGenerationService {
  constructor(private readonly repository: Repository) {}

  async start(
    runId: string,
    turnId: string,
    raw: unknown,
    operationId = "create_interaction_cards",
  ): Promise<CardGeneration> {
    const rawObject = raw && typeof raw === "object" && !Array.isArray(raw) ? raw as Record<string, unknown> : {}
    const title = typeof rawObject.title === "string" && rawObject.title.trim()
      ? rawObject.title.trim().slice(0, 120)
      : undefined
    const description = typeof rawObject.description === "string" && rawObject.description.trim()
      ? rawObject.description.trim().slice(0, 500)
      : undefined
    const placement: "inline" | "turn_end" = rawObject.placement === "turn_end" ? "turn_end" : "inline"
    const generationId = createId("aicardgen")
    const placeholderArguments = {
      schemaVersion: 1 as const,
      generationId,
      ...(title ? { title } : {}),
      ...(description ? { description } : {}),
      placement,
    }
    const itemId = createId("aiitm")
    const toolCallId = createId("aitool")
    const { item } = await this.repository.appendItemWithEvent({
      id: itemId,
      runId,
      turnId,
      type: "tool_call",
      status: "streaming",
      content: {
        toolCallId,
        operationId: "create_interaction_cards",
        modelOperationId: operationId,
        titleKey: "aiAssistant.cards.preparingToolTitle",
        status: "running",
        arguments: placeholderArguments,
        result: {
          summaryKey: "aiAssistant.cards.preparing",
          generationId,
          attempt: 0,
          maxAttempts: maxCardRepairAttempts,
        },
      },
    }, "tool.started", {
      itemId,
      toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: operationId,
      titleKey: "aiAssistant.cards.preparingToolTitle",
      arguments: placeholderArguments,
    })
    const generation: CardGeneration = {
      operationId,
      itemId,
      toolCallId,
      timelineIndex: item.timelineIndex,
      generationId,
      placeholderArguments,
      attempt: 0,
      status: "streaming",
    }
    agentMetrics.cards.add(1, { phase: "started" })
    telemetryLog("agent.card.started", "info", {
      "luna.run.id": runId,
      "tool.name": operationId,
      "card.placement": placement,
    })
    return generation
  }

  async create(
    runId: string,
    raw: unknown,
    generation: CardGeneration,
  ) {
    const parsed = createInteractionCardsInput.safeParse(raw)
    if (!parsed.success) {
      const issues = validationIssues(parsed.error.issues)
      const attempt = await this.recordRepairFailure(runId, generation, issues, "ai.interaction_card_schema_invalid")
      const failure = cardValidationFailure("create", issues, attempt, generation.generationId, "ai.interaction_card_schema_invalid", generation.operationId)
      agentMetrics.cards.add(1, { phase: "rejected", mode: "unknown" })
      telemetryLog("agent.card.schema_rejected", "warn", {
        "luna.run.id": runId,
        "tool.name": generation.operationId,
        "operation": "agent.card.validate",
        "outcome": "rejected",
        "error.code": "ai.provider_invalid_tool_arguments",
        "error.type": "AgentCardValidationError",
        "error.message": "ai.provider_invalid_tool_arguments",
        "card.issue_count": issues.length,
      })
      return { accepted: false as const, failure }
    }
    const input = {
      ...parsed.data,
      generationId: generation.generationId,
    } as InteractionCardGroup
    const { itemId, toolCallId, timelineIndex } = generation
    const result = {
      summaryKey: "aiAssistant.cards.created",
      title: input.title,
      description: input.description,
    }
    await this.repository.updateItemWithEvent(itemId, "completed", {
      toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: generation.operationId,
      titleKey: "aiAssistant.cards.toolTitle",
      status: "succeeded",
      arguments: input,
      result,
    }, "tool.completed", {
      itemId,
      toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: generation.operationId,
      titleKey: "aiAssistant.cards.toolTitle",
      arguments: input,
      result,
      timelineIndex,
    })
    generation.status = "completed"
    agentMetrics.cards.add(1, { phase: "created", mode: input.mode })
    telemetryLog("agent.card.created", "info", {
      "luna.run.id": runId,
      "tool.name": generation.operationId,
      "card.mode": input.mode,
      "card.placement": input.placement ?? "inline",
    })
    return { accepted: true as const, mode: input.mode }
  }

  async recordRepairFailure(
    runId: string,
    generation: CardGeneration,
    issues: InteractionCardValidationIssue[],
    errorCode: InteractionCardValidationFailure["errorCode"],
  ): Promise<number> {
    const attempt = generation.attempt + 1
    generation.attempt = attempt
    generation.issues = issues
    if (attempt >= maxCardRepairAttempts) {
      await this.fail(runId, generation, errorCode)
      return attempt
    }
    await this.repository.updateItemWithEvent(generation.itemId, "streaming", {
      toolCallId: generation.toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: generation.operationId,
      titleKey: "aiAssistant.cards.preparingToolTitle",
      status: "running",
      arguments: generation.placeholderArguments,
      result: {
        summaryKey: "aiAssistant.cards.repairing",
        generationId: generation.generationId,
        attempt,
        maxAttempts: maxCardRepairAttempts,
        issues,
      },
    }, "tool.progress", {
      itemId: generation.itemId,
      toolCallId: generation.toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: generation.operationId,
      timelineIndex: generation.timelineIndex,
      result: cardValidationFailure("create", issues, attempt, generation.generationId, errorCode, generation.operationId),
    })
    return attempt
  }

  async fail(runId: string, generation: CardGeneration, errorCode: string) {
    if (generation.status !== "streaming") return
    generation.status = "failed"
    const canceled = errorCode === "ai.run_canceled"
    agentMetrics.cards.add(1, { phase: canceled ? "canceled" : "failed" })
    telemetryLog(canceled ? "agent.card.canceled" : "agent.card.failed", canceled ? "info" : "error", {
      "luna.run.id": runId,
      "tool.name": generation.operationId,
      "operation": "agent.card.generate",
      "outcome": canceled ? "cancelled" : "failed",
      ...(canceled
        ? {}
        : {
            "error.code": errorCode,
            "error.type": "AgentCardError",
            "error.message": errorCode,
          }),
    })
    const result = {
      summaryKey: "aiAssistant.cards.failed",
      errorCode,
      generationId: generation.generationId,
      attempt: generation.attempt,
      maxAttempts: maxCardRepairAttempts,
      ...(generation.issues?.length ? { issues: generation.issues } : {}),
    }
    await this.repository.updateItemWithEvent(generation.itemId, "failed", {
      toolCallId: generation.toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: generation.operationId,
      titleKey: "aiAssistant.cards.preparingToolTitle",
      status: "failed",
      arguments: generation.placeholderArguments,
      errorCode,
      result,
    }, "tool.failed", {
      itemId: generation.itemId,
      toolCallId: generation.toolCallId,
      operationId: "create_interaction_cards",
      modelOperationId: generation.operationId,
      titleKey: "aiAssistant.cards.preparingToolTitle",
      errorCode,
      result,
      timelineIndex: generation.timelineIndex,
      runId,
    })
  }
}

export function validationIssues(issues: readonly { code: string, path: PropertyKey[], message: string }[]): InteractionCardValidationIssue[] {
  return issues.slice(0, 12).map((issue) => {
    const details = issue as unknown as Record<string, unknown>
    const expected = typeof details.expected === "string" ? details.expected : undefined
    return {
      code: issue.code,
      path: issue.path.map(String).join(".") || "$",
      message: issue.message,
      ...(expected ? { expected } : {}),
    }
  })
}

export function cardValidationFailure(
  phase: InteractionCardValidationFailure["phase"],
  issues: InteractionCardValidationIssue[],
  attempt: number,
  generationId?: string,
  errorCode: InteractionCardValidationFailure["errorCode"] = "ai.interaction_card_schema_invalid",
  operationId = "create_interaction_cards",
): InteractionCardValidationFailure {
  const retryable = attempt < maxCardRepairAttempts
  return {
    status: "rejected",
    errorCode,
    phase,
    ...(generationId ? { generationId } : {}),
    retryable,
    attempt,
    maxAttempts: maxCardRepairAttempts,
    issues,
    guidance: retryable
      ? `只修正 issues 中列出的字段并重新调用 ${operationId}；不要提供 generationId，Agent 会复用当前占位项。`
      : "已达到自动修正上限。不要再次生成同一张卡片；请向用户简要说明卡片生成失败，并保留当前业务上下文。",
  }
}

export function providerArgumentFailure(
  error: NonNullable<ModelToolCall["argumentError"]>,
  attempt: number,
  generationId?: string,
  operationId = "create_interaction_cards",
): InteractionCardValidationFailure {
  const failure = cardValidationFailure("provider", [{
    code: error.code,
    path: "$",
    message: error.message,
    expected: "完整 JSON 对象",
  }], attempt, generationId, "ai.tool_arguments_json_invalid", operationId)
  return {
    ...failure,
    guidance: failure.retryable
      ? "重新生成完整的 JSON 工具参数；不要提供 generationId，也不要复用被截断的参数文本。"
      : failure.guidance,
  }
}
