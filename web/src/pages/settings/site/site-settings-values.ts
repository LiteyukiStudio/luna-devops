export function changedConfigValues(values: Record<string, unknown>, current: Record<string, string>) {
  return Object.fromEntries(
    Object.entries(values).filter(([key, value]) => String(value ?? '') !== (current[key] ?? '')),
  )
}
