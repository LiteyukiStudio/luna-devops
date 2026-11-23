import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingSpendTimeseriesBucket } from '@/api'
import { render, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { BillingSpendChart } from './billing-spend-chart'
import * as chartRuntime from './billing-spend-chart-runtime'

const billingDisplay: BillingDisplay = {
  buildMinuteCost: () => 0,
  creditsPerFiatUnit: 10,
  currencyUnit: 'Credits',
  fiatCurrencyUnit: 'CNY',
  formatAmount: value => `${value ?? 0}`,
  formatAmountWithUnit: value => `${value ?? 0} Credits`,
  formatFiatAmount: value => `${value ?? 0} CNY`,
  formatSignedAmountWithUnit: value => `${value ?? 0} Credits`,
  runtimeHourCostRange: () => ({ ceiling: 0, floor: 0 }),
}

const buckets: BillingSpendTimeseriesBucket[] = Array.from({ length: 24 }, (_, idx) => {
  const hour = String(idx).padStart(2, '0')
  return {
    bucketStart: `2026-11-23T${hour}:00:00Z`,
    categories: idx === 18 ? [{ amountCredits: '6.5', category: 'ai' }] : [],
    periodEnd: idx === 23 ? '2026-11-24T00:00:00Z' : `2026-11-23T${String(idx + 1).padStart(2, '0')}:00:00Z`,
    periodStart: `2026-11-23T${hour}:00:00Z`,
    totalCredits: idx === 18 ? '6.5' : '0',
  }
})

function createCanvas2DMock(): CanvasRenderingContext2D {
  const gradient = { addColorStop: vi.fn() }
  return new Proxy({
    canvas: { height: 240, width: 480 },
    createLinearGradient: () => gradient,
    createPattern: () => ({}),
    createRadialGradient: () => gradient,
    getImageData: () => ({ data: new Uint8ClampedArray([37, 99, 235, 255]) }),
    measureText: (text: string) => ({ width: text.length * 6 }),
  }, {
    get(target, prop) {
      if (prop in target)
        return target[prop as keyof typeof target]
      return vi.fn()
    },
    set(target, prop, value) {
      ;(target as Record<PropertyKey, unknown>)[prop] = value
      return true
    },
  }) as unknown as CanvasRenderingContext2D
}

beforeAll(async () => {
  await i18next.changeLanguage('en-US')
})

describe('billing spend chart ECharts click hit-testing', () => {
  const originalGetContext = HTMLCanvasElement.prototype.getContext

  afterEach(() => {
    HTMLCanvasElement.prototype.getContext = originalGetContext
    vi.restoreAllMocks()
  })

  it.each(['bar', 'area'] as const)('resolves bucket index on ZRender click in %s mode (including 0-spend columns)', async (chartType) => {
    HTMLCanvasElement.prototype.getContext = vi.fn(() => createCanvas2DMock()) as unknown as typeof HTMLCanvasElement.prototype.getContext

    let capturedChart: ReturnType<typeof chartRuntime.initBillingSpendChart> | undefined
    const realInit = chartRuntime.initBillingSpendChart
    vi.spyOn(chartRuntime, 'initBillingSpendChart').mockImplementation((dom) => {
      capturedChart = realInit(dom, null, { height: 240, width: 480 })
      return capturedChart
    })

    const clientWidthSpy = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(480)
    const clientHeightSpy = vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(240)
    const onSelectBucket = vi.fn()

    render(
      <BillingSpendChart
        activeCategory="all"
        billingDisplay={billingDisplay}
        buckets={buckets}
        canManageBilling
        chartType={chartType}
        granularity="hour"
        locale="en-US"
        timeZone="UTC"
        onSelectBucket={onSelectBucket}
      />,
    )

    await waitFor(() => {
      expect(capturedChart).toBeDefined()
    })

    // Convert bucket index 5 (a 0-spend column!) to pixel X using real ECharts Grid math, then trigger ZRender click
    const pixelX = capturedChart!.convertToPixel({ xAxisIndex: 0 }, 5)
    expect(typeof pixelX).toBe('number')
    expect(Number.isFinite(pixelX)).toBe(true)

    capturedChart!.getZr().trigger('click', {
      offsetX: pixelX,
      offsetY: 120,
    })

    expect(onSelectBucket).toHaveBeenCalledTimes(1)
    expect(onSelectBucket).toHaveBeenCalledWith(buckets[5])

    clientWidthSpy.mockRestore()
    clientHeightSpy.mockRestore()
  })
})
