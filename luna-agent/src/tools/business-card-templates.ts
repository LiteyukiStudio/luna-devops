import { z } from "zod"

const identifier = z.string().regex(/^[a-zA-Z0-9_-]{1,64}$/)
const shortText = z.string().trim().min(1).max(120)
const description = z.string().trim().max(500).optional()
const tone = z.enum(["neutral", "success", "warning", "error"])

const badge = z.object({ label: z.string().trim().min(1).max(60), tone })
const keyValueItem = z.object({
  label: shortText,
  value: z.string().max(2000),
  format: z.enum(["text", "code", "status", "duration", "date_time", "bytes", "currency"]).optional(),
  copyable: z.boolean().optional(),
})
const statusItem = z.object({
  id: identifier,
  label: shortText,
  detail: z.string().trim().max(500).optional(),
  status: z.enum(["pending", "running", "success", "warning", "error", "skipped"]),
})
const metricItem = z.object({
  label: shortText,
  value: z.string().max(120),
  change: z.string().max(120).optional(),
  trend: z.enum(["up", "down", "flat"]).optional(),
  tone: tone.optional(),
})

const selectOption = z.object({
  value: z.string().min(1).max(240),
  label: shortText,
  description,
  disabled: z.boolean().optional(),
})

const templateField = z.discriminatedUnion("type", [
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("text"),
    defaultValue: z.string().max(4000).optional(), placeholder: z.string().max(200).optional(),
    format: z.enum(["plain", "identifier", "namespace", "hostname", "email", "url", "image_ref", "cpu", "memory", "storage"]).optional(),
    minLength: z.number().int().min(0).max(4000).optional(), maxLength: z.number().int().min(1).max(4000).optional(),
  }),
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("textarea"),
    defaultValue: z.string().max(12000).optional(), placeholder: z.string().max(200).optional(), rows: z.number().int().min(2).max(6).optional(),
  }),
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("number"),
    defaultValue: z.number().optional(), integer: z.boolean().optional(), min: z.number().optional(), max: z.number().optional(),
    step: z.number().positive().optional(), unit: z.string().trim().max(40).optional(),
  }),
  z.object({ id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("boolean"), defaultValue: z.boolean().optional() }),
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("select"),
    defaultValue: z.string().max(240).optional(), placeholder: z.string().max(200).optional(),
    display: z.enum(["select", "radio", "segmented"]).optional(), submissionFormat: z.enum(["value", "label_value"]).optional(), options: z.array(selectOption).min(1).max(50),
  }),
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("multi_select"),
    defaultValue: z.array(z.string().max(240)).max(50).optional(), placeholder: z.string().max(200).optional(),
    minItems: z.number().int().min(0).max(50).optional(), maxItems: z.number().int().min(1).max(50).optional(),
    submissionFormat: z.enum(["value", "label_value"]).optional(), options: z.array(selectOption).min(1).max(50),
  }),
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("key_value"),
    defaultValue: z.array(z.object({ key: z.string().max(200), value: z.string().max(2000) })).max(30).optional(),
    keyFormat: z.enum(["plain", "identifier", "environment_variable"]).optional(), valueMode: z.enum(["plain", "secret"]).optional(),
    minItems: z.number().int().min(0).max(30).optional(), maxItems: z.number().int().min(1).max(30).optional(),
  }),
  z.object({
    id: identifier, label: shortText, description, required: z.boolean().optional(), type: z.literal("secret"),
    placeholder: z.string().max(200).optional(), generation: z.enum(["disabled", "optional", "required"]),
    defaultMode: z.enum(["manual", "generate"]).optional(),
  }),
])

const literalBinding = z.object({
  target: z.string().regex(/^\/(?:[^/~]|~[01])+(?:\/(?:[^/~]|~[01])+)*$/).max(240),
  value: z.union([z.string(), z.number(), z.boolean(), z.null()]),
})
const fieldBinding = z.object({
  target: z.string().regex(/^\/(?:[^/~]|~[01])+(?:\/(?:[^/~]|~[01])+)*$/).max(240),
  fieldId: identifier,
})
const submitAction = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("send_message"), label: shortText, description, message: z.string().trim().min(1).max(2000),
  }),
  z.object({
    type: z.literal("tool"), label: shortText, description,
    operationId: z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/),
    literalBindings: z.array(literalBinding).max(30).optional(), fieldBindings: z.array(fieldBinding).max(30).optional(),
  }),
])

