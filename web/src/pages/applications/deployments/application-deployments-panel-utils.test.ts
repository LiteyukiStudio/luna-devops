import type { DeploymentTarget, Release } from '@/api'
import { describe, expect, it } from 'vitest'
import {
  formatMetricsBytes,
  formatMetricsCPU,
  formatMetricsPair,
  formatMetricsPercent,
  formatTargetRuntimeSize,
  latestReleaseByTarget,
  previousSuccessfulRelease,
  resourceUsageToneClass,
} from './application-deployments-panel-utils'

describe('deployment panel utils', () => {
  it('formats runtime specs without repeating replicas', () => {
    const format = (_key: string, options?: Record<string, unknown>) => `${options?.cpu} · ${options?.memory}`
    const target = (cpuRequest: string, memoryRequest: string) => ({ cpuRequest, memoryRequest }) as DeploymentTarget

    expect(formatTargetRuntimeSize(target('500m', '1Gi'), format)).toBe('0.5 · 1G')
    expect(formatTargetRuntimeSize(target('125m', '512Mi'), format)).toBe('0.125 · 0.5G')
    expect(formatTargetRuntimeSize(target('', '512Mi'), format)).toBe('- · 0.5G')
    expect(formatTargetRuntimeSize(target('', ''), key => key)).toBe('common.notConfigured')
  })

  it('keeps the newest release for each deployment target', () => {
    const release = (id: string, deploymentTargetId: string, createdAt: string) => ({ createdAt, deploymentTargetId, id }) as Release
    const latest = latestReleaseByTarget([
      release('rel_old', 'target_1', '2026-08-17T00:00:00Z'),
      release('rel_new', 'target_1', '2026-08-18T00:00:00Z'),
      release('rel_other', 'target_2', '2026-08-16T00:00:00Z'),
    ])

    expect(latest.target_1?.id).toBe('rel_new')
    expect(latest.target_2?.id).toBe('rel_other')
  })

  it.each([
    { name: 'no bounded limit', value: undefined, want: '-' },
    { name: 'idle within limit', value: 0, want: '0%' },
    { name: 'partial usage', value: 42.345, want: '42.3%' },
  ])('formats metrics percent: $name', ({ value, want }) => {
    expect(formatMetricsPercent(value, 'en-US')).toBe(want)
  })

  it.each([
    { milli: 0, want: '-' },
    { milli: 250, want: '250m' },
    { milli: 1500, want: '1.5' },
  ])('formats metrics CPU $milli milli', ({ milli, want }) => {
    expect(formatMetricsCPU(milli, 'en-US')).toBe(want)
  })

  it.each([
    { bytes: 0, want: '-' },
    { bytes: Math.round(9.5 * 1024 ** 2), want: '9.5Mi' },
    { bytes: Math.round(1.62 * 1024 ** 3), want: '1.62Gi' },
    { bytes: Math.round(4.68 * 1024 ** 3), want: '4.68Gi' },
    { bytes: 8 * 1024 ** 3, want: '8Gi' },
  ])('formats metrics memory bytes $want', ({ bytes, want }) => {
    expect(formatMetricsBytes(bytes, 'en-US')).toBe(want)
  })

  it.each([
    { left: '1.2Gi', right: '8Gi', want: '1.2 / 8Gi' },
    { left: '4.68Gi', right: '6Gi', want: '4.68 / 6Gi' },
    { left: '250m', right: '500m', want: '250 / 500m' },
    { left: '4.68 Gi', right: '6 Gi', want: '4.68 / 6Gi' },
    { left: '9.5Mi', right: '8Gi', want: '9.5Mi / 8Gi' },
    { left: '9.5 Mi', right: '8 Gi', want: '9.5Mi / 8Gi' },
    { left: '1m', right: '1', want: '1m / 1' },
    { left: '2.76', right: '3', want: '2.76 / 3' },
    { left: '-', right: '8Gi', want: '- / 8Gi' },
  ])('formats resource pair $left and $right -> $want', ({ left, right, want }) => {
    expect(formatMetricsPair(left, right)).toBe(want)
  })

  it.each([
    { percent: undefined, want: 'text-muted-foreground' },
    { percent: 0, want: 'text-success' },
    { percent: 69.9, want: 'text-success' },
    { percent: 70, want: 'text-warning' },
    { percent: 89.9, want: 'text-warning' },
    { percent: 90, want: 'text-danger' },
  ])('maps usage percent $percent to $want', ({ percent, want }) => {
    expect(resourceUsageToneClass(percent)).toBe(want)
  })

  it('picks the rollback target with the backend rule: latest earlier successful revision of the same target', () => {
    const release = (id: string, deploymentTargetId: string, revision: number, status: Release['status'], createdAt = '2026-08-17T00:00:00Z') => ({ applicationId: 'app_1', createdAt, deploymentTargetId, id, revision, status }) as Release
    const releases = [
      release('rel_1', 'target_1', 1, 'succeeded'),
      release('rel_2_old', 'target_1', 2, 'succeeded', '2026-08-17T00:00:00Z'),
      release('rel_2_new', 'target_1', 2, 'succeeded', '2026-08-18T00:00:00Z'),
      release('rel_3', 'target_1', 3, 'failed'),
      release('rel_other', 'target_2', 3, 'succeeded'),
      release('rel_4', 'target_1', 4, 'succeeded'),
    ]

    expect(previousSuccessfulRelease(releases, releases[5])?.id).toBe('rel_2_new')
    expect(previousSuccessfulRelease(releases, releases[0])).toBeUndefined()
  })
})
