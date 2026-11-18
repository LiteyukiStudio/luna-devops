import { z } from "zod"
import type { InteractionCardGroup } from "@luna-devops/ai-interaction-card-contract"
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

const cardPresentation = z.object({
  variant: z.enum(["application", "resource", "form", "finding", "plan", "task", "receipt", "summary"]),
  title: shortText,
  subtitle: z.string().trim().max(160).optional(),
  description,
  icon: icon.optional(),
  badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone })).max(6).optional(),
})

const presentCardPresentation = z.object({
  variant: z.enum(["application", "resource", "finding", "plan", "task", "receipt", "summary"]),
  title: shortText,
  subtitle: z.string().trim().max(160).optional(),
  description,
  icon: icon.optional(),
  badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone })).max(6).optional(),
})

const requestInputPresentation = z.object({
  variant: z.enum(["form", "application", "resource", "summary"]).default("form"),
  title: shortText,
  subtitle: z.string().trim().max(160).optional(),
  description,
  icon: icon.optional(),
  badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone })).max(6).optional(),
})

const requestChoicePresentation = z.object({
  variant: z.enum(["application", "resource", "plan", "summary"]).default("resource"),
  title: shortText,
  subtitle: z.string().trim().max(160).optional(),
  description,
  icon: icon.optional(),
  badges: z.array(z.object({ label: z.string().trim().min(1).max(60), tone })).max(6).optional(),
})

const displayConfig = z.object({
  density: z.enum(["comfortable", "compact"]).optional(),
})

const lightweightBlockBase = {
  id: identifier,
  title: shortText.optional(),
  collapsible: z.boolean().optional(),
  defaultExpanded: z.boolean().optional(),
}

const blockBase = {
  ...lightweightBlockBase,
  sourceRefIds: z.array(identifier).max(8).optional(),
}

const keyValueItems = z.array(z.object({
  label: shortText,
  value: z.string().max(2000),
  format: z.enum(["text", "code", "status", "duration", "date_time", "bytes", "currency"]).optional(),
  copyable: z.boolean().optional(),
})).min(1).max(16)

const lightweightMarkdownBlock = z.object({ ...lightweightBlockBase, type: z.literal("markdown"), content: z.string().trim().min(1).max(8000) })
const lightweightCalloutBlock = z.object({ ...lightweightBlockBase, type: z.literal("callout"), tone, content: z.string().trim().min(1).max(2000) })
const lightweightKeyValueBlock = z.object({ ...lightweightBlockBase, type: z.literal("key_value"), items: keyValueItems })

const markdownBlock = z.object({ ...blockBase, type: z.literal("markdown"), content: z.string().trim().min(1).max(8000) })
const calloutBlock = z.object({ ...blockBase, type: z.literal("callout"), tone, content: z.string().trim().min(1).max(2000) })
const keyValueBlock = z.object({ ...blockBase, type: z.literal("key_value"), items: keyValueItems })

const lightweightContextBlock = z.discriminatedUnion("type", [
  lightweightMarkdownBlock,
  lightweightCalloutBlock,
  lightweightKeyValueBlock,
])

const choiceContextBlock = z.discriminatedUnion("type", [
  lightweightMarkdownBlock,
  lightweightKeyValueBlock,
])

const contentBlock = z.discriminatedUnion("type", [
  markdownBlock,
  calloutBlock,
  keyValueBlock,
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
      "平台资源使用 label_value（回传“名称 (ID)”）；普通枚举使用 value。",
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
      "平台资源使用 label_value（回传“名称 (ID)”）；普通枚举使用 value。",
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
    valueMode: z.literal("secret"),
  }).strict(),
  z.object({
    ...fieldBase,
    type: z.literal("secret"),
    placeholder: z.string().max(200).optional(),
    generation: z.enum(["disabled", "optional", "required"]),
    defaultMode: z.enum(["manual", "generate"]).optional(),
  }).strict(),
])

const nonFieldBindingValue = z.discriminatedUnion("type", [
  z.object({ type: z.literal("card"), property: z.literal("id") }),
  z.object({ type: z.literal("literal"), value: z.union([z.string(), z.number(), z.boolean(), z.null()]) }),
])
const nonFieldBinding = z.object({
  target: safeJsonPointer,
  value: nonFieldBindingValue,
})

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

