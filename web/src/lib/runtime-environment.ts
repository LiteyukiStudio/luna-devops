import type { RuntimeEnvironmentVariable, RuntimeEnvironmentVariableInput } from '@/api'

export function publicRuntimeEnvironmentRecord(items: RuntimeEnvironmentVariable[] | RuntimeEnvironmentVariableInput[] | undefined) {
  const values: Record<string, string> = {}
  for (const item of items ?? []) {
    if (item.valueMode === 'public')
      values[item.key] = item.value ?? ''
  }
  return values
}

export function publicRuntimeEnvironmentInputs(values: Record<string, string> | undefined): RuntimeEnvironmentVariableInput[] {
  return Object.entries(values ?? {})
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => ({ key, value, valueMode: 'public' }))
}

export function runtimeSecretKeys(items: RuntimeEnvironmentVariable[] | undefined) {
  return (items ?? [])
    .filter(item => item.valueMode === 'secret' && item.configured)
    .map(item => item.key)
    .sort((left, right) => left.localeCompare(right))
}
