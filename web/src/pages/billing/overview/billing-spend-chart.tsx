import type { BarSeriesOption, LineSeriesOption } from 'echarts/charts'
import type { GridComponentOption, TooltipComponentOption } from 'echarts/components'
import type { ComposeOption, EChartsType } from 'echarts/core'
import type { KeyboardEvent } from 'react'
import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingChartType } from './billing-page-utils'
import type { BillingSpendGranularity, BillingSpendTimeseriesBucket } from '@/api'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BILLING_CATEGORY_COLOR_VARS,
  BILLING_CATEGORY_ORDER,
  bucketCategoryAmount,
  bucketDisplayedCredits,
  categoryTranslatedLabel,
  formatTimeseriesAxisLabel,
  formatTimeseriesBucketRange,
} from './billing-page-utils'

type SpendChartOption = ComposeOption<BarSeriesOption | LineSeriesOption | GridComponentOption | TooltipComponentOption>

interface BillingSpendChartProps {
  activeCategory: string
  billingDisplay: BillingDisplay
  buckets: BillingSpendTimeseriesBucket[]
  canManageBilling: boolean
  chartType: BillingChartType
  focusedBucketStart?: string
  granularity: BillingSpendGranularity
  locale: string
  timeZone: string
  onSelectBucket?: (bucket: BillingSpendTimeseriesBucket) => void
}

