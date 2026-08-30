import { describe, expect, it } from 'vitest'
import { clusterResourcePolicySchema } from '../resources/cluster-resource-policy-schema'

const schema = clusterResourcePolicySchema(key => key)

describe('clusterResourcePolicySchema', () => {
  it('accepts defaults, all-zero, request-only, and limit-only policies', () => {
    expect(schema.safeParse({ cpuRequestPercent: 10, memoryRequestPercent: 25, cpuLimitPercent: 100, memoryLimitPercent: 100 }).success).toBe(true)
    expect(schema.safeParse({ cpuRequestPercent: 0, memoryRequestPercent: 0, cpuLimitPercent: 0, memoryLimitPercent: 0 }).success).toBe(true)
    expect(schema.safeParse({ cpuRequestPercent: 50, memoryRequestPercent: 50, cpuLimitPercent: 0, memoryLimitPercent: 0 }).success).toBe(true)
    expect(schema.safeParse({ cpuRequestPercent: 0, memoryRequestPercent: 0, cpuLimitPercent: 50, memoryLimitPercent: 50 }).success).toBe(true)
  })

  it('rejects values outside 0-100 and requests above enabled limits', () => {
    expect(schema.safeParse({ cpuRequestPercent: -1, memoryRequestPercent: 25, cpuLimitPercent: 100, memoryLimitPercent: 100 }).success).toBe(false)
    expect(schema.safeParse({ cpuRequestPercent: 51, memoryRequestPercent: 25, cpuLimitPercent: 50, memoryLimitPercent: 100 }).success).toBe(false)
    expect(schema.safeParse({ cpuRequestPercent: 10, memoryRequestPercent: 51, cpuLimitPercent: 100, memoryLimitPercent: 50 }).success).toBe(false)
  })
})
