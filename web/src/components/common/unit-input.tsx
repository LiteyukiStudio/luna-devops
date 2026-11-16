import type { InputHTMLAttributes } from 'react'
import type { UnitInputUnit } from '@/lib/resource-units'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { cpuUnits, formatUnitInputValue, memoryUnits, normalizeUnitInputAmount, parseUnitInputValue, storageUnits } from '@/lib/resource-units'
import { cn } from '@/lib/utils'

export type { UnitInputUnit }

export interface UnitInputProps {
  value: string
  units: UnitInputUnit[]
  onChange: (value: string) => void
  className?: string
  disabled?: boolean
  inputProps?: Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'disabled' | 'inputMode' | 'onChange' | 'pattern' | 'value'>
  unitSelectLabel?: string
}

export type QuotaInputProps = Omit<UnitInputProps, 'units'>

export function UnitInput({ className, disabled, inputProps, onChange, unitSelectLabel, units, value }: UnitInputProps) {
  const hasValue = Boolean(String(value || '').trim())
  const parsed = parseUnitInputValue(value, units)
  const [retainedUnit, setRetainedUnit] = useState(() => parsed.unit)
  const rawActiveUnit = hasValue ? parsed.unit : retainedUnit
  const activeUnit = units.some(item => item.value === rawActiveUnit) ? rawActiveUnit : (units[0]?.value ?? '')
  const isInvalid = inputProps?.['aria-invalid'] === true || inputProps?.['aria-invalid'] === 'true'

  function updateAmount(amount: string) {
    setRetainedUnit(activeUnit)
    onChange(formatUnitInputValue(normalizeUnitInputAmount(amount), activeUnit, units))
  }

  function updateUnit(unit: string) {
    setRetainedUnit(unit)
    onChange(formatUnitInputValue(parsed.amount, unit, units))
  }

  return (
    <div
      className={cn(
        'flex h-9 w-full min-w-0 overflow-hidden rounded-control border border-input bg-surface shadow-xs transition-[color,box-shadow] dark:bg-input/30',
        isInvalid
          ? 'border-danger ring-danger/20 focus-within:border-danger focus-within:ring-3 focus-within:ring-danger/20 dark:ring-danger/40'
          : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
        disabled && 'opacity-50',
        className,
      )}
    >
      <Input
        {...inputProps}
        className="h-full min-w-0 flex-1 rounded-none border-0 bg-transparent px-3 font-mono text-sm tabular-nums shadow-none focus-visible:ring-0 aria-invalid:border-0 aria-invalid:ring-0 dark:bg-transparent"
        disabled={disabled}
        inputMode="decimal"
        value={parsed.amount}
        onBlur={(event) => {
          if (parsed.amount.endsWith('.'))
            onChange(formatUnitInputValue(parsed.amount.slice(0, -1), activeUnit, units))
          inputProps?.onBlur?.(event)
        }}
        onChange={event => updateAmount(event.target.value)}
      />
      <div className="my-1.5 w-px shrink-0 bg-border" />
      <NativeSelect
        aria-label={unitSelectLabel}
        className="h-full w-auto rounded-none border-0 bg-transparent px-2.5 pr-7 text-xs font-medium text-muted-foreground shadow-none disabled:opacity-100 focus-visible:border-transparent focus-visible:ring-0 dark:bg-transparent"
        containerClassName="h-full shrink-0 [&_svg]:right-2"
        disabled={disabled}
        value={activeUnit}
        onChange={event => updateUnit(event.target.value)}
      >
        {units.map(unit => <option key={unit.value} value={unit.value}>{unit.label}</option>)}
      </NativeSelect>
    </div>
  )
}

/** 统一 CPU 配额输入（自由数字/小数 + 毫核/核单位选择）。 */
export function CpuQuotaInput({ unitSelectLabel, ...props }: QuotaInputProps) {
  const { t } = useTranslation()
  return (
    <UnitInput
      {...props}
      unitSelectLabel={unitSelectLabel ?? t('common.cpuUnitLabel')}
      units={cpuUnits(t)}
    />
  )
}

/** 统一内存配额输入（自由数字/小数 + Mi/Gi 单位选择）。 */
export function MemoryQuotaInput({ unitSelectLabel, ...props }: QuotaInputProps) {
  const { t } = useTranslation()
  return (
    <UnitInput
      {...props}
      unitSelectLabel={unitSelectLabel ?? t('common.memoryUnitLabel')}
      units={memoryUnits}
    />
  )
}

/** 统一存储容量输入（自由数字/小数 + Gi/Mi/Ti 单位选择）。 */
export function StorageQuotaInput({ unitSelectLabel, ...props }: QuotaInputProps) {
  const { t } = useTranslation()
  return (
    <UnitInput
      {...props}
      unitSelectLabel={unitSelectLabel ?? t('common.storageUnitLabel')}
      units={storageUnits}
    />
  )
}
