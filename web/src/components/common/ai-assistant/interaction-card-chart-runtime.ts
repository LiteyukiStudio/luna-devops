import { BarChart, LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { init, use as registerECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'

registerECharts([
  BarChart,
  LineChart,
  PieChart,
  GridComponent,
  TooltipComponent,
  CanvasRenderer,
])

export { init as initCompactChart }
