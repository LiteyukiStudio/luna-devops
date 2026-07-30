import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import { registeredRouteName, routeIdentifiers } from "./ui-route.js"

const identifier = z.string().regex(/^[a-zA-Z0-9_-]{1,64}$/)
const shortText = z.string().trim().min(1).max(120)
const description = z.string().trim().max(500).optional()
const tone = z.enum(["neutral", "success", "warning", "error"])

const sourceRef = z.object({
  type: z.enum(["app_template", "web_search_result", "web_page", "platform_resource", "platform_event", "tool_result"]),
  refId: z.string().trim().min(1).max(200),
  label: z.string().trim().min(1).max(160),
  trust: z.enum(["platform", "official", "community"]),
})

const icon = z.discriminatedUnion("type", [
  z.object({ type: z.literal("asset"), assetRef: z.string().trim().min(1).max(240), alt: shortText }),
  z.object({
    type: z.literal("category"),
    name: z.enum([
      "database", "cache", "queue", "storage", "observability", "application", "repository",
      "registry", "cluster", "build", "deployment", "gateway", "security", "billing", "notification",
    ]),
    alt: shortText,
  }),
])

const blockBase = {
  id: identifier,
  title: shortText.optional(),
  sourceRefIds: z.array(identifier).max(8).optional(),
  collapsible: z.boolean().optional(),
  defaultExpanded: z.boolean().optional(),
}

const contentBlock = z.discriminatedUnion("type", [
  z.object({ ...blockBase, type: z.literal("markdown"), content: z.string().trim().min(1).max(8000) }),
  z.object({ ...blockBase, type: z.literal("callout"), tone, content: z.string().trim().min(1).max(2000) }),
  z.object({
    ...blockBase,
    type: z.literal("key_value"),
    items: z.array(z.object({
      label: shortText,
      value: z.string().max(2000),
      format: z.enum(["text", "code", "status", "duration", "date_time", "bytes", "currency"]).optional(),
      copyable: z.boolean().optional(),
    })).min(1).max(16),
  }),
  z.object({
    ...blockBase,
    type: z.literal("metrics"),
    items: z.array(z.object({
      label: shortText,
      value: z.string().max(120),
      change: z.string().max(120).optional(),
      trend: z.enum(["up", "down", "flat"]).optional(),
      tone: tone.optional(),
    })).min(1).max(6),
  }),
  z.object({
    ...blockBase,
    type: z.literal("item_list"),
    items: z.array(z.object({
      id: identifier,
      primary: shortText,
      secondary: z.string().trim().max(300).optional(),
      meta: z.string().trim().max(160).optional(),
      icon: icon.optional(),
    })).min(1).max(20),
  }),
  z.object({
    ...blockBase,
    type: z.literal("status_list"),
    items: z.array(z.object({
      id: identifier,
      label: shortText,
      detail: z.string().trim().max(500).optional(),
      status: z.enum(["pending", "running", "success", "warning", "error", "skipped"]),
    })).min(1).max(20),
  }),
  z.object({
    ...blockBase,
    type: z.literal("data_table"),
    columns: z.array(z.object({
      key: identifier,
      label: shortText,
      format: z.enum(["text", "code", "status", "duration", "date_time", "bytes", "currency"]).optional(),
    })).min(1).max(8),
    rows: z.array(z.object({ id: identifier, cells: z.record(z.string(), z.string().max(2000)) })).max(30),
    rowSelection: z.enum(["none", "single", "multiple"]).optional(),
  }),
  z.object({ ...blockBase, type: z.literal("code"), language: z.string().trim().min(1).max(40), content: z.string().max(16000), filename: z.string().trim().max(200).optional() }),
  z.object({
    ...blockBase,
    type: z.literal("diff"),
    language: z.string().trim().max(40).optional(),
    beforeLabel: shortText.optional(),
    afterLabel: shortText.optional(),
    unifiedDiff: z.string().min(1).max(16000),
  }),
  z.object({
    ...blockBase,
    type: z.literal("timeline"),
    items: z.array(z.object({
      id: identifier,
      title: shortText,
      detail: z.string().trim().max(500).optional(),
      timestamp: z.string().trim().max(80).optional(),
      status: z.enum(["pending", "running", "success", "warning", "error"]).optional(),
    })).min(1).max(20),
  }),
  z.object({
    ...blockBase,
    type: z.literal("chart"),
    chartType: z.enum(["line", "bar", "area", "donut"]),
    xAxis: z.array(z.string().max(120)).max(60).optional(),
    series: z.array(z.object({
      name: shortText,
      values: z.array(z.number()).max(60),
      unit: z.string().trim().max(40).optional(),
    })).min(1).max(4),
  }),
  z.object({
    ...blockBase,
    type: z.literal("relations"),
    nodes: z.array(z.object({
      id: identifier,
      label: shortText,
      category: z.string().trim().min(1).max(60),
      status: z.enum(["neutral", "success", "warning", "error"]).optional(),
    })).min(1).max(30),
    edges: z.array(z.object({
      source: identifier,
      target: identifier,
      label: z.string().trim().max(120).optional(),
    })).max(50),
  }),
  z.object({
    ...blockBase,
    type: z.literal("progress"),
    mode: z.enum(["determinate", "indeterminate"]),
    value: z.number().min(0).max(100).optional(),
    label: shortText,
    detail: z.string().trim().max(500).optional(),
  }),
  z.object({
    ...blockBase,
    type: z.literal("resource_links"),
    links: z.array(z.object({
      label: shortText,
      routeName: registeredRouteName.optional(),
      routeParams: routeIdentifiers.optional(),
      sourceRefId: identifier.optional(),
    })).min(1).max(8),
  }),
])

