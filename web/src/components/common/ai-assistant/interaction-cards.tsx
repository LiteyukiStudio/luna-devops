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
import { useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { InteractionCardChart } from './interaction-card-chart'
import { interactionCardGroupSchema } from './interaction-card-schema'
import { interactionCardDensity, interactionCardTemplateConfigs, shouldExpandInteractionCard } from './interaction-card-templates'
import { AIInlineMarkdown, AIMarkdown } from './markdown'

const compactActionClassName = 'h-auto min-h-7 max-w-full gap-1.5 whitespace-normal px-2.5 py-1 !text-[11px] leading-4 [&_svg]:size-3.5'

interface AIInteractionCardsProps {
  arguments: Record<string, unknown>
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
  const parsed = useMemo(() => interactionCardGroupSchema.safeParse(rawArguments), [rawArguments])
  if (!parsed.success) {
    return (
      <div className="rounded-container bg-danger-subtle px-3 py-2 text-xs text-danger" role="alert">
        {t('aiAssistant.cards.invalid')}
      </div>
    )
  }
  const group = parsed.data
  const density = interactionCardDensity(group)
  const templateConfig = interactionCardTemplateConfigs[group.template]
  return (
    <section className={cn('grid min-w-0 grid-cols-[minmax(0,1fr)]', density === 'compact' ? 'gap-2' : 'gap-2.5')} data-ai-card-density={density} data-ai-card-group={group.template} data-ai-card-mode={group.mode}>
      <header className="px-0.5">
        <p className="text-[10px] font-medium uppercase tracking-wide text-primary-text">{t(`aiAssistant.cards.templates.${group.template}`)}</p>
        <h3 className="mt-0.5 text-[13px] font-semibold leading-5"><AIInlineMarkdown>{group.title}</AIInlineMarkdown></h3>
        {group.description && <AIMarkdown className="mt-0.5 text-[11px] leading-4 text-muted-foreground">{group.description}</AIMarkdown>}
      </header>
      <div className={cn('grid min-w-0', density === 'compact' ? 'gap-1.5' : 'gap-2', templateConfig.gridClassName)}>
        {group.cards.map(card => <InteractionCardView key={card.id} card={card} density={density} group={group} onAction={onAction} />)}
      </div>
      {group.groupActions && group.groupActions.length > 0 && (
        <div className="flex flex-wrap justify-end gap-1.5">
          {group.groupActions.map(action => <CardActionButton key={action.id} action={action} cardId="group" values={{}} onAction={onAction} />)}
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
  const hasDetails = Boolean(card.blocks?.length || card.form)
  const primaryAction = card.actions?.find(action => action.emphasis === 'primary') ?? card.actions?.[0]
  const secondaryActions = card.actions?.filter(action => action !== primaryAction) ?? []

  return (
    <article className="min-w-0 overflow-hidden rounded-container bg-surface" data-ai-card={card.presentation.variant} data-ai-card-template={group.template}>
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
              {card.presentation.badges.map(badge => (
                <StatusBadge key={`${badge.label}-${badge.tone}`} className="px-1.5 py-0 text-[9px]" tone={badge.tone === 'error' ? 'danger' : badge.tone}>
                  {badge.label}
                </StatusBadge>
              ))}
            </div>
          )}
          {card.sourceRefs && card.sourceRefs.length > 0 && (
            <div className="mt-1.5 flex min-w-0 flex-wrap gap-1" data-ai-card-sources>
              {card.sourceRefs.slice(0, 4).map(source => (
                <span key={`${source.type}-${source.refId}`} className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-full bg-surface-inset px-1.5 py-0.5 text-[9px] text-muted-foreground" title={source.label}>
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
          {card.blocks?.map(block => <ContentBlock key={block.id} block={block} onAction={onAction} />)}
          {card.form?.sections.map(section => (
            <fieldset key={section.id} className="grid min-w-0 gap-2">
              {section.title && <legend className="text-[11px] font-semibold"><AIInlineMarkdown>{section.title}</AIInlineMarkdown></legend>}
              {section.description && <AIMarkdown className="text-[10px] leading-4 text-muted-foreground">{section.description}</AIMarkdown>}
              {section.fields.filter(field => isFieldVisible(field, watchedValues)).map(field => (
                <DynamicField key={field.id} control={form.control} field={field} error={form.formState.errors[field.id]?.message as string | undefined} />
              ))}
            </fieldset>
          ))}
          {(primaryAction || secondaryActions.length > 0) && (
            <div className="flex flex-wrap justify-end gap-1.5 pt-0.5">
              {secondaryActions.map(action => <CardActionButton key={action.id} action={action} cardId={card.id} disabled={actionNeedsValidForm(action) && !form.formState.isValid} values={publicFormValues(fields, form.getValues())} onAction={onAction} />)}
              {primaryAction && <CardActionButton action={primaryAction} cardId={card.id} disabled={actionNeedsValidForm(primaryAction) && !form.formState.isValid} values={publicFormValues(fields, form.getValues())} onAction={onAction} />}
            </div>
          )}
        </form>
      )}
      {!expanded && primaryAction && !card.form && (
        <div className="flex justify-end border-t border-separator-subtle px-2.5 py-2">
          <CardActionButton action={primaryAction} cardId={card.id} values={{}} onAction={onAction} />
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
          {block.items.map(item => (
            <div key={`${item.label}-${item.value}`} className="grid grid-cols-[minmax(4.5rem,35%)_minmax(0,1fr)] gap-2">
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
          {block.items.map(item => (
            <div
              key={`${item.label}-${item.value}`}
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
          {block.items.map(item => (
            <div key={item.id} className="flex gap-2 py-1.5">
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
          {block.items.map(item => (
            <div key={item.id} className="flex items-start gap-2 text-[11px]">
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
            <thead className="bg-surface-inset"><tr>{block.columns.map(column => <th key={column.key} className="whitespace-nowrap px-2 py-1.5 font-medium"><AIInlineMarkdown>{column.label}</AIInlineMarkdown></th>)}</tr></thead>
            <tbody>{block.rows.map(row => <tr key={row.id} className="border-t border-separator-subtle">{block.columns.map(column => <td key={column.key} className={cn('max-w-52 px-2 py-1.5 [overflow-wrap:anywhere]', column.format === 'code' && 'font-mono')}><AIInlineMarkdown>{row.cells[column.key] ?? '—'}</AIInlineMarkdown></td>)}</tr>)}</tbody>
          </table>
        </div>
      )
    }
    if (block.type === 'code' || block.type === 'diff')
      return <pre className="max-w-full overflow-x-auto rounded-control bg-surface-inset p-2 font-mono text-[10px] leading-4"><code>{block.type === 'code' ? block.content : block.unifiedDiff}</code></pre>
    if (block.type === 'timeline') {
      return (
        <div className="grid gap-2 border-l border-separator-strong pl-2.5">
          {block.items.map(item => (
            <div key={item.id} className="relative text-[11px] before:absolute before:-left-[13px] before:top-1 before:size-1.5 before:rounded-full before:bg-primary">
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
          {block.edges.map(edge => (
            <div key={`${edge.source}-${edge.target}-${edge.label ?? ''}`} className="flex min-w-0 items-center gap-1.5 rounded-control bg-surface-inset px-2 py-1.5 text-[10px]">
              <span className="truncate font-medium">{nodes.get(edge.source)?.label ?? edge.source}</span>
              <ChevronRight className="size-3 shrink-0 text-muted-foreground" />
              <span className="truncate font-medium">{nodes.get(edge.target)?.label ?? edge.target}</span>
              {edge.label && <span className="ml-auto shrink-0 text-[9px] text-muted-foreground">{edge.label}</span>}
            </div>
          ))}
          {block.edges.length === 0 && block.nodes.map(node => <div key={node.id} className="rounded-control bg-surface-inset px-2 py-1.5 text-[10px] font-medium">{node.label}</div>)}
        </div>
      )
    }
    if (block.type === 'progress') {
      return (
        <div className="grid gap-1">
          <div className="flex justify-between gap-2 text-[10px]">
            <span>{block.label}</span>
            {block.mode === 'determinate' && <strong>{`${block.value ?? 0}%`}</strong>}
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-surface-inset"><div className={cn('h-full rounded-full bg-primary transition-[width]', block.mode === 'indeterminate' && 'w-1/3 animate-pulse')} style={block.mode === 'determinate' ? { width: `${block.value ?? 0}%` } : undefined} /></div>
          {block.detail && <p className="text-[9px] text-muted-foreground">{block.detail}</p>}
        </div>
      )
    }
    if (block.type === 'resource_links') {
      return (
        <div className="flex flex-wrap gap-1.5">
          {block.links.filter(link => link.routeName).map(link => (
            <Button key={`${link.label}-${link.routeName}`} className={compactActionClassName} size="sm" variant="outline" onClick={() => void onAction({ version: 1, type: 'navigate', label: link.label, payload: { routeName: link.routeName!, params: link.routeParams ?? {}, query: {} } })}>
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
  return (
    <Controller
      control={control}
      name={field.id}
      render={({ field: input }) => (
        <div className="grid gap-1">
          <Label className="text-[10px]" htmlFor={`ai-card-field-${field.id}`}>
            <AIInlineMarkdown>{field.label}</AIInlineMarkdown>
            {field.required && <span className="text-primary"> *</span>}
          </Label>
          {field.description && <AIMarkdown className="text-[9px] leading-3.5 text-muted-foreground">{field.description}</AIMarkdown>}
          {field.type === 'textarea'
            ? <Textarea {...input} id={`ai-card-field-${field.id}`} rows={field.rows ?? 3} value={String(input.value ?? '')} />
            : field.type === 'number'
              ? <Input id={`ai-card-field-${field.id}`} max={field.max} min={field.min} step={field.step} type="number" value={input.value === undefined ? '' : String(input.value)} onChange={event => input.onChange(event.target.value === '' ? undefined : Number(event.target.value))} />
              : field.type === 'boolean'
                ? (
                    <div className="flex items-center gap-2">
                      <Checkbox checked={Boolean(input.value)} id={`ai-card-field-${field.id}`} onCheckedChange={value => input.onChange(value === true)} />
                      <span className="text-[10px] text-muted-foreground">{input.value ? t('common.enabled') : t('common.disabled')}</span>
                    </div>
                  )
                : field.type === 'select'
                  ? <SelectField field={field} value={String(input.value ?? '')} onChange={input.onChange} />
                  : field.type === 'multi_select'
                    ? (
                        <div className="grid gap-1.5">
                          {field.options.map(option => (
                            <label key={option.value} className="flex items-start gap-2 text-[10px]">
                              <Checkbox
                                checked={Array.isArray(input.value) && input.value.includes(option.value)}
                                disabled={option.disabled}
                                onCheckedChange={(checked) => {
                                  const current = Array.isArray(input.value) ? input.value as string[] : []
                                  input.onChange(checked ? [...current, option.value] : current.filter(value => value !== option.value))
                                }}
                              />
                              <span>
                                {option.label}
                                {option.description && <span className="block text-[9px] text-muted-foreground">{option.description}</span>}
                              </span>
                            </label>
                          ))}
                        </div>
                      )
                    : field.type === 'key_value'
                      ? field.valueMode === 'secret'
                        ? <div className="rounded-control bg-info-subtle px-2.5 py-2 text-[10px] text-info">{t('aiAssistant.cards.secretManualUnavailable')}</div>
                        : <KeyValueInput value={Array.isArray(input.value) ? input.value as Array<{ key: string, value: string }> : []} secret={false} onChange={input.onChange} />
                      : field.type === 'secret'
                        ? <div className="rounded-control bg-info-subtle px-2.5 py-2 text-[10px] text-info">{t(field.generation === 'disabled' ? 'aiAssistant.cards.secretManualUnavailable' : 'aiAssistant.cards.secretGenerated')}</div>
                        : <Input {...input} id={`ai-card-field-${field.id}`} placeholder={field.placeholder} type="text" value={String(input.value ?? '')} />}
          {error && <p className="text-[9px] text-danger" role="alert">{error}</p>}
        </div>
      )}
    />
  )
}

function SelectField({ field, value, onChange }: {
  field: Extract<InteractionFormField, { type: 'select' }>
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  if (!field.display || field.display === 'select') {
    return (
      <NativeSelect id={`ai-card-field-${field.id}`} value={value} onChange={event => onChange(event.target.value)}>
        <option value="">{field.placeholder ?? t('aiAssistant.cards.selectPlaceholder')}</option>
        {field.options.map(option => <option key={option.value} disabled={option.disabled} value={option.value}>{option.label}</option>)}
      </NativeSelect>
    )
  }
  const segmented = field.display === 'segmented'
  return (
    <RadioGroup
      aria-label={field.label}
      className={cn(segmented ? 'flex flex-wrap gap-1' : 'gap-1.5')}
      value={value}
      onValueChange={onChange}
    >
      {field.options.map(option => (
        <Label
          key={option.value}
          className={cn(
            'flex min-w-0 cursor-pointer items-start gap-2 rounded-control text-[10px]',
            segmented
              ? 'border border-separator-subtle px-2 py-1.5 data-[selected=true]:border-primary-border data-[selected=true]:bg-primary-subtle data-[selected=true]:text-primary-text'
              : 'px-1 py-0.5',
            option.disabled && 'cursor-not-allowed opacity-50',
          )}
          data-selected={value === option.value}
          htmlFor={`ai-card-field-${field.id}-${option.value}`}
        >
          <RadioGroupItem
            className={cn('mt-0.5', segmented && 'sr-only')}
            disabled={option.disabled}
            id={`ai-card-field-${field.id}-${option.value}`}
            value={option.value}
          />
          <span className="min-w-0">
            <span className="block [overflow-wrap:anywhere]">{option.label}</span>
            {option.description && <span className="block text-[9px] leading-3.5 text-muted-foreground [overflow-wrap:anywhere]">{option.description}</span>}
          </span>
        </Label>
      ))}
    </RadioGroup>
  )
}

function KeyValueInput({ value, secret, onChange }: { value: Array<{ key: string, value: string }>, secret: boolean, onChange: (value: Array<{ key: string, value: string }>) => void }) {
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
    <div className="grid gap-1.5">
      {value.map((entry, index) => (
        <div key={rowIds[index]} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-1">
          <Input aria-label={t('aiAssistant.cards.key')} value={entry.key} onChange={event => onChange(value.map((item, itemIndex) => itemIndex === index ? { ...item, key: event.target.value } : item))} />
          <Input aria-label={t('aiAssistant.cards.value')} type={secret ? 'password' : 'text'} value={entry.value} onChange={event => onChange(value.map((item, itemIndex) => itemIndex === index ? { ...item, value: event.target.value } : item))} />
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

function CardActionButton({ action, cardId, values, disabled = false, onAction }: { action: InteractionCardAction, cardId: string, values: FormValues, disabled?: boolean, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const [pending, setPending] = useState(false)
  const [done, setDone] = useState(false)
  const repeatable = action.repeatable ?? action.type === 'navigate'
  const run = async () => {
    if (pending || (done && !repeatable))
      return
    setPending(true)
    try {
      const success = await executeCardAction(action, cardId, values, onAction)
      if (success && !repeatable)
        setDone(true)
      if (!success)
        throw new Error('ai.card_action_failed')
    }
    catch {
      toast.error(action.label)
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

async function executeCardAction(action: InteractionCardAction, cardId: string, values: FormValues, onAction: (action: AIUIAction) => Promise<boolean>) {
  if (action.type === 'navigate')
    return onAction({ version: 1, id: action.id, repeatable: action.repeatable ?? true, type: 'navigate', label: action.label, description: action.description, payload: { routeName: action.routeName, params: action.routeParams ?? {}, query: {} } })
  if (action.type === 'send_message')
    return onAction({ version: 1, id: action.id, repeatable: action.repeatable ?? false, type: 'send_message', label: action.label, description: action.description, payload: { message: renderMessageTemplate(action.message, values) } })
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
  let current = target
  parts.forEach((part, index) => {
    if (index === parts.length - 1) {
      current[part] = value
      return
    }
    const next = current[part]
    if (!next || typeof next !== 'object' || Array.isArray(next))
      current[part] = {}
    current = current[part] as Record<string, unknown>
  })
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
      ? z.array(z.object({ key: z.string(), value: z.string() }))
      : z.array(z.object({ key: z.string().min(1), value: z.string() }))
    if (field.required)
      entriesSchema = entriesSchema.min(field.minItems ?? 1)
    else if (field.minItems !== undefined)
      entriesSchema = entriesSchema.min(field.minItems)
    if (field.maxItems !== undefined)
      entriesSchema = entriesSchema.max(field.maxItems)
    schema = entriesSchema
  }
  else if (field.type === 'secret' && field.generation === 'required') {
    schema = z.string().optional()
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
  return field.required && !(field.type === 'secret' && field.generation === 'required') ? schema : schema.optional()
}

function defaultValues(fields: InteractionFormField[]): FormValues {
  return Object.fromEntries(fields.map((field) => {
    if ('defaultValue' in field && field.defaultValue !== undefined)
      return [field.id, field.defaultValue]
    if (field.type === 'boolean')
      return [field.id, false]
    if (field.type === 'multi_select' || field.type === 'key_value')
      return [field.id, []]
    return [field.id, '']
  }))
}

function publicFormValues(fields: InteractionFormField[], values: FormValues): FormValues {
  const sensitiveIds = new Set(fields.filter(field => field.type === 'secret' || (field.type === 'key_value' && field.valueMode === 'secret')).map(field => field.id))
  return Object.fromEntries(Object.entries(values).filter(([key]) => !sensitiveIds.has(key)))
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
  return <Button aria-label={t('common.copy')} className="ml-1 size-6" size="icon" type="button" variant="ghost" onClick={() => void navigator.clipboard.writeText(value)}><Copy className="size-3" /></Button>
}
