import type { Control } from 'react-hook-form'
import type { InteractionCard, InteractionCardAction, InteractionContentBlock, InteractionFormField } from './interaction-card-schema'
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
  LoaderCircle,
  Package,
  Play,
  Plus,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { interactionCardGroupSchema } from './interaction-card-schema'
import { AIMarkdown } from './markdown'

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
  return (
    <section className="grid min-w-0 gap-2.5" data-ai-card-group={group.template}>
      <header className="px-0.5">
        <p className="text-[10px] font-medium uppercase tracking-wide text-primary-text">{t(`aiAssistant.cards.templates.${group.template}`)}</p>
        <h3 className="mt-0.5 text-sm font-semibold">{group.title}</h3>
        {group.description && <p className="mt-0.5 text-[11px] leading-4 text-muted-foreground">{group.description}</p>}
      </header>
      <div className={cn('grid min-w-0 gap-2', group.template === 'catalog' && 'min-[560px]:grid-cols-2')}>
        {group.cards.map(card => <InteractionCardView key={card.id} card={card} onAction={onAction} />)}
      </div>
      {group.groupActions && group.groupActions.length > 0 && (
        <div className="flex flex-wrap justify-end gap-1.5">
          {group.groupActions.map(action => <CardActionButton key={action.id} action={action} cardId="group" values={{}} onAction={onAction} />)}
        </div>
      )}
    </section>
  )
}

