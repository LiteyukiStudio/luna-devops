import type { BarSeriesOption, LineSeriesOption, PieSeriesOption } from 'echarts/charts'
import type { GridComponentOption, TooltipComponentOption } from 'echarts/components'
import type { ComposeOption, EChartsType } from 'echarts/core'
import type { InteractionContentBlock } from './interaction-card-schema'
import { useEffect, useMemo, useRef, useState } from 'react'

type ChartBlock = Extract<InteractionContentBlock, { type: 'chart' }>
type CompactChartOption = ComposeOption<BarSeriesOption | LineSeriesOption | PieSeriesOption | GridComponentOption | TooltipComponentOption>

interface InteractionCardChartProps {
  block: ChartBlock
  label: string
}

function keyedSeries<T extends { name: string }>(series: readonly T[]) {
  const occurrences = new Map<string, number>()
  return series.map((item) => {
    const occurrence = occurrences.get(item.name) ?? 0
    occurrences.set(item.name, occurrence + 1)
    return { item, key: `${item.name}:${occurrence}` }
  })
}

export function InteractionCardChart({ block, label }: InteractionCardChartProps) {
  const elementRef = useRef<HTMLDivElement>(null)
  const chartRef = useRef<EChartsType | null>(null)
  const optionRef = useRef<CompactChartOption | null>(null)
  const [loadError, setLoadError] = useState<unknown>()
  const [themeVersion, setThemeVersion] = useState(0)
  const option = useMemo(() => buildOption(block, themeVersion), [block, themeVersion])
  optionRef.current = option

  useEffect(() => {
    const element = elementRef.current
    if (!element || typeof ResizeObserver === 'undefined' || element.clientWidth === 0)
      return
    let cancelled = false
    let resizeObserver: ResizeObserver | undefined
    void Promise.all([
      import('echarts/core'),
      import('echarts/charts'),
      import('echarts/components'),
      import('echarts/renderers'),
    ]).then(([core, charts, components, renderers]) => {
      if (cancelled)
        return
      core.use([
        charts.BarChart,
        charts.LineChart,
        charts.PieChart,
        components.GridComponent,
        components.TooltipComponent,
        renderers.CanvasRenderer,
      ])
      const chart = core.init(element)
      chartRef.current = chart
      if (optionRef.current)
        chart.setOption(optionRef.current, true)
      resizeObserver = new ResizeObserver(() => {
        if (!chart.isDisposed())
          chart.resize()
      })
      resizeObserver.observe(element)
    }).catch((error: unknown) => {
      if (!cancelled)
        setLoadError(error)
    })
    return () => {
      cancelled = true
      resizeObserver?.disconnect()
      chartRef.current?.dispose()
      chartRef.current = null
    }
  }, [])

  useEffect(() => {
    const chart = chartRef.current
    if (!chart || chart.isDisposed())
      return
    chart.setOption(option, true)
  }, [option])

  useEffect(() => {
    const observer = new MutationObserver(() => setThemeVersion(version => version + 1))
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] })
    return () => observer.disconnect()
  }, [])

  if (loadError)
    throw loadError

  return (
    <div className="min-w-0" data-ai-chart-type={block.chartType} role="img" aria-label={label}>
      <div ref={elementRef} className="h-32 w-full" />
      <div className="sr-only">
        {keyedSeries(block.series).map(({ item: series, key }) => (
          <p key={key}>
            {series.name}
            {': '}
            {series.values.map((value, index) => `${block.xAxis?.[index] ?? index + 1}: ${value}${series.unit ?? ''}`).join(', ')}
          </p>
        ))}
      </div>
    </div>
  )
}

function buildOption(block: ChartBlock, _themeVersion: number): CompactChartOption {
  const styles = getComputedStyle(document.documentElement)
  const primary = chartThemeColor(styles, '--primary', '#2563eb')
  const secondary = chartThemeColor(styles, '--theme-secondary', '#14b8a6')
  const supporting = chartThemeColor(styles, '--theme-supporting', '#8b5cf6')
  const highlight = chartThemeColor(styles, '--theme-highlight', '#f59e0b')
  const foreground = chartThemeColor(styles, '--foreground', '#18181b')
  const muted = chartThemeColor(styles, '--muted-foreground', '#71717a')
  const border = chartThemeColor(styles, '--border', '#e4e4e7')
  const colors = [primary, secondary, supporting, highlight]
  const common: CompactChartOption = {
    animationDuration: 280,
    color: colors,
    grid: { left: 36, right: 8, top: 12, bottom: 24 },
    textStyle: { color: foreground, fontSize: 9 },
    tooltip: { trigger: block.chartType === 'donut' ? 'item' : 'axis', confine: true },
  }
  if (block.chartType === 'donut') {
    return {
      ...common,
      series: block.series.map((series, seriesIndex) => {
        const ringWidth = 46 / block.series.length
        const innerRadius = 22 + seriesIndex * ringWidth
        return {
          type: 'pie' as const,
          name: series.name,
          radius: [`${innerRadius}%`, `${innerRadius + ringWidth - 3}%`],
          center: ['50%', '50%'],
          label: {
            color: muted,
            fontSize: 9,
            formatter: seriesIndex === block.series.length - 1 ? '{b}: {c}' : '',
          },
          data: series.values.map((value, index) => ({
            name: block.xAxis?.[index] ?? String(index + 1),
            value,
          })),
        }
      }),
    }
  }
  return {
    ...common,
    xAxis: {
      type: 'category',
      data: block.xAxis ?? block.series[0]?.values.map((_, index) => String(index + 1)),
      axisLabel: { color: muted, fontSize: 9, hideOverlap: true },
      axisLine: { lineStyle: { color: border } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: muted, fontSize: 9 },
      splitLine: { lineStyle: { color: border, opacity: 0.55 } },
    },
    series: block.series.map(series => block.chartType === 'bar'
      ? {
          type: 'bar' as const,
          name: series.name,
          data: series.values,
          barMaxWidth: 18,
          itemStyle: { borderRadius: [3, 3, 0, 0] },
        }
      : {
          type: 'line' as const,
          name: series.name,
          data: series.values,
          showSymbol: series.values.length <= 12,
          symbolSize: 5,
          smooth: true,
          areaStyle: block.chartType === 'area' ? { opacity: 0.14 } : undefined,
        }),
  }
}

function chartThemeColor(styles: CSSStyleDeclaration, variable: string, fallback: string) {
  const value = styles.getPropertyValue(variable).trim()
  if (!value || value.startsWith('var('))
    return fallback
  const hslParts = value.split(/\s+/)
  if (hslParts.length === 3 && hslParts[1]?.endsWith('%') && hslParts[2]?.endsWith('%'))
    return `hsl(${hslParts[0]}, ${hslParts[1]}, ${hslParts[2]})`
  return value
}
