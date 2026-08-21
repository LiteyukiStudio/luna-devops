import { z } from "zod"
import type { InteractionCardGroup } from "@luna-devops/ai-interaction-card-contract"
import {
  compileBusinessCardTemplate,
  createBusinessCardTemplateInput,
} from "./business-card-templates.js"
import { safeJsonPointer } from "./json-pointer-schema.js"
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
    type: z.literal("live_progress"),
    binding: z.object({
      operationType: z.enum(["build_run", "release", "hook_run", "app_template_installation"]),
      projectId: z.string().trim().min(1).max(120),
      operationId: z.string().trim().min(1).max(120),
    }),
    label: shortText.optional(),
    detail: z.string().trim().max(500).optional(),
  }).describe("绑定平台权威异步任务的实时进度。禁止把运行中状态写成静态百分比或静态步骤。"),
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
const keyValueFieldBase = {
  ...fieldBase,
  type: z.literal("key_value"),
  keyFormat: z.enum(["plain", "identifier", "environment_variable"]).optional(),
  minItems: z.number().int().min(0).max(30).optional(),
  maxItems: z.number().int().min(1).max(30).optional(),
}
const keyValueEntry = z.object({ key: z.string().max(200), value: z.string().max(2000) })
const formField = z.union([
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
    submissionFormat: z.enum(["value", "label_value"]).optional().describe(
      "生成 send_message 回复时的选择值格式。平台资源必须使用 label_value，使回复包含“资源名称 (资源 ID)”；普通枚举使用 value。工具参数始终提交原始 value。",
    ),
    options: z.array(selectOption).min(1).max(50),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("multi_select"),
    defaultValue: z.array(z.string().max(240)).max(50).optional(),
    placeholder: z.string().max(200).optional(),
    minItems: z.number().int().min(0).max(50).optional(),
    maxItems: z.number().int().min(1).max(50).optional(),
    submissionFormat: z.enum(["value", "label_value"]).optional().describe(
      "生成 send_message 回复时的选择值格式。平台资源必须使用 label_value，使每项包含“资源名称 (资源 ID)”；普通枚举使用 value。工具参数始终提交原始 value。",
    ),
    options: z.array(selectOption).min(1).max(50),
  }),
  z.object({
    ...keyValueFieldBase,
    defaultValue: z.array(keyValueEntry).max(30).optional(),
    valueMode: z.literal("plain").optional(),
  }),
  z.object({
    ...keyValueFieldBase,
    defaultValue: z.never().optional().describe("Secret 键值字段禁止模型提供默认值，必须由用户当次手动输入。"),
    valueMode: z.literal("secret"),
  }),
  z.object({
    ...fieldBase,
    type: z.literal("secret"),
    defaultValue: z.never().optional().describe("Secret 字段禁止模型提供默认值，必须由用户当次手动输入。"),
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
  target: safeJsonPointer,
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
  title: shortText,
  description,
  mode: z.enum(["presentation", "interactive"]).describe(
    "卡片组的会话职责。presentation 表示当前任务已经回答完，只呈现事实或结果；interactive 表示当前工作流正在等待用户选择、填写或确认后才能继续。",
  ),
  placement: z.enum(["inline", "turn_end"]).optional().describe(
    "卡片在本轮助手回复中的渲染位置。inline 保持真实事件位置；turn_end 将单张、等待提交的交互表单投影到本轮回复末尾。默认 inline。",
  ),
  template: z.enum(["candidates", "form", "change_review", "result", "live_task"]),
  display: z.object({
    density: z.enum(["comfortable", "compact"]).optional(),
  }).optional(),
  cards: z.array(card).min(1).max(12),
  groupActions: z.array(cardAction).max(3).optional(),
}).strict().superRefine((input, context) => {
  const formCards = input.cards.filter(item => item.form)
  const responseActions = [
    ...input.cards.flatMap(item => item.actions ?? []),
    ...(input.groupActions ?? []),
  ].filter(action => action.type === "send_message" || action.type === "tool")

  if (input.mode === "presentation" && formCards.length > 0) {
    context.addIssue({
      code: "custom",
      message: "Presentation cards cannot contain input fields. Use interactive mode.",
      path: ["mode"],
    })
  }
  if (input.mode === "interactive" && responseActions.length === 0) {
    context.addIssue({
      code: "custom",
      message: "Interactive cards must provide a send_message or tool action that submits the user's decision.",
      path: ["mode"],
    })
  }
  if ((input.placement ?? "inline") === "turn_end" && (
    input.mode !== "interactive"
    || input.cards.length !== 1
    || formCards.length !== 1
  )) {
    context.addIssue({
      code: "custom",
      message: "turn_end placement is only valid for one interactive card containing a form. Use inline for presentation cards, multiple cards, or non-form interactions.",
      path: ["placement"],
    })
  }
  if (input.template === "form" && input.mode !== "interactive") {
    context.addIssue({
      code: "custom",
      message: `${input.template} templates must use interactive mode.`,
      path: ["mode"],
    })
  }
  if (input.template === "form" && formCards.length === 0) {
    context.addIssue({
      code: "custom",
      message: `${input.template} templates must contain input fields.`,
      path: ["cards"],
    })
  }
  if (input.template === "change_review" && input.mode !== "interactive") {
    context.addIssue({
      code: "custom",
      message: "Change-review templates must use interactive mode so the user can confirm or revise the proposed change.",
      path: ["mode"],
    })
  }
  if (input.template === "result" && input.mode !== "presentation") {
    context.addIssue({
      code: "custom",
      message: "Result templates must use presentation mode because they only present facts or completed outcomes.",
      path: ["mode"],
    })
  }
  const liveProgressBlocks = input.cards.flatMap(item => item.blocks ?? []).filter(block => block.type === "live_progress")
  if (input.template === "live_task" && (input.mode !== "presentation" || liveProgressBlocks.length === 0)) {
    context.addIssue({
      code: "custom",
      message: "Live-task templates must be presentation cards bound to at least one authoritative live progress operation.",
      path: ["template"],
    })
  }
  if (input.template !== "live_task" && liveProgressBlocks.length > 0) {
    context.addIssue({
      code: "custom",
      message: "Live progress blocks are only valid in the live_task template.",
      path: ["template"],
    })
  }

  const cardIds = new Set<string>()
  const groupActionIds = new Set<string>()
  input.groupActions?.forEach((action, actionIndex) => {
    if (groupActionIds.has(action.id)) {
      context.addIssue({
        code: "custom",
        message: "Group action IDs must be unique.",
        path: ["groupActions", actionIndex, "id"],
      })
    }
    groupActionIds.add(action.id)
  })
  input.cards.forEach((item, cardIndex) => {
    if (cardIds.has(item.id))
      context.addIssue({ code: "custom", message: "Card IDs must be unique.", path: ["cards", cardIndex, "id"] })
    cardIds.add(item.id)
    const sectionIds = new Set<string>()
    item.form?.sections.forEach((section, sectionIndex) => {
      if (sectionIds.has(section.id)) {
        context.addIssue({
          code: "custom",
          message: "Section IDs inside a card must be unique.",
          path: ["cards", cardIndex, "form", "sections", sectionIndex, "id"],
        })
      }
      sectionIds.add(section.id)
    })
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
    const cardResponseActions = (item.actions ?? []).filter(action => action.type === "send_message" || action.type === "tool")
    if (input.mode === "interactive" && fields.length > 0 && cardResponseActions.length === 0) {
      context.addIssue({
        code: "custom",
        message: "Every interactive card with input fields must provide its own submit action.",
        path: ["cards", cardIndex, "actions"],
      })
    }
    if (
      input.mode === "interactive"
      && fields.length === 0
      && item.blocks?.some(block => block.type === "item_list" && block.items.length > 1)
    ) {
      context.addIssue({
        code: "custom",
        message: "An interactive multi-item list must expose the candidates as a select field, or use one actionable card per candidate.",
        path: ["cards", cardIndex, "blocks"],
      })
    }
    const sourceRefIds = new Set((item.sourceRefs ?? []).map(source => source.refId))
    if (sourceRefIds.size !== (item.sourceRefs?.length ?? 0)) {
      const seenSourceRefIds = new Set<string>()
      item.sourceRefs?.forEach((source, sourceIndex) => {
        if (seenSourceRefIds.has(source.refId)) {
          context.addIssue({
            code: "custom",
            message: "Source reference IDs inside a card must be unique.",
            path: ["cards", cardIndex, "sourceRefs", sourceIndex, "refId"],
          })
        }
        seenSourceRefIds.add(source.refId)
      })
    }
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
        if (nodeIds.size !== block.nodes.length) {
          const seenNodeIds = new Set<string>()
          block.nodes.forEach((node, nodeIndex) => {
            if (seenNodeIds.has(node.id)) {
              context.addIssue({
                code: "custom",
                message: "Relation node IDs must be unique within a block.",
                path: ["cards", cardIndex, "blocks", blockIndex, "nodes", nodeIndex, "id"],
              })
            }
            seenNodeIds.add(node.id)
          })
        }
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
        const seenColumnKeys = new Set<string>()
        block.columns.forEach((column, columnIndex) => {
          if (seenColumnKeys.has(column.key)) {
            context.addIssue({
              code: "custom",
              message: "Table column keys must be unique within a block.",
              path: ["cards", cardIndex, "blocks", blockIndex, "columns", columnIndex, "key"],
            })
          }
          seenColumnKeys.add(column.key)
        })
        const seenRowIds = new Set<string>()
        block.rows.forEach((row, rowIndex) => {
          if (seenRowIds.has(row.id)) {
            context.addIssue({
              code: "custom",
              message: "Table row IDs must be unique within a block.",
              path: ["cards", cardIndex, "blocks", blockIndex, "rows", rowIndex, "id"],
            })
          }
          seenRowIds.add(row.id)
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
        const seenSeriesNames = new Set<string>()
        block.series.forEach((series, seriesIndex) => {
          if (seenSeriesNames.has(series.name)) {
            context.addIssue({
              code: "custom",
              message: "Chart series names must be unique within a block.",
              path: ["cards", cardIndex, "blocks", blockIndex, "series", seriesIndex, "name"],
            })
          }
          seenSeriesNames.add(series.name)
        })
        const lengths = new Set(block.series.map(series => series.values.length))
        if (lengths.size > 1 || (block.xAxis && [...lengths].some(length => length !== block.xAxis!.length))) {
          context.addIssue({
            code: "custom",
            message: "Chart series and x-axis must use the same number of points.",
            path: ["cards", cardIndex, "blocks", blockIndex],
          })
        }
      }
      if (block.type === "item_list" || block.type === "status_list" || block.type === "timeline") {
        const seenItemIds = new Set<string>()
        block.items.forEach((blockItem, itemIndex) => {
          if (seenItemIds.has(blockItem.id)) {
            context.addIssue({
              code: "custom",
              message: "Item IDs must be unique within a content block.",
              path: ["cards", cardIndex, "blocks", blockIndex, "items", itemIndex, "id"],
            })
          }
          seenItemIds.add(blockItem.id)
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
      if (field.type === "select" || field.type === "multi_select") {
        const seenOptionValues = new Set<string>()
        field.options.forEach((option, optionIndex) => {
          if (seenOptionValues.has(option.value)) {
            context.addIssue({
              code: "custom",
              message: "Option values must be unique within a field.",
              path: ["cards", cardIndex, "form", "fields", fieldIndex, "options", optionIndex, "value"],
            })
          }
          seenOptionValues.add(option.value)
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
    if (action.type === "tool" && action.bindings.some(binding => binding.value.type === "field")) {
      context.addIssue({
        code: "custom",
        message: "组级动作不能引用卡片表单字段。",
        path: ["groupActions", actionIndex, "bindings"],
      })
    }
    if (action.type === "send_message" && messageTemplateFieldIds(action.message).length > 0) {
      context.addIssue({
        code: "custom",
        message: "组级动作不能引用卡片表单字段。",
        path: ["groupActions", actionIndex, "message"],
      })
    }
  })
  if (Buffer.byteLength(JSON.stringify(input), "utf8") > 96 * 1024)
    context.addIssue({ code: "custom", message: "Card group exceeds 96 KiB." })
})

export type CreateInteractionCardsInput = Omit<InteractionCardGroup, "generationId">


const createInteractionCardsRequestInput = z.union([
  createBusinessCardTemplateInput,
  createInteractionCardsInput,
])

export function normalizeInteractionCardsInput(raw: unknown): unknown {
  if (!raw || typeof raw !== "object" || Array.isArray(raw))
    return raw
  const input = structuredClone(raw) as Record<string, unknown>
  if (input.schemaVersion === "1")
    input.schemaVersion = 1
  const businessTemplate = createBusinessCardTemplateInput.safeParse(input)
  const normalizedInput = businessTemplate.success
    ? compileBusinessCardTemplate(businessTemplate.data)
    : input
  if (!normalizedInput || typeof normalizedInput !== "object" || Array.isArray(normalizedInput))
    return normalizedInput
  return normalizeCardSections(normalizedInput as Record<string, unknown>)
}

function normalizeCardSections(input: Record<string, unknown>): unknown {
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

// 仅供历史 create_interaction_cards payload 校验与回放；它不是模型工具
// 定义，避免旧 operationId 再次被加入未来模型的工具集合。
export const legacyInteractionCardsInputSchema = cardInputJsonSchema()

function messageTemplateFieldIds(message: string): string[] {
  return [...message.matchAll(/\{\{([a-zA-Z0-9_-]{1,64})\}\}/g)].map(match => match[1]!)
}

function cardInputJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(createInteractionCardsRequestInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  // OpenAI-compatible providers require function parameters to declare an
  // object at the schema root, even when the object variants are expressed by
  // a top-level anyOf generated from a Zod union.
  return { ...schema, type: "object" }
}