const condition = z.object({
  fieldId: identifier,
  operator: z.enum(["equals", "not_equals", "contains", "is_empty", "is_not_empty"]),
  value: z.union([z.string(), z.number(), z.boolean()]).optional(),
})

const fieldBase = {
  id: identifier,
  label: shortText,
  description,
  required: z.boolean().optional(),
  visibleWhen: condition.optional(),
}
const selectOption = z.object({
  value: z.string().min(1).max(240),
  label: shortText,
  description,
  disabled: z.boolean().optional(),
})
const formField = z.discriminatedUnion("type", [
  z.object({
    ...fieldBase,
    type: z.literal("text"),
    defaultValue: z.string().max(4000).optional(),
    placeholder: z.string().max(200).optional(),
    format: z.enum(["plain", "identifier", "namespace", "hostname", "email", "url", "image_ref", "cpu", "memory", "storage"]).optional(),
    minLength: z.number().int().min(0).max(4000).optional(),
    maxLength: z.number().int().min(1).max(4000).optional(),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("textarea"),
    defaultValue: z.string().max(12000).optional(),
    placeholder: z.string().max(200).optional(),
    minLength: z.number().int().min(0).max(12000).optional(),
    maxLength: z.number().int().min(1).max(12000).optional(),
    rows: z.number().int().min(2).max(6).optional(),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("number"),
    defaultValue: z.number().optional(),
    integer: z.boolean().optional(),
    min: z.number().optional(),
    max: z.number().optional(),
    step: z.number().positive().optional(),
    unit: z.string().trim().max(40).optional(),
  }),
  z.object({ ...fieldBase, type: z.literal("boolean"), defaultValue: z.boolean().optional() }),
  z.object({
    ...fieldBase,
    type: z.literal("select"),
    defaultValue: z.string().max(240).optional(),
    placeholder: z.string().max(200).optional(),
    display: z.enum(["select", "radio", "segmented"]).optional(),
    options: z.array(selectOption).min(1).max(50),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("multi_select"),
    defaultValue: z.array(z.string().max(240)).max(50).optional(),
    placeholder: z.string().max(200).optional(),
    minItems: z.number().int().min(0).max(50).optional(),
    maxItems: z.number().int().min(1).max(50).optional(),
    options: z.array(selectOption).min(1).max(50),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("key_value"),
    defaultValue: z.array(z.object({ key: z.string().max(200), value: z.string().max(2000) })).max(30).optional(),
    keyFormat: z.enum(["plain", "identifier", "environment_variable"]).optional(),
    valueMode: z.enum(["plain", "secret"]).optional(),
    minItems: z.number().int().min(0).max(30).optional(),
    maxItems: z.number().int().min(1).max(30).optional(),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("secret"),
    placeholder: z.string().max(200).optional(),
    generation: z.enum(["disabled", "optional", "required"]),
    defaultMode: z.enum(["manual", "generate"]).optional(),
  }),
])

