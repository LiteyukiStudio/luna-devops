import type { RuntimeHourCostInput } from './billing-display'
import type { BillingRateRule } from '@/api'
import { describe, expect, it } from 'vitest'
import { estimateRuntimeHourCostRange } from './billing-display'

function rule(meter: string, creditsPerUnit: string, enabled = true): BillingRateRule {
  return { createdAt: '', creditsPerUnit, description: '', enabled, id: meter, meter, unit: '', updatedAt: '' }
}

const rules = [rule('runtime.cpu_vcpu_hour', '30'), rule('runtime.memory_gib_hour', '10')]
const defaultPolicy = { cpuLimitPercent: 100, cpuRequestPercent: 10, memoryLimitPercent: 100, memoryRequestPercent: 25 }

function input(overrides: Partial<RuntimeHourCostInput> = {}): RuntimeHourCostInput {
  return { ceilingReplicas: 2, cpuQuota: '1', floorReplicas: 2, memoryQuota: '2Gi', policy: defaultPolicy, ...overrides }
}

describe('estimateRuntimeHourCostRange', () => {
  it.each([
    {
      name: 'default policy: request floor to limit ceiling',
      input: input(),
      // floor: 0.1 vCPU × 2 × 30 + 0.5 GiB × 2 × 10; ceiling: 1 × 2 × 30 + 2 × 2 × 10
      want: { ceiling: 100, floor: 16 },
    },
    {
      name: 'autoscaling uses minimum replicas for the floor and maximum for the ceiling',
      input: input({ ceilingReplicas: 5, floorReplicas: 1 }),
      want: { ceiling: 250, floor: 8 },
    },
    {
      name: 'request percent 0 omits the request floor',
      input: input({ policy: { ...defaultPolicy, cpuRequestPercent: 0, memoryRequestPercent: 0 } }),
      want: { ceiling: 100, floor: 0 },
    },
    {
      name: 'limit percent 0 leaves the cost without a ceiling',
      input: input({ policy: { ...defaultPolicy, memoryLimitPercent: 0 } }),
      want: { ceiling: null, floor: 16 },
    },
    {
      name: 'scale to zero costs nothing',
      input: input({ ceilingReplicas: 0, floorReplicas: 0 }),
      want: { ceiling: 0, floor: 0 },
    },
  ])('$name', ({ input: value, want }) => {
    const got = estimateRuntimeHourCostRange(rules, value)
    expect(got.floor).toBeCloseTo(want.floor)
    if (want.ceiling === null)
      expect(got.ceiling).toBeNull()
    else
      expect(got.ceiling).toBeCloseTo(want.ceiling)
  })

  it('ignores disabled meters, so an unlimited unbilled resource keeps a ceiling', () => {
    const got = estimateRuntimeHourCostRange([rule('runtime.cpu_vcpu_hour', '30'), rule('runtime.memory_gib_hour', '10', false)], input({ policy: { ...defaultPolicy, memoryLimitPercent: 0 } }))
    expect(got).toEqual({ ceiling: 60, floor: 6 })
  })
})