export function BillingSpendChart({
  activeCategory,
  billingDisplay,
  buckets,
  canManageBilling,
  chartType,
  focusedBucketStart,
  granularity,
  locale,
  timeZone,
  onSelectBucket,
}: BillingSpendChartProps) {
  const { t } = useTranslation()
  const elementRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<EChartsType | null>(null)
  const optionRef = useRef<SpendChartOption | null>(null)
  const bucketsRef = useRef(buckets)
  const onSelectBucketRef = useRef(onSelectBucket)
  const [loadError, setLoadError] = useState<unknown>()
  const [themeVersion, setThemeVersion] = useState(0)
  const [keyboardIndex, setKeyboardIndex] = useState<number | null>(null)

  bucketsRef.current = buckets
  onSelectBucketRef.current = onSelectBucket

  const activeKeyboardIndex = keyboardIndex !== null && keyboardIndex >= 0 && keyboardIndex < buckets.length
    ? keyboardIndex
    : null
  const keyboardBucket = activeKeyboardIndex !== null ? buckets[activeKeyboardIndex] : undefined

  const option = useMemo(
    () => buildSpendChartOption({
      activeCategory,
      billingDisplay,
      buckets,
      canManageBilling,
      chartType,
      focusedBucketStart,
      granularity,
      keyboardBucketStart: keyboardBucket?.bucketStart,
      locale,
      t,
      themeVersion,
      timeZone,
    }),
    [
      activeCategory,
      billingDisplay,
      buckets,
      canManageBilling,
      chartType,
      focusedBucketStart,
      granularity,
      keyboardBucket?.bucketStart,
      locale,
      t,
      themeVersion,
      timeZone,
    ],
  )
  optionRef.current = option

  useEffect(() => {
    const element = elementRef.current
    if (!element || typeof ResizeObserver === 'undefined')
      return

    let cancelled = false
    let initializing = false

    const ensureChart = () => {
      if (cancelled || chartRef.current || initializing || element.clientWidth === 0 || element.clientHeight === 0)
        return
      initializing = true
      void import('./billing-spend-chart-runtime').then(({ initBillingSpendChart }) => {
        initializing = false
        if (cancelled || chartRef.current)
          return
        const chart = initBillingSpendChart(element)
        chartRef.current = chart
        // Use ZRender grid hit-testing so clicking anywhere in a time column works
        // identically across both bar (including 0-spend slots) and area charts.
        chart.getZr().on('click', (event) => {
          const x = event.offsetX ?? (event as { zrX?: number }).zrX
          const y = event.offsetY ?? (event as { zrY?: number }).zrY
          if (typeof x !== 'number' || typeof y !== 'number')
            return
          const point: [number, number] = [x, y]
          if (!chart.containPixel('grid', point))
            return
          const converted = chart.convertFromPixel({ xAxisIndex: 0 }, x)
          const dataIndex = typeof converted === 'number' && Number.isFinite(converted)
            ? Math.round(converted)
            : -1
          const target = dataIndex >= 0 && dataIndex < bucketsRef.current.length
            ? bucketsRef.current[dataIndex]
            : undefined
          if (target)
            onSelectBucketRef.current?.(target)
        })
        if (optionRef.current)
          chart.setOption(optionRef.current, true)
      }).catch((error: unknown) => {
        initializing = false
        if (!cancelled)
          setLoadError(error)
      })
    }

    ensureChart()
    const resizeObserver = new ResizeObserver(() => {
      if (!chartRef.current) {
        ensureChart()
        return
      }
      if (!chartRef.current.isDisposed())
        chartRef.current.resize()
    })
    resizeObserver.observe(element)

    return () => {
      cancelled = true
      resizeObserver.disconnect()
      chartRef.current?.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    const chart = chartRef.current
    if (!chart || chart.isDisposed())
      return
    chart.setOption(option, true)
    if (activeKeyboardIndex !== null) {
      chart.dispatchAction({ dataIndex: activeKeyboardIndex, seriesIndex: 0, type: 'showTip' })
    }
  }, [activeKeyboardIndex, option])

  useEffect(() => {
    const observer = new MutationObserver(() => setThemeVersion(version => version + 1))
    observer.observe(document.documentElement, {
      attributeFilter: ['class', 'style', 'data-theme', 'data-brand-theme'],
      attributes: true,
    })
    return () => observer.disconnect()
  }, [])

  if (loadError)
    throw loadError

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (buckets.length === 0)
      return
    if (event.key === 'ArrowRight') {
      event.preventDefault()
      setKeyboardIndex(current => current === null ? 0 : Math.min(buckets.length - 1, current + 1))
      return
    }
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      setKeyboardIndex(current => current === null ? buckets.length - 1 : Math.max(0, current - 1))
      return
    }
    if (event.key === 'Home') {
      event.preventDefault()
      setKeyboardIndex(0)
      return
    }
    if (event.key === 'End') {
      event.preventDefault()
      setKeyboardIndex(buckets.length - 1)
      return
    }
    if ((event.key === 'Enter' || event.key === ' ') && activeKeyboardIndex !== null) {
      event.preventDefault()
      const target = buckets[activeKeyboardIndex]
      if (target)
        onSelectBucket?.(target)
    }
  }

  return (
    <div
      aria-label={t('billingPage.spendChartAriaLabel')}
      className="relative min-w-0 rounded-control focus-visible:outline-2 focus-visible:outline-ring"
      data-billing-active-category={activeCategory}
      data-billing-chart-type={chartType}
      data-billing-granularity={granularity}
      role="group"
      tabIndex={0}
      onBlur={() => {
        setKeyboardIndex(null)
        const chart = chartRef.current
        if (chart && !chart.isDisposed())
          chart.dispatchAction({ type: 'hideTip' })
      }}
      onKeyDown={handleKeyDown}
    >
      {keyboardBucket && (
        <div
          aria-live="polite"
          className="pointer-events-none absolute top-1 right-2 z-10 inline-flex items-center gap-2 rounded-control border border-border bg-surface-raised/95 px-2.5 py-1 text-xs shadow-sm tabular-nums"
        >
          <span className="text-muted-foreground">
            {formatTimeseriesBucketRange(keyboardBucket, granularity, locale, timeZone)}
          </span>
          <span className="font-semibold text-foreground">
            {billingDisplay.formatAmountWithUnit(bucketDisplayedCredits(keyboardBucket, activeCategory))}
          </span>
        </div>
      )}
      <div ref={elementRef} className="h-64 w-full @2xl/workspace:h-72" />
      <div className="sr-only">
        {buckets.map(bucket => (
          <span key={bucket.bucketStart}>
            {`${formatTimeseriesBucketRange(bucket, granularity, locale, timeZone)}: ${billingDisplay.formatAmountWithUnit(bucketDisplayedCredits(bucket, activeCategory))}`}
          </span>
        ))}
      </div>
    </div>
  )
}

