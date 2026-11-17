import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import {
  createInteractionCardsInput,
  normalizePresentCardInput,
  normalizeRequestChoiceInput,
  normalizeRequestInputInput,
  presentCardToolInput,
  requestChoiceToolInput,
  requestInputToolInput,
} from "./ui-cards.js"
import { cardToolOperationIds } from "./internal-operation-ids.js"

/** 模型只看到三个各自独立裁剪 Schema 的卡片工具，服务端统一归一化为 InteractionCardGroup v1。 */
export const presentCardInput = presentCardToolInput
export const requestInputInput = requestInputToolInput
export const requestChoiceInput = requestChoiceToolInput

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
    "呈现已经由可信工具结果确认的事实、诊断、健康状态、变更评审摘要、权威实时进度（live_task）或终态结果。服务端固定收敛为 presentation 模式；卡片只负责呈现，不代表执行了任何业务操作。",
    presentCardInput,
  ),
  modelTool(
    "request_input",
    "收集当前工作流缺少的结构化表单输入。服务端固定收敛为 interactive + form；Secret 不得预填或出现在消息正文，工具动作仍接受平台权限与审批。",
    requestInputInput,
  ),
  modelTool(
    "request_choice",
    "让用户从可信工具结果提供的真实候选中做单选或多选决策。服务端固定收敛为 interactive + candidates；不得编造候选或稳定 ID。",
    requestChoiceInput,
  ),
]

export function isBusinessCardToolOperationId(operationId: string): operationId is BusinessCardToolOperationId {
  return businessCardToolOperationIds.has(operationId)
}

export function compileBusinessCardToolInput(
  operationId: BusinessCardToolOperationId,
  raw: unknown,
): z.output<typeof createInteractionCardsInput> {
  switch (operationId) {
    case "present_card":
      return createInteractionCardsInput.parse(normalizePresentCardInput(presentCardInput.parse(raw)))
    case "request_input":
      return createInteractionCardsInput.parse(normalizeRequestInputInput(requestInputInput.parse(raw)))
    case "request_choice":
      return createInteractionCardsInput.parse(normalizeRequestChoiceInput(requestChoiceInput.parse(raw)))
  }
}

function modelTool<T extends z.ZodType>(operationId: BusinessCardToolOperationId, descriptionText: string, input: T): ModelToolDefinition {
  const schema = z.toJSONSchema(input, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return { operationId, description: descriptionText, inputSchema: { ...schema, type: "object" } }
}
