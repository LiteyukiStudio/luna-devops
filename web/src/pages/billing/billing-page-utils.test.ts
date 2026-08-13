import type { BillingLedgerEntry } from '@/api'
import { describe, expect, it } from 'vitest'
import { ledgerDescription } from './billing-page-utils'

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