const candidate = z.object({
  id: identifier,
  title: shortText,
  subtitle: z.string().trim().max(160).optional(),
  description,
  badges: z.array(badge).max(4).optional(),
  facts: z.array(keyValueItem).max(8).optional(),
  selectionLabel: shortText,
  selectionMessage: z.string().trim().min(1).max(1000),
})

const candidatePicker = z.object({
  templateId: z.literal("candidate_picker"),
  title: shortText,
  description,
  candidates: z.array(candidate).min(2).max(5),
})

const candidateSelect = z.object({
  templateId: z.literal("candidate_select"),
  title: shortText,
  description,
  fieldLabel: shortText,
  fieldDescription: description,
  candidates: z.array(selectOption).min(6).max(50),
  submitLabel: shortText,
  submitMessage: z.string().trim().min(1).max(2000),
}).refine(input => input.submitMessage.includes("{{candidate}}"), {
  message: "候选选择模板的 submitMessage 必须包含 {{candidate}}。",
  path: ["submitMessage"],
})

const resourceConfiguration = z.object({
  templateId: z.literal("resource_configuration"),
  title: shortText,
  description,
  resourceTitle: shortText,
  resourceSubtitle: z.string().trim().max(160).optional(),
  facts: z.array(keyValueItem).max(12).optional(),
  sections: z.array(z.object({ id: identifier, title: shortText.optional(), description, fields: z.array(templateField).min(1).max(12) })).min(1).max(6),
  submit: submitAction,
})

const changeReview = z.object({
  templateId: z.literal("change_review"),
  title: shortText,
  description,
  resourceTitle: shortText,
  resourceSubtitle: z.string().trim().max(160).optional(),
  changes: z.array(keyValueItem).min(1).max(16),
  risks: z.array(statusItem).max(12).optional(),
  submit: submitAction,
})

const diagnosisReport = z.object({
  templateId: z.literal("diagnosis_report"),
  title: shortText,
  description,
  conclusion: z.string().trim().min(1).max(2000),
  conclusionTone: tone,
  findings: z.array(statusItem).min(1).max(20),
  evidence: z.array(keyValueItem).max(16).optional(),
})

const liveProgressBinding = z.object({
  operationType: z.enum(["build_run", "release", "hook_run", "app_template_installation"]),
  projectId: z.string().trim().min(1).max(120),
  operationId: z.string().trim().min(1).max(120),
})

const executionProgress = z.object({
  templateId: z.literal("execution_progress"),
  title: shortText.describe("稳定的任务名称，不得包含正在、已完成、失败等会随任务变化的状态。"),
  description: description.describe("稳定的任务说明，不得写入会随任务变化的状态、百分比或步骤。"),
  binding: liveProgressBinding,
  label: shortText.optional(),
  detail: description,
}).describe("只用于绑定平台已经创建且仍在运行的权威任务。不得由模型填写百分比、步骤状态或在标题、说明中固化动态状态。")

const operationResult = z.object({
  templateId: z.literal("operation_result"),
  title: shortText,
  description,
  outcome: z.enum(["success", "warning", "error"]),
  summary: z.string().trim().min(1).max(2000),
  facts: z.array(keyValueItem).max(16).optional(),
  steps: z.array(statusItem).max(20).optional(),
})

const healthOverview = z.object({
  templateId: z.literal("health_overview"),
  title: shortText,
  description,
  metrics: z.array(metricItem).min(1).max(6),
  statuses: z.array(statusItem).min(1).max(20),
})

export const businessCardTemplate = z.discriminatedUnion("templateId", [
  candidatePicker,
  candidateSelect,
  resourceConfiguration,
  changeReview,
  diagnosisReport,
  executionProgress,
  operationResult,
  healthOverview,
])

export const createBusinessCardTemplateInput = z.object({
  schemaVersion: z.literal(1),
  generationId: identifier,
  businessTemplate: businessCardTemplate,
})

export type CreateBusinessCardTemplateInput = z.infer<typeof createBusinessCardTemplateInput>