const navigateAction = z.object({
  ...actionBase,
  type: z.literal("navigate"),
  routeName: registeredRouteName,
  routeParams: routeIdentifiers.optional(),
})

const nonFieldCardAction = z.discriminatedUnion("type", [
  z.object({
    ...actionBase,
    type: z.literal("tool"),
    operationId: z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/),
    bindings: z.array(nonFieldBinding).max(40),
  }),
  z.object({
    ...actionBase,
    type: z.literal("send_message"),
    message: z.string().trim().min(1).max(2000),
  }),
  navigateAction,
])

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
      .describe("发送给助手的消息。带入非敏感字段值时使用 {{field_id}}。"),
  }),
  navigateAction,
])

const choiceSelectionSchema = z.object({
  id: identifier,
  label: shortText,
  description,
  required: z.boolean().optional(),
  multiple: z.boolean().optional(),
  display: z.enum(["select", "radio", "segmented"]).optional(),
  defaultValue: z.union([z.string().max(240), z.array(z.string().max(240)).max(50)]).optional(),
  minItems: z.number().int().min(0).max(50).optional(),
  maxItems: z.number().int().min(1).max(50).optional(),
  submissionFormat: z.enum(["value", "label_value"]).optional().describe(
    "平台资源使用 label_value（回传“名称 (ID)”）；普通枚举使用 value。",
  ),
  options: z.array(selectOption).min(1).max(50),
})

const presentCardItem = z.object({
  id: identifier,
  presentation: presentCardPresentation,
  sourceRefs: z.array(sourceRef).max(12).optional(),
  blocks: z.array(contentBlock).max(12).optional(),
  actions: z.array(nonFieldCardAction).max(4).optional(),
})

const requestInputCardItem = z.object({
  id: identifier,
  presentation: requestInputPresentation,
  blocks: z.array(lightweightContextBlock).max(4).optional(),
  form: z.object({
    sections: z.array(z.object({
      id: identifier,
      title: shortText.optional(),
      description,
      fields: z.array(formField).min(1).max(12),
    })).min(1).max(6),
  }),
  actions: z.array(cardAction).min(1).max(4),
})

const requestChoiceCardItem = z.object({
  id: identifier,
  presentation: requestChoicePresentation,
  blocks: z.array(choiceContextBlock).max(4).optional(),
  selection: choiceSelectionSchema.optional(),
  actions: z.array(cardAction).max(4).optional(),
})