function buildSpendChartOption({
  activeCategory,
  billingDisplay,
  buckets,
  canManageBilling,
  chartType,
  focusedBucketStart,
  granularity,
  keyboardBucketStart,
  locale,
  t,
  themeVersion: _themeVersion,
  timeZone,
}: {
  activeCategory: string
  billingDisplay: BillingDisplay
  buckets: BillingSpendTimeseriesBucket[]
  canManageBilling: boolean
  chartType: BillingChartType
  focusedBucketStart?: string
  granularity: BillingSpendGranularity
  keyboardBucketStart?: string
  locale: string
  t: (key: string, options?: Record<string, unknown>) => string
  themeVersion: number
  timeZone: string
}): SpendChartOption {
  const styles = getComputedStyle(document.documentElement)
  const foreground = resolveCssColor(styles, '--foreground', '#18181b')
  const muted = resolveCssColor(styles, '--muted-foreground', '#71717a')
  const border = resolveCssColor(styles, '--border', '#e4e4e7')
  const surface = resolveCssColor(styles, '--surface', '#ffffff')
  const primary = resolveCssColor(styles, '--primary', '#2563eb')
  const highlightedBucketStart = keyboardBucketStart ?? focusedBucketStart
  const prefersReducedMotion = typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches

  const axisLabels = buckets.map(bucket => formatTimeseriesAxisLabel(bucket.bucketStart, granularity, locale, timeZone))

  const seriesList: Array<BarSeriesOption | LineSeriesOption> = []

  if (chartType === 'bar') {
    if (activeCategory === 'all') {
      const presentCategories = BILLING_CATEGORY_ORDER.filter(cat =>
        buckets.some(bucket => bucketCategoryAmount(bucket.categories, cat) > 0),
      )
      const categoriesToRender = presentCategories.length > 0 ? presentCategories : ['runtime'] as const
      const topCategoryByBucket = buckets.map((bucket) => {
        let topCat = ''
        for (const cat of categoriesToRender) {
          if (bucketCategoryAmount(bucket.categories, cat) > 0)
            topCat = cat
        }
        return topCat
      })

      for (const cat of categoriesToRender) {
        const color = resolveCategoryColor(styles, cat)
        seriesList.push({
          barMaxWidth: 28,
          cursor: 'pointer',
          data: buckets.map((bucket, idx) => {
            const value = bucketCategoryAmount(bucket.categories, cat)
            const isHighlighted = !highlightedBucketStart || bucket.bucketStart === highlightedBucketStart
            const isTop = topCategoryByBucket[idx] === cat && value > 0
            return {
              itemStyle: {
                borderRadius: isTop ? [3, 3, 0, 0] : [0, 0, 0, 0],
                color,
                opacity: isHighlighted ? 1 : 0.36,
              },
              value,
            }
          }),
          name: categoryTranslatedLabel(cat, t),
          stack: 'spend',
          type: 'bar',
        })
      }
    }
    else {
      const color = resolveCategoryColor(styles, activeCategory)
      seriesList.push({
        barMaxWidth: 28,
        cursor: 'pointer',
        data: buckets.map((bucket) => {
          const value = bucketDisplayedCredits(bucket, activeCategory)
          const isHighlighted = !highlightedBucketStart || bucket.bucketStart === highlightedBucketStart
          return {
            itemStyle: {
              borderRadius: value > 0 ? [3, 3, 0, 0] : [0, 0, 0, 0],
              color,
              opacity: isHighlighted ? 1 : 0.36,
            },
            value,
          }
        }),
        name: categoryTranslatedLabel(activeCategory, t),
        type: 'bar',
      })
    }
  }
  else {
    const lineColor = activeCategory === 'all' ? primary : resolveCategoryColor(styles, activeCategory)
    seriesList.push({
      areaStyle: {
        color: {
          colorStops: [
            { color: lineColor, offset: 0 },
            { color: 'rgba(0, 0, 0, 0)', offset: 1 },
          ],
          type: 'linear',
          x: 0,
          x2: 0,
          y: 0,
          y2: 1,
        },
        opacity: 0.2,
      },
      cursor: 'pointer',
      data: buckets.map((bucket) => {
        const value = bucketDisplayedCredits(bucket, activeCategory)
        const isHighlighted = highlightedBucketStart === bucket.bucketStart
        return {
          itemStyle: {
            borderColor: lineColor,
            borderWidth: 2,
            color: isHighlighted ? lineColor : surface,
          },
          symbolSize: isHighlighted ? 8 : 5,
          value,
        }
      }),
      lineStyle: { color: lineColor, width: 2.2 },
      name: activeCategory === 'all' ? t('billingPage.allCategories') : categoryTranslatedLabel(activeCategory, t),
      showSymbol: buckets.length <= 14 || Boolean(highlightedBucketStart),
      smooth: true,
      smoothMonotone: 'x',
      symbol: 'circle',
      type: 'line',
    })
  }

  return {
    animationDuration: prefersReducedMotion ? 0 : 220,
    grid: {
      bottom: 28,
      left: 52,
      right: 14,
      top: 14,
    },
    series: seriesList,
    textStyle: {
      color: foreground,
      fontFamily: 'Inter, ui-sans-serif, system-ui, sans-serif',
      fontSize: 11,
    },
    tooltip: {
      axisPointer: {
        lineStyle: { color: border, type: 'dashed' },
        type: chartType === 'bar' ? 'shadow' : 'line',
      },
      backgroundColor: surface,
      borderColor: border,
      borderWidth: 1,
      confine: true,
      extraCssText: 'border-radius: 10px; box-shadow: 0 8px 24px -6px rgba(0,0,0,0.16); padding: 10px 12px;',
      formatter: (params: unknown) => {
        const first = Array.isArray(params) ? params[0] : params
        const dataIndex = typeof (first as { dataIndex?: unknown } | undefined)?.dataIndex === 'number'
          ? (first as { dataIndex: number }).dataIndex
          : -1
        const bucket = dataIndex >= 0 ? buckets[dataIndex] : undefined
        if (!bucket)
          return ''
        const header = escapeHtml(formatTimeseriesBucketRange(bucket, granularity, locale, timeZone))
        const displayedTotal = bucketDisplayedCredits(bucket, activeCategory)
        const totalText = escapeHtml(billingDisplay.formatAmountWithUnit(displayedTotal))
        const fiatRaw = canManageBilling ? billingDisplay.formatFiatAmount(displayedTotal) : ''
        const fiatSuffix = fiatRaw ? ` <span style="color:${escapeHtml(muted)};font-weight:400;">(${escapeHtml(fiatRaw)})</span>` : ''
        const totalLabel = escapeHtml(
          activeCategory === 'all' ? t('billingPage.tooltipTotal') : categoryTranslatedLabel(activeCategory, t),
        )

        const breakdownRows = bucket.categories
          .filter(cat => (activeCategory === 'all' || cat.category === activeCategory) && Number.parseFloat(cat.amountCredits) > 0)
          .sort((a, b) => Number.parseFloat(b.amountCredits) - Number.parseFloat(a.amountCredits))
          .map((cat) => {
            const swatch = escapeHtml(resolveCategoryColor(styles, cat.category))
            const name = escapeHtml(categoryTranslatedLabel(cat.category, t))
            const amt = escapeHtml(billingDisplay.formatAmountWithUnit(cat.amountCredits))
            return [
              `<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;font-variant-numeric:tabular-nums;">`,
              `<span style="display:inline-flex;align-items:center;gap:6px;color:${escapeHtml(muted)};">`,
              `<span style="width:8px;height:8px;border-radius:2px;background:${swatch};display:inline-block;"></span>${name}</span>`,
              `<span style="font-weight:500;color:${escapeHtml(foreground)};">${amt}</span></div>`,
            ].join('')
          })
          .join('')

        const hint = escapeHtml(t('billingPage.tooltipDrilldownHint'))
        const breakdownSection = breakdownRows
          ? `<div style="margin-top:6px;padding-top:6px;border-top:1px solid ${escapeHtml(border)};display:grid;gap:3px;">${breakdownRows}</div>`
          : ''
        return [
          `<div style="min-width:180px;font-size:12px;line-height:1.5;color:${escapeHtml(foreground)};">`,
          `<div style="font-weight:600;padding-bottom:4px;margin-bottom:6px;border-bottom:1px solid ${escapeHtml(border)};font-variant-numeric:tabular-nums;">${header}</div>`,
          `<div style="display:flex;align-items:baseline;justify-content:space-between;gap:16px;font-variant-numeric:tabular-nums;">`,
          `<span style="color:${escapeHtml(muted)};">${totalLabel}</span>`,
          `<span style="font-weight:600;">${totalText}${fiatSuffix}</span></div>`,
          breakdownSection,
          `<div style="margin-top:6px;font-size:11px;color:${escapeHtml(muted)};">${hint}</div>`,
          `</div>`,
        ].join('')
      },
      textStyle: { color: foreground, fontSize: 12 },
      trigger: 'axis',
    },
    xAxis: {
      axisLabel: {
        color: muted,
        fontSize: 11,
        hideOverlap: true,
        margin: 10,
      },
      axisLine: { lineStyle: { color: border } },
      axisTick: { show: false },
      boundaryGap: chartType === 'bar',
      data: axisLabels,
      type: 'category',
    },
    yAxis: {
      axisLabel: {
        color: muted,
        fontSize: 11,
        formatter: (value: number) => billingDisplay.formatAmount(value),
      },
      min: 0,
      splitLine: { lineStyle: { color: border, opacity: 0.55 } },
      type: 'value',
    },
  }
}

