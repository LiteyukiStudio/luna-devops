import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import {
  changeReviewTemplate,
  compileBusinessCardTemplate,
  diagnosisReportTemplate,
  executionProgressTemplate,
  healthOverviewTemplate,
  operationResultTemplate,
  resourceConfigurationTemplate,
} from "./business-card-templates.js"
import { createInteractionCardsInput, normalizeInteractionCardsInput } from "./ui-cards.js"

const identifier = z.string().regex(/^[a-zA-Z0-9_-]{1,64}$/)
const shortText = z.string().trim().min(1).max(120)
const description = z.string().trim().max(500).optional()
const tone = z.enum(["neutral", "success", "warning", "error"])

/** 未来模型只看到三个按会话职责划分的卡片工具，三者统一产出 InteractionCardGroup v1。 */
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

export const businessCardToolOperationIds = new Set<BusinessCardToolOperationId>(
  Object.keys(businessCardToolInputs) as BusinessCardToolOperationId[],
)

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
  return businessCardToolOperationIds.has(operationId as BusinessCardToolOperationId)
}

export function compileBusinessCardToolInput(operationId: BusinessCardToolOperationId, raw: unknown): unknown {
  return normalizeInteractionCardsInput(businessCardToolInputs[operationId].parse(raw))
}

/*
 * 旧 operationId 只用于解析历史事件和离线数据迁移，绝不注册给未来模型。
 * 保留解析器能让已有 Timeline 在瘦身后继续渲染。
 */
const resourceChoiceCandidate = z.object({
  id: identifier,
  title: shortText,
  subtitle: z.string().trim().max(160).optional(),
  description,
  badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone })).max(4).optional(),
  facts: z.array(z.object({
    label: shortText,
    value: z.string().max(2000),
    format: z.enum(["text", "code", "status", "duration", "date_time", "bytes", "currency"]).optional(),
    copyable: z.boolean().optional(),
  })).max(8).optional(),
}).strict()

export const requestResourceChoiceInput = z.object({
  title: shortText,
  description,
  fieldLabel: shortText.optional(),
  candidates: z.array(resourceChoiceCandidate).min(2).max(50),
  selectionLabel: shortText,
  selectionMessage: z.string().trim().min(1).max(2000).refine(message => message.includes("{{candidate}}")),
  creationAction: z.object({ label: shortText, message: z.string().trim().min(1).max(1000) }).strict().optional(),
}).strict()
export const requestToolInputInput = resourceConfigurationTemplate.omit({ templateId: true }).strict()
export const reviewToolActionInput = changeReviewTemplate.omit({ templateId: true }).strict()
export const presentDiagnosisInput = diagnosisReportTemplate.omit({ templateId: true }).strict()
export const presentHealthOverviewInput = healthOverviewTemplate.omit({ templateId: true }).strict()
export const presentExecutionProgressInput = executionProgressTemplate.omit({ templateId: true }).strict()
export const presentOperationResultInput = operationResultTemplate.omit({ templateId: true }).strict()

export const legacyBusinessCardToolInputs = {
  request_resource_choice: requestResourceChoiceInput,
  request_tool_input: requestToolInputInput,
  review_tool_action: reviewToolActionInput,
  present_diagnosis: presentDiagnosisInput,
  present_health_overview: presentHealthOverviewInput,
  present_execution_progress: presentExecutionProgressInput,
  present_operation_result: presentOperationResultInput,
} as const
export type LegacyBusinessCardToolOperationId = keyof typeof legacyBusinessCardToolInputs

export function compileLegacyBusinessCardToolInput(operationId: LegacyBusinessCardToolOperationId, raw: unknown): unknown {
  switch (operationId) {
    case "request_resource_choice": {
      const input = requestResourceChoiceInput.parse(raw)
      if (input.candidates.length <= 5) {
        return compileBusinessCardTemplate({
          schemaVersion: 1,
          placement: "inline",
          businessTemplate: {
            templateId: "candidate_picker",
            title: input.title,
            description: input.description,
            candidates: input.candidates.map(candidate => ({
              ...candidate,
              selectionLabel: input.selectionLabel,
              selectionMessage: input.selectionMessage.replaceAll("{{candidate}}", `${candidate.title} (${candidate.id})`),
            })),
            creationAction: input.creationAction,
          },
        })
      }
      return compileBusinessCardTemplate({
        schemaVersion: 1,
        placement: "turn_end",
        businessTemplate: {
          templateId: "candidate_select",
          title: input.title,
          description: input.description,
          fieldLabel: input.fieldLabel ?? input.title,
          candidates: input.candidates.map(candidate => ({
            value: candidate.id,
            label: candidate.title,
            description: candidate.description ?? candidate.subtitle,
          })),
          submitLabel: input.selectionLabel,
          submitMessage: input.selectionMessage,
          creationAction: input.creationAction,
        },
      })
    }
    case "request_tool_input":
      return compileBusinessCardTemplate({ schemaVersion: 1, placement: "turn_end", businessTemplate: { templateId: "resource_configuration", ...requestToolInputInput.parse(raw) } })
    case "review_tool_action":
      return compileBusinessCardTemplate({ schemaVersion: 1, businessTemplate: { templateId: "change_review", ...reviewToolActionInput.parse(raw) } })
    case "present_diagnosis":
      return compileBusinessCardTemplate({ schemaVersion: 1, businessTemplate: { templateId: "diagnosis_report", ...presentDiagnosisInput.parse(raw) } })
    case "present_health_overview":
      return compileBusinessCardTemplate({ schemaVersion: 1, businessTemplate: { templateId: "health_overview", ...presentHealthOverviewInput.parse(raw) } })
    case "present_execution_progress":
      return compileBusinessCardTemplate({ schemaVersion: 1, businessTemplate: { templateId: "execution_progress", ...presentExecutionProgressInput.parse(raw) } })
    case "present_operation_result":
      return compileBusinessCardTemplate({ schemaVersion: 1, businessTemplate: { templateId: "operation_result", ...presentOperationResultInput.parse(raw) } })
  }
}

function modelTool<T extends z.ZodType>(operationId: BusinessCardToolOperationId, descriptionText: string, input: T): ModelToolDefinition {
  const schema = z.toJSONSchema(input, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return { operationId, description: descriptionText, inputSchema: { ...schema, type: "object" } }
}
