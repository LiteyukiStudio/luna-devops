export type InteractionCardTone = 'neutral' | 'success' | 'warning' | 'error'
export type InteractionCardStatus = 'pending' | 'running' | 'success' | 'warning' | 'error' | 'skipped'
export type AIToolVisibility = 'normal' | 'internal'

export const aiOptionIconNames: readonly [
  'activity',
  'book-open',
  'box',
  'circle-help',
  'cloud',
  'code',
  'database',
  'folder-kanban',
  'gauge',
  'git-branch',
  'globe',
  'list-checks',
  'message-circle',
  'package',
  'rocket',
  'search',
  'settings',
  'shield-check',
  'terminal',
  'wrench',
]
export type AIOptionIconName = typeof aiOptionIconNames[number]
export type AIOptionVisual
  = | { type: 'emoji', value: string }
    | { type: 'icon', value: AIOptionIconName }
    | { type: 'img', value: string }

export type InteractionCardRouteName
  = | 'dashboard' | 'projects' | 'project.workspace' | 'application.detail' | 'events'
    | 'code-repositories' | 'registries' | 'clusters' | 'app-templates' | 'billing'
    | 'settings.account' | 'settings.auth-providers' | 'settings.notifications'
    | 'settings.operations' | 'settings.site' | 'settings.users'

export type InteractionCardIcon
  = | { type: 'asset', assetRef: string, alt: string }
    | { type: 'category', name: 'database' | 'cache' | 'queue' | 'storage' | 'observability' | 'application' | 'repository' | 'registry' | 'cluster' | 'build' | 'deployment' | 'gateway' | 'security' | 'billing' | 'notification', alt: string }

export interface InteractionCardSourceRef {
  type: 'app_template' | 'web_search_result' | 'web_page' | 'platform_resource' | 'platform_event' | 'tool_result'
  refId: string
  label: string
  trust: 'platform' | 'official' | 'community'
}

interface InteractionContentBlockBase {
  id: string
  title?: string
  sourceRefIds?: string[]
  collapsible?: boolean
  defaultExpanded?: boolean
}

export type InteractionContentBlock
  = | InteractionContentBlockBase & { type: 'markdown', content: string }
    | InteractionContentBlockBase & { type: 'callout', tone: InteractionCardTone, content: string }
    | InteractionContentBlockBase & { type: 'key_value', items: Array<{ label: string, value: string, format?: 'text' | 'code' | 'status' | 'duration' | 'date_time' | 'bytes' | 'currency', copyable?: boolean }> }
    | InteractionContentBlockBase & { type: 'metrics', items: Array<{ label: string, value: string, change?: string, trend?: 'up' | 'down' | 'flat', tone?: InteractionCardTone }> }
    | InteractionContentBlockBase & { type: 'item_list', items: Array<{ id: string, primary: string, secondary?: string, meta?: string, icon?: InteractionCardIcon }> }
    | InteractionContentBlockBase & { type: 'status_list', items: Array<{ id: string, label: string, detail?: string, status: InteractionCardStatus }> }
    | InteractionContentBlockBase & { type: 'data_table', columns: Array<{ key: string, label: string, format?: 'text' | 'code' | 'status' | 'duration' | 'date_time' | 'bytes' | 'currency' }>, rows: Array<{ id: string, cells: Record<string, string> }> }
    | InteractionContentBlockBase & { type: 'code', language: string, content: string, filename?: string }
    | InteractionContentBlockBase & { type: 'diff', language?: string, beforeLabel?: string, afterLabel?: string, unifiedDiff: string }
    | InteractionContentBlockBase & { type: 'timeline', items: Array<{ id: string, title: string, detail?: string, timestamp?: string, status?: Exclude<InteractionCardStatus, 'skipped'> }> }
    | InteractionContentBlockBase & { type: 'chart', chartType: 'line' | 'bar' | 'area' | 'donut', xAxis?: string[], series: Array<{ name: string, values: number[], unit?: string }> }
    | InteractionContentBlockBase & { type: 'relations', nodes: Array<{ id: string, label: string, category: string, status?: InteractionCardTone }>, edges: Array<{ source: string, target: string, label?: string }> }
    | InteractionContentBlockBase & { type: 'live_progress', binding: { operationType: 'build_run' | 'release' | 'hook_run' | 'app_template_installation', projectId: string, operationId: string }, label?: string, detail?: string }
    | InteractionContentBlockBase & { type: 'resource_links', links: Array<{ label: string, routeName?: InteractionCardRouteName, routeParams?: Record<string, string>, sourceRefId?: string }> }

export interface InteractionCardCondition {
  fieldId: string
  operator: 'equals' | 'not_equals' | 'contains' | 'is_empty' | 'is_not_empty'
  value?: string | number | boolean
}

