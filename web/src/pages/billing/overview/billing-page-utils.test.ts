import type { BillingLedgerEntry, BillingSpendTimeseriesBucket } from '@/api'
import { describe, expect, it } from 'vitest'
import {
  billingPeriodToQuery,
  bucketDisplayedCredits,
  defaultGranularityForPeriod,
  formatTimeseriesBucketRange,
  isGranularityAllowedForPeriod,
  ledgerDescription,
  periodSelectionForPreset,
} from './billing-page-utils'

function ledgerEntry(description: string): BillingLedgerEntry {
  return {
    id: 'bled_test',
    userId: 'usr_test',
    projectId: '',
    applicationId: '',
    applicationName: '',
    applicationIdentifier: '',
    type: 'credit',
    amountCredits: '10',
    balanceAfterCredits: '10',
    reason: 'billing.recharge',
    meter: '',
    usageRecordId: '',
    resourceType: 'user_wallet',
    resourceId: 'usr_test',
    description,
    createdBy: 'usr_admin',
    createdAt: '2026-08-13T00:00:00Z',
    occurredAt: '2026-08-13T00:00:00Z',
  }
}

describe('ledgerDescription', () => {
  it('returns a trimmed ledger description', () => {
    expect(ledgerDescription(ledgerEntry('  测试环境充值  '), '手动充值')).toBe('测试环境充值')
  })

  it('hides empty or duplicate descriptions', () => {
    expect(ledgerDescription(ledgerEntry('   '), '手动充值')).toBe('')
    expect(ledgerDescription(ledgerEntry('Manual Recharge'), 'manual recharge')).toBe('')
  })
})

describe('billing period and granularity helpers', () => {
  it('builds rolling 24h and calendar today selections with hourly default granularity', () => {
    const anchor = new Date(2026, 10, 23, 10, 25, 0)
    const last24h = periodSelectionForPreset('last24Hours', anchor)
    const query24h = billingPeriodToQuery(last24h)
    expect(query24h.periodStart).toBeDefined()
    expect(query24h.periodEnd).toBeDefined()
    expect((Date.parse(query24h.periodEnd!) - Date.parse(query24h.periodStart!)) / 3_600_000).toBe(24)
    expect(defaultGranularityForPeriod(last24h)).toBe('hour')

    const today = periodSelectionForPreset('today', anchor)
    expect(today.startDate).toBe('2026-11-23')
    expect(today.endDate).toBe('2026-11-23')
    expect(defaultGranularityForPeriod(today)).toBe('hour')
  })

  it('selects sensible default granularities and guards overly wide hourly ranges', () => {
    const anchor = new Date(2026, 10, 23, 10, 0, 0)
    const thisMonth = periodSelectionForPreset('thisMonth', anchor)
    expect(defaultGranularityForPeriod(thisMonth)).toBe('day')
    expect(isGranularityAllowedForPeriod('hour', thisMonth)).toBe(true)

    const thisYear = periodSelectionForPreset('thisYear', anchor)
    expect(defaultGranularityForPeriod(thisYear)).toBe('month')
    expect(isGranularityAllowedForPeriod('hour', thisYear)).toBe(false)
    expect(isGranularityAllowedForPeriod('day', thisYear)).toBe(true)
    expect(isGranularityAllowedForPeriod('month', { endDate: '2026-11-23', preset: 'custom', startDate: '1900-01-01' })).toBe(false)
  })

  it('shows actual clamped date ranges when month or day buckets are clipped at period boundaries', () => {
    const clippedOct = formatTimeseriesBucketRange(
      {
        bucketStart: '2026-10-01T00:00:00Z',
        periodEnd: '2026-11-01T00:00:00Z',
        periodStart: '2026-10-25T00:00:00Z',
      },
      'month',
      'en-US',
      'UTC',
    )
    expect(clippedOct).toContain('10/25/2026')
    expect(clippedOct).toContain('10/31/2026')

    const fullOct = formatTimeseriesBucketRange(
      {
        bucketStart: '2026-10-01T00:00:00Z',
        periodEnd: '2026-11-01T00:00:00Z',
        periodStart: '2026-10-01T00:00:00Z',
      },
      'month',
      'en-US',
      'UTC',
    )
    expect(fullOct).toBe('Oct 2026')
  })

  it('computes displayed bucket credits for all vs single category', () => {
    const bucket: BillingSpendTimeseriesBucket = {
      bucketStart: '2026-11-23T02:00:00Z',
      categories: [
        { amountCredits: '3.5', category: 'runtime' },
        { amountCredits: '6.25', category: 'ai' },
      ],
      periodEnd: '2026-11-23T03:00:00Z',
      periodStart: '2026-11-23T02:00:00Z',
      totalCredits: '9.75',
    }
    expect(bucketDisplayedCredits(bucket, 'all')).toBe(9.75)
    expect(bucketDisplayedCredits(bucket, 'ai')).toBe(6.25)
    expect(bucketDisplayedCredits(bucket, 'build')).toBe(0)
  })
})
