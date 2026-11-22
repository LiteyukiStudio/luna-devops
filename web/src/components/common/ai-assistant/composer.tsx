import type { TFunction } from 'i18next'
import type { KeyboardEvent, RefObject } from 'react'
import type { WorkspaceContextVisualKind } from './workspace-context-display'
import type { AIApprovalMode, AIContextUsage, AIModelOption } from '@/api'
import {
  AppWindow,
  ArrowUp,
  Boxes,
  ChevronDown,
  FolderKanban,
  LayoutDashboard,
  LoaderCircle,
  Settings,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Square,
  Zap,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { UsageRing } from '@/components/common/usage-ring'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { isAIInputWithinLimit } from './input-size'

export interface AIAssistantComposerProps {
  activeRun: boolean
  approvalMode?: AIApprovalMode
  approvalModeChanging?: boolean
  canceling: boolean
  canCancel: boolean
  /** 当前页面语义类型，用于匹配顶部外框指示器图标。 */
  contextKind?: WorkspaceContextVisualKind
  /** 当前页面上下文提示，例如“应用：Demo / Web”（不含站点后缀）。 */
  contextLabel?: string
  models?: AIModelOption[]
  modelAvailable?: boolean
  modelChanging?: boolean
  modelSelectionDisabled?: boolean
  selectedModelId?: string
  /** 尚未发起过模型调用的新会话可以安全地展示为 0% 上下文用量。 */
  isNewConversation?: boolean
  /** 会话最近一次已确认的上下文大小；新 Run 启动时保持不变。 */
  contextUsage?: AIContextUsage
  draft: string
  inputRef: RefObject<HTMLTextAreaElement | null>
  maxInputBytes?: number
  sending: boolean
  submitting: boolean
  surface?: 'page' | 'window'
  waitingExternal?: boolean
  waitingInput: boolean
  onApprovalModeChange?: (mode: AIApprovalMode) => void
  onCancel: () => void
  onDraftChange: (value: string) => void
  onModelChange?: (modelId: string) => void
  onSubmit: () => void
}

function WorkspaceContextIcon({
  className,
  kind = 'page',
}: {
  className?: string
  kind?: WorkspaceContextVisualKind
}) {
  switch (kind) {
    case 'application':
      return <Boxes aria-hidden="true" className={className} />
    case 'project':
      return <FolderKanban aria-hidden="true" className={className} />
    case 'dashboard':
      return <LayoutDashboard aria-hidden="true" className={className} />
    case 'settings':
      return <Settings aria-hidden="true" className={className} />
    case 'page':
      return <AppWindow aria-hidden="true" className={className} />
  }
}

function getApprovalModeCatalog(t: TFunction) {
  return [
    {
      value: 'strict' as const,
      icon: ShieldCheck,
      label: t('aiAssistant.approvalMode.options.strict.label'),
      description: t('aiAssistant.approvalMode.options.strict.description'),
    },
    {
      value: 'default' as const,
      icon: Shield,
      label: t('aiAssistant.approvalMode.options.default.label'),
      description: t('aiAssistant.approvalMode.options.default.description'),
    },
    {
      value: 'auto' as const,
      icon: Zap,
      label: t('aiAssistant.approvalMode.options.auto.label'),
      description: t('aiAssistant.approvalMode.options.auto.description'),
    },
    {
      value: 'full_access' as const,
      icon: ShieldAlert,
      label: t('aiAssistant.approvalMode.options.full_access.label'),
      description: t('aiAssistant.approvalMode.options.full_access.description'),
    },
  ]
}

function isConfirmingIME(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
  return event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229
}

function formatTokenRatio(used: number, total: number): { used: string, total: string } {
  if (total < 1000)
    return { used: String(used), total: String(total) }

  const formatKValue = (value: number) => {
    const k = value / 1000
    return k >= 100 ? String(Math.round(k)) : k.toFixed(1).replace(/\.0$/, '')
  }
  return { used: formatKValue(used), total: `${formatKValue(total)}k` }
}

function ContextUsageRing({
  ratio,
  used,
  total,
}: {
  ratio: number
  used: number
  total: number
}) {
  const { t } = useTranslation()
  const percent = Math.round(Math.min(1, Math.max(0, ratio)) * 100)
  const contextTokens = formatTokenRatio(used, total)
  const contextLabel = t('aiAssistant.contextUsage', { ...contextTokens, percent })
  return (
    <UsageRing
      ariaLabel={contextLabel}
      ratio={ratio}
      tooltip={contextLabel}
    />
  )
}

function UnavailableContextUsageRing() {
  const { t } = useTranslation()
  const unavailableLabel = t('aiAssistant.contextUsageUnavailable')
  return (
    <UsageRing
      ariaLabel={unavailableLabel}
      ratio={0}
      tooltip={unavailableLabel}
    />
  )
}

export function AIAssistantComposer({
  activeRun,
  approvalMode = 'default',
  approvalModeChanging = false,
  canceling,
  canCancel,
  contextKind = 'page',
  contextLabel,
  models = [],
  modelAvailable = true,
  modelChanging = false,
  modelSelectionDisabled = false,
  selectedModelId,
  isNewConversation = false,
  contextUsage,
  draft,
  inputRef,
  maxInputBytes,
  sending,
  submitting,
  surface = 'window',
  waitingExternal = false,
  waitingInput,
  onApprovalModeChange,
  onCancel,
  onDraftChange,
  onModelChange,
  onSubmit,
}: AIAssistantComposerProps) {
  const { t } = useTranslation()
  const page = surface === 'page'
  const busy = sending || submitting
  const canSubmit = modelAvailable && (!activeRun || waitingInput)
  const normalizedDraft = draft.trim()
  const inputTooLarge = !isAIInputWithinLimit(normalizedDraft, maxInputBytes)
  const inputLimitLabel = maxInputBytes === undefined
    ? ''
    : maxInputBytes % 1024 === 0
      ? `${maxInputBytes / 1024} KiB`
      : `${maxInputBytes} B`
  const submitEnabled = Boolean(normalizedDraft) && !busy && canSubmit && !inputTooLarge
  const selectedModel = models.find(model => model.id === selectedModelId)
  const approvalModes = getApprovalModeCatalog(t)
  const selectedApprovalOption = approvalModes.find(option => option.value === approvalMode) ?? approvalModes[1]!
  const ApprovalIcon = selectedApprovalOption.icon
  const isFullAccess = approvalMode === 'full_access'
  const isAutoMode = approvalMode === 'auto'
  const hasReportedUsage = contextUsage?.status === 'reported'
    && contextUsage.modelId === selectedModel?.id
    && typeof contextUsage.usedTokens === 'number'
    && typeof contextUsage.maxContextTokensSnapshot === 'number'
    && contextUsage.maxContextTokensSnapshot > 0
  const contextTotal = hasReportedUsage ? contextUsage.maxContextTokensSnapshot : 0
  const contextUsed = hasReportedUsage ? contextUsage.usedTokens : 0
  const contextRatio = hasReportedUsage ? contextUsed / contextTotal : 0
  const hasInitialContextUsage = isNewConversation
    && typeof selectedModel?.maxContextTokens === 'number'
    && selectedModel.maxContextTokens > 0

  const modelValue = (
    <>
      <span className="truncate">
        <SelectValue placeholder={t('aiAssistant.modelEmpty')} />
      </span>
      {modelChanging
        ? <LoaderCircle className="size-3 shrink-0 animate-spin motion-reduce:animate-none" />
        : <ChevronDown className="size-3 shrink-0 opacity-60" />}
    </>
  )

  const approvalModeValue = (
    <>
      <span className="sr-only"><SelectValue /></span>
      <ApprovalIcon
        aria-hidden="true"
        className={cn(
          'size-3.5 shrink-0',
          isFullAccess ? 'text-warning' : isAutoMode ? 'text-info' : 'text-muted-foreground',
        )}
      />
      <span className="truncate">{selectedApprovalOption.label}</span>
      {approvalModeChanging
        ? <LoaderCircle className="size-3 shrink-0 animate-spin motion-reduce:animate-none" />
        : <ChevronDown className="size-3 shrink-0 opacity-60" />}
    </>
  )

  return (
    <footer
      className={page
        ? 'relative z-10 shrink-0 bg-gradient-to-t from-surface via-surface/92 to-transparent pb-[max(0.75rem,env(safe-area-inset-bottom))] pl-[max(0.75rem,env(safe-area-inset-left))] pr-[max(0.75rem,env(safe-area-inset-right))] pt-1.5'
        : 'relative z-10 shrink-0 bg-gradient-to-t from-surface via-surface/92 to-transparent px-3 pb-2.5 pt-1.5'}
      data-ai-assistant-surface={surface}
    >
      <div className="mx-auto w-full max-w-3xl">
        {waitingExternal && (
          <div
            className={cn(
              'mb-1.5 flex items-center gap-2 rounded-control border border-info-border bg-info-subtle px-3 py-1.5 text-info',
              page ? 'text-xs sm:text-sm' : 'text-xs',
            )}
            data-ai-waiting-external
            role="status"
          >
            <LoaderCircle aria-hidden="true" className="size-3.5 shrink-0 animate-spin motion-reduce:animate-none" />
            <span className="min-w-0 flex-1 leading-5">{t('aiAssistant.waitingExternalStatus')}</span>
          </div>
        )}
        <div
          className="rounded-[18px] border border-border/75 bg-surface-subtle/85 p-1 shadow-xs backdrop-blur-sm dark:bg-surface-raised/65"
          data-ai-composer-shell
        >
          {contextLabel && (
            <div
              className="flex min-w-0 select-none items-center gap-1.5 px-2.5 py-1 text-xs text-muted-foreground"
              data-ai-composer-context
              title={contextLabel}
            >
              <WorkspaceContextIcon className="size-3.5 shrink-0 opacity-75" kind={contextKind} />
              <span className="min-w-0 truncate leading-4">{contextLabel}</span>
            </div>
          )}
          <div
            className={cn(
              'flex flex-col rounded-[14px] border border-border/80 bg-surface shadow-2xs transition-[border-color,box-shadow] duration-fast focus-within:border-primary-border focus-within:ring-2 focus-within:ring-primary/12',
              inputTooLarge && 'border-danger-border focus-within:border-danger-border focus-within:ring-danger/15',
              busy && 'bg-surface-subtle',
            )}
            data-ai-composer-state={inputTooLarge ? 'invalid' : busy ? 'sending' : waitingExternal ? 'waiting_external' : activeRun && !waitingInput ? 'running' : 'idle'}
          >
            <textarea
              ref={inputRef}
              aria-describedby={inputTooLarge ? 'ai-assistant-input-size-error' : undefined}
              aria-invalid={inputTooLarge}
              aria-label={t('aiAssistant.inputLabel')}
              className={page
                ? 'min-h-12 max-h-40 w-full resize-none overflow-y-auto bg-transparent px-3.5 pt-2.5 pb-1 !text-base leading-6 outline-none [field-sizing:content] placeholder:text-muted-foreground/80 disabled:cursor-not-allowed sm:!text-[14px]'
                : 'min-h-11 max-h-36 w-full resize-none overflow-y-auto bg-transparent px-3.5 pt-2.5 pb-1 !text-base leading-5.5 outline-none [field-sizing:content] placeholder:text-muted-foreground/80 disabled:cursor-not-allowed sm:!text-[13.5px]'}
              disabled={busy}
              placeholder={waitingInput ? t('aiAssistant.inputRequired') : waitingExternal ? t('aiAssistant.inputWaitingExternal') : activeRun ? t('aiAssistant.inputRunning') : t('aiAssistant.inputPlaceholder')}
              rows={1}
              value={draft}
              onChange={event => onDraftChange(event.target.value)}
              onKeyDown={(event) => {
                const submitShortcut = !event.shiftKey || event.metaKey || event.ctrlKey
                if (event.key === 'Enter' && submitShortcut && !isConfirmingIME(event) && submitEnabled) {
                  event.preventDefault()
                  onSubmit()
                }
              }}
            />
            {inputTooLarge && (
              <p
                id="ai-assistant-input-size-error"
                className={cn('px-3.5 pt-0.5 text-danger', page ? 'text-sm' : 'text-xs')}
                role="alert"
              >
                {t('aiAssistant.inputTooLarge', { limit: inputLimitLabel })}
              </p>
            )}
            <div className={cn('flex min-w-0 items-center justify-between gap-1.5', page ? 'px-1.5 pb-1 pointer-fine:px-2 pointer-fine:pb-1.5' : 'px-2 pb-1.5')}>
              <Select
                disabled={modelSelectionDisabled || modelChanging || models.length === 0}
                value={selectedModelId ?? ''}
                onValueChange={onModelChange}
              >
                <SelectTrigger
                  aria-label={t('aiAssistant.modelLabel')}
                  className={page
                    ? 'group/model !h-11 w-auto min-w-0 max-w-[44%] shrink rounded-md border-0 bg-transparent p-0 shadow-none hover:bg-transparent focus-visible:ring-2 focus-visible:ring-ring pointer-fine:!h-7 dark:bg-transparent dark:hover:bg-transparent [&>svg]:hidden'
                    : '!h-7 w-auto min-w-0 max-w-[44%] shrink gap-1 rounded-md border-0 bg-transparent px-2 text-xs font-normal text-foreground/80 shadow-none hover:bg-muted/70 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring dark:bg-transparent dark:hover:bg-muted/70 [&>svg]:hidden'}
                >
                  {page
                    ? (
                        <span className="flex h-7 min-w-0 max-w-full items-center gap-1 rounded-md px-2.5 text-xs font-normal text-foreground/80 transition-colors group-hover/model:bg-muted/70 group-hover/model:text-foreground" data-ai-composer-model-visual>
                          {modelValue}
                        </span>
                      )
                    : modelValue}
                </SelectTrigger>
                <SelectContent
                  align="start"
                  className="max-h-72 min-w-48 max-w-72 rounded-xl border border-border/80 bg-popover p-1 shadow-overlay [&>[data-radix-select-viewport]]:h-auto [&>[data-radix-select-viewport]]:p-0"
                  position="popper"
                  side="top"
                  sideOffset={6}
                >
                  <SelectGroup>
                    <SelectLabel className="px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                      {t('aiAssistant.modelLabel')}
                    </SelectLabel>
                    {models.map(model => (
                      <SelectItem
                        key={model.id}
                        className={cn(
                          'min-h-7 rounded-lg py-1.5 pr-7 pl-2.5 text-xs font-normal [&_[data-slot=select-item-indicator]]:right-2 [&_svg]:size-3.5',
                          page && 'min-h-10 pointer-fine:min-h-7',
                        )}
                        value={model.id}
                      >
                        <span className="truncate">{model.name}</span>
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>

              <div className="ml-auto flex min-w-0 shrink-0 items-center gap-1">
                <Select
                  disabled={approvalModeChanging}
                  value={approvalMode}
                  onValueChange={value => onApprovalModeChange?.(value as AIApprovalMode)}
                >
                  <SelectTrigger
                    aria-label={t('aiAssistant.approvalMode.label')}
                    className={cn(
                      page
                        ? 'group/approval !h-11 w-auto min-w-0 shrink rounded-md border-0 bg-transparent p-0 shadow-none hover:bg-transparent focus-visible:ring-2 focus-visible:ring-ring pointer-fine:!h-7 dark:bg-transparent dark:hover:bg-transparent [&>svg]:hidden'
                        : '!h-7 w-auto min-w-0 shrink gap-1 rounded-md border-0 bg-transparent px-2 text-xs shadow-none focus-visible:ring-2 focus-visible:ring-ring dark:bg-transparent [&>svg]:hidden',
                      !page && (isFullAccess
                        ? 'font-medium text-warning hover:bg-warning-subtle/60 dark:hover:bg-warning-subtle/60'
                        : isAutoMode
                          ? 'font-medium text-info hover:bg-info-subtle/60 dark:hover:bg-info-subtle/60'
                          : 'font-normal text-muted-foreground hover:bg-muted/70 hover:text-foreground dark:hover:bg-muted/70'),
                    )}
                    data-ai-approval-mode={approvalMode}
                  >
                    {page
                      ? (
                          <span
                            className={cn(
                              'flex h-7 min-w-0 max-w-full items-center gap-1 rounded-md px-2 text-xs transition-colors',
                              isFullAccess
                                ? 'font-medium text-warning group-hover/approval:bg-warning-subtle/60'
                                : isAutoMode
                                  ? 'font-medium text-info group-hover/approval:bg-info-subtle/60'
                                  : 'font-normal text-muted-foreground group-hover/approval:bg-muted/70 group-hover/approval:text-foreground',
                            )}
                            data-ai-composer-approval-visual
                          >
                            {approvalModeValue}
                          </span>
                        )
                      : approvalModeValue}
                  </SelectTrigger>
                  <SelectContent
                    align="end"
                    className="w-68 rounded-xl border border-border/80 bg-popover p-1 shadow-overlay [&>[data-radix-select-viewport]]:h-auto [&>[data-radix-select-viewport]]:p-0"
                    position="popper"
                    side="top"
                    sideOffset={6}
                  >
                    <SelectGroup>
                      <SelectLabel className="px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                        {t('aiAssistant.approvalMode.label')}
                      </SelectLabel>
                      {approvalModes.map((option) => {
                        const OptionIcon = option.icon
                        return (
                          <SelectItem
                            key={option.value}
                            className={cn(
                              'items-start rounded-lg py-1.5 pr-7 pl-2.5 text-xs [&_[data-slot=select-item-indicator]]:top-2 [&_[data-slot=select-item-indicator]]:right-2 [&_svg]:size-3.5',
                              page && 'min-h-11 pointer-fine:min-h-0',
                            )}
                            value={option.value}
                          >
                            <span className="flex items-start gap-2">
                              <OptionIcon
                                aria-hidden="true"
                                className={cn(
                                  'mt-0.5 size-3.5 shrink-0',
                                  option.value === 'full_access' ? 'text-warning' : option.value === 'auto' ? 'text-info' : 'text-muted-foreground',
                                )}
                              />
                              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                                <span className="text-xs font-medium leading-4 text-foreground">{option.label}</span>
                                <span className="whitespace-normal text-[11px] leading-4 text-muted-foreground">{option.description}</span>
                              </span>
                            </span>
                          </SelectItem>
                        )
                      })}
                    </SelectGroup>
                  </SelectContent>
                </Select>

                <div className={page ? 'grid size-11 shrink-0 place-items-center pointer-fine:size-7 [&_button]:size-11 pointer-fine:[&_button]:size-7' : 'contents'}>
                  {hasReportedUsage
                    ? (
                        <ContextUsageRing
                          ratio={contextRatio}
                          total={contextTotal}
                          used={contextUsed}
                        />
                      )
                    : hasInitialContextUsage
                      ? (
                          <ContextUsageRing
                            ratio={0}
                            total={selectedModel.maxContextTokens}
                            used={0}
                          />
                        )
                      : <UnavailableContextUsageRing />}
                </div>

                {activeRun && !waitingInput
                  ? (
                      <Button
                        aria-label={t('aiAssistant.stop')}
                        className={page ? 'group size-11 shrink-0 rounded-full p-0 hover:bg-transparent pointer-fine:size-7' : 'size-7 shrink-0 rounded-full border border-border bg-surface text-foreground hover:bg-muted'}
                        disabled={canceling || !canCancel}
                        size="icon"
                        title={t('aiAssistant.stop')}
                        variant="ghost"
                        onClick={onCancel}
                      >
                        {page
                          ? (
                              <span className="grid size-7 place-items-center rounded-full border border-border bg-surface text-foreground transition-colors group-hover:bg-muted" data-ai-composer-stop-visual>
                                {canceling ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Square className="size-3 fill-current" />}
                              </span>
                            )
                          : canceling ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Square className="size-3 fill-current" />}
                      </Button>
                    )
                  : (
                      <Button
                        aria-label={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')}
                        className={page ? 'group size-11 shrink-0 rounded-full p-0 hover:bg-transparent pointer-fine:size-7' : 'size-7 shrink-0 rounded-full disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100'}
                        disabled={!submitEnabled}
                        size="icon"
                        title={waitingInput ? t('aiAssistant.continue') : t('aiAssistant.send')}
                        variant={page ? 'ghost' : 'default'}
                        onClick={onSubmit}
                      >
                        {page
                          ? (
                              <span className="grid size-7 place-items-center rounded-full bg-primary text-primary-foreground transition-colors group-hover:bg-primary-hover group-disabled:bg-muted group-disabled:text-muted-foreground" data-ai-composer-submit-visual>
                                {busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <ArrowUp className="size-3.5" />}
                              </span>
                            )
                          : busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <ArrowUp className="size-3.5" />}
                      </Button>
                    )}
              </div>
            </div>
          </div>
        </div>
        {!modelAvailable && <p className={cn('mt-1.5 px-1 text-danger', page ? 'text-sm' : 'text-xs')}>{t('aiAssistant.modelUnavailable')}</p>}
        <p
          className={cn(
            'mt-1.5 truncate px-1 text-center',
            page ? 'text-xs' : 'text-[11px]',
            isFullAccess ? 'font-medium text-warning' : 'text-muted-foreground',
          )}
          data-ai-full-access-notice={isFullAccess ? 'true' : undefined}
        >
          {isFullAccess ? t('aiAssistant.approvalMode.fullAccessNotice') : t('aiAssistant.securityHint')}
        </p>
      </div>
    </footer>
  )
}