const card = z.object({
  id: identifier,
  presentation: cardPresentation,
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

/**
 * 1. present_card 专属输入 Schema：仅展示态（result / change_review / live_task），物理移除 form 与全部表单字段。
 */
export const presentCardToolInput = z.object({
  schemaVersion: z.literal(1).optional(),
  mode: z.literal("presentation").optional(),
  title: shortText,
  description,
  template: z.enum(["result", "change_review", "live_task"]),
  display: displayConfig.optional(),
  cards: z.array(presentCardItem).min(1).max(12),
  groupActions: z.array(nonFieldCardAction).max(3).optional(),
}).strict().superRefine((input, context) => {
  const normalized = normalizePresentCardInput(input)
  validateNormalizedCardGroup(normalized, context)
})

/**
 * 2. request_input 专属输入 Schema：仅表单收集（interactive + form），物理移除 chart / relations / diff / live_progress 等重型展示块。
 */
export const requestInputToolInput = z.object({
  schemaVersion: z.literal(1).optional(),
  mode: z.literal("interactive").optional(),
  template: z.literal("form").optional(),
  placement: z.enum(["inline", "turn_end"]).optional(),
  title: shortText,
  description,
  display: displayConfig.optional(),
  cards: z.array(requestInputCardItem).min(1).max(12),
}).strict().superRefine((input, context) => {
  const normalized = normalizeRequestInputInput(input)
  validateNormalizedCardGroup(normalized, context)
})

/**
 * 3. request_choice 专属输入 Schema：仅候选选择（interactive + candidates），物理移除 form 块与重型展示块。
 */
export const requestChoiceToolInput = z.object({
  schemaVersion: z.literal(1).optional(),
  mode: z.literal("interactive").optional(),
  template: z.literal("candidates").optional(),
  title: shortText,
  description,
  display: displayConfig.optional(),
  cards: z.array(requestChoiceCardItem).min(1).max(12),
}).strict().superRefine((input, context) => {
  input.cards.forEach((item, cardIndex) => {
    if (!item.selection) return
    if (item.selection.minItems !== undefined && item.selection.maxItems !== undefined && item.selection.minItems > item.selection.maxItems) {
      context.addIssue({
        code: "custom",
        message: "minItems must be less than or equal to maxItems.",
        path: ["cards", cardIndex, "selection", "maxItems"],
      })
    }
    if (item.selection.multiple && typeof item.selection.defaultValue === "string") {
      context.addIssue({
        code: "custom",
        message: "Multi-select defaultValue must be an array of option values.",
        path: ["cards", cardIndex, "selection", "defaultValue"],
      })
    }
    if (!item.selection.multiple && Array.isArray(item.selection.defaultValue)) {
      context.addIssue({
        code: "custom",
        message: "Single-select defaultValue must be a string option value.",
        path: ["cards", cardIndex, "selection", "defaultValue"],
      })
    }
  })
  const normalized = normalizeRequestChoiceInput(input)
  validateNormalizedCardGroup(normalized, mapChoiceRefinementContext(context))
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
  display: displayConfig.optional(),
  cards: z.array(card).min(1).max(12),
  groupActions: z.array(cardAction).max(3).optional(),
}).strict().superRefine((input, context) => {
  validateNormalizedCardGroup(input, context)
})

export type CreateInteractionCardsInput = Omit<InteractionCardGroup, "generationId">
export type PresentCardToolInput = z.infer<typeof presentCardToolInput>
export type RequestInputToolInput = z.infer<typeof requestInputToolInput>
export type RequestChoiceToolInput = z.infer<typeof requestChoiceToolInput>

export function normalizePresentCardInput(input: z.output<typeof presentCardToolInput>): z.input<typeof createInteractionCardsInput> {
  return {
    ...input,
    schemaVersion: 1,
    mode: "presentation",
    cards: input.cards.map(item => ({
      ...item,
      ...(item.actions ? { actions: item.actions } : {}),
    })),
  }
}

export function normalizeRequestInputInput(input: z.output<typeof requestInputToolInput>): z.input<typeof createInteractionCardsInput> {
  return {
    ...input,
    schemaVersion: 1,
    mode: "interactive",
    template: "form",
  }
}

export function normalizeRequestChoiceInput(input: z.output<typeof requestChoiceToolInput>): z.input<typeof createInteractionCardsInput> {
  return {
    schemaVersion: 1,
    title: input.title,
    ...(input.description !== undefined ? { description: input.description } : {}),
    mode: "interactive",
    template: "candidates",
    ...(input.display !== undefined ? { display: input.display } : {}),
    cards: input.cards.map((item) => {
      const { selection, ...rest } = item
      if (!selection) return rest
      const field = selection.multiple
        ? {
            id: selection.id,
            type: "multi_select" as const,
            label: selection.label,
            ...(selection.description !== undefined ? { description: selection.description } : {}),
            required: selection.required ?? true,
            ...(Array.isArray(selection.defaultValue) ? { defaultValue: selection.defaultValue } : {}),
            ...(selection.minItems !== undefined ? { minItems: selection.minItems } : {}),
            ...(selection.maxItems !== undefined ? { maxItems: selection.maxItems } : {}),
            ...(selection.submissionFormat !== undefined ? { submissionFormat: selection.submissionFormat } : {}),
            options: selection.options,
          }
        : {
            id: selection.id,
            type: "select" as const,
            label: selection.label,
            ...(selection.description !== undefined ? { description: selection.description } : {}),
            required: selection.required ?? true,
            display: selection.display ?? "radio",
            ...(typeof selection.defaultValue === "string" ? { defaultValue: selection.defaultValue } : {}),
            ...(selection.submissionFormat !== undefined ? { submissionFormat: selection.submissionFormat } : {}),
            options: selection.options,
          }
      return {
        ...rest,
        form: {
          sections: [{
            id: `${selection.id}_section`.slice(0, 64),
            fields: [field],
          }],
        },
      }
    }),
  }
}

type ValidationIssueSink = {
  addIssue: (issue: { code: "custom", message: string, path?: PropertyKey[] }) => void
}

function validateNormalizedCardGroup(input: z.input<typeof createInteractionCardsInput>, context: ValidationIssueSink): void {
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
      (block as { sourceRefIds?: string[] }).sourceRefIds?.forEach((refId, refIndex) => {
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
      if ((field.type === "secret" || (field.type === "key_value" && field.valueMode === "secret"))
        && "defaultValue" in (field as Record<string, unknown>)
        && (field as Record<string, unknown>).defaultValue !== undefined) {
        context.addIssue({
          code: "custom",
          message: "Secret fields must not define defaultValue.",
          path: ["cards", cardIndex, "form", "fields", fieldIndex, "defaultValue"],
        })
      }
      if ((field.type === "text" || field.type === "textarea")
        && field.minLength !== undefined
        && field.maxLength !== undefined
        && field.minLength > field.maxLength) {
        context.addIssue({
          code: "custom",
          message: "minLength must be less than or equal to maxLength.",
          path: ["cards", cardIndex, "form", "fields", fieldIndex, "maxLength"],
        })
      }
      if (field.type === "number" && field.min !== undefined && field.max !== undefined && field.min > field.max) {
        context.addIssue({
          code: "custom",
          message: "min must be less than or equal to max.",
          path: ["cards", cardIndex, "form", "fields", fieldIndex, "max"],
        })
      }
      if ((field.type === "multi_select" || field.type === "key_value")
        && field.minItems !== undefined
        && field.maxItems !== undefined
        && field.minItems > field.maxItems) {
        context.addIssue({
          code: "custom",
          message: "minItems must be less than or equal to maxItems.",
          path: ["cards", cardIndex, "form", "fields", fieldIndex, "maxItems"],
        })
      }
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
        if (field.type === "select" && field.defaultValue !== undefined && !seenOptionValues.has(field.defaultValue)) {
          context.addIssue({
            code: "custom",
            message: `Default value "${field.defaultValue}" must match one of options[].value.`,
            path: ["cards", cardIndex, "form", "fields", fieldIndex, "defaultValue"],
          })
        }
        if (field.type === "multi_select" && field.defaultValue !== undefined) {
          field.defaultValue.forEach((defaultItem, defaultIndex) => {
            if (!seenOptionValues.has(defaultItem)) {
              context.addIssue({
                code: "custom",
                message: `Default value "${defaultItem}" must match one of options[].value.`,
                path: ["cards", cardIndex, "form", "fields", fieldIndex, "defaultValue", defaultIndex],
              })
            }
          })
        }
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
    if (action.type === "tool" && action.bindings.some(itemBinding => itemBinding.value.type === "field")) {
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
}

function mapChoiceRefinementContext(context: ValidationIssueSink): ValidationIssueSink {
  return {
    addIssue(issue) {
      const path = issue.path ? mapChoiceIssuePath(issue.path) : undefined
      context.addIssue(path ? { ...issue, path } : issue)
    },
  }
}

function mapChoiceIssuePath(path: readonly PropertyKey[]): PropertyKey[] {
  if (path[0] === "cards" && typeof path[1] === "number" && path[2] === "form") {
    if (path[3] === "fields" && path[4] === 0) {
      return ["cards", path[1], "selection", ...path.slice(5)]
    }
    if (path[3] === "sections" && path[4] === 0 && path[5] === "fields" && path[6] === 0) {
      return ["cards", path[1], "selection", ...path.slice(7)]
    }
    return ["cards", path[1], "selection", ...path.slice(3)]
  }
  return [...path]
}

function messageTemplateFieldIds(message: string): string[] {
  return [...message.matchAll(/\{\{([a-zA-Z0-9_-]{1,64})\}\}/g)].map(match => match[1]!)
}