function resolveCategoryColor(styles: CSSStyleDeclaration, category: string) {
  const entry = BILLING_CATEGORY_COLOR_VARS[category] ?? BILLING_CATEGORY_COLOR_VARS.other
  return resolveCssColor(styles, entry.variable, entry.fallback)
}

let colorProbeCtx: CanvasRenderingContext2D | null | undefined

function normalizeColorForZRender(color: string, fallback: string) {
  // ZRender's color parser only understands #hex / rgb(a) / hsl(a); Radix P3
  // tokens return `color(display-p3 ...)` on wide-gamut displays.
  if (color.startsWith('#') || color.startsWith('rgb') || color.startsWith('hsl'))
    return color
  if (typeof document === 'undefined')
    return fallback
  if (colorProbeCtx === undefined) {
    try {
      const canvas = document.createElement('canvas')
      canvas.width = 1
      canvas.height = 1
      colorProbeCtx = canvas.getContext('2d', { willReadFrequently: true })
    }
    catch {
      colorProbeCtx = null
    }
  }
  if (!colorProbeCtx)
    return fallback
  try {
    colorProbeCtx.clearRect(0, 0, 1, 1)
    colorProbeCtx.fillStyle = color
    colorProbeCtx.fillRect(0, 0, 1, 1)
    const [r, g, b, a] = colorProbeCtx.getImageData(0, 0, 1, 1).data
    if (a === 0)
      return fallback
    if (a < 255)
      return `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(3)})`
    return `rgb(${r}, ${g}, ${b})`
  }
  catch {
    return fallback
  }
}

function resolveCssColor(styles: CSSStyleDeclaration, variable: string, fallback: string) {
  const value = styles.getPropertyValue(variable).trim()
  if (!value || value.startsWith('var('))
    return fallback
  const hslParts = value.split(/\s+/)
  if (hslParts.length === 3 && hslParts[1]?.endsWith('%') && hslParts[2]?.endsWith('%'))
    return `hsl(${hslParts[0]}, ${hslParts[1]}, ${hslParts[2]})`
  return normalizeColorForZRender(value, fallback)
}

function escapeHtml(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll('\'', '&#39;')
}
