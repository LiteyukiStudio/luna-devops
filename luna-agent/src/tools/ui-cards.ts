import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import { registeredRouteName, routeIdentifiers } from "./ui-route.js"

const identifier = z.string().regex(/^[a-zA-Z0-9_-]{1,64}$/)
const shortText = z.string().trim().min(1).max(120)
const description = z.string().trim().max(500).optional()
const tone = z.enum(["neutral", "success", "warning", "error"])

const sourceRef = z.object({
  type: z.enum(["app_template", "web_search_result", "web_page", "platform_resource", "platform_event", "tool_result"]),
  refId: identifier,
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
    badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone })).max(6).optional(),
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
  generationId: identifier,
  title: shortText,
  description,
  template: z.enum(["catalog", "comparison", "inspector", "form", "wizard", "diagnosis", "plan", "progress", "result", "dashboard"]),
  display: z.object({
    density: z.enum(["comfortable", "compact"]).optional(),
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
    const sourceRefIds = new Set((item.sourceRefs ?? []).map(source => source.refId))
    item.blocks?.forEach((block, blockIndex) => {
      block.sourceRefIds?.forEach((refId, refIndex) => {
        if (!sourceRefIds.has(refId)) {
          context.addIssue({
            code: "custom",
            message: `Content block references unknown source "${refId}".`,
            path: ["cards", cardIndex, "blocks", blockIndex, "sourceRefIds", refIndex],
          })
        }
      })
      if (block.type === "relations") {
        const nodeIds = new Set(block.nodes.map(node => node.id))
        block.edges.forEach((edge, edgeIndex) => {
          if (!nodeIds.has(edge.source) || !nodeIds.has(edge.target)) {
            context.addIssue({
              code: "custom",
              message: "Relation edges must reference nodes in the same block.",
              path: ["cards", cardIndex, "blocks", blockIndex, "edges", edgeIndex],
            })
          }
        })
      }
      if (block.type === "data_table") {
        const columnKeys = new Set(block.columns.map(column => column.key))
        block.rows.forEach((row, rowIndex) => {
          Object.keys(row.cells).forEach((key) => {
            if (!columnKeys.has(key)) {
              context.addIssue({
                code: "custom",
                message: `Table row contains unknown column "${key}".`,
                path: ["cards", cardIndex, "blocks", blockIndex, "rows", rowIndex, "cells", key],
              })
            }
          })
        })
      }
      if (block.type === "chart") {
        const lengths = new Set(block.series.map(series => series.values.length))
        if (lengths.size > 1 || (block.xAxis && [...lengths].some(length => length !== block.xAxis!.length))) {
          context.addIssue({
            code: "custom",
            message: "Chart series and x-axis must use the same number of points.",
            path: ["cards", cardIndex, "blocks", blockIndex],
          })
        }
      }
      if (block.type === "progress" && block.mode === "determinate" && block.value === undefined) {
        context.addIssue({
          code: "custom",
          message: "Determinate progress requires a value.",
          path: ["cards", cardIndex, "blocks", blockIndex, "value"],
        })
      }
    })
    fields.forEach((field, fieldIndex) => {
      if (field.visibleWhen && !fieldsById.has(field.visibleWhen.fieldId)) {
        context.addIssue({
          code: "custom",
          message: `Visibility condition references unknown field "${field.visibleWhen.fieldId}".`,
          path: ["cards", cardIndex, "form", "fields", fieldIndex, "visibleWhen", "fieldId"],
        })
      }
    })
    item.actions?.forEach((action, actionIndex) => {
      if (action.type === "tool") {
        action.bindings.forEach((itemBinding, bindingIndex) => {
          if (itemBinding.value.type !== "field")
            return
          const field = fieldsById.get(itemBinding.value.fieldId)
          if (!field) {
            context.addIssue({
              code: "custom",
              message: `Tool binding references unknown field "${itemBinding.value.fieldId}".`,
              path: ["cards", cardIndex, "actions", actionIndex, "bindings", bindingIndex],
            })
          }
          else if (field.type === "secret" || (field.type === "key_value" && field.valueMode === "secret")) {
            context.addIssue({
              code: "custom",
              message: `Tool binding cannot expose sensitive field "${itemBinding.value.fieldId}".`,
              path: ["cards", cardIndex, "actions", actionIndex, "bindings", bindingIndex],
            })
          }
        })
      }
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

export const prepareInteractionCardsInput = z.object({
  schemaVersion: z.literal(1),
  generationId: identifier,
  title: shortText,
  description,
})

export type PrepareInteractionCardsInput = z.infer<typeof prepareInteractionCardsInput>

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
  description: "完成一组受控的声明式内容与交互卡片。调用前必须先单独调用 prepare_interaction_cards，等待其返回 accepted，再使用完全相同的 generationId 调用本工具；前端会用最终卡片原位替换准备动画。template 必须按当前工作流阶段选择：catalog 用于候选发现，comparison 用于同维度比较，inspector 用于已知资源事实，form 用于一轮内收集结构化参数，wizard 用于字段存在依赖或需要分阶段收集，diagnosis 用于结论、证据和修复，plan 用于执行前计划，progress 用于长任务状态，result 用于最终回执，dashboard 用于指标与健康概览。只要下一步需要用户填写、选择、切换或组合一个或多个结构化操作参数，就必须使用 form 或 wizard，不得用 create_options、纯文本问题或空白消息模板代替。卡片只能引用当前工具结果中的真实资源和标识符；不能生成 HTML、CSS、脚本、任意 URL 或虚构状态。展示文本可以使用受控 Markdown，但 HTML 会被忽略。tool action 只能引用当前模型工具列表中已经存在的 operationId，平台仍会重新鉴权并按风险要求确认或 MFA；没有对应写入工具时只能展示或用 send_message 收集选择。send_message 需要带入表单值时只能在 message 中使用 {{field_id}}，不得自创路径或模板语法；敏感字段永远不能插入消息。简单的 2～5 个无需结构化输入的后续建议继续使用 create_options。",
  inputSchema: cardInputJsonSchema(),
}

export const prepareInteractionCardsTool: ModelToolDefinition = {
  operationId: "prepare_interaction_cards",
  description: "在开始组织复杂交互卡片前显示准备动画。只有已经取得生成卡片所需的可信工具结果，并且下一步确定要生成 create_interaction_cards 时才调用。必须单独调用本工具，等待 accepted 后再调用 create_interaction_cards；两次调用必须使用相同的 generationId。title 和 description 应简短说明正在组织什么内容，不得声称卡片或业务操作已经完成。",
  inputSchema: {
    type: "object",
    additionalProperties: false,
    required: ["schemaVersion", "generationId", "title"],
    properties: {
      schemaVersion: { const: 1 },
      generationId: { type: "string", pattern: "^[a-zA-Z0-9_-]{1,64}$" },
      title: { type: "string", minLength: 1, maxLength: 120 },
      description: { type: "string", maxLength: 500 },
    },
  },
}

function messageTemplateFieldIds(message: string): string[] {
  return [...message.matchAll(/\{\{([a-zA-Z0-9_-]{1,64})\}\}/g)].map(match => match[1]!)
}

function cardInputJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(createInteractionCardsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
