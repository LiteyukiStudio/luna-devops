import { useTranslation } from 'react-i18next'
import { APPLICATION_ICON_NAMES, applicationIconComponents, isApplicationIconImage, isApplicationIconName, normalizeApplicationIconName } from '@/components/common/application-icons'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

export function ApplicationIcon({ className, name, size = 18 }: { className?: string, name?: string, size?: number }) {
  if (isApplicationIconImage(name)) {
    return (
      <img
        alt=""
        className={cn('rounded-sm object-contain', className)}
        height={size}
        src={name?.trim()}
        width={size}
        onError={(event) => {
          const image = event.currentTarget
          if (image.dataset.fallbackApplied)
            return
          image.dataset.fallbackApplied = 'true'
          image.src = '/app-templates/icons/fallback.svg'
        }}
      />
    )
  }
  const Icon = applicationIconComponents[normalizeApplicationIconName(name)]
  return <Icon className={className} height={size} width={size} />
}

export function ApplicationIconPicker({ compact = false, disabled, value, onChange }: {
  compact?: boolean
  disabled?: boolean
  value?: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const selected = normalizeApplicationIconName(value)
  const customIconValue = value && !isApplicationIconName(value) ? value : ''
  const iconTitle = customIconValue ? t('apps.customIcon') : t(`apps.icons.${selected}`)

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          aria-label={t('apps.iconPickerAria')}
          className={compact ? 'size-9 rounded-md border-0 bg-transparent px-0 text-foreground shadow-none hover:bg-muted hover:text-foreground' : undefined}
          disabled={disabled}
          title={iconTitle}
          type="button"
          variant={compact ? 'ghost' : 'secondary'}
        >
          <ApplicationIcon name={value || selected} />
          {!compact && <span>{iconTitle}</span>}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 p-2">
        <div className="grid grid-cols-8 gap-1">
          {APPLICATION_ICON_NAMES.map((name) => {
            const active = name === selected
            return (
              <Button
                key={name}
                aria-label={t(`apps.icons.${name}`)}
                className={cn(
                  'size-8 rounded-md border border-transparent p-0 text-muted-foreground transition hover:border-border hover:bg-muted hover:text-foreground [&_svg]:size-[18px]',
                  active && 'border-primary bg-primary/10 text-primary-text',
                )}
                size="icon"
                title={t(`apps.icons.${name}`)}
                type="button"
                variant="ghost"
                onClick={() => onChange(name)}
              >
                <ApplicationIcon name={name} />
              </Button>
            )
          })}
        </div>
        <div className="mt-3 border-t border-border pt-3">
          <label className="grid gap-2 text-xs font-medium text-muted-foreground">
            {t('apps.iconUrl')}
            <Input
              aria-label={t('apps.iconUrl')}
              placeholder={t('apps.iconUrlPlaceholder')}
              value={customIconValue}
              onChange={event => onChange(event.target.value)}
            />
          </label>
          <p className="mt-2 text-xs text-muted-foreground">{t('apps.iconUrlHint')}</p>
        </div>
      </PopoverContent>
    </Popover>
  )
}
