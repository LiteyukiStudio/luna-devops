import type { Control } from 'react-hook-form'
import type { InteractionCard, InteractionCardAction, InteractionCardGroup, InteractionContentBlock, InteractionFormField } from './interaction-card-schema'
import type { AIUIAction } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  Activity,
  AlertCircle,
  Boxes,
  Check,
  ChevronDown,
  ChevronRight,
  Circle,
  CircleDashed,
  CircleDot,
  Copy,
  Database,
  ExternalLink,
  Globe2,
  LoaderCircle,
  Minus,
  Package,
  Plus,
  ShieldCheck,
  Trash2,
  TrendingDown,
  TrendingUp,
  Users,
} from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { SearchSelect } from '@/components/common/search-select'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { CopyableCodeBlock } from './copyable-code-block'
import { InteractionCardChart } from './interaction-card-chart'
import { InteractionCardErrorBoundary } from './interaction-card-error-boundary'
import { readValidatedInteractionCardGroup } from './interaction-card-schema'
import { interactionCardDensity, interactionCardTemplateConfigs, shouldExpandInteractionCard } from './interaction-card-templates'
import { LiveProgressBlock } from './live-progress-block'
import { AIInlineMarkdown, AIMarkdown } from './markdown'

const compactActionClassName = 'h-auto min-h-7 max-w-full gap-1.5 whitespace-normal px-2.5 py-1 !text-[11px] leading-4 [&_svg]:size-3.5'

function withStableKeys<T>(items: readonly T[], identity: (item: T) => string) {
  const occurrences = new Map<string, number>()
  return items.map((item, ordinal) => {
    const base = identity(item)
    const occurrence = occurrences.get(base) ?? 0
    occurrences.set(base, occurrence + 1)
    return { item, key: `${base}:${occurrence}`, ordinal }
  })
}

interface AIInteractionCardsProps {
  arguments: unknown
  onAction: (action: AIUIAction) => Promise<boolean>
}

type FormValues = Record<string, unknown>

const statusClasses = {
  pending: 'text-muted-foreground',
  running: 'text-info',
  success: 'text-success',
  warning: 'text-warning',
  error: 'text-danger',
  skipped: 'text-muted-foreground',
} as const

export function AIInteractionCards({ arguments: rawArguments, onAction }: AIInteractionCardsProps) {
  const { t } = useTranslation()
  const group = useMemo(() => readValidatedInteractionCardGroup(rawArguments), [rawArguments])
  if (!group) {
    return (
      <div className="rounded-container bg-danger-subtle px-3 py-2 text-xs text-danger" role="alert">
        {t('aiAssistant.cards.invalid')}
      </div>
    )
  }
  return (
    <InteractionCardErrorBoundary resetKey={group.generationId} scope="group">
      <InteractionCardGroupView group={group} onAction={onAction} />
    </InteractionCardErrorBoundary>
  )
}

