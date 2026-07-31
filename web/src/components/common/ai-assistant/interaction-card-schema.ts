import { z } from 'zod'
import { aiInternalRouteNames } from './internal-routes'

const id = z.string().regex(/^[\w-]{1,64}$/)
const text = z.string().trim().min(1).max(120)
const tone = z.enum(['neutral', 'success', 'warning', 'error'])
const sourceType = z.enum(['app_template', 'web_search_result', 'web_page', 'platform_resource', 'platform_event', 'tool_result'])
const icon = z.discriminatedUnion('type', [
  z.object({ type: z.literal('asset'), assetRef: z.string().min(1).max(240), alt: text }),
  z.object({ type: z.literal('category'), name: z.string().min(1).max(40), alt: text }),
])
const blockBase = {
  id,
  title: text.optional(),
  sourceRefIds: z.array(id).max(8).optional(),
  collapsible: z.boolean().optional(),
  defaultExpanded: z.boolean().optional(),
}

export const interactionContentBlockSchema = z.discriminatedUnion('type', [
  z.object({ ...blockBase, type: z.literal('markdown'), content: z.string().min(1).max(8000) }),
  z.object({ ...blockBase, type: z.literal('callout'), tone, content: z.string().min(1).max(2000) }),
  z.object({
    ...blockBase,
    type: z.literal('key_value'),
    items: z.array(z.object({
      label: text,
      value: z.string().max(2000),
      format: z.enum(['text', 'code', 'status', 'duration', 'date_time', 'bytes', 'currency']).optional(),
      copyable: z.boolean().optional(),
    })).min(1).max(16),
  }),
  z.object({
    ...blockBase,
    type: z.literal('metrics'),
    items: z.array(z.object({
      label: text,
      value: z.string().max(120),
      change: z.string().max(120).optional(),
      trend: z.enum(['up', 'down', 'flat']).optional(),
      tone: tone.optional(),
    })).min(1).max(6),
  }),
  z.object({
    ...blockBase,
    type: z.literal('item_list'),
    items: z.array(z.object({ id, primary: text, secondary: z.string().max(300).optional(), meta: z.string().max(160).optional(), icon: icon.optional() })).min(1).max(20),
  }),
  z.object({
    ...blockBase,
    type: z.literal('status_list'),
    items: z.array(z.object({ id, label: text, detail: z.string().max(500).optional(), status: z.enum(['pending', 'running', 'success', 'warning', 'error', 'skipped']) })).min(1).max(20),
  }),
  z.object({
    ...blockBase,
    type: z.literal('data_table'),
    columns: z.array(z.object({ key: id, label: text, format: z.string().max(40).optional() })).min(1).max(8),
    rows: z.array(z.object({ id, cells: z.record(z.string(), z.string().max(2000)) })).max(30),
  }),
  z.object({ ...blockBase, type: z.literal('code'), language: z.string().min(1).max(40), content: z.string().max(16000), filename: z.string().max(200).optional() }),
  z.object({ ...blockBase, type: z.literal('diff'), language: z.string().max(40).optional(), beforeLabel: text.optional(), afterLabel: text.optional(), unifiedDiff: z.string().min(1).max(16000) }),
  z.object({
    ...blockBase,
    type: z.literal('timeline'),
    items: z.array(z.object({ id, title: text, detail: z.string().max(500).optional(), timestamp: z.string().max(80).optional(), status: z.enum(['pending', 'running', 'success', 'warning', 'error']).optional() })).min(1).max(20),
  }),
  z.object({
    ...blockBase,
    type: z.literal('chart'),
    chartType: z.enum(['line', 'bar', 'area', 'donut']),
    xAxis: z.array(z.string().max(120)).max(60).optional(),
    series: z.array(z.object({ name: text, values: z.array(z.number()).max(60), unit: z.string().max(40).optional() })).min(1).max(4),
  }),
  z.object({
    ...blockBase,
    type: z.literal('relations'),
    nodes: z.array(z.object({ id, label: text, category: z.string().min(1).max(60), status: z.enum(['neutral', 'success', 'warning', 'error']).optional() })).min(1).max(30),
    edges: z.array(z.object({ source: id, target: id, label: z.string().max(120).optional() })).max(50),
  }),
  z.object({ ...blockBase, type: z.literal('progress'), mode: z.enum(['determinate', 'indeterminate']), value: z.number().min(0).max(100).optional(), label: text, detail: z.string().max(500).optional() }),
  z.object({
    ...blockBase,
    type: z.literal('resource_links'),
    links: z.array(z.object({
      label: text,
      routeName: z.enum(aiInternalRouteNames).optional(),
      routeParams: z.record(z.string(), z.string()).optional(),
      sourceRefId: id.optional(),
    })).min(1).max(8),
  }),
])