function InteractionCardView({ card, onAction }: { card: InteractionCard, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(Boolean(card.form || card.blocks?.some(block => block.defaultExpanded)))
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
    <article className="min-w-0 overflow-hidden rounded-container bg-surface" data-ai-card={card.presentation.variant}>
      <div className="flex min-w-0 items-start gap-2.5 p-2.5">
        <span className="grid size-9 shrink-0 place-items-center rounded-control bg-primary-subtle text-primary-text">
          <CardIcon category={card.presentation.icon?.type === 'category' ? card.presentation.icon.name : card.presentation.variant} />
        </span>
        <div className="min-w-0 flex-1">
          <h4 className="truncate text-xs font-semibold">{card.presentation.title}</h4>
          {card.presentation.subtitle && <p className="truncate text-[10px] text-muted-foreground">{card.presentation.subtitle}</p>}
          {card.presentation.description && <p className="mt-1 text-[11px] leading-4 text-muted-foreground">{card.presentation.description}</p>}
          {card.presentation.badges && card.presentation.badges.length > 0 && (
            <div className="mt-1.5 flex flex-wrap gap-1">
              {card.presentation.badges.map(badge => <Badge key={`${badge.label}-${badge.tone}`} className="px-1.5 py-0 text-[9px]" variant="outline">{badge.label}</Badge>)}
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
          className="grid min-w-0 gap-3 border-t border-separator-subtle p-2.5"
          onSubmit={form.handleSubmit(async (values) => {
            if (primaryAction)
              await executeCardAction(primaryAction, card.id, publicFormValues(fields, values), onAction)
          })}
        >
          {card.blocks?.map(block => <ContentBlock key={block.id} block={block} onAction={onAction} />)}
          {card.form?.sections.map(section => (
            <fieldset key={section.id} className="grid min-w-0 gap-2">
              {section.title && <legend className="text-[11px] font-semibold">{section.title}</legend>}
              {section.description && <p className="text-[10px] leading-4 text-muted-foreground">{section.description}</p>}
              {section.fields.filter(field => isFieldVisible(field, watchedValues)).map(field => (
                <DynamicField key={field.id} control={form.control} field={field} error={form.formState.errors[field.id]?.message as string | undefined} />
              ))}
            </fieldset>
          ))}
          {(primaryAction || secondaryActions.length > 0) && (
            <div className="flex flex-wrap justify-end gap-1.5 pt-0.5">
              {secondaryActions.map(action => <CardActionButton key={action.id} action={action} cardId={card.id} disabled={actionNeedsValidForm(action) && !form.formState.isValid} values={publicFormValues(fields, form.getValues())} onAction={onAction} />)}
              {primaryAction && (
                <Button disabled={form.formState.isSubmitting || (actionNeedsValidForm(primaryAction) && !form.formState.isValid)} size="sm" type={primaryAction.type === 'tool' ? 'submit' : 'button'} variant="default" onClick={primaryAction.type === 'tool' ? undefined : () => void executeCardAction(primaryAction, card.id, publicFormValues(fields, form.getValues()), onAction)}>
                  {form.formState.isSubmitting ? <LoaderCircle className="animate-spin motion-reduce:animate-none" /> : <Play />}
                  {primaryAction.label}
                </Button>
              )}
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
              <dt className="text-muted-foreground">{item.label}</dt>
              <dd className={cn('min-w-0 break-words font-medium', item.format === 'code' && 'font-mono text-[10px]')}>
                {item.value}
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
            <div key={`${item.label}-${item.value}`} className="rounded-control bg-surface-inset p-2">
              <p className="text-[9px] text-muted-foreground">{item.label}</p>
              <strong className="mt-0.5 block text-xs">{item.value}</strong>
              {item.change && <span className="text-[9px] text-muted-foreground">{item.change}</span>}
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
                <p className="text-[11px] font-medium">{item.primary}</p>
                {item.secondary && <p className="text-[10px] leading-4 text-muted-foreground">{item.secondary}</p>}
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
                <p className="font-medium">{item.label}</p>
                {item.detail && <p className="text-[10px] leading-4 text-muted-foreground">{item.detail}</p>}
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
            <thead className="bg-surface-inset"><tr>{block.columns.map(column => <th key={column.key} className="whitespace-nowrap px-2 py-1.5 font-medium">{column.label}</th>)}</tr></thead>
            <tbody>{block.rows.map(row => <tr key={row.id} className="border-t border-separator-subtle">{block.columns.map(column => <td key={column.key} className={cn('max-w-52 px-2 py-1.5 [overflow-wrap:anywhere]', column.format === 'code' && 'font-mono')}>{row.cells[column.key] ?? '—'}</td>)}</tr>)}</tbody>
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
      const maximum = Math.max(1, ...block.series.flatMap(series => series.values.map(value => Math.abs(value))))
      return (
        <div className="grid gap-2" role="img" aria-label={block.title ?? t('aiAssistant.cards.chart')}>
          {block.series.map(series => (
            <div key={series.name} className="grid gap-1">
              <div className="flex justify-between gap-2 text-[9px] text-muted-foreground">
                <span>{series.name}</span>
                <span>{series.unit}</span>
              </div>
              <div className="flex h-14 items-end gap-0.5 rounded-control bg-surface-inset p-1">
                {chartPoints(series.values, block.xAxis).map(point => (
                  <span
                    key={point.id}
                    aria-label={`${point.label}: ${point.value}${series.unit ?? ''}`}
                    className="min-w-1 flex-1 rounded-sm bg-primary/70"
                    style={{ height: `${Math.max(3, Math.abs(point.value) / maximum * 100)}%` }}
                    title={`${point.label}: ${point.value}${series.unit ?? ''}`}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )
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
            <Button key={`${link.label}-${link.routeName}`} size="sm" variant="outline" onClick={() => void onAction({ version: 1, type: 'navigate', label: link.label, payload: { routeName: link.routeName!, params: link.routeParams ?? {}, query: {} } })}>
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
    return content
  if (block.collapsible) {
    return (
      <details open={block.defaultExpanded}>
        <summary className="cursor-pointer text-[11px] font-semibold">{block.title}</summary>
        <div className="mt-1.5">{content}</div>
      </details>
    )
  }
  return (
    <section className="grid gap-1.5">
      <h5 className="text-[11px] font-semibold">{block.title}</h5>
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
            {field.label}
            {field.required && <span className="text-primary"> *</span>}
          </Label>
          {field.description && <p className="text-[9px] leading-3.5 text-muted-foreground">{field.description}</p>}
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
                  ? (
                      <NativeSelect id={`ai-card-field-${field.id}`} value={String(input.value ?? '')} onChange={input.onChange}>
                        <option value="">{field.placeholder ?? t('aiAssistant.cards.selectPlaceholder')}</option>
                        {field.options.map(option => <option key={option.value} disabled={option.disabled} value={option.value}>{option.label}</option>)}
                      </NativeSelect>
                    )
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
      <Button className="justify-start" size="sm" type="button" variant="outline" onClick={add}>
        <Plus />
        {t('aiAssistant.cards.addEntry')}
      </Button>
    </div>
  )
}

function CardActionButton({ action, cardId, values, disabled = false, onAction }: { action: InteractionCardAction, cardId: string, values: FormValues, disabled?: boolean, onAction: (action: AIUIAction) => Promise<boolean> }) {
  const [pending, setPending] = useState(false)
  const [done, setDone] = useState(false)
  const run = async () => {
    if (pending || (done && !action.repeatable))
      return
    setPending(true)
    try {
      const success = await executeCardAction(action, cardId, values, onAction)
      if (success && !action.repeatable)
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
    <Button disabled={disabled || pending || done} size="sm" type="button" variant={action.emphasis === 'primary' ? 'default' : 'outline'} onClick={() => void run()}>
      {pending ? <LoaderCircle className="animate-spin motion-reduce:animate-none" /> : done ? <Check /> : action.type === 'navigate' ? <ExternalLink /> : <ChevronRight />}
      {action.label}
    </Button>
  )
}

async function executeCardAction(action: InteractionCardAction, cardId: string, values: FormValues, onAction: (action: AIUIAction) => Promise<boolean>) {
  if (action.type === 'navigate')
    return onAction({ version: 1, id: action.id, repeatable: action.repeatable ?? true, type: 'navigate', label: action.label, description: action.description, payload: { routeName: action.routeName, params: action.routeParams ?? {}, query: {} } })
  if (action.type === 'send_message')
    return onAction({ version: 1, id: action.id, repeatable: false, type: 'send_message', label: action.label, description: action.description, payload: { message: renderMessageTemplate(action.message, values) } })
  const argumentsValue = bindArguments(action, cardId, values)
  return onAction({
    version: 1,
    id: action.id,
    repeatable: false,
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

function chartPoints(values: number[], labels?: string[]) {
  const occurrences = new Map<string, number>()
  return values.map((value, index) => {
    const label = labels?.[index] ?? String(index + 1)
    const signature = `${label}:${value}`
    const occurrence = (occurrences.get(signature) ?? 0) + 1
    occurrences.set(signature, occurrence)
    return { id: `${signature}:${occurrence}`, label, value }
  })
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
