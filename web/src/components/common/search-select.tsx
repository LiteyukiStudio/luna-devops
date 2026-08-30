import type { ReactNode } from 'react'
import { Check, ChevronDown, Search, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

export interface SearchSelectOption {
  label: string
  value: string
  description?: string
  disabled?: boolean
  icon?: ReactNode
  keywords?: string
}

export type SearchSelectSize = 'sm' | 'default'

interface SearchSelectCommonProps {
  ariaLabel?: string
  className?: string
  disabled?: boolean
  emptyLabel?: string
  filterLocally?: boolean
  limited?: boolean
  loading?: boolean
  maxVisible?: number
  options: SearchSelectOption[]
  placeholder?: string
  search?: string
  searchPlaceholder?: string
  size?: SearchSelectSize
  onSearchChange?: (value: string) => void
}

interface SearchSelectProps extends SearchSelectCommonProps {
  value: string
  onValueChange: (value: string) => void
}

interface SearchMultiSelectProps extends SearchSelectCommonProps {
  selectedLabel?: (options: SearchSelectOption[]) => string
  value: string[]
  onValueChange: (value: string[]) => void
}

/**
 * 可搜索的单选基础组件。
 * 资源候选较多或需要远程搜索时使用；少量固定枚举继续使用 shadcn Select 或 NativeSelect。
 */
export function SearchSelect({
  ariaLabel,
  className,
  disabled,
  emptyLabel,
  filterLocally,
  limited,
  loading,
  maxVisible = 50,
  options,
  placeholder,
  search,
  searchPlaceholder,
  size = 'default',
  value,
  onSearchChange,
  onValueChange,
}: SearchSelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const searchState = useSearchState(search, onSearchChange)
  const visibleOptions = useVisibleOptions(options, searchState.value, maxVisible, filterLocally ?? search === undefined)
  const selected = options.find(option => option.value === value)
  const isLimited = limited || visibleOptions.total > visibleOptions.items.length

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <SelectTriggerButton
        ariaLabel={ariaLabel ?? placeholder ?? t('common.select')}
        className={className}
        disabled={disabled}
        open={open}
        placeholder={!selected}
        size={size}
      >
        {selected?.icon}
        <span className="min-w-0 flex-1 truncate text-left">{selected?.label ?? placeholder ?? t('common.select')}</span>
      </SelectTriggerButton>
      <SearchOptionsContent
        emptyLabel={emptyLabel}
        isLimited={isLimited}
        loading={loading}
        options={visibleOptions.items}
        search={searchState.value}
        searchPlaceholder={searchPlaceholder}
        size={size}
        onSearchChange={searchState.onChange}
        renderOption={option => (
          <SearchOptionButton
            key={option.value}
            checked={option.value === value}
            option={option}
            size={size}
            onSelect={() => {
              onValueChange(option.value)
              setOpen(false)
            }}
          />
        )}
      />
    </Popover>
  )
}

/**
 * 可搜索的多选基础组件。
 * 项目空间、用户、应用、部署配置和列表筛选等多值场景应在业务包装器中复用此组件。
 */
export function SearchMultiSelect({
  ariaLabel,
  className,
  disabled,
  emptyLabel,
  filterLocally,
  limited,
  loading,
  maxVisible = 50,
  options,
  placeholder,
  search,
  searchPlaceholder,
  selectedLabel,
  size = 'default',
  value,
  onSearchChange,
  onValueChange,
}: SearchMultiSelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const searchState = useSearchState(search, onSearchChange)
  const visibleOptions = useVisibleOptions(options, searchState.value, maxVisible, filterLocally ?? search === undefined)
  const selectedValues = useMemo(() => new Set(value), [value])
  const selectedOptions = useMemo(() => options.filter(option => selectedValues.has(option.value)), [options, selectedValues])
  const isLimited = limited || visibleOptions.total > visibleOptions.items.length
  const label = selectedLabel?.(selectedOptions)
    ?? (selectedOptions.length === 1 ? selectedOptions[0]?.label : '')
    ?? ''
  const summary = label || (selectedOptions.length > 1 ? t('common.selectedCount', { count: selectedOptions.length }) : placeholder ?? t('common.select'))

  function toggleValue(option: SearchSelectOption) {
    if (option.disabled)
      return
    const next = new Set(value)
    if (next.has(option.value))
      next.delete(option.value)
    else
      next.add(option.value)
    const knownValues = options.filter(item => next.has(item.value)).map(item => item.value)
    const unknownValues = value.filter(item => next.has(item) && !options.some(optionItem => optionItem.value === item))
    onValueChange([...unknownValues, ...knownValues])
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <SelectTriggerButton
        ariaLabel={ariaLabel ?? placeholder ?? t('common.select')}
        className={className}
        disabled={disabled}
        open={open}
        placeholder={selectedOptions.length === 0}
        size={size}
      >
        <span className="min-w-0 flex-1 truncate text-left">{summary}</span>
      </SelectTriggerButton>
      <SearchOptionsContent
        emptyLabel={emptyLabel}
        footer={selectedOptions.length > 0 && (
          <div className="flex items-center justify-between gap-3 border-t border-border px-3 py-2 text-xs text-muted-foreground">
            <span>{t('common.selectedCount', { count: selectedOptions.length })}</span>
            <Button className="h-auto gap-1 p-0 text-xs font-normal text-foreground hover:bg-transparent hover:text-primary-text [&_svg]:size-3.5" type="button" variant="ghost" onClick={() => onValueChange([])}>
              <X className="size-3.5" />
              {t('common.clearSelection')}
            </Button>
          </div>
        )}
        isLimited={isLimited}
        loading={loading}
        options={visibleOptions.items}
        search={searchState.value}
        searchPlaceholder={searchPlaceholder}
        size={size}
        onSearchChange={searchState.onChange}
        renderOption={option => (
          <SearchOptionButton
            key={option.value}
            checked={selectedValues.has(option.value)}
            checkbox
            option={option}
            size={size}
            onSelect={() => toggleValue(option)}
          />
        )}
      />
    </Popover>
  )
}

