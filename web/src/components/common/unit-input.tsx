import type { InputHTMLAttributes } from 'react'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { cn } from '@/lib/utils'

export interface UnitInputUnit {
  value: string
  label: string
  suffix?: string
}

export interface UnitInputProps {
  value: string
  units: UnitInputUnit[]
  onChange: (value: string) => void
  className?: string
  disabled?: boolean
  inputProps?: Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'disabled' | 'inputMode' | 'onChange' | 'pattern' | 'value'>
  unitSelectLabel?: string
}

export function UnitInput({ className, disabled, inputProps, onChange, unitSelectLabel, units, value }: UnitInputProps) {
  const parsed = parseUnitInputValue(value, units)

  function updateAmount(amount: string) {
    onChange(formatUnitInputValue(normalizeUnitInputAmount(amount), parsed.unit, units))
  }

  function updateUnit(unit: string) {
    onChange(formatUnitInputValue(parsed.amount, unit, units))
  }

  return (
    <div className={cn('flex h-9 min-w-40 overflow-hidden rounded-md border border-input bg-background shadow-xs transition focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/50', disabled && 'opacity-50', className)}>
      <Input
        {...inputProps}
        className="h-full min-w-0 flex-1 rounded-none border-0 bg-transparent px-4 shadow-none focus-visible:ring-0"
        disabled={disabled}
        inputMode="numeric"
        pattern="[0-9]*"
        value={parsed.amount}
        onChange={event => updateAmount(event.target.value)}
      />
      <div className="my-1 w-px bg-border" />
      <NativeSelect
        aria-label={unitSelectLabel}
        className="h-full w-auto rounded-none border-0 bg-transparent px-3 pr-8 text-sm font-medium text-muted-foreground shadow-none disabled:opacity-100 focus-visible:border-transparent focus-visible:ring-0 dark:bg-transparent"
        containerClassName="h-full shrink-0 [&_svg]:right-2"
        disabled={disabled}
        value={parsed.unit}
        onChange={event => updateUnit(event.target.value)}
      >
        {units.map(unit => <option key={unit.value} value={unit.value}>{unit.label}</option>)}
      </NativeSelect>
    </div>
  )
}

function parseUnitInputValue(value: string, units: UnitInputUnit[]) {
  const normalized = String(value || '').trim()
  const fallbackUnit = units[0]?.value ?? ''
  if (!normalized)
    return { amount: '', unit: fallbackUnit }

  const suffixUnits = [...units]
    .map(unit => ({ ...unit, suffix: unit.suffix ?? unit.value }))
    .filter(unit => unit.suffix)
    .sort((a, b) => String(b.suffix).length - String(a.suffix).length)

  const matched = suffixUnits.find(unit => normalized.endsWith(unit.suffix ?? ''))
  if (matched)
    return { amount: normalizeUnitInputAmount(normalized.slice(0, -String(matched.suffix).length)), unit: matched.value }

  const bareUnit = units.find(unit => (unit.suffix ?? unit.value) === '')
  return { amount: normalizeUnitInputAmount(normalized), unit: bareUnit?.value ?? fallbackUnit }
}

function formatUnitInputValue(amount: string, unitValue: string, units: UnitInputUnit[]) {
  const normalizedAmount = normalizeUnitInputAmount(amount)
  if (!normalizedAmount)
    return ''
  const unit = units.find(item => item.value === unitValue) ?? units[0]
  const suffix = unit ? (unit.suffix ?? unit.value) : ''
  return `${normalizedAmount}${suffix}`
}

function normalizeUnitInputAmount(value: string) {
  return value.replace(/\D/g, '')
}
