import type { BrandColorPreset, UserBrandColorPreference } from '@/app/brand-theme'
import { Check, Link2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { brandColorPresets, normalizeBrandColorPreset, normalizeUserBrandColorPreference } from '@/app/brand-theme'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

const brandColorPresetSet = new Set<string>(brandColorPresets)
const inheritValue = '__follow_platform__'

export function BrandColorPresetField({ ariaLabel, inheritedPreset, inheritLabel, value, options, onValueChange }: {
  ariaLabel: string
  inheritedPreset?: string
  inheritLabel?: string
  value: string
  options?: string[]
  onValueChange: (value: UserBrandColorPreference) => void
}) {
  const { t } = useTranslation()
  const allowsInheritance = Boolean(inheritLabel)
  const selectedPreset = allowsInheritance ? normalizeUserBrandColorPreference(value) : normalizeBrandColorPreset(value)
  const platformPreset = normalizeBrandColorPreset(inheritedPreset)
  const configuredPresets = (options ?? brandColorPresets).filter((preset): preset is BrandColorPreset => brandColorPresetSet.has(preset))

  return (
    <RadioGroup
      aria-label={ariaLabel}
      className="flex flex-wrap items-center gap-3"
      value={selectedPreset || inheritValue}
      onValueChange={nextValue => onValueChange(nextValue === inheritValue ? '' : normalizeBrandColorPreset(nextValue))}
    >
      {allowsInheritance && (
        <BrandColorOption
          checked={selectedPreset === ''}
          id="brand-color-follow-platform"
          label={inheritLabel ?? ''}
          preset={platformPreset}
          value={inheritValue}
        />
      )}
      {configuredPresets.map(preset => (
        <BrandColorOption
          key={preset}
          checked={selectedPreset === preset}
          id={`brand-color-${preset}`}
          label={t(`settings.brandColorPresets.${preset}`)}
          preset={preset}
          value={preset}
        />
      ))}
    </RadioGroup>
  )
}

function BrandColorOption({ checked, id, label, preset, value }: {
  checked: boolean
  id: string
  label: string
  preset: BrandColorPreset
  value: string
}) {
  const showLabel = value === inheritValue

  return (
    <div className="shrink-0">
      <RadioGroupItem id={id} className="peer sr-only" value={value} />
      <Tooltip>
        <TooltipTrigger asChild>
          <Label
            className={cn(
              'relative flex size-12 cursor-pointer items-center justify-center rounded-full border-2 border-transparent bg-surface p-1.5 transition-colors duration-standard ease-standard hover:border-primary-border peer-data-[state=checked]:border-primary peer-focus-visible:ring-3 peer-focus-visible:ring-ring/50',
            )}
            htmlFor={id}
          >
            <span
              className="relative flex size-full items-center justify-center overflow-hidden rounded-full border border-border/70"
              style={{ background: `var(--${preset}-9)` }}
            >
              <span className={cn(
                'relative z-10 flex size-5 items-center justify-center rounded-full bg-surface text-foreground transition-opacity',
                checked ? 'opacity-100' : 'opacity-0',
              )}
              >
                <Check className="size-3.5" />
              </span>
            </span>
            {showLabel && (
              <span className="absolute -right-1 -bottom-1 flex size-4 items-center justify-center rounded-full border border-border bg-surface text-muted-foreground">
                <Link2 className="size-2.5" />
              </span>
            )}
            <span className="sr-only">{label}</span>
          </Label>
        </TooltipTrigger>
        <TooltipContent sideOffset={4}>
          {label}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