function InteractionCardGroupView({ group, onAction }: { group: InteractionCardGroup, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const density = interactionCardDensity(group)
  const templateConfig = interactionCardTemplateConfigs[group.template]
  return (
    <section className={cn('grid min-w-0 grid-cols-[minmax(0,1fr)]', density === 'compact' ? 'gap-2' : 'gap-2.5')} data-ai-card-density={density} data-ai-card-generation={group.generationId} data-ai-card-group={group.template} data-ai-card-mode={group.mode}>
      <header className="px-0.5">
        <p className="text-[10px] font-medium uppercase tracking-wide text-primary-text">{t(`aiAssistant.cards.templates.${group.template}`)}</p>
        <h3 className="mt-0.5 text-[13px] font-semibold leading-5"><AIInlineMarkdown>{group.title}</AIInlineMarkdown></h3>
        {group.description && <AIMarkdown className="mt-0.5 text-[11px] leading-4 text-muted-foreground">{group.description}</AIMarkdown>}
      </header>
      <div className={cn('grid min-w-0', density === 'compact' ? 'gap-1.5' : 'gap-2', templateConfig.gridClassName)}>
        {withStableKeys(group.cards, card => card.id).map(({ item: card, key: cardKey }) => (
          <InteractionCardErrorBoundary key={cardKey} resetKey={`${group.generationId}:${cardKey}`} scope="card">
            <InteractionCardView card={card} density={density} group={group} onAction={onAction} />
          </InteractionCardErrorBoundary>
        ))}
      </div>
      {group.groupActions && group.groupActions.length > 0 && (
        <div className="flex flex-wrap justify-end gap-1.5">
          {withStableKeys(group.groupActions, action => action.id).map(({ item: action, key: actionKey }) => (
            <InteractionCardErrorBoundary key={actionKey} resetKey={`${group.generationId}:group:${actionKey}`} scope="action">
              <CardActionButton action={action} cardId="group" values={{}} onAction={onAction} />
            </InteractionCardErrorBoundary>
          ))}
        </div>
      )}
    </section>
  )
}

function InteractionCardView({ card, density, group, onAction }: {
  card: InteractionCard
  density: 'comfortable' | 'compact'
  group: InteractionCardGroup
  onAction: (action: AIUIAction) => Promise<boolean>
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(() => shouldExpandInteractionCard(group, Boolean(card.form), Boolean(card.blocks?.some(block => block.defaultExpanded))))
  const fields = useMemo(() => card.form?.sections.flatMap(section => section.fields) ?? [], [card.form])
  const formSchema = useMemo(() => buildFormSchema(fields), [fields])
  const form = useForm<FormValues>({
    defaultValues: defaultValues(fields),
    resolver: zodResolver(formSchema),
    mode: 'onChange',
  })
  const watchedValues = form.watch()
  const toolFormValues = getToolFormValues(fields, watchedValues)
  const publicFormValues = getPublicFormValues(fields, watchedValues)
  const messageValues = messageFormValues(fields, publicFormValues)
  const hasDetails = Boolean(card.blocks?.length || card.form)
  const primaryAction = card.actions?.find(action => action.emphasis === 'primary') ?? card.actions?.[0]
  const secondaryActions = card.actions?.filter(action => action !== primaryAction) ?? []

  return (
    <article className="min-w-0 overflow-hidden rounded-container bg-surface" data-ai-card={card.presentation.variant} data-ai-card-id={card.id} data-ai-card-template={group.template}>
      <div className={cn('flex min-w-0 items-start', density === 'compact' ? 'gap-2 p-2' : 'gap-2.5 p-2.5')}>
        <span className={cn('grid shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text', density === 'compact' ? 'size-8' : 'size-9')}>
          <CardIcon category={card.presentation.icon?.type === 'category' ? card.presentation.icon.name : card.presentation.variant} />
        </span>
        <div className="min-w-0 flex-1">
          <h4 className="text-xs font-semibold leading-4"><AIInlineMarkdown>{card.presentation.title}</AIInlineMarkdown></h4>
          {card.presentation.subtitle && <AIInlineMarkdown className="block text-[10px] leading-4 text-muted-foreground">{card.presentation.subtitle}</AIInlineMarkdown>}
          {card.presentation.description && <AIMarkdown className="mt-1 text-[11px] leading-4 text-muted-foreground">{card.presentation.description}</AIMarkdown>}
          {card.presentation.badges && card.presentation.badges.length > 0 && (
            <div className="mt-1.5 flex flex-wrap gap-1">
              {withStableKeys(card.presentation.badges, badge => `${badge.label}:${badge.tone}`).map(({ item: badge, key }) => (
                <StatusBadge key={key} className="px-1.5 py-0 text-[9px]" tone={badge.tone === 'error' ? 'danger' : badge.tone}>
                  {badge.label}
                </StatusBadge>
              ))}
            </div>
          )}
          {card.sourceRefs && card.sourceRefs.length > 0 && (
            <div className="mt-1.5 flex min-w-0 flex-wrap gap-1" data-ai-card-sources>
              {withStableKeys(card.sourceRefs.slice(0, 4), source => `${source.type}:${source.refId}`).map(({ item: source, key }) => (
                <span key={key} className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-full bg-surface-inset px-1.5 py-0.5 text-[9px] text-muted-foreground" title={source.label}>
                  <SourceTrustIcon trust={source.trust} />
                  <span className="truncate">{source.label}</span>
                  <span className="sr-only">{t(`aiAssistant.cards.trust.${source.trust}`)}</span>
                </span>
              ))}
            </div>
          )}
        </div>
        {hasDetails && (
          <Button aria-expanded={expanded} aria-label={t('aiAssistant.cards.toggleDetails')} className="size-7" size="icon" variant="ghost" onClick={() => setExpanded(value => !value)}>
            <ChevronDown className={cn('size-3.5 transition-transform', expanded && 'rotate-180')} />
          </Button>
        )}
      </div>
      {expanded && (
        <form
          className={cn('grid min-w-0 border-t border-separator-subtle', density === 'compact' ? 'gap-2.5 p-2' : 'gap-3 p-2.5')}
          onSubmit={event => event.preventDefault()}
        >
          {card.blocks && withStableKeys(card.blocks, block => block.id).map(({ item: block, key: blockKey }) => (
            <InteractionCardErrorBoundary key={blockKey} resetKey={`${group.generationId}:${card.id}:block:${blockKey}`} scope="content">
              <ContentBlock block={block} onAction={onAction} />
            </InteractionCardErrorBoundary>
          ))}
          {card.form && withStableKeys(card.form.sections, section => section.id).map(({ item: section, key: sectionKey }) => (
            <fieldset key={sectionKey} className="grid min-w-0 gap-2" data-ai-section-id={section.id}>
              {section.title && <legend className="text-[11px] font-semibold"><AIInlineMarkdown>{section.title}</AIInlineMarkdown></legend>}
              {section.description && <AIMarkdown className="text-[10px] leading-4 text-muted-foreground">{section.description}</AIMarkdown>}
              {withStableKeys(section.fields.filter(field => isFieldVisible(field, watchedValues)), field => field.id).map(({ item: field, key: fieldKey }) => (
                <InteractionCardErrorBoundary key={fieldKey} resetKey={`${group.generationId}:${card.id}:section:${sectionKey}:field:${fieldKey}`} scope="field">
                  <DynamicField control={form.control} field={field} error={form.formState.errors[field.id]?.message as string | undefined} />
                </InteractionCardErrorBoundary>
              ))}
            </fieldset>
          ))}
          {(primaryAction || secondaryActions.length > 0) && (
            <div className="flex flex-wrap justify-end gap-1.5 pt-0.5">
              {withStableKeys(secondaryActions, action => action.id).map(({ item: action, key: actionKey }) => (
                <InteractionCardErrorBoundary key={actionKey} resetKey={`${group.generationId}:${card.id}:action:${actionKey}`} scope="action">
                  <CardActionButton action={action} cardId={card.id} disabled={actionNeedsValidForm(action) && !form.formState.isValid} messageValues={messageValues} values={toolFormValues} onAction={onAction} />
                </InteractionCardErrorBoundary>
              ))}
              {primaryAction && (
                <InteractionCardErrorBoundary resetKey={`${group.generationId}:${card.id}:action:${primaryAction.id}`} scope="action">
                  <CardActionButton action={primaryAction} cardId={card.id} disabled={actionNeedsValidForm(primaryAction) && !form.formState.isValid} messageValues={messageValues} values={toolFormValues} onAction={onAction} />
                </InteractionCardErrorBoundary>
              )}
            </div>
          )}
        </form>
      )}
      {!expanded && primaryAction && !card.form && (
        <div className="flex justify-end border-t border-separator-subtle px-2.5 py-2">
          <InteractionCardErrorBoundary resetKey={`${group.generationId}:${card.id}:action:${primaryAction.id}`} scope="action">
            <CardActionButton action={primaryAction} cardId={card.id} values={{}} onAction={onAction} />
          </InteractionCardErrorBoundary>
        </div>
      )}
    </article>
  )
}

function ContentBlock({ block, onAction }: { block: InteractionContentBlock, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const content = (() => {
    if (block.type === 'markdown')
      return <AIMarkdown className="text-xs leading-5">{block.content}</AIMarkdown>
    if (block.type === 'callout')
      return <div className={cn('rounded-control px-2.5 py-2 text-[11px] leading-4', block.tone === 'error' ? 'bg-danger-subtle text-danger' : block.tone === 'warning' ? 'bg-warning-subtle text-warning' : block.tone === 'success' ? 'bg-success-subtle text-success' : 'bg-info-subtle text-info')}>{block.content}</div>
    if (block.type === 'key_value') {
      return (
        <dl className="grid gap-1.5 text-[11px]">
          {withStableKeys(block.items, item => `${item.label}:${item.value}`).map(({ item, key }) => (
            <div key={key} className="grid grid-cols-[minmax(4.5rem,35%)_minmax(0,1fr)] gap-2">
              <dt className="min-w-0 text-muted-foreground"><AIInlineMarkdown>{item.label}</AIInlineMarkdown></dt>
              <dd className={cn('min-w-0 break-words font-medium', item.format === 'code' && 'font-mono text-[10px]')}>
                <AIInlineMarkdown>{item.value}</AIInlineMarkdown>
                {item.copyable && <CopyButton value={item.value} />}
              </dd>
            </div>
          ))}
        </dl>
      )
    }
    if (block.type === 'metrics') {
      return (
        <div className="grid grid-cols-2 gap-1.5">
          {withStableKeys(block.items, item => `${item.label}:${item.value}`).map(({ item, key }) => (
            <div
              key={key}
              className={cn(
                'rounded-control p-2',
                item.tone === 'success'
                  ? 'bg-success-subtle text-success'
                  : item.tone === 'warning'
                    ? 'bg-warning-subtle text-warning'
                    : item.tone === 'error'
                      ? 'bg-danger-subtle text-danger'
                      : 'bg-surface-inset',
              )}
            >
              <AIInlineMarkdown className="block text-[9px] text-muted-foreground">{item.label}</AIInlineMarkdown>
              <AIInlineMarkdown className="mt-0.5 block text-xs font-semibold">{item.value}</AIInlineMarkdown>
              {item.change && (
                <span className="mt-0.5 flex items-center gap-1 text-[9px] text-muted-foreground">
                  <MetricTrendIcon trend={item.trend} />
                  <AIInlineMarkdown>{item.change}</AIInlineMarkdown>
                </span>
              )}
            </div>
          ))}
        </div>
      )
    }
    if (block.type === 'item_list') {
      return (
        <div className="divide-y divide-separator-subtle">
          {withStableKeys(block.items, item => item.id).map(({ item, key }) => (
            <div key={key} className="flex gap-2 py-1.5">
              <Package className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <AIInlineMarkdown className="block text-[11px] font-medium">{item.primary}</AIInlineMarkdown>
                {item.secondary && <AIMarkdown className="text-[10px] leading-4 text-muted-foreground">{item.secondary}</AIMarkdown>}
              </div>
              {item.meta && <span className="ml-auto shrink-0 text-[9px] text-muted-foreground">{item.meta}</span>}
            </div>
          ))}
        </div>
      )
    }
    if (block.type === 'status_list') {
      return (
        <div className="grid gap-1.5">
          {withStableKeys(block.items, item => item.id).map(({ item, key }) => (
            <div key={key} className="flex items-start gap-2 text-[11px]">
              <StatusIcon status={item.status} />
              <div>
                <AIInlineMarkdown className="block font-medium">{item.label}</AIInlineMarkdown>
                {item.detail && <AIMarkdown className="text-[10px] leading-4 text-muted-foreground">{item.detail}</AIMarkdown>}
              </div>
            </div>
          ))}
        </div>
      )
    }
    if (block.type === 'data_table') {
      return (
        <div className="max-w-full overflow-x-auto rounded-control border border-separator-subtle">
          <table className="w-max min-w-full text-left text-[10px]">
            <thead className="bg-surface-inset"><tr>{withStableKeys(block.columns, column => column.key).map(({ item: column, key }) => <th key={key} className="whitespace-nowrap px-2 py-1.5 font-medium"><AIInlineMarkdown>{column.label}</AIInlineMarkdown></th>)}</tr></thead>
            <tbody>{withStableKeys(block.rows, row => row.id).map(({ item: row, key: rowKey }) => <tr key={rowKey} className="border-t border-separator-subtle">{withStableKeys(block.columns, column => column.key).map(({ item: column, key: columnKey }) => <td key={columnKey} className={cn('max-w-52 px-2 py-1.5 [overflow-wrap:anywhere]', column.format === 'code' && 'font-mono')}><AIInlineMarkdown>{row.cells[column.key] ?? '—'}</AIInlineMarkdown></td>)}</tr>)}</tbody>
          </table>
        </div>
      )
    }
    if (block.type === 'code' || block.type === 'diff') {
      const content = block.type === 'code' ? block.content : block.unifiedDiff
      return <CopyableCodeBlock className="text-[10px]" value={content}><code>{content}</code></CopyableCodeBlock>
    }
    if (block.type === 'timeline') {
      return (
        <div className="grid gap-2 border-l border-separator-strong pl-2.5">
          {withStableKeys(block.items, item => item.id).map(({ item, key }) => (
            <div key={key} className="relative text-[11px] before:absolute before:-left-[13px] before:top-1 before:size-1.5 before:rounded-full before:bg-primary">
              <p className="font-medium">{item.title}</p>
              {item.detail && <p className="text-[10px] text-muted-foreground">{item.detail}</p>}
              {item.timestamp && <time className="text-[9px] text-muted-foreground">{item.timestamp}</time>}
            </div>
          ))}
        </div>
      )
    }
    if (block.type === 'chart') {
      return <InteractionCardChart block={block} label={block.title ?? t('aiAssistant.cards.chart')} />
    }
    if (block.type === 'relations') {
      const nodes = new Map(block.nodes.map(node => [node.id, node]))
      return (
        <div className="grid gap-1.5">
          {withStableKeys(block.edges, edge => `${edge.source}:${edge.target}:${edge.label ?? ''}`).map(({ item: edge, key }) => (
            <div key={key} className="flex min-w-0 items-center gap-1.5 rounded-control bg-surface-inset px-2 py-1.5 text-[10px]">
              <span className="truncate font-medium">{nodes.get(edge.source)?.label ?? edge.source}</span>
              <ChevronRight className="size-3 shrink-0 text-muted-foreground" />
              <span className="truncate font-medium">{nodes.get(edge.target)?.label ?? edge.target}</span>
              {edge.label && <span className="ml-auto shrink-0 text-[9px] text-muted-foreground">{edge.label}</span>}
            </div>
          ))}
          {block.edges.length === 0 && withStableKeys(block.nodes, node => node.id).map(({ item: node, key }) => <div key={key} className="rounded-control bg-surface-inset px-2 py-1.5 text-[10px] font-medium">{node.label}</div>)}
        </div>
      )
    }
    if (block.type === 'live_progress')
      return <LiveProgressBlock key={`${block.binding.projectId}:${block.binding.operationType}:${block.binding.operationId}`} block={block} />
    if (block.type === 'resource_links') {
      return (
        <div className="flex flex-wrap gap-1.5">
          {withStableKeys(block.links.filter(link => link.routeName), link => `${link.label}:${link.routeName}`).map(({ item: link, key }) => (
            <Button key={key} className={compactActionClassName} size="sm" variant="outline" onClick={() => void runInlineCardAction({ version: 1, type: 'navigate', label: link.label, payload: { routeName: link.routeName!, params: link.routeParams ?? {}, query: {} } }, onAction, t('aiAssistant.cards.actionFailed'))}>
              <ExternalLink />
              {link.label}
            </Button>
          ))}
        </div>
      )
    }
    return null
  })()
  if (!content)
    return null
  if (!block.title)
    return <div className="min-w-0" data-ai-content-block={block.type}>{content}</div>
  if (block.collapsible) {
    return (
      <details data-ai-content-block={block.type} open={block.defaultExpanded}>
        <summary className="cursor-pointer text-[11px] font-semibold">{block.title}</summary>
        <div className="mt-1.5">{content}</div>
      </details>
    )
  }
  return (
    <section className="grid gap-1.5" data-ai-content-block={block.type}>
      <h5 className="text-[11px] font-semibold"><AIInlineMarkdown>{block.title ?? ''}</AIInlineMarkdown></h5>
      {content}
      <span className="sr-only">{t('aiAssistant.cards.contentSection')}</span>
    </section>
  )
}

function DynamicField({ control, field, error }: { control: Control<FormValues>, field: InteractionFormField, error?: string }) {
  const { t } = useTranslation()
  const instanceId = useId()
  const controlId = `${instanceId}-control`
  const labelId = `${instanceId}-label`
  const descriptionId = field.description ? `${instanceId}-description` : undefined
  const errorId = error ? `${instanceId}-error` : undefined
  const describedBy = [descriptionId, errorId].filter(Boolean).join(' ') || undefined
  const grouped = field.type === 'multi_select'
    || field.type === 'key_value'
    || field.type === 'secret'
    || (field.type === 'select' && (field.options.length >= 6 || (field.display !== undefined && field.display !== 'select')))
  const labelContent = (
    <>
      <AIInlineMarkdown>{field.label}</AIInlineMarkdown>
      {field.required && <span className="text-primary"> *</span>}
    </>
  )
  return (
    <Controller
      control={control}
      name={field.id}
      render={({ field: input }) => (
        <div className="grid gap-1" data-ai-field-id={field.id}>
          {grouped
            ? <div id={labelId} className="text-[10px] font-medium">{labelContent}</div>
            : <Label id={labelId} className="text-[10px]" htmlFor={controlId}>{labelContent}</Label>}
          {field.description && <div id={descriptionId}><AIMarkdown className="text-[9px] leading-3.5 text-muted-foreground">{field.description}</AIMarkdown></div>}
          {field.type === 'textarea'
            ? <Textarea {...input} id={controlId} aria-describedby={describedBy} aria-invalid={Boolean(error)} aria-required={field.required} rows={field.rows ?? 3} value={String(input.value ?? '')} />
            : field.type === 'number'
              ? <Input ref={input.ref} id={controlId} aria-describedby={describedBy} aria-invalid={Boolean(error)} aria-required={field.required} max={field.max} min={field.min} name={input.name} step={field.step} type="number" value={input.value === undefined ? '' : String(input.value)} onBlur={input.onBlur} onChange={event => input.onChange(event.target.value === '' ? undefined : Number(event.target.value))} />
              : field.type === 'boolean'
                ? (
                    <div className="flex items-center gap-2">
                      <Checkbox ref={input.ref} checked={Boolean(input.value)} id={controlId} aria-describedby={describedBy} aria-invalid={Boolean(error)} aria-required={field.required} name={input.name} onBlur={input.onBlur} onCheckedChange={value => input.onChange(value === true)} />
                      <span className="text-[10px] text-muted-foreground">{input.value ? t('common.enabled') : t('common.disabled')}</span>
                    </div>
                  )
                : field.type === 'select'
                  ? <SelectField controlId={controlId} describedBy={describedBy} error={Boolean(error)} field={field} labelId={labelId} name={`${input.name}-${instanceId}`} required={field.required} value={String(input.value ?? '')} onBlur={input.onBlur} onChange={input.onChange} />
                  : field.type === 'multi_select'
                    ? (
                        <div aria-describedby={describedBy} aria-labelledby={labelId} aria-required={field.required} className="grid gap-1.5" role="group">
                          {withStableKeys(field.options, option => option.value).map(({ item: option, key, ordinal }) => {
                            const optionId = `${controlId}-option-${ordinal}`
                            return (
                              <div key={key} className="flex items-start gap-2 text-[10px]">
                                <Checkbox
                                  id={optionId}
                                  aria-invalid={Boolean(error)}
                                  checked={Array.isArray(input.value) && input.value.includes(option.value)}
                                  disabled={option.disabled}
                                  name={input.name}
                                  onBlur={input.onBlur}
                                  onCheckedChange={(checked) => {
                                    const current = Array.isArray(input.value) ? input.value as string[] : []
                                    input.onChange(checked ? [...current, option.value] : current.filter(value => value !== option.value))
                                  }}
                                />
                                <Label className="block min-w-0 cursor-pointer text-[10px] leading-4" htmlFor={optionId}>
                                  <span>
                                    {option.label}
                                    {option.description && <span className="block text-[9px] text-muted-foreground">{option.description}</span>}
                                  </span>
                                </Label>
                              </div>
                            )
                          })}
                        </div>
                      )
                    : field.type === 'key_value'
                      ? <KeyValueInput controlId={controlId} describedBy={describedBy} labelId={labelId} value={Array.isArray(input.value) ? input.value as Array<{ key: string, value: string }> : []} secret={field.valueMode === 'secret'} onChange={input.onChange} />
                      : field.type === 'secret'
                        ? <Input {...input} id={controlId} aria-describedby={describedBy} aria-invalid={Boolean(error)} aria-labelledby={labelId} aria-required={field.required} placeholder={field.placeholder} type="password" value={String(input.value ?? '')} />
                        : <Input {...input} id={controlId} aria-describedby={describedBy} aria-invalid={Boolean(error)} aria-required={field.required} placeholder={field.placeholder} type="text" value={String(input.value ?? '')} />}
          {error && <p id={errorId} className="text-[9px] text-danger" role="alert">{error}</p>}
        </div>
      )}
    />
  )
}

function SelectField({ controlId, describedBy, error, field, labelId, name, required, value, onBlur, onChange }: {
  controlId: string
  describedBy?: string
  error: boolean
  field: Extract<InteractionFormField, { type: 'select' }>
  labelId: string
  name: string
  required?: boolean
  value: string
  onBlur: () => void
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  if (!field.display || field.display === 'select') {
    if (field.options.length >= 6) {
      return (
        <SearchSelect
          ariaLabel={`${field.label}${required ? ' *' : ''}`}
          filterLocally
          options={field.options}
          placeholder={field.placeholder ?? t('aiAssistant.cards.selectPlaceholder')}
          searchPlaceholder={t('common.search')}
          size="sm"
          value={value}
          onValueChange={(next) => {
            onChange(next)
            onBlur()
          }}
        />
      )
    }
    return (
      <NativeSelect id={controlId} aria-describedby={describedBy} aria-invalid={error} aria-required={required} name={name} value={value} onBlur={onBlur} onChange={event => onChange(event.target.value)}>
        <option value="">{field.placeholder ?? t('aiAssistant.cards.selectPlaceholder')}</option>
        {withStableKeys(field.options, option => option.value).map(({ item: option, key }) => <option key={key} disabled={option.disabled} value={option.value}>{option.label}</option>)}
      </NativeSelect>
    )
  }
  const segmented = field.display === 'segmented'
  return (
    <RadioGroup
      aria-describedby={describedBy}
      aria-invalid={error}
      aria-labelledby={labelId}
      aria-required={required}
      className={cn(segmented ? 'flex flex-wrap gap-1' : 'gap-1.5')}
      name={name}
      value={value}
      onBlur={onBlur}
      onValueChange={onChange}
    >
      {withStableKeys(field.options, option => option.value).map(({ item: option, key, ordinal }) => {
        const optionId = `${controlId}-option-${ordinal}`
        const content = (
          <span className="min-w-0">
            <span className="block [overflow-wrap:anywhere]">{option.label}</span>
            {option.description && <span className="block text-[9px] leading-3.5 text-muted-foreground [overflow-wrap:anywhere]">{option.description}</span>}
          </span>
        )
        if (segmented) {
          return (
            <RadioGroupItem
              key={key}
              className="flex h-auto min-h-8 w-auto min-w-0 max-w-full aspect-auto items-start gap-2 rounded-control! border-separator-subtle px-2 py-1.5 text-left text-[10px] shadow-none focus-visible:ring-2! focus-visible:ring-primary/35! data-[state=checked]:border-primary-border data-[state=checked]:bg-primary-subtle data-[state=checked]:text-primary-text [&_[data-slot=radio-group-indicator]]:hidden"
              disabled={option.disabled}
              id={optionId}
              value={option.value}
            >
              {content}
            </RadioGroupItem>
          )
        }
        return (
          <div key={key} className={cn('flex min-w-0 items-start gap-2 rounded-control px-1 py-0.5 text-[10px]', option.disabled && 'opacity-50')}>
            <RadioGroupItem className="mt-0.5" disabled={option.disabled} id={optionId} value={option.value} />
            <Label className={cn('block min-w-0 text-[10px] leading-4', option.disabled ? 'cursor-not-allowed' : 'cursor-pointer')} htmlFor={optionId}>{content}</Label>
          </div>
        )
      })}
    </RadioGroup>
  )
}

function KeyValueInput({ controlId, describedBy, labelId, value, secret, onChange }: { controlId: string, describedBy?: string, labelId: string, value: Array<{ key: string, value: string }>, secret: boolean, onChange: (value: Array<{ key: string, value: string }>) => void }) {
  const { t } = useTranslation()
  const [rowIds, setRowIds] = useState(() => value.map(() => crypto.randomUUID()))
  const remove = (index: number) => {
    setRowIds(current => current.filter((_, itemIndex) => itemIndex !== index))
    onChange(value.filter((_, itemIndex) => itemIndex !== index))
  }
  const add = () => {
    setRowIds(current => [...current, crypto.randomUUID()])
    onChange([...value, { key: '', value: '' }])
  }
  return (
    <div aria-describedby={describedBy} aria-labelledby={labelId} className="grid gap-1.5" role="group">
      {value.map((entry, index) => (
        <div key={rowIds[index]} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-1">
          <Input id={`${controlId}-key-${rowIds[index]}`} aria-label={t('aiAssistant.cards.key')} value={entry.key} onChange={event => onChange(value.map((item, itemIndex) => itemIndex === index ? { ...item, key: event.target.value } : item))} />
          <Input id={`${controlId}-value-${rowIds[index]}`} aria-label={t('aiAssistant.cards.value')} type={secret ? 'password' : 'text'} value={entry.value} onChange={event => onChange(value.map((item, itemIndex) => itemIndex === index ? { ...item, value: event.target.value } : item))} />
          <Button aria-label={t('common.delete')} className="size-8" size="icon" type="button" variant="ghost" onClick={() => remove(index)}><Trash2 /></Button>
        </div>
      ))}
      <Button className={cn(compactActionClassName, 'justify-start')} size="sm" type="button" variant="outline" onClick={add}>
        <Plus />
        {t('aiAssistant.cards.addEntry')}
      </Button>
    </div>
  )
}

function CardActionButton({ action, cardId, values, messageValues = values, disabled = false, onAction }: { action: InteractionCardAction, cardId: string, values: FormValues, messageValues?: FormValues, disabled?: boolean, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const [done, setDone] = useState(false)
  const repeatable = action.repeatable ?? action.type === 'navigate'
  const run = async () => {
    if (pending || (done && !repeatable))
      return
    setPending(true)
    try {
      const success = await executeCardAction(action, cardId, values, messageValues, onAction)
      if (success && !repeatable)
        setDone(true)
      if (!success)
        throw new Error('ai.card_action_failed')
    }
    catch {
      toast.error(t('aiAssistant.cards.actionFailed'))
    }
    finally {
      setPending(false)
    }
  }
  return (
    <Button className={compactActionClassName} disabled={disabled || pending || (done && !repeatable)} size="sm" type="button" variant={action.emphasis === 'primary' ? 'default' : 'outline'} onClick={() => void run()}>
      {pending ? <LoaderCircle className="animate-spin motion-reduce:animate-none" /> : done ? <Check /> : action.type === 'navigate' ? <ExternalLink /> : <ChevronRight />}
      {action.label}
    </Button>
  )
}

async function runInlineCardAction(action: AIUIAction, onAction: (action: AIUIAction) => Promise<boolean>, failureMessage: string) {
  try {
    if (!await onAction(action))
      toast.error(failureMessage)
  }
  catch {
    toast.error(failureMessage)
  }
}

async function executeCardAction(action: InteractionCardAction, cardId: string, values: FormValues, messageValues: FormValues, onAction: (action: AIUIAction) => Promise<boolean>) {
  if (action.type === 'navigate')
    return onAction({ version: 1, id: action.id, repeatable: action.repeatable ?? true, type: 'navigate', label: action.label, description: action.description, payload: { routeName: action.routeName, params: action.routeParams ?? {}, query: {} } })
  if (action.type === 'send_message')
    return onAction({ version: 1, id: action.id, repeatable: action.repeatable ?? false, type: 'send_message', label: action.label, description: action.description, payload: { message: renderMessageTemplate(action.message, messageValues) } })
  const argumentsValue = bindArguments(action, cardId, values)
  return onAction({
    version: 1,
    id: action.id,
    repeatable: action.repeatable ?? false,
    type: 'request_tool',
    label: action.label,
    description: action.description,
    payload: {
      operationId: action.operationId,
      arguments: argumentsValue,
      message: action.description ?? action.label,
    },
  })
}

function actionNeedsValidForm(action: InteractionCardAction) {
  return action.type === 'tool' || (action.type === 'send_message' && /\{\{[\w-]{1,64}\}\}/.test(action.message))
}

function renderMessageTemplate(message: string, values: FormValues) {
  return message.replace(/\{\{([\w-]{1,64})\}\}/g, (_, fieldId: string) => formatMessageValue(values[fieldId]))
}

function formatMessageValue(value: unknown) {
  if (Array.isArray(value))
    return value.map(item => typeof item === 'string' ? item : JSON.stringify(item)).join('、')
  if (typeof value === 'boolean')
    return value ? 'true' : 'false'
  if (value === undefined || value === null)
    return ''
  return String(value)
}

function bindArguments(action: Extract<InteractionCardAction, { type: 'tool' }>, cardId: string, values: FormValues) {
  const result: Record<string, unknown> = {}
  for (const binding of action.bindings) {
    const value = binding.value.type === 'field'
      ? values[binding.value.fieldId]
      : binding.value.type === 'card'
        ? cardId
        : binding.value.value
    if (value === undefined || value === '')
      continue
    setJsonPointer(result, binding.target, value)
  }
  return result
}

function setJsonPointer(target: Record<string, unknown>, pointer: string, value: unknown) {
  const parts = pointer.split('/').slice(1).map(part => part.replaceAll('~1', '/').replaceAll('~0', '~'))
  if (parts.length === 0 || parts.some(isUnsafeJsonPointerPart))
    throw new Error('ai.card_binding_invalid')

  let current: Record<string, unknown> | unknown[] = target
  parts.forEach((part, index) => {
    if (index === parts.length - 1) {
      setJsonPointerPart(current, part, value)
      return
    }
    const nextPart = parts[index + 1]!
    const expectedArray = isJsonPointerArrayIndex(nextPart)
    const next = readJsonPointerPart(current, part)
    if (!next || typeof next !== 'object' || Array.isArray(next) !== expectedArray) {
      const container = expectedArray ? [] : {}
      setJsonPointerPart(current, part, container)
      current = container
      return
    }
    current = next as Record<string, unknown> | unknown[]
  })
}

function readJsonPointerPart(current: Record<string, unknown> | unknown[], part: string): unknown {
  return Array.isArray(current) ? current[jsonPointerArrayIndex(part)] : current[part]
}

function setJsonPointerPart(current: Record<string, unknown> | unknown[], part: string, value: unknown) {
  if (Array.isArray(current)) {
    current[jsonPointerArrayIndex(part)] = value
    return
  }
  current[part] = value
}

function jsonPointerArrayIndex(part: string): number {
  if (!isJsonPointerArrayIndex(part))
    throw new Error('ai.card_binding_invalid')
  const value = Number(part)
  if (!Number.isSafeInteger(value) || value > 999)
    throw new Error('ai.card_binding_invalid')
  return value
}

function isJsonPointerArrayIndex(part: string) {
  return /^(?:0|[1-9]\d*)$/.test(part)
}

function isUnsafeJsonPointerPart(part: string) {
  return part === '__proto__' || part === 'prototype' || part === 'constructor'
}

function buildFormSchema(fields: InteractionFormField[]) {
  return z.object(Object.fromEntries(fields.map(field => [field.id, fieldSchema(field)])))
}

function fieldSchema(field: InteractionFormField): z.ZodType {
  let schema: z.ZodType
  if (field.type === 'number') {
    let numberSchema = z.number()
    if (field.integer)
      numberSchema = numberSchema.int()
    if (field.min !== undefined)
      numberSchema = numberSchema.min(field.min)
    if (field.max !== undefined)
      numberSchema = numberSchema.max(field.max)
    schema = numberSchema
  }
  else if (field.type === 'boolean') {
    schema = z.boolean()
  }
  else if (field.type === 'multi_select') {
    let arraySchema = z.array(z.string())
    if (field.minItems !== undefined || field.required)
      arraySchema = arraySchema.min(field.minItems ?? 1)
    if (field.maxItems !== undefined)
      arraySchema = arraySchema.max(field.maxItems)
    schema = arraySchema
  }
  else if (field.type === 'key_value') {
    let entriesSchema = field.valueMode === 'secret'
      ? z.array(z.object({ key: z.string().trim().min(1), value: z.string().min(1) }))
      : z.array(z.object({ key: z.string().min(1), value: z.string() }))
    if (field.required)
      entriesSchema = entriesSchema.min(field.minItems ?? 1)
    else if (field.minItems !== undefined)
      entriesSchema = entriesSchema.min(field.minItems)
    if (field.maxItems !== undefined)
      entriesSchema = entriesSchema.max(field.maxItems)
    schema = entriesSchema
  }
  else if (field.type === 'secret') {
    let secretSchema = z.string()
    if (field.required && field.generation === 'disabled')
      secretSchema = secretSchema.min(1)
    schema = secretSchema
  }
  else {
    let stringSchema = z.string()
    if (field.required)
      stringSchema = stringSchema.min(1)
    if ('minLength' in field && field.minLength !== undefined)
      stringSchema = stringSchema.min(field.minLength)
    if ('maxLength' in field && field.maxLength !== undefined)
      stringSchema = stringSchema.max(field.maxLength)
    schema = stringSchema
  }
  return field.required && !(field.type === 'secret' && field.generation !== 'disabled') ? schema : schema.optional()
}

function defaultValues(fields: InteractionFormField[]): FormValues {
  return Object.fromEntries(fields.map((field) => {
    // Agent 校验是第一道边界；这里继续忽略修复前持久化或恶意载荷中的 Secret 默认值。
    if (field.type === 'secret')
      return [field.id, '']
    if (field.type === 'key_value' && field.valueMode === 'secret')
      return [field.id, []]
    if ('defaultValue' in field && field.defaultValue !== undefined)
      return [field.id, field.defaultValue]
    if (field.type === 'boolean')
      return [field.id, false]
    if (field.type === 'multi_select' || field.type === 'key_value')
      return [field.id, []]
    return [field.id, '']
  }))
}

function getToolFormValues(fields: InteractionFormField[], values: FormValues): FormValues {
  return Object.fromEntries(fields.flatMap((field) => {
    const value = values[field.id]
    if (field.type === 'secret')
      return typeof value === 'string' && value.length > 0 ? [[field.id, value]] : []
    if (field.type === 'key_value' && field.valueMode === 'secret') {
      const entries = Array.isArray(value)
        ? value.filter((entry): entry is { key: string, value: string } => Boolean(
            entry
            && typeof entry === 'object'
            && typeof entry.key === 'string'
            && entry.key.trim().length > 0
            && typeof entry.value === 'string'
            && entry.value.length > 0,
          ))
        : []
      return entries.length > 0 ? [[field.id, entries]] : []
    }
    return value === undefined ? [] : [[field.id, value]]
  }))
}

function getPublicFormValues(fields: InteractionFormField[], values: FormValues): FormValues {
  const sensitiveIds = new Set(fields.filter(field => field.type === 'secret' || (field.type === 'key_value' && field.valueMode === 'secret')).map(field => field.id))
  return Object.fromEntries(Object.entries(values).filter(([key]) => !sensitiveIds.has(key)))
}

function messageFormValues(fields: InteractionFormField[], values: FormValues): FormValues {
  const fieldsById = new Map(fields.map(field => [field.id, field]))
  return Object.fromEntries(Object.entries(values).map(([fieldId, value]) => {
    const field = fieldsById.get(fieldId)
    if (!field || (field.type !== 'select' && field.type !== 'multi_select') || field.submissionFormat !== 'label_value')
      return [fieldId, value]
    const formatOption = (optionValue: string) => {
      const option = field.options.find(candidate => candidate.value === optionValue)
      return option ? `${option.label} (${option.value})` : optionValue
    }
    return [fieldId, Array.isArray(value) ? value.map(item => formatOption(String(item))) : formatOption(String(value ?? ''))]
  }))
}

function isFieldVisible(field: InteractionFormField, values: FormValues) {
  if (!field.visibleWhen)
    return true
  const current = values[field.visibleWhen.fieldId]
  const expected = field.visibleWhen.value
  if (field.visibleWhen.operator === 'equals')
    return current === expected
  if (field.visibleWhen.operator === 'not_equals')
    return current !== expected
  if (field.visibleWhen.operator === 'contains')
    return Array.isArray(current) ? current.includes(expected) : String(current ?? '').includes(String(expected ?? ''))
  if (field.visibleWhen.operator === 'is_empty')
    return current === undefined || current === '' || (Array.isArray(current) && current.length === 0)
  return current !== undefined && current !== '' && (!Array.isArray(current) || current.length > 0)
}

function CardIcon({ category }: { category: string }) {
  if (category === 'database' || category === 'cache')
    return <Database className="size-4" />
  if (category === 'build' || category === 'deployment' || category === 'task')
    return <Activity className="size-4" />
  if (category === 'finding')
    return <AlertCircle className="size-4" />
  if (category === 'cluster')
    return <Boxes className="size-4" />
  return <Package className="size-4" />
}

function SourceTrustIcon({ trust }: { trust: 'platform' | 'official' | 'community' }) {
  if (trust === 'platform')
    return <ShieldCheck aria-hidden="true" className="size-3 shrink-0 text-primary-text" />
  if (trust === 'official')
    return <Globe2 aria-hidden="true" className="size-3 shrink-0 text-info" />
  return <Users aria-hidden="true" className="size-3 shrink-0" />
}

function MetricTrendIcon({ trend }: { trend?: 'up' | 'down' | 'flat' }) {
  if (trend === 'up')
    return <TrendingUp aria-hidden="true" className="size-3 shrink-0" />
  if (trend === 'down')
    return <TrendingDown aria-hidden="true" className="size-3 shrink-0" />
  if (trend === 'flat')
    return <Minus aria-hidden="true" className="size-3 shrink-0" />
  return null
}

function StatusIcon({ status }: { status: 'pending' | 'running' | 'success' | 'warning' | 'error' | 'skipped' }) {
  const className = cn('mt-0.5 size-3.5 shrink-0', statusClasses[status])
  if (status === 'running')
    return <LoaderCircle className={cn(className, 'animate-spin motion-reduce:animate-none')} />
  if (status === 'success')
    return <Check className={className} />
  if (status === 'error')
    return <AlertCircle className={className} />
  if (status === 'warning')
    return <CircleDot className={className} />
  if (status === 'skipped')
    return <Circle className={className} />
  return <CircleDashed className={className} />
}

function CopyButton({ value }: { value: string }) {
  const { t } = useTranslation()
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
    }
    catch {
      toast.error(t('common.copyFailed'))
    }
  }
  return <Button aria-label={t('common.copy')} className="ml-1 size-6" size="icon" type="button" variant="ghost" onClick={() => void copy()}><Copy className="size-3" /></Button>
}
