import type { AgentObservabilitySeries } from '@/api'
import { useMemo } from 'react'

const chartColors = ['var(--primary)', 'var(--color-info)', 'var(--color-warning)', 'var(--color-success)', 'var(--color-danger)']
const gridLines = [5, 65, 125, 185]

export function AgentObservabilityChart({ label, series, valueFormatter = formatCompact }: {
  label: string
  series: AgentObservabilitySeries[]
  valueFormatter?: (value: number) => string
}) {
  const chart = useMemo(() => buildChart(series), [series])
  if (!chart)
    return <div className="grid h-48 place-items-center text-sm text-muted-foreground">—</div>

  return (
    <div>
      <svg aria-label={label} className="h-48 w-full overflow-visible" role="img" viewBox="0 0 640 190">
        {gridLines.map(y => <line key={y} stroke="var(--separator-subtle)" strokeWidth="1" x1="0" x2="640" y1={y} y2={y} />)}
        {chart.paths.map((path, index) => (
          <path
            key={path.key}
            d={path.d}
            fill="none"
            stroke={chartColors[index % chartColors.length]}
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth="2.5"
          />
        ))}
      </svg>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
        <div className="flex flex-wrap gap-3">
          {chart.paths.map((path, index) => (
            <span key={path.key} className="inline-flex items-center gap-1.5">
              <span className="size-2 rounded-full" style={{ background: chartColors[index % chartColors.length] }} />
              {path.label}
            </span>
          ))}
        </div>
        <span>{valueFormatter(chart.maximum)}</span>
      </div>
    </div>
  )
}

function buildChart(series: AgentObservabilitySeries[]) {
  const populated = series.filter(item => item.points.length > 0)
  const points = populated.flatMap(item => item.points)
  if (points.length === 0)
    return null
  const minimumTime = Math.min(...points.map(point => point.timestamp))
  const maximumTime = Math.max(...points.map(point => point.timestamp))
  const maximum = Math.max(...points.map(point => point.value), 0.000001)
  const timeSpan = Math.max(1, maximumTime - minimumTime)
  return {
    maximum,
    paths: populated.map((item, index) => ({
      key: JSON.stringify(item.labels),
      label: seriesLabel(item, index),
      d: item.points.map((point, pointIndex) => {
        const x = ((point.timestamp - minimumTime) / timeSpan) * 640
        const y = 180 - (point.value / maximum) * 170
        return `${pointIndex === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
      }).join(' '),
    })),
  }
}

function seriesLabel(series: AgentObservabilitySeries, index: number) {
  return series.labels.direction || series.labels.tool || series.labels.outcome || series.labels.quantile || `#${index + 1}`
}

function formatCompact(value: number) {
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 2 }).format(value)
}