export function compileBusinessCardTemplate(input: CreateBusinessCardTemplateInput): unknown {
  const template = input.businessTemplate
  const base = { schemaVersion: 1, generationId: input.generationId, title: template.title, description: template.description }
  switch (template.templateId) {
    case "candidate_picker":
      return {
        ...base, mode: "interactive", template: "catalog", display: { density: "comfortable" },
        cards: template.candidates.map(candidate => ({
          id: candidate.id,
          presentation: {
            variant: "resource", title: candidate.title, subtitle: candidate.subtitle, description: candidate.description,
            badges: candidate.badges,
          },
          blocks: candidate.facts?.length ? [{ id: "facts", type: "key_value", items: candidate.facts, collapsible: true }] : undefined,
          actions: [{ id: "select", type: "send_message", label: candidate.selectionLabel, message: candidate.selectionMessage, emphasis: "primary" }],
        })),
      }
    case "candidate_select":
      return {
        ...base, mode: "interactive", template: "form", display: { density: "compact" },
        cards: [{
          id: "candidate_selection",
          presentation: { variant: "form", title: template.title, description: template.description },
          form: { sections: [{ id: "selection", fields: [{ id: "candidate", type: "select", label: template.fieldLabel, description: template.fieldDescription, required: true, submissionFormat: "label_value", options: template.candidates }] }] },
          actions: [{ id: "submit", type: "send_message", label: template.submitLabel, message: template.submitMessage, emphasis: "primary" }],
        }],
      }
    case "resource_configuration":
      return {
        ...base, mode: "interactive", template: "form", display: { density: "comfortable" },
        cards: [{
          id: "resource_configuration",
          presentation: { variant: "form", title: template.resourceTitle, subtitle: template.resourceSubtitle, description: template.description },
          blocks: template.facts?.length ? [{ id: "resource_facts", type: "key_value", items: template.facts, collapsible: true }] : undefined,
          form: { sections: template.sections }, actions: [compileSubmitAction(template.submit)],
        }],
      }
    case "change_review":
      return {
        ...base, mode: "interactive", template: "plan", display: { density: "compact" },
        cards: [{
          id: "change_review",
          presentation: { variant: "plan", title: template.resourceTitle, subtitle: template.resourceSubtitle, description: template.description },
          blocks: [
            { id: "changes", type: "key_value", items: template.changes },
            ...(template.risks?.length ? [{ id: "risks", type: "status_list", items: template.risks }] : []),
          ],
          actions: [compileSubmitAction(template.submit)],
        }],
      }
    case "diagnosis_report":
      return {
        ...base, mode: "presentation", template: "diagnosis", display: { density: "compact" },
        cards: [{
          id: "diagnosis_report", presentation: { variant: "finding", title: template.title, description: template.description },
          blocks: [
            { id: "conclusion", type: "callout", tone: template.conclusionTone, content: template.conclusion },
            { id: "findings", type: "status_list", items: template.findings },
            ...(template.evidence?.length ? [{ id: "evidence", type: "key_value", items: template.evidence, collapsible: true }] : []),
          ],
        }],
      }
    case "execution_progress":
      return {
        ...base, mode: "presentation", template: "progress", display: { density: "compact" },
        cards: [{
          id: "execution_progress", presentation: { variant: "task", title: template.title, description: template.description },
          blocks: [{ id: "progress", type: "live_progress", binding: template.binding, label: template.label, detail: template.detail }],
        }],
      }
    case "operation_result":
      return {
        ...base, mode: "presentation", template: "result", display: { density: "compact" },
        cards: [{
          id: "operation_result", presentation: { variant: "receipt", title: template.title, description: template.description },
          blocks: [
            { id: "summary", type: "callout", tone: template.outcome, content: template.summary },
            ...(template.facts?.length ? [{ id: "facts", type: "key_value", items: template.facts }] : []),
            ...(template.steps?.length ? [{ id: "steps", type: "status_list", items: template.steps }] : []),
          ],
        }],
      }
    case "health_overview":
      return {
        ...base, mode: "presentation", template: "dashboard", display: { density: "compact" },
        cards: [{
          id: "health_overview", presentation: { variant: "summary", title: template.title, description: template.description },
          blocks: [{ id: "metrics", type: "metrics", items: template.metrics }, { id: "statuses", type: "status_list", items: template.statuses }],
        }],
      }
  }
}

function compileSubmitAction(action: z.infer<typeof submitAction>): unknown {
  if (action.type === "send_message")
    return { id: "submit", type: "send_message", label: action.label, description: action.description, message: action.message, emphasis: "primary" }
  return {
    id: "submit", type: "tool", label: action.label, description: action.description, operationId: action.operationId, emphasis: "primary",
    bindings: [
      ...(action.literalBindings ?? []).map(binding => ({ target: binding.target, value: { type: "literal", value: binding.value } })),
      ...(action.fieldBindings ?? []).map(binding => ({ target: binding.target, value: { type: "field", fieldId: binding.fieldId } })),
    ],
  }
}