export interface InteractionCardSelectOption {
  value: string
  label: string
  description?: string
  disabled?: boolean
}

interface InteractionFormFieldBase {
  id: string
  label: string
  description?: string
  required?: boolean
  visibleWhen?: InteractionCardCondition
}

export type InteractionFormField
  = | InteractionFormFieldBase & { type: 'text', defaultValue?: string, placeholder?: string, format?: 'plain' | 'identifier' | 'namespace' | 'hostname' | 'email' | 'url' | 'image_ref' | 'cpu' | 'memory' | 'storage', minLength?: number, maxLength?: number }
    | InteractionFormFieldBase & { type: 'textarea', defaultValue?: string, placeholder?: string, minLength?: number, maxLength?: number, rows?: number }
    | InteractionFormFieldBase & { type: 'number', defaultValue?: number, integer?: boolean, min?: number, max?: number, step?: number, unit?: string }
    | InteractionFormFieldBase & { type: 'boolean', defaultValue?: boolean }
    | InteractionFormFieldBase & { type: 'select', defaultValue?: string, placeholder?: string, display?: 'select' | 'radio' | 'segmented', submissionFormat?: 'value' | 'label_value', options: InteractionCardSelectOption[] }
    | InteractionFormFieldBase & { type: 'multi_select', defaultValue?: string[], placeholder?: string, minItems?: number, maxItems?: number, submissionFormat?: 'value' | 'label_value', options: InteractionCardSelectOption[] }
    | InteractionFormFieldBase & { type: 'key_value', defaultValue?: Array<{ key: string, value: string }>, keyFormat?: 'plain' | 'identifier' | 'environment_variable', valueMode?: 'plain' | 'secret', minItems?: number, maxItems?: number }
    | InteractionFormFieldBase & { type: 'secret', placeholder?: string, generation: 'disabled' | 'optional' | 'required', defaultMode?: 'manual' | 'generate' }

export type InteractionCardBindingValue
  = | { type: 'field', fieldId: string }
    | { type: 'card', property: 'id' }
    | { type: 'literal', value: string | number | boolean | null }

interface InteractionCardActionBase {
  id: string
  label: string
  description?: string
  emphasis?: 'primary' | 'secondary' | 'ghost'
  repeatable?: boolean
}

export type InteractionCardAction
  = | InteractionCardActionBase & { type: 'tool', operationId: string, bindings: Array<{ target: string, value: InteractionCardBindingValue }> }
    | InteractionCardActionBase & { type: 'send_message', message: string }
    | InteractionCardActionBase & { type: 'navigate', routeName: InteractionCardRouteName, routeParams?: Record<string, string> }

export interface InteractionCard {
  id: string
  presentation: {
    variant: 'application' | 'resource' | 'form' | 'finding' | 'plan' | 'task' | 'receipt' | 'summary'
    title: string
    subtitle?: string
    description?: string
    icon?: InteractionCardIcon
    badges?: Array<{ label: string, tone: InteractionCardTone }>
  }
  sourceRefs?: InteractionCardSourceRef[]
  blocks?: InteractionContentBlock[]
  form?: { sections: Array<{ id: string, title?: string, description?: string, fields: InteractionFormField[] }> }
  actions?: InteractionCardAction[]
}

export interface InteractionCardGroup {
  schemaVersion: 1
  generationId: string
  title: string
  description?: string
  mode: 'presentation' | 'interactive'
  /** inline 保持真实事件位置；turn_end 仅用于单张、阻塞后续流程的交互表单。省略时按 inline 处理。 */
  placement?: 'inline' | 'turn_end'
  template: 'catalog' | 'comparison' | 'inspector' | 'form' | 'wizard' | 'diagnosis' | 'plan' | 'progress' | 'result' | 'dashboard'
  display?: { density?: 'comfortable' | 'compact' }
  cards: InteractionCard[]
  groupActions?: InteractionCardAction[]
}

export interface PrepareInteractionCardsInput {
  schemaVersion: 1
  title: string
  description?: string
  /** 必须与随后 create_interaction_cards 使用的位置一致；省略时按 inline 处理。 */
  placement?: 'inline' | 'turn_end'
}

export interface InteractionCardValidationIssue {
  code: string
  path: string
  message: string
  expected?: string
  received?: string
}

export interface InteractionCardValidationFailure {
  status: 'rejected'
  errorCode: 'ai.interaction_card_schema_invalid' | 'ai.tool_arguments_json_invalid'
  phase: 'prepare' | 'create' | 'provider'
  generationId?: string
  retryable: boolean
  attempt: number
  maxAttempts: number
  issues: InteractionCardValidationIssue[]
  guidance: string
}
