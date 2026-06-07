import { Check, ChevronDown, Search } from 'lucide-react'
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
}

/**
 * 搜索驱动的下拉选择器。
 * 用于分支、仓库、命名空间等候选很多、需要后端边搜边筛的资源；少量静态选项优先使用 NativeSelect 或 SegmentedControl。
 */
export function SearchSelect({
  disabled,
  emptyLabel,
  limited,
  loading,
  maxVisible = 50,
  options,
  placeholder,
  search,
  value,
  onSearchChange,
  onValueChange,
}: {
  disabled?: boolean
  emptyLabel?: string
  limited?: boolean
  loading?: boolean
  maxVisible?: number
  options: SearchSelectOption[]
  placeholder?: string
  search: string
  value: string
  onSearchChange: (value: string) => void
  onValueChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const selected = options.find(option => option.value === value)
  const visibleOptions = useMemo(() => options.slice(0, maxVisible), [maxVisible, options])
  const isLimited = limited || options.length > visibleOptions.length

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          aria-expanded={open}
          className="h-9 w-full justify-between rounded-full px-4"
          disabled={disabled}
          type="button"
          variant="outline"
        >
          <span className={cn('min-w-0 flex-1 truncate text-left', !selected && 'text-muted-foreground')}>
            {selected?.label ?? placeholder ?? t('common.select')}
          </span>
          <ChevronDown className={cn('size-4 shrink-0 transition-transform', open && 'rotate-180')} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="grid max-h-72 w-[var(--radix-popover-trigger-width)] min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden p-0"
        sideOffset={6}
      >
        <div className="flex items-center gap-2 border-b border-border p-2">
          <Search className="size-4 shrink-0 text-muted-foreground" />
          <Input
            autoFocus
            className="h-8 rounded-md border-0 px-0 shadow-none focus-visible:ring-0"
            placeholder={t('common.search')}
            value={search}
            onChange={event => onSearchChange(event.target.value)}
          />
        </div>
        <div className="min-h-0 overflow-y-auto overscroll-contain p-1" onWheel={event => event.stopPropagation()}>
          {loading && <p className="px-3 py-2 text-sm text-muted-foreground">{t('common.loading')}</p>}
          {!loading && visibleOptions.length === 0 && (
            <p className="px-3 py-2 text-sm text-muted-foreground">{emptyLabel ?? t('common.noOptions')}</p>
          )}
          {!loading && visibleOptions.map(option => (
            <button
              key={option.value}
              className="flex w-full min-w-0 items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-muted"
              type="button"
              onClick={() => {
                onValueChange(option.value)
                setOpen(false)
              }}
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{option.label}</span>
                {option.description && <span className="block truncate text-xs text-muted-foreground">{option.description}</span>}
              </span>
              {option.value === value && <Check className="size-4 shrink-0 text-primary" />}
            </button>
          ))}
          {!loading && isLimited && (
            <p className="px-3 py-2 text-xs text-muted-foreground">
              {t('common.searchSelectLimited', { count: visibleOptions.length })}
            </p>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