const bindingValue = z.discriminatedUnion("type", [
  z.object({ type: z.literal("field"), fieldId: identifier }),
  z.object({ type: z.literal("card"), property: z.literal("id") }),
  z.object({ type: z.literal("literal"), value: z.union([z.string(), z.number(), z.boolean(), z.null()]) }),
])
const binding = z.object({
  target: z.string().regex(/^\/(?:[^/~]|~[01])+(?:\/(?:[^/~]|~[01])+)*$/).max(240),
  value: bindingValue,
})
const actionBase = {
  id: identifier,
  label: shortText,
  description,
  emphasis: z.enum(["primary", "secondary", "ghost"]).optional(),
  repeatable: z.boolean().optional(),
}
const cardAction = z.discriminatedUnion("type", [
  z.object({
    ...actionBase,
    type: z.literal("tool"),
    operationId: z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/),
    bindings: z.array(binding).max(40),
  }),
  z.object({
    ...actionBase,
    type: z.literal("send_message"),
    message: z.string().trim().min(1).max(2000)
      .describe("发送给助手的消息。需要带入非敏感表单值时使用 {{field_id}}；field_id 必须是当前卡片的字段 ID。"),
  }),
  z.object({ ...actionBase, type: z.literal("navigate"), routeName: registeredRouteName, routeParams: routeIdentifiers.optional() }),
])

const card = z.object({
  id: identifier,
  presentation: z.object({
    variant: z.enum(["application", "resource", "form", "finding", "plan", "task", "receipt", "summary"]),
    title: shortText,
    subtitle: z.string().trim().max(160).optional(),
    description,
    icon: icon.optional(),
    badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone: z.enum(["neutral", "success", "warning"]) })).max(6).optional(),
  }),
  sourceRefs: z.array(sourceRef).max(12).optional(),
  blocks: z.array(contentBlock).max(12).optional(),
  form: z.object({
    sections: z.array(z.object({
      id: identifier,
      title: shortText.optional(),
      description,
      fields: z.array(formField).min(1).max(12),
    })).min(1).max(6),
  }).optional(),
  actions: z.array(cardAction).max(4).optional(),
})

