import { describe, expect, it } from 'vitest'
import { initialEventSeverityFilters } from './event-filter-defaults'

describe('event severity defaults', () => {
  it('shows warning and error events by default', () => {
    expect(initialEventSeverityFilters(new URLSearchParams())).toEqual(['warning', 'error'])
  })

  it('keeps explicit severity filters from the route', () => {
    expect(initialEventSeverityFilters(new URLSearchParams('severity=info'))).toEqual(['info'])
    expect(initialEventSeverityFilters(new URLSearchParams('severities=warning,error'))).toEqual(['warning', 'error'])
  })
})