const condition = z.object({
  fieldId: id,
  operator: z.enum(['equals', 'not_equals', 'contains', 'is_empty', 'is_not_empty']),
  value: z.union([z.string(), z.number(), z.boolean()]).optional(),
})
const fieldBase = { id, label: text, description: z.string().max(500).optional(), required: z.boolean().optional(), visibleWhen: condition.optional() }
const option = z.object({ value: z.string().min(1).max(240), label: text, description: z.string().max(500).optional(), disabled: z.boolean().optional() })
export const interactionFormFieldSchema = z.discriminatedUnion('type', [
  z.object({ ...fieldBase, type: z.literal('text'), defaultValue: z.string().max(4000).optional(), placeholder: z.string().max(200).optional(), format: z.string().max(40).optional(), minLength: z.number().int().min(0).optional(), maxLength: z.number().int().positive().optional() }),
  z.object({ ...fieldBase, type: z.literal('textarea'), defaultValue: z.string().max(12000).optional(), placeholder: z.string().max(200).optional(), minLength: z.number().int().min(0).optional(), maxLength: z.number().int().positive().optional(), rows: z.number().int().min(2).max(6).optional() }),
  z.object({ ...fieldBase, type: z.literal('number'), defaultValue: z.number().optional(), integer: z.boolean().optional(), min: z.number().optional(), max: z.number().optional(), step: z.number().positive().optional(), unit: z.string().max(40).optional() }),
  z.object({ ...fieldBase, type: z.literal('boolean'), defaultValue: z.boolean().optional() }),
  z.object({ ...fieldBase, type: z.literal('select'), defaultValue: z.string().max(240).optional(), placeholder: z.string().max(200).optional(), display: z.enum(['select', 'radio', 'segmented']).optional(), options: z.array(option).min(1).max(50) }),
  z.object({ ...fieldBase, type: z.literal('multi_select'), defaultValue: z.array(z.string()).max(50).optional(), placeholder: z.string().max(200).optional(), minItems: z.number().int().min(0).optional(), maxItems: z.number().int().positive().max(50).optional(), options: z.array(option).min(1).max(50) }),
  z.object({ ...fieldBase, type: z.literal('key_value'), defaultValue: z.array(z.object({ key: z.string().max(200), value: z.string().max(2000) })).max(30).optional(), keyFormat: z.string().max(40).optional(), valueMode: z.enum(['plain', 'secret']).optional(), minItems: z.number().int().min(0).optional(), maxItems: z.number().int().positive().max(30).optional() }),
  z.object({ ...fieldBase, type: z.literal('secret'), placeholder: z.string().max(200).optional(), generation: z.enum(['disabled', 'optional', 'required']), defaultMode: z.enum(['manual', 'generate']).optional() }),
])

const binding = z.object({
  target: z.string().startsWith('/').max(240),
  value: z.discriminatedUnion('type', [
    z.object({ type: z.literal('field'), fieldId: id }),
    z.object({ type: z.literal('card'), property: z.literal('id') }),
    z.object({ type: z.literal('literal'), value: z.union([z.string(), z.number(), z.boolean(), z.null()]) }),
  ]),
})
const actionBase = { id, label: text, description: z.string().max(500).optional(), emphasis: z.enum(['primary', 'secondary', 'ghost']).optional(), repeatable: z.boolean().optional() }
export const interactionCardActionSchema = z.discriminatedUnion('type', [
  z.object({ ...actionBase, type: z.literal('tool'), operationId: z.string().regex(/^[a-z][\w.-]{2,100}$/i), bindings: z.array(binding).max(40) }),
  z.object({ ...actionBase, type: z.literal('send_message'), message: z.string().min(1).max(2000) }),
  z.object({ ...actionBase, type: z.literal('navigate'), routeName: z.enum(aiInternalRouteNames), routeParams: z.record(z.string(), z.string()).optional() }),
])

