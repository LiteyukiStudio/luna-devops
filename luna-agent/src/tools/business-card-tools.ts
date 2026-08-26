import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import { createInteractionCardsInput } from "./ui-cards.js"
import { cardToolOperationIds } from "./internal-operation-ids.js"

/** 模型只看到三个按会话职责划分的卡片工具，三者统一产出 InteractionCardGroup v1。 */
export const presentCardInput = createInteractionCardsInput.refine(input => input.mode === "presentation", {
  message: "present_card 只能创建 presentation 卡片。",
  path: ["mode"],
})
export const requestInputInput = createInteractionCardsInput.refine(input => input.mode === "interactive" && input.template === "form", {
  message: "request_input 必须创建 interactive form 卡片。",
  path: ["template"],
})
export const requestChoiceInput = createInteractionCardsInput.refine(input => input.mode === "interactive" && input.template === "candidates", {
  message: "request_choice 必须创建 interactive candidates 卡片。",
  path: ["template"],
})

export const businessCardToolInputs = {
  present_card: presentCardInput,
  request_input: requestInputInput,
  request_choice: requestChoiceInput,
} as const

export type BusinessCardToolOperationId = keyof typeof businessCardToolInputs

export const businessCardToolOperationIds = cardToolOperationIds

export const businessCardTools: ModelToolDefinition[] = [
  modelTool(
    "present_card",
    "使用 InteractionCardGroup v1 呈现已经由可信工具结果确认的事实、诊断、健康状态、进度或终态结果。mode 必须为 presentation；卡片只负责呈现，不代表执行了任何业务操作。",
    presentCardInput,
  ),
  modelTool(
    "request_input",
    "使用 InteractionCardGroup v1 收集当前工作流缺少的结构化输入。mode 必须为 interactive、template 必须为 form；Secret 不得预填或出现在消息正文，工具动作仍接受平台权限与审批。",
    requestInputInput,
  ),
  modelTool(
    "request_choice",
    "使用 InteractionCardGroup v1 让用户从可信工具结果提供的真实候选中选择。mode 必须为 interactive、template 必须为 candidates；不得编造候选或稳定 ID。",
    requestChoiceInput,
  ),
]

export function isBusinessCardToolOperationId(operationId: string): operationId is BusinessCardToolOperationId {
  return businessCardToolOperationIds.has(operationId)
}

export function compileBusinessCardToolInput(operationId: BusinessCardToolOperationId, raw: unknown): unknown {
  return businessCardToolInputs[operationId].parse(raw)
}

function modelTool<T extends z.ZodType>(operationId: BusinessCardToolOperationId, descriptionText: string, input: T): ModelToolDefinition {
  const schema = z.toJSONSchema(input, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return { operationId, description: descriptionText, inputSchema: { ...schema, type: "object" } }
}