export const createInteractionCardsInput = z.object({
  schemaVersion: z.literal(1),
  title: shortText,
  description,
  template: z.enum(["catalog", "comparison", "inspector", "form", "wizard", "diagnosis", "plan", "progress", "result", "dashboard"]),
  display: z.object({
    density: z.enum(["comfortable", "compact"]).optional(),
    selection: z.enum(["none", "single", "multiple"]).optional(),
  }).optional(),
  cards: z.array(card).min(1).max(12),
  groupActions: z.array(cardAction).max(3).optional(),
}).superRefine((input, context) => {
  const cardIds = new Set<string>()
  input.cards.forEach((item, cardIndex) => {
    if (cardIds.has(item.id))
      context.addIssue({ code: "custom", message: "Card IDs must be unique.", path: ["cards", cardIndex, "id"] })
    cardIds.add(item.id)
    const ids = new Set<string>()
    const values = [
      ...(item.blocks ?? []).map(block => ({ id: block.id, path: "blocks" })),
      ...(item.form?.sections.flatMap(section => section.fields).map(field => ({ id: field.id, path: "form" })) ?? []),
      ...(item.actions ?? []).map(action => ({ id: action.id, path: "actions" })),
    ]
    values.forEach((value) => {
      if (ids.has(value.id))
        context.addIssue({ code: "custom", message: "IDs inside a card must be unique.", path: ["cards", cardIndex, value.path] })
      ids.add(value.id)
    })
    const fields = item.form?.sections.flatMap(section => section.fields) ?? []
    const fieldsById = new Map(fields.map(field => [field.id, field]))
    item.actions?.forEach((action, actionIndex) => {
      if (action.type !== "send_message")
        return
      for (const fieldId of messageTemplateFieldIds(action.message)) {
        const field = fieldsById.get(fieldId)
        if (!field) {
          context.addIssue({
            code: "custom",
            message: `Message template references unknown field "${fieldId}".`,
            path: ["cards", cardIndex, "actions", actionIndex, "message"],
          })
          continue
        }
        if (field.type === "secret" || (field.type === "key_value" && field.valueMode === "secret")) {
          context.addIssue({
            code: "custom",
            message: `Message template cannot expose sensitive field "${fieldId}".`,
            path: ["cards", cardIndex, "actions", actionIndex, "message"],
          })
        }
      }
    })
  })
  input.groupActions?.forEach((action, actionIndex) => {
    if (action.type === "send_message" && messageTemplateFieldIds(action.message).length > 0) {
      context.addIssue({
        code: "custom",
        message: "Group actions cannot reference card form fields.",
        path: ["groupActions", actionIndex, "message"],
      })
    }
  })
  if (Buffer.byteLength(JSON.stringify(input), "utf8") > 96 * 1024)
    context.addIssue({ code: "custom", message: "Card group exceeds 96 KiB." })
})

export type CreateInteractionCardsInput = z.infer<typeof createInteractionCardsInput>

export function normalizeInteractionCardsInput(raw: unknown): unknown {
  if (!raw || typeof raw !== "object" || Array.isArray(raw))
    return raw
  const input = structuredClone(raw) as Record<string, unknown>
  if (input.schemaVersion === "1")
    input.schemaVersion = 1
  if (!Array.isArray(input.cards))
    return input
  const cards = input.cards as unknown[]
  input.cards = cards.map((cardValue: unknown, cardIndex) => {
    if (!cardValue || typeof cardValue !== "object" || Array.isArray(cardValue))
      return cardValue
    const card = cardValue as Record<string, unknown>
    if (!card.form || typeof card.form !== "object" || Array.isArray(card.form))
      return card
    const form = card.form as Record<string, unknown>
    if (!Array.isArray(form.sections))
      return card
    const sections = form.sections as unknown[]
    form.sections = sections.map((sectionValue: unknown, sectionIndex) => {
      if (!sectionValue || typeof sectionValue !== "object" || Array.isArray(sectionValue))
        return sectionValue
      const section = sectionValue as Record<string, unknown>
      if (typeof section.id !== "string" || section.id.trim() === "")
        section.id = `section_${cardIndex + 1}_${sectionIndex + 1}`
      return section
    })
    return card
  })
  return input
}

export const createInteractionCardsTool: ModelToolDefinition = {
  operationId: "create_interaction_cards",
  description: "在当前回复中创建受控的声明式内容与交互卡片。适用于资源候选、对比、详情、诊断、计划、进度、结果和需要结构化输入的任务。卡片只能引用当前工具结果中的真实资源和标识符；不能生成 HTML、CSS、脚本、任意 URL 或虚构状态。tool action 只能引用当前模型工具列表中已经存在的 operationId，平台仍会重新鉴权并按风险要求确认或 MFA；没有对应写入工具时只能展示或用 send_message 收集选择。send_message 需要带入表单值时只能在 message 中使用 {{field_id}}，不得自创路径或模板语法；敏感字段永远不能插入消息。简单的 2～5 个后续建议继续使用 create_options。",
  inputSchema: cardInputJsonSchema(),
}

function messageTemplateFieldIds(message: string): string[] {
  return [...message.matchAll(/\{\{([a-zA-Z0-9_-]{1,64})\}\}/g)].map(match => match[1]!)
}

function cardInputJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(createInteractionCardsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