export const interactionCardGroupSchema = z.object({
  schemaVersion: z.literal(1),
  generationId: id,
  title: text,
  description: z.string().max(500).optional(),
  mode: z.enum(['presentation', 'interactive']),
  template: z.enum(['catalog', 'comparison', 'inspector', 'form', 'wizard', 'diagnosis', 'plan', 'progress', 'result', 'dashboard']),
  display: z.object({ density: z.enum(['comfortable', 'compact']).optional() }).optional(),
  cards: z.array(z.object({
    id,
    presentation: z.object({
      variant: z.enum(['application', 'resource', 'form', 'finding', 'plan', 'task', 'receipt', 'summary']),
      title: text,
      subtitle: z.string().max(160).optional(),
      description: z.string().max(500).optional(),
      icon: icon.optional(),
      badges: z.array(z.object({ label: z.string().min(1).max(60), tone })).max(6).optional(),
    }),
    sourceRefs: z.array(z.object({ type: sourceType, refId: id, label: text, trust: z.enum(['platform', 'official', 'community']) })).max(12).optional(),
    blocks: z.array(interactionContentBlockSchema).max(12).optional(),
    form: z.object({ sections: z.array(z.object({ id, title: text.optional(), description: z.string().max(500).optional(), fields: z.array(interactionFormFieldSchema).min(1).max(12) })).min(1).max(6) }).optional(),
    actions: z.array(interactionCardActionSchema).max(4).optional(),
  })).min(1).max(12),
  groupActions: z.array(interactionCardActionSchema).max(3).optional(),
}).superRefine((group, context) => {
  const formCards = group.cards.filter(card => card.form)
  const responseActions = [
    ...group.cards.flatMap(card => card.actions ?? []),
    ...(group.groupActions ?? []),
  ].filter(action => action.type === 'send_message' || action.type === 'tool')

  if (group.mode === 'presentation' && formCards.length > 0) {
    context.addIssue({
      code: 'custom',
      message: 'Presentation cards cannot contain input fields.',
      path: ['mode'],
    })
  }
  if (group.mode === 'interactive' && responseActions.length === 0) {
    context.addIssue({
      code: 'custom',
      message: 'Interactive cards require a submit action.',
      path: ['mode'],
    })
  }
  if ((group.template === 'form' || group.template === 'wizard') && (group.mode !== 'interactive' || formCards.length === 0)) {
    context.addIssue({
      code: 'custom',
      message: 'Form and wizard templates require interactive mode and input fields.',
      path: ['template'],
    })
  }
  group.cards.forEach((card, cardIndex) => {
    const fields = card.form?.sections.flatMap(section => section.fields) ?? []
    const cardResponseActions = (card.actions ?? []).filter(action => action.type === 'send_message' || action.type === 'tool')
    if (group.mode === 'interactive' && fields.length > 0 && cardResponseActions.length === 0) {
      context.addIssue({
        code: 'custom',
        message: 'Interactive input cards require their own submit action.',
        path: ['cards', cardIndex, 'actions'],
      })
    }
    if (
      group.mode === 'interactive'
      && fields.length === 0
      && card.blocks?.some(block => block.type === 'item_list' && block.items.length > 1)
    ) {
      context.addIssue({
        code: 'custom',
        message: 'Interactive multi-item lists require a select field or one actionable card per candidate.',
        path: ['cards', cardIndex, 'blocks'],
      })
    }
  })
})

export type InteractionCardGroup = z.infer<typeof interactionCardGroupSchema>
export type InteractionCard = InteractionCardGroup['cards'][number]
export type InteractionCardAction = z.infer<typeof interactionCardActionSchema>
export type InteractionContentBlock = z.infer<typeof interactionContentBlockSchema>
export type InteractionFormField = z.infer<typeof interactionFormFieldSchema>
