export const DEFAULT_EVENT_SEVERITIES = ['warning', 'error']

export function initialEventSeverityFilters(searchParams: URLSearchParams) {
  const configured = initialEventFilterValues(searchParams, 'severities', 'severity')
  return configured.length > 0 ? configured : [...DEFAULT_EVENT_SEVERITIES]
}

export function initialEventFilterValues(searchParams: URLSearchParams, plural: string, singular: string) {
  const values = [...searchParams.getAll(plural), ...searchParams.getAll(singular)]
    .flatMap(value => value.split(','))
    .map(value => value.trim())
    .filter(Boolean)
  return [...new Set(values)]
}