function SelectTriggerButton({ ariaLabel, children, className, disabled, open, placeholder, size }: {
  ariaLabel: string
  children: ReactNode
  className?: string
  disabled?: boolean
  open: boolean
  placeholder: boolean
  size: SearchSelectSize
}) {
  return (
    <PopoverTrigger asChild>
      <Button
        aria-expanded={open}
        aria-label={ariaLabel}
        className={cn(
          'w-full justify-between rounded-md font-normal',
          size === 'sm' ? 'h-8 px-2.5 text-xs' : 'h-9 px-3 text-sm',
          className,
        )}
        disabled={disabled}
        type="button"
        variant="outline"
      >
        <span className={cn('flex min-w-0 flex-1 items-center gap-2', placeholder && 'text-muted-foreground')}>{children}</span>
        <ChevronDown className={cn(size === 'sm' ? 'size-3.5' : 'size-4', 'shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </Button>
    </PopoverTrigger>
  )
}

function SearchOptionsContent({ emptyLabel, footer, isLimited, loading, options, renderOption, search, searchPlaceholder, size, onSearchChange }: {
  emptyLabel?: string
  footer?: ReactNode
  isLimited: boolean
  loading?: boolean
  options: SearchSelectOption[]
  renderOption: (option: SearchSelectOption) => ReactNode
  search: string
  searchPlaceholder?: string
  size: SearchSelectSize
  onSearchChange: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <PopoverContent
      align="start"
      className="grid max-h-80 w-[var(--radix-popover-trigger-width)] min-w-64 grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden p-0"
      sideOffset={6}
    >
      <div className={cn('flex items-center gap-2 border-b border-border', size === 'sm' ? 'px-2 py-1.5' : 'p-2')}>
        <Search className={cn(size === 'sm' ? 'size-3.5' : 'size-4', 'shrink-0 text-muted-foreground')} />
        <Input
          autoFocus
          className={cn('rounded-md border-0 px-0 shadow-none focus-visible:ring-0', size === 'sm' ? 'h-7 text-xs' : 'h-8 text-sm')}
          placeholder={searchPlaceholder ?? t('common.search')}
          value={search}
          onChange={event => onSearchChange(event.target.value)}
        />
      </div>
      <div className="min-h-0 overflow-y-auto overscroll-contain p-1" onWheel={event => event.stopPropagation()}>
        {loading && <p className={cn('px-3 py-2 text-muted-foreground', size === 'sm' ? 'text-xs' : 'text-sm')}>{t('common.loading')}</p>}
        {!loading && options.length === 0 && <p className={cn('px-3 py-2 text-muted-foreground', size === 'sm' ? 'text-xs' : 'text-sm')}>{emptyLabel ?? t('common.noOptions')}</p>}
        {!loading && options.map(renderOption)}
        {!loading && isLimited && <p className="px-3 py-2 text-xs text-muted-foreground">{t('common.searchSelectLimited', { count: options.length })}</p>}
      </div>
      {footer || <span />}
    </PopoverContent>
  )
}

function SearchOptionButton({ checked, checkbox, option, size, onSelect }: {
  checked: boolean
  checkbox?: boolean
  option: SearchSelectOption
  size: SearchSelectSize
  onSelect: () => void
}) {
  return (
    <Button
      className={cn(
        'h-auto w-full min-w-0 justify-start gap-2 whitespace-normal rounded-md text-left font-normal hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50',
        size === 'sm' ? 'px-2 py-1.5 text-xs' : 'px-3 py-2 text-sm',
      )}
      disabled={option.disabled}
      type="button"
      variant="ghost"
      onClick={onSelect}
    >
      {checkbox && (
        <span className={cn('flex shrink-0 items-center justify-center rounded border border-border', size === 'sm' ? 'size-3.5' : 'size-4', checked && 'border-primary bg-primary text-primary-foreground')}>
          {checked && <Check className={size === 'sm' ? '!size-2.5' : '!size-3'} />}
        </span>
      )}
      {option.icon}
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">{option.label}</span>
        {option.description && <span className="block truncate text-xs text-muted-foreground">{option.description}</span>}
      </span>
      {!checkbox && checked && <Check className={cn(size === 'sm' ? '!size-3.5' : '!size-4', 'shrink-0 text-primary-text')} />}
    </Button>
  )
}

function useSearchState(search: string | undefined, onSearchChange: ((value: string) => void) | undefined) {
  const [localSearch, setLocalSearch] = useState('')
  return {
    value: search ?? localSearch,
    onChange: onSearchChange ?? setLocalSearch,
  }
}

function useVisibleOptions(options: SearchSelectOption[], search: string, maxVisible: number, filterLocally: boolean) {
  return useMemo(() => {
    const keyword = search.trim().toLowerCase()
    const filtered = filterLocally && keyword
      ? options.filter(option => [option.label, option.description, option.keywords].some(value => value?.toLowerCase().includes(keyword)))
      : options
    return {
      items: filtered.slice(0, maxVisible),
      total: filtered.length,
    }
  }, [filterLocally, maxVisible, options, search])
}
