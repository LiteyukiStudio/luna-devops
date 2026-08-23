import type { RuntimeClusterPressure } from '@/api'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import { RuntimeClusterPressureBadge, RuntimeClusterPressureRings } from '@/components/common/runtime-cluster-pressure'

const pressure: RuntimeClusterPressure = {
  clusterId: 'clu_demo',
  status: 'ready',
  pressureLevel: 'moderate',
  pressureScore: 58,
  observedAt: '2026-08-24T00:00:00Z',
  details: {
    cpu: { requests: 2000, allocatable: 4000, usage: 1000, requestPercent: 50, usagePercent: 25 },
    memory: { requests: 4 * 1024 ** 3, allocatable: 8 * 1024 ** 3, requestPercent: 50 },
    metricsAvailable: false,
    nodeCount: 1,
    podCount: 3,
  },
}

describe('runtime cluster pressure presentation', () => {
  it('distinguishes an initial observation from an unavailable result', () => {
    render(<RuntimeClusterPressureBadge loading />)
    expect(screen.getByText(i18next.t('clustersPage.pressureLevels.checking'))).toBeInTheDocument()
  })

  it('shows only the derived level in the compact badge', () => {
    render(<RuntimeClusterPressureBadge pressure={{ ...pressure, details: undefined, pressureScore: undefined }} />)
    expect(screen.getByText(i18next.t('clustersPage.pressureLevels.moderate'))).toBeInTheDocument()
  })

  it('shows accessible CPU and memory allocation rings for detailed observations', () => {
    render(<RuntimeClusterPressureRings pressure={pressure} />)
    expect(screen.getByRole('button', { name: /CPU.*50%/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: new RegExp(`${i18next.t('clustersPage.memoryShort')}.*50%`) })).toBeInTheDocument()
  })
})
